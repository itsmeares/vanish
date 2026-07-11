package manualcleanup

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
	"github.com/itsmeares/vanish/internal/instagram"
)

type Store struct {
	dir string
}

var errUnreadableProgress = errors.New("manual cleanup progress is unreadable")

type progressEvent struct {
	ID       string    `json:"id"`
	At       time.Time `json:"at"`
	Kind     string    `json:"kind"`
	ActionID string    `json:"action_id,omitempty"`
	Outcome  Outcome   `json:"outcome,omitempty"`
	Position int       `json:"position"`
	State    State     `json:"state"`
}

func NewStore(workspaceDir string) Store {
	return Store{dir: filepath.Join(filepath.Clean(workspaceDir), "manual-cleanup")}
}

func (store Store) Start(session Session) error {
	if err := validateManifest(session.Manifest); err != nil {
		return err
	}
	if err := os.MkdirAll(store.dir, 0o700); err != nil {
		return err
	}
	manifestPath, progressPath := store.paths(session.PlanID)
	if err := writeJSONAtomic(manifestPath, session.Manifest); err != nil {
		return err
	}
	event := progressEvent{ID: session.ID, At: session.CreatedAt, Kind: "started", Position: 0, State: StateActive}
	return replaceProgress(progressPath, event)
}

func (store Store) Load(planID string) (Session, bool, error) {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return Session{}, false, errUnreadableProgress
	}
	manifestPath, progressPath := store.paths(planID)
	manifest, found, err := loadManifest(manifestPath, planID)
	if err != nil || !found {
		return Session{}, found, err
	}
	session := sessionFromManifest(manifest)
	if err := replayProgress(progressPath, &session); err != nil {
		return Session{}, false, err
	}
	return session, true, nil
}

func loadManifest(path, requestedPlanID string) (Manifest, bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Manifest{}, false, nil
	}
	if err != nil {
		return Manifest{}, false, err
	}
	defer file.Close()

	var manifest Manifest
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, false, errUnreadableProgress
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return Manifest{}, false, errUnreadableProgress
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, false, err
	}
	if requestedPlanID != "" && manifest.PlanID != strings.TrimSpace(requestedPlanID) {
		return Manifest{}, false, errUnreadableProgress
	}
	return manifest, true, nil
}

func sessionFromManifest(manifest Manifest) Session {
	session := Session{Manifest: manifest}
	session.initializeProgress()
	return session
}

func (store Store) LatestUnfinished() (Session, bool, error) {
	entries, err := os.ReadDir(store.dir)
	if errors.Is(err, os.ErrNotExist) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, err
	}
	var latest Session
	found := false
	var firstErr error
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		manifestPath := filepath.Join(store.dir, entry.Name())
		manifest, ok, err := loadManifest(manifestPath, "")
		if err != nil || !ok {
			if err != nil && firstErr == nil {
				firstErr = err
			}
			continue
		}
		expectedManifestPath, progressPath := store.paths(manifest.PlanID)
		if filepath.Clean(expectedManifestPath) != filepath.Clean(manifestPath) {
			if firstErr == nil {
				firstErr = errUnreadableProgress
			}
			continue
		}
		session := sessionFromManifest(manifest)
		if err := replayProgress(progressPath, &session); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if session.State == StateCompleted {
			continue
		}
		if !found || session.UpdatedAt.After(latest.UpdatedAt) {
			latest = session
			found = true
		}
	}
	if found {
		return latest, true, nil
	}
	return Session{}, false, firstErr
}

func (store Store) StartOver(planID string, at time.Time) (Session, error) {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return Session{}, errUnreadableProgress
	}
	manifestPath, progressPath := store.paths(planID)
	manifest, found, err := loadManifest(manifestPath, planID)
	if err != nil {
		return Session{}, err
	}
	if !found {
		return Session{}, os.ErrNotExist
	}
	event := progressEvent{ID: manifest.ID, At: normalizedTime(at), Kind: "started", Position: 0, State: StateActive}
	if err := replaceProgress(progressPath, event); err != nil {
		return Session{}, err
	}
	session := sessionFromManifest(manifest)
	session.UpdatedAt = event.At
	return session, nil
}

func (store Store) Resume(session *Session, at time.Time) error {
	return store.recordState(session, "resumed", StateActive, at)
}

func (store Store) Stop(session *Session, at time.Time) error {
	return store.recordState(session, "stopped", StateStopped, at)
}

func (store Store) Mark(session *Session, outcome Outcome, at time.Time) (bool, error) {
	if outcome != OutcomeDone && outcome != OutcomeSkipped {
		return false, errors.New("manual cleanup outcome is not supported")
	}
	if session.State != StateActive {
		return false, errors.New("manual cleanup is not active")
	}
	action, ok := session.Current()
	if !ok {
		return false, errors.New("manual cleanup has no pending action")
	}
	if session.CurrentPosition >= len(session.Outcomes) || session.Outcomes[session.CurrentPosition] != OutcomePending {
		return false, errUnreadableProgress
	}
	position := session.CurrentPosition + 1
	state := StateActive
	if position >= len(session.Actions) {
		state = StateCompleted
	}
	event := progressEvent{
		ID:       session.ID,
		At:       normalizedTime(at),
		Kind:     string(outcome),
		ActionID: action.ActionID,
		Outcome:  outcome,
		Position: position,
		State:    state,
	}
	if err := store.append(session.PlanID, event); err != nil {
		return false, err
	}
	if err := applyProgressEvent(session, event, false); err != nil {
		return false, err
	}
	return state == StateCompleted, nil
}

func (store Store) recordState(session *Session, kind string, state State, at time.Time) error {
	event := progressEvent{
		ID:       session.ID,
		At:       normalizedTime(at),
		Kind:     kind,
		Position: session.CurrentPosition,
		State:    state,
	}
	next := *session
	if err := applyProgressEvent(&next, event, false); err != nil {
		return err
	}
	if err := store.append(session.PlanID, event); err != nil {
		return err
	}
	*session = next
	return nil
}

func (store Store) append(planID string, event progressEvent) error {
	if err := os.MkdirAll(store.dir, 0o700); err != nil {
		return err
	}
	_, progressPath := store.paths(planID)
	file, err := os.OpenFile(progressPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func (store Store) paths(planID string) (string, string) {
	sum := sha256.Sum256([]byte(strings.TrimSpace(planID)))
	name := hex.EncodeToString(sum[:16])
	return filepath.Join(store.dir, name+".json"), filepath.Join(store.dir, name+".events.jsonl")
}

func replayProgress(path string, session *Session) error {
	file, err := os.Open(path)
	if err != nil {
		return errUnreadableProgress
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event progressEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			return errUnreadableProgress
		}
		if err := applyProgressEvent(session, event, first); err != nil {
			return errUnreadableProgress
		}
		first = false
	}
	if err := scanner.Err(); err != nil {
		return errUnreadableProgress
	}
	if first {
		return errUnreadableProgress
	}
	return nil
}

func applyProgressEvent(session *Session, event progressEvent, first bool) error {
	if event.ID != session.ID || event.At.IsZero() {
		return errUnreadableProgress
	}
	switch event.Kind {
	case "started":
		if !first || event.ActionID != "" || event.Outcome != "" || event.Position != 0 || event.State != StateActive {
			return errUnreadableProgress
		}
		session.initializeProgress()
	case string(OutcomeDone), string(OutcomeSkipped):
		if first || event.Outcome != Outcome(event.Kind) || session.State != StateActive {
			return errUnreadableProgress
		}
		index := session.CurrentPosition
		if index < 0 || index >= len(session.Actions) || event.ActionID != session.Actions[index].ActionID || event.Position != index+1 {
			return errUnreadableProgress
		}
		expectedState := StateActive
		if event.Position == len(session.Actions) {
			expectedState = StateCompleted
		}
		if event.State != expectedState || session.recordOutcome(index, event.Outcome) != nil {
			return errUnreadableProgress
		}
		session.CurrentPosition = event.Position
		session.State = event.State
	case "resumed":
		if first || event.ActionID != "" || event.Outcome != "" || event.Position != session.CurrentPosition || event.State != StateActive || session.CurrentPosition >= len(session.Actions) || (session.State != StateActive && session.State != StateStopped) {
			return errUnreadableProgress
		}
		session.State = StateActive
	case "stopped":
		if first || event.ActionID != "" || event.Outcome != "" || event.Position != session.CurrentPosition || event.State != StateStopped || session.CurrentPosition >= len(session.Actions) || session.State != StateActive {
			return errUnreadableProgress
		}
		session.State = StateStopped
	default:
		return errUnreadableProgress
	}
	session.UpdatedAt = event.At
	return nil
}

func validateManifest(manifest Manifest) error {
	if manifest.FormatVersion != FormatVersion || strings.TrimSpace(manifest.ID) == "" || strings.TrimSpace(manifest.PlanID) == "" {
		return errors.New("manual cleanup progress is invalid")
	}
	if manifest.Mode != ModeInstagramManual || manifest.CreatedAt.IsZero() || len(manifest.Actions) == 0 {
		return errUnreadableProgress
	}
	if err := manifest.PlanSnapshot.Validate(); err != nil || manifest.PlanSnapshot.Platform != domain.PlatformInstagram || manifest.PlanSnapshot.ID != manifest.PlanID {
		return errUnreadableProgress
	}
	planActions := make(map[string]domain.CleanupAction, len(manifest.PlanSnapshot.Actions))
	planPositions := make(map[string]int, len(manifest.PlanSnapshot.Actions))
	for index, action := range manifest.PlanSnapshot.Actions {
		if _, exists := planActions[action.ID]; exists {
			return errUnreadableProgress
		}
		planActions[action.ID] = action
		planPositions[action.ID] = index
	}
	seen := make(map[string]struct{}, len(manifest.Actions))
	lastPosition := -1
	for _, action := range manifest.Actions {
		if strings.TrimSpace(action.ActionID) == "" || strings.TrimSpace(action.TargetURL) == "" {
			return errUnreadableProgress
		}
		if _, ok := seen[action.ActionID]; ok {
			return errUnreadableProgress
		}
		seen[action.ActionID] = struct{}{}
		planned, ok := planActions[action.ActionID]
		if !ok || planned.Type != action.Type || planPositions[action.ActionID] <= lastPosition {
			return errUnreadableProgress
		}
		lastPosition = planPositions[action.ActionID]
		plannedTarget, err := instagram.ValidateCleanupTarget(planned.Type, planned.TargetURL, planned.TargetID)
		if err != nil || plannedTarget.URL != action.TargetURL || plannedTarget.Kind != action.TargetKind || plannedTarget.Identifier != action.TargetID {
			return errUnreadableProgress
		}
		if action.Actor != "" {
			actor, valid := instagram.NormalizeUsername(action.Actor)
			if !valid || actor != action.Actor {
				return errUnreadableProgress
			}
		}
	}
	return nil
}

func replaceProgress(path string, event progressEvent) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	encoded, err := json.Marshal(event)
	if err == nil {
		_, err = temp.Write(append(encoded, '\n'))
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("replace manual cleanup progress: %w", err)
	}
	return nil
}

func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func normalizedTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}
