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
)

type Store struct {
	dir string
}

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
	manifestPath, progressPath := store.paths(planID)
	file, err := os.Open(manifestPath)
	if errors.Is(err, os.ErrNotExist) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, err
	}
	defer file.Close()

	var manifest Manifest
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Session{}, false, errors.New("manual cleanup progress is unreadable")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return Session{}, false, errors.New("manual cleanup progress is unreadable")
	}
	if err := validateManifest(manifest); err != nil {
		return Session{}, false, err
	}

	session := Session{
		Manifest:  manifest,
		UpdatedAt: manifest.CreatedAt,
		State:     StateActive,
		Outcomes:  make([]Outcome, len(manifest.Actions)),
	}
	for i := range session.Outcomes {
		session.Outcomes[i] = OutcomePending
	}
	if err := replayProgress(progressPath, &session); err != nil {
		return Session{}, false, err
	}
	return session, true, nil
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
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		file, err := os.Open(filepath.Join(store.dir, entry.Name()))
		if err != nil {
			continue
		}
		var manifest Manifest
		err = json.NewDecoder(file).Decode(&manifest)
		_ = file.Close()
		if err != nil || strings.TrimSpace(manifest.PlanID) == "" {
			continue
		}
		session, ok, err := store.Load(manifest.PlanID)
		if err != nil || !ok || session.State == StateCompleted {
			continue
		}
		if !found || session.UpdatedAt.After(latest.UpdatedAt) {
			latest = session
			found = true
		}
	}
	return latest, found, nil
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
	action, ok := session.Current()
	if !ok {
		return false, errors.New("manual cleanup has no pending action")
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
	applyProgressEvent(session, event)
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
	if err := store.append(session.PlanID, event); err != nil {
		return err
	}
	applyProgressEvent(session, event)
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
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event progressEvent
		if json.Unmarshal([]byte(line), &event) != nil || event.ID != session.ID {
			continue
		}
		applyProgressEvent(session, event)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func applyProgressEvent(session *Session, event progressEvent) {
	if event.Outcome != "" {
		for i, action := range session.Actions {
			if action.ActionID == event.ActionID {
				session.Outcomes[i] = event.Outcome
				break
			}
		}
	}
	if event.Position >= 0 && event.Position <= len(session.Actions) {
		session.CurrentPosition = event.Position
	}
	if event.State != "" {
		session.State = event.State
	}
	if !event.At.IsZero() {
		session.UpdatedAt = event.At
	}
}

func validateManifest(manifest Manifest) error {
	if manifest.FormatVersion != FormatVersion || strings.TrimSpace(manifest.ID) == "" || strings.TrimSpace(manifest.PlanID) == "" {
		return errors.New("manual cleanup progress is invalid")
	}
	if manifest.Mode != ModeInstagramManual || manifest.CreatedAt.IsZero() || len(manifest.Actions) == 0 {
		return errors.New("manual cleanup progress is invalid")
	}
	seen := make(map[string]struct{}, len(manifest.Actions))
	for _, action := range manifest.Actions {
		if strings.TrimSpace(action.ActionID) == "" || strings.TrimSpace(action.TargetURL) == "" {
			return errors.New("manual cleanup progress is invalid")
		}
		if _, ok := seen[action.ActionID]; ok {
			return errors.New("manual cleanup progress is invalid")
		}
		seen[action.ActionID] = struct{}{}
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
