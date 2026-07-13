package apply

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

const (
	executionsDirName    = "executions"
	identityLocksDirName = ".identity-locks"
	manifestFileName     = "manifest.json"
	journalFileName      = "journal.jsonl"
	summaryFileName      = "summary.json"
	writerLockFileName   = "writer.lock"
)

type executionStoreHooks struct {
	beforeManifest      func() error
	beforeAppend        func(JournalEvent) error
	beforeSummary       func() error
	beforeDirectorySync func(string) error
	beforeJournalRepair func() error
	onAppend            func(JournalEvent)
}

type ExecutionStore struct {
	root  string
	hooks executionStoreHooks
}

type ExecutionWriter struct {
	store        *ExecutionStore
	manifest     ExecutionManifest
	lock         *flock.Flock
	nextSequence int64
	closed       bool
}

func NewExecutionStore(workspaceDir string) *ExecutionStore {
	workspaceDir = filepath.Clean(strings.TrimSpace(workspaceDir))
	if workspaceDir == "" || workspaceDir == "." {
		return nil
	}
	return &ExecutionStore{root: filepath.Join(workspaceDir, executionsDirName)}
}

func (store *ExecutionStore) Root() string {
	if store == nil {
		return ""
	}
	return store.root
}

func (store *ExecutionStore) Create(manifest ExecutionManifest, now time.Time) (*ExecutionWriter, ExecutionSummary, error) {
	if store == nil {
		return nil, ExecutionSummary{}, ErrExecutionStoreUnavailable
	}
	if err := validateExecutionManifest(manifest); err != nil {
		return nil, ExecutionSummary{}, err
	}
	if err := store.ensureRoots(); err != nil {
		return nil, ExecutionSummary{}, err
	}
	identityLockPath := store.identityLockPath(manifest.Fingerprint)
	if err := rejectExistingSymlink(identityLockPath); err != nil {
		return nil, ExecutionSummary{}, err
	}
	identityLock := flock.New(identityLockPath, flock.SetPermissions(0o600))
	locked, err := identityLock.TryLock()
	if err != nil {
		_ = identityLock.Close()
		return nil, ExecutionSummary{}, err
	}
	if !locked {
		_ = identityLock.Close()
		return nil, ExecutionSummary{}, ErrExecutionLocked
	}
	defer identityLock.Close()

	if existing, ok, err := store.FindByFingerprint(manifest.Fingerprint); err != nil {
		return nil, ExecutionSummary{}, err
	} else if ok {
		return nil, existing, ExistingExecutionError{Summary: existing}
	}

	dir, err := store.executionDir(manifest.ExecutionID)
	if err != nil {
		return nil, ExecutionSummary{}, err
	}
	if _, err := os.Lstat(dir); err == nil {
		return nil, ExecutionSummary{}, ErrExecutionExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, ExecutionSummary{}, err
	}
	if err := ensurePrivateDir(dir); err != nil {
		return nil, ExecutionSummary{}, err
	}
	writerLockPath := filepath.Join(dir, writerLockFileName)
	if err := rejectExistingSymlink(writerLockPath); err != nil {
		return nil, ExecutionSummary{}, err
	}
	writerLock := flock.New(writerLockPath, flock.SetPermissions(0o600))
	locked, err = writerLock.TryLock()
	if err != nil || !locked {
		_ = writerLock.Close()
		if err != nil {
			return nil, ExecutionSummary{}, err
		}
		return nil, ExecutionSummary{}, ErrExecutionLocked
	}
	writer := &ExecutionWriter{store: store, manifest: manifest, lock: writerLock, nextSequence: 1}
	if store.hooks.beforeManifest != nil {
		if err := store.hooks.beforeManifest(); err != nil {
			writer.Close()
			return nil, ExecutionSummary{}, err
		}
	}
	if err := writeJSONAtomic(filepath.Join(dir, manifestFileName), manifest); err != nil {
		writer.Close()
		return nil, ExecutionSummary{}, err
	}
	if now.IsZero() {
		now = manifest.CreatedAt
	}
	summary := initialExecutionSummary(manifest, now)
	event := JournalEvent{Timestamp: now.UTC(), Kind: JournalExecutionStarted}
	committed, err := writer.Append(event, summary)
	if err != nil {
		writer.Close()
		return nil, ExecutionSummary{}, err
	}
	summary.LastSequence = committed.Sequence
	return writer, summary, nil
}

func (store *ExecutionStore) OpenWriter(id ExecutionID) (*ExecutionWriter, ExecutionView, error) {
	if store == nil {
		return nil, ExecutionView{}, ErrExecutionStoreUnavailable
	}
	dir, err := store.executionDir(id)
	if err != nil {
		return nil, ExecutionView{}, err
	}
	if err := rejectSymlink(dir); err != nil {
		return nil, ExecutionView{}, err
	}
	writerLockPath := filepath.Join(dir, writerLockFileName)
	if err := rejectExistingSymlink(writerLockPath); err != nil {
		return nil, ExecutionView{}, err
	}
	writerLock := flock.New(writerLockPath, flock.SetPermissions(0o600))
	locked, err := writerLock.TryLock()
	if err != nil {
		_ = writerLock.Close()
		return nil, ExecutionView{}, err
	}
	if !locked {
		_ = writerLock.Close()
		return nil, ExecutionView{}, ErrExecutionLocked
	}
	writer := &ExecutionWriter{store: store, lock: writerLock}
	view, err := store.Replay(id)
	if err != nil {
		writer.Close()
		return nil, ExecutionView{}, err
	}
	if view.ignoredPartialTail {
		if err := writer.repairPartialJournal(view); err != nil {
			writer.Close()
			return nil, ExecutionView{}, err
		}
		view, err = store.Replay(id)
		if err != nil || view.ignoredPartialTail {
			writer.Close()
			return nil, ExecutionView{}, ErrExecutionCorrupt
		}
	}
	writer.manifest = view.Manifest
	writer.nextSequence = view.LastSequence + 1
	return writer, view, nil
}

func (writer *ExecutionWriter) Append(event JournalEvent, summary ExecutionSummary) (JournalEvent, error) {
	if writer == nil || writer.closed || writer.store == nil {
		return JournalEvent{}, ErrExecutionStoreUnavailable
	}
	if writer.store.hooks.beforeAppend != nil {
		if err := writer.store.hooks.beforeAppend(event); err != nil {
			return JournalEvent{}, err
		}
	}
	event.ExecutionID = writer.manifest.ExecutionID
	event.Fingerprint = writer.manifest.Fingerprint
	event.Sequence = writer.nextSequence
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	} else {
		event.Timestamp = event.Timestamp.UTC()
	}
	dir, err := writer.store.executionDir(writer.manifest.ExecutionID)
	if err != nil {
		return JournalEvent{}, err
	}
	if err := writer.store.validateRoot(); err != nil {
		return JournalEvent{}, err
	}
	if err := rejectSymlink(dir); err != nil {
		return JournalEvent{}, err
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return JournalEvent{}, err
	}
	encoded = append(encoded, '\n')
	journalPath := filepath.Join(dir, journalFileName)
	createdJournal := false
	if info, statErr := os.Lstat(journalPath); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return JournalEvent{}, ErrExecutionCorrupt
		}
	} else if errors.Is(statErr, os.ErrNotExist) {
		createdJournal = true
	} else {
		return JournalEvent{}, statErr
	}
	file, err := os.OpenFile(journalPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return JournalEvent{}, err
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return JournalEvent{}, err
	}
	written, writeErr := file.Write(encoded)
	if writeErr == nil && written != len(encoded) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		return JournalEvent{}, writeErr
	}
	if createdJournal {
		if err := writer.store.syncDirectory(dir); err != nil {
			return JournalEvent{}, err
		}
	}
	writer.nextSequence++
	if writer.store.hooks.onAppend != nil {
		writer.store.hooks.onAppend(event)
	}
	info, err := os.Stat(journalPath)
	if err != nil {
		return event, err
	}
	summary.FormatVersion = ExecutionJournalFormatVersion
	summary.ExecutionID = writer.manifest.ExecutionID
	summary.Fingerprint = writer.manifest.Fingerprint
	summary.CreatedAt = writer.manifest.CreatedAt
	summary.UpdatedAt = event.Timestamp
	summary.SourceLabel = writer.manifest.Summary.SourceLabel
	summary.Platform = writer.manifest.Platform
	summary.Mode = writer.manifest.Mode
	summary.LastSequence = event.Sequence
	summary.JournalBytes = info.Size()
	if writer.store.hooks.beforeSummary != nil {
		if err := writer.store.hooks.beforeSummary(); err != nil {
			return event, err
		}
	}
	if err := writeJSONAtomic(filepath.Join(dir, summaryFileName), summary); err != nil {
		return event, err
	}
	return event, nil
}

func (writer *ExecutionWriter) repairPartialJournal(view ExecutionView) error {
	if writer == nil || writer.store == nil || writer.lock == nil || !writer.lock.Locked() || writer.closed || !view.ignoredPartialTail || view.journalCompleteAt < 0 {
		return ErrExecutionCorrupt
	}
	if writer.store.hooks.beforeJournalRepair != nil {
		if err := writer.store.hooks.beforeJournalRepair(); err != nil {
			return err
		}
	}
	dir, err := writer.store.executionDir(view.Manifest.ExecutionID)
	if err != nil {
		return err
	}
	if err := writer.store.validateRoot(); err != nil {
		return err
	}
	if err := rejectSymlink(dir); err != nil {
		return err
	}
	journalPath := filepath.Join(dir, journalFileName)
	if err := rejectSymlink(journalPath); err != nil {
		return err
	}
	file, err := os.OpenFile(journalPath, os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	info, statErr := file.Stat()
	if statErr != nil || info.Size() <= view.journalCompleteAt {
		file.Close()
		return ErrExecutionCorrupt
	}
	repairErr := file.Truncate(view.journalCompleteAt)
	if repairErr == nil {
		repairErr = file.Sync()
	}
	closeErr := file.Close()
	if repairErr == nil {
		repairErr = closeErr
	}
	if repairErr != nil {
		return repairErr
	}
	return writer.store.syncDirectory(dir)
}

func (store *ExecutionStore) syncDirectory(path string) error {
	if store != nil && store.hooks.beforeDirectorySync != nil {
		if err := store.hooks.beforeDirectorySync(path); err != nil {
			return err
		}
	}
	return syncDirectory(path)
}

func (writer *ExecutionWriter) Close() error {
	if writer == nil || writer.closed {
		return nil
	}
	writer.closed = true
	if writer.lock == nil {
		return nil
	}
	return writer.lock.Close()
}

func (store *ExecutionStore) List() ([]ExecutionSummary, error) {
	if store == nil {
		return nil, ErrExecutionStoreUnavailable
	}
	if err := store.validateRoot(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	entries, err := os.ReadDir(store.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	summaries := make([]ExecutionSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validStoreKey(entry.Name()) {
			continue
		}
		dir := filepath.Join(store.root, entry.Name())
		summary, err := loadExecutionSummary(filepath.Join(dir, summaryFileName))
		if err != nil || executionStoreKey(summary.ExecutionID) != entry.Name() {
			summary = ExecutionSummary{FormatVersion: ExecutionJournalFormatVersion, Resumability: ResumabilityCorrupt, State: ExecutionStateFailed, BlockReason: "Execution data is unreadable.", storeKey: entry.Name()}
			if manifest, manifestErr := loadExecutionManifest(filepath.Join(dir, manifestFileName)); manifestErr == nil {
				if executionStoreKey(manifest.ExecutionID) == entry.Name() {
					summary.ExecutionID = manifest.ExecutionID
					summary.Fingerprint = manifest.Fingerprint
					summary.CreatedAt = manifest.CreatedAt
					summary.UpdatedAt = manifest.CreatedAt
					summary.SourceLabel = manifest.Summary.SourceLabel
					summary.Platform = manifest.Platform
					summary.Mode = manifest.Mode
				}
			}
			summaries = append(summaries, summary)
			continue
		}
		summary.storeKey = entry.Name()
		journalPath := filepath.Join(dir, journalFileName)
		if linkErr := rejectExistingSymlink(journalPath); linkErr != nil {
			summary.Resumability = ResumabilityCorrupt
			summary.BlockReason = "Execution data is unreadable."
			summaries = append(summaries, summary)
			continue
		}
		if info, statErr := os.Stat(journalPath); statErr != nil || info.Size() != summary.JournalBytes {
			summary.RecoveryWarning = "Execution needs recovery before its state can be confirmed."
		}
		writerLockPath := filepath.Join(dir, writerLockFileName)
		if err := rejectExistingSymlink(writerLockPath); err != nil {
			summary.Resumability = ResumabilityCorrupt
			summary.BlockReason = "Execution data is unreadable."
			summaries = append(summaries, summary)
			continue
		}
		lock := flock.New(writerLockPath, flock.SetPermissions(0o600))
		locked, lockErr := lock.TryLock()
		if lockErr == nil && locked {
			_ = lock.Close()
		} else if lockErr == nil {
			_ = lock.Close()
			summary.Resumability = ResumabilityLocked
			summary.BlockReason = "Execution is active in another Vanish process."
		} else {
			_ = lock.Close()
			summary.Resumability = ResumabilityCorrupt
			summary.BlockReason = "Execution data is unreadable."
		}
		summaries = append(summaries, summary)
	}
	sort.SliceStable(summaries, func(i, j int) bool { return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt) })
	return summaries, nil
}

func (store *ExecutionStore) FindByFingerprint(fingerprint string) (ExecutionSummary, bool, error) {
	if !validFingerprint(fingerprint) {
		return ExecutionSummary{}, false, ErrExecutionIdentityMismatch
	}
	if err := store.validateRoot(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ExecutionSummary{}, false, nil
		}
		return ExecutionSummary{}, false, err
	}
	entries, err := os.ReadDir(store.root)
	if err != nil {
		return ExecutionSummary{}, false, err
	}
	var match ExecutionSummary
	found := false
	for _, entry := range entries {
		if !entry.IsDir() || !validStoreKey(entry.Name()) {
			continue
		}
		dir := filepath.Join(store.root, entry.Name())
		manifest, manifestErr := loadExecutionManifest(filepath.Join(dir, manifestFileName))
		if manifestErr != nil || executionStoreKey(manifest.ExecutionID) != entry.Name() {
			return ExecutionSummary{}, false, ErrExecutionCorrupt
		}
		if manifest.Fingerprint != fingerprint {
			continue
		}
		if found {
			return ExecutionSummary{}, false, ErrExecutionCorrupt
		}
		match = initialExecutionSummary(manifest, manifest.CreatedAt)
		if summary, summaryErr := loadExecutionSummary(filepath.Join(dir, summaryFileName)); summaryErr == nil && executionStoreKey(summary.ExecutionID) == entry.Name() && summary.Fingerprint == manifest.Fingerprint {
			match = summary
		} else {
			match.State = ExecutionStateFailed
			match.Resumability = ResumabilityCorrupt
			match.BlockReason = "Execution data is unreadable."
		}
		found = true
	}
	return match, found, nil
}

func (store *ExecutionStore) RefreshSummary(view ExecutionView) error {
	if store == nil {
		return ErrExecutionStoreUnavailable
	}
	dir, err := store.executionDir(view.Manifest.ExecutionID)
	if err != nil {
		return err
	}
	if err := store.validateRoot(); err != nil {
		return err
	}
	if err := rejectSymlink(dir); err != nil {
		return err
	}
	journalPath := filepath.Join(dir, journalFileName)
	if err := rejectExistingSymlink(journalPath); err != nil {
		return err
	}
	info, err := os.Stat(journalPath)
	if err != nil {
		return err
	}
	summary := summaryFromView(view)
	summary.JournalBytes = info.Size()
	return writeJSONAtomic(filepath.Join(dir, summaryFileName), summary)
}

func (store *ExecutionStore) Abandon(id ExecutionID, at time.Time) (ExecutionView, error) {
	writer, view, err := store.OpenWriter(id)
	if err != nil {
		return ExecutionView{}, err
	}
	defer writer.Close()
	if view.Resumability == ResumabilityTerminal {
		return ExecutionView{}, ErrExecutionTerminal
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	summary := summaryFromView(view)
	summary.State = ExecutionStateAbandoned
	summary.Resumability = ResumabilityTerminal
	summary.BlockReason = "Execution was abandoned."
	if _, err := writer.Append(JournalEvent{Timestamp: at, Kind: JournalExecutionAbandoned}, summary); err != nil {
		return ExecutionView{}, err
	}
	view, err = store.Replay(id)
	if err == nil {
		err = store.RefreshSummary(view)
	}
	return view, err
}

func (store *ExecutionStore) Delete(summary ExecutionSummary) error {
	if store == nil {
		return ErrExecutionStoreUnavailable
	}
	key := summary.storeKey
	if key == "" && summary.ExecutionID != "" {
		key = executionStoreKey(summary.ExecutionID)
	}
	if !validStoreKey(key) {
		return ErrExecutionCorrupt
	}
	dir := filepath.Join(store.root, key)
	if err := store.validateRoot(); err != nil {
		return err
	}
	if err := rejectSymlink(dir); err != nil {
		return err
	}
	writerLockPath := filepath.Join(dir, writerLockFileName)
	if err := rejectExistingSymlink(writerLockPath); err != nil {
		return err
	}
	lock := flock.New(writerLockPath, flock.SetPermissions(0o600))
	locked, err := lock.TryLock()
	if err != nil {
		_ = lock.Close()
		return err
	}
	if !locked {
		_ = lock.Close()
		return ErrExecutionLocked
	}
	if summary.Resumability != ResumabilityCorrupt && summary.ExecutionID != "" {
		view, replayErr := store.Replay(summary.ExecutionID)
		if replayErr == nil && view.Resumability != ResumabilityTerminal {
			_ = lock.Close()
			return ErrExecutionMustAbandon
		}
		if replayErr != nil {
			_ = lock.Close()
			return replayErr
		}
	}
	// Windows cannot remove a directory while its lock file is open. The
	// terminal-state check above occurs under the exclusive writer lock; close
	// it immediately before deleting the now-inactive directory.
	if err := lock.Close(); err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	// Identity lock files are intentionally retained. Reusing the same stable
	// lock path serializes any later recreation of this execution identity and
	// avoids unlinking a lock another process may already have opened. A full
	// local-data wipe removes the entire executions root, including these files.
	return syncDirectory(store.root)
}

func (store *ExecutionStore) ensureRoots() error {
	if err := ensurePrivateDirNoSymlink(store.root); err != nil {
		return err
	}
	return ensurePrivateDirNoSymlink(filepath.Join(store.root, identityLocksDirName))
}

func (store *ExecutionStore) validateRoot() error {
	if store == nil {
		return ErrExecutionStoreUnavailable
	}
	info, err := os.Lstat(store.root)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return ErrExecutionCorrupt
	}
	return nil
}

func (store *ExecutionStore) executionDir(id ExecutionID) (string, error) {
	if store == nil || strings.TrimSpace(string(id)) == "" {
		return "", ErrExecutionStoreUnavailable
	}
	key := executionStoreKey(id)
	path := filepath.Join(store.root, key)
	if err := ensureWithin(store.root, path); err != nil {
		return "", err
	}
	return path, nil
}

func (store *ExecutionStore) identityLockPath(fingerprint string) string {
	return filepath.Join(store.root, identityLocksDirName, fingerprint+".lock")
}

func executionStoreKey(id ExecutionID) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])
}

func validStoreKey(value string) bool {
	return validFingerprint(value)
}

func initialExecutionSummary(manifest ExecutionManifest, at time.Time) ExecutionSummary {
	return ExecutionSummary{
		FormatVersion: ExecutionJournalFormatVersion,
		ExecutionID:   manifest.ExecutionID,
		Fingerprint:   manifest.Fingerprint,
		CreatedAt:     manifest.CreatedAt,
		UpdatedAt:     at.UTC(),
		SourceLabel:   manifest.Summary.SourceLabel,
		Platform:      manifest.Platform,
		Mode:          manifest.Mode,
		State:         ExecutionStateRunning,
		Resumability:  ResumabilityResumable,
		Counts:        CountsForPlan(manifest.Plan),
	}
}

func summaryFromView(view ExecutionView) ExecutionSummary {
	return ExecutionSummary{
		FormatVersion:   ExecutionJournalFormatVersion,
		ExecutionID:     view.Manifest.ExecutionID,
		Fingerprint:     view.Manifest.Fingerprint,
		CreatedAt:       view.Manifest.CreatedAt,
		UpdatedAt:       view.UpdatedAt,
		SourceLabel:     view.Manifest.Summary.SourceLabel,
		Platform:        view.Manifest.Platform,
		Mode:            view.Manifest.Mode,
		State:           view.State,
		Resumability:    view.Resumability,
		BlockReason:     view.BlockReason,
		Counts:          view.Counts,
		LastSequence:    view.LastSequence,
		RecoveryWarning: view.RecoveryWarning,
	}
}

func loadExecutionManifest(path string) (ExecutionManifest, error) {
	if err := rejectSymlink(path); err != nil {
		return ExecutionManifest{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return ExecutionManifest{}, err
	}
	defer file.Close()
	var manifest ExecutionManifest
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return ExecutionManifest{}, ErrExecutionCorrupt
	}
	if err := validateExecutionManifest(manifest); err != nil {
		return ExecutionManifest{}, err
	}
	return manifest, nil
}

func loadExecutionSummary(path string) (ExecutionSummary, error) {
	if err := rejectSymlink(path); err != nil {
		return ExecutionSummary{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ExecutionSummary{}, err
	}
	var summary ExecutionSummary
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&summary); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return ExecutionSummary{}, ErrExecutionCorrupt
	}
	if summary.FormatVersion != ExecutionJournalFormatVersion || summary.ExecutionID == "" || !validFingerprint(summary.Fingerprint) || summary.CreatedAt.IsZero() || summary.UpdatedAt.IsZero() || summary.UpdatedAt.Before(summary.CreatedAt) || !knownExecutionState(summary.State) || !knownResumability(summary.Resumability) || summary.Mode != ExecutionModeSimulation || strings.TrimSpace(string(summary.Platform)) == "" || summary.LastSequence < 1 || summary.JournalBytes < 0 || !validResultCounts(summary.Counts) || !knownSummaryBlockReason(summary.BlockReason) || !knownRecoveryWarning(summary.RecoveryWarning) {
		return ExecutionSummary{}, ErrExecutionCorrupt
	}
	return summary, nil
}

func knownExecutionState(state ExecutionState) bool {
	switch state {
	case ExecutionStatePending, ExecutionStateRunning, ExecutionStateDone, ExecutionStateFailed, ExecutionStateSkipped, ExecutionStateStopped, ExecutionStateCancelled, ExecutionStateHalted, ExecutionStateAbandoned:
		return true
	default:
		return false
	}
}

func validResultCounts(counts ResultCounts) bool {
	return counts.Pending >= 0 && counts.Running >= 0 && counts.Done >= 0 && counts.Failed >= 0 && counts.Skipped >= 0 && counts.Stopped >= 0 && counts.Cancelled >= 0
}

func knownSummaryBlockReason(reason string) bool {
	switch reason {
	case "", "Action result is pending.", "A previous action has an unknown result.", "Reconnect the account before resuming.", "Retry time has not arrived.", "Execution paused. Resume is explicit.", "Execution was stopped.", "Execution was stopped with no remaining work.", "Execution completed.", "Execution was cancelled.", "Execution failed.", "Execution was abandoned.", "Execution ended.", "Execution data is unreadable.", "Execution is active in another Vanish process.":
		return true
	default:
		return false
	}
}

func knownRecoveryWarning(warning string) bool {
	switch warning {
	case "", "An interrupted final journal write was ignored.", "Execution needs recovery before its state can be confirmed.":
		return true
	default:
		return false
	}
}

func knownResumability(value Resumability) bool {
	switch value {
	case ResumabilityResumable, ResumabilityWaitingRetry, ResumabilityWaitingProvider, ResumabilityResolution, ResumabilityTerminal, ResumabilityCorrupt, ResumabilityLocked:
		return true
	default:
		return false
	}
}

func writeJSONAtomic(path string, value any) error {
	if err := ensurePrivateDir(filepath.Dir(path)); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func ensurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func ensurePrivateDirNoSymlink(path string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return ErrExecutionCorrupt
		}
		return os.Chmod(path, 0o700)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err = os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return ErrExecutionCorrupt
	}
	return os.Chmod(path, 0o700)
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func ensureWithin(root, candidate string) error {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return ErrExecutionCorrupt
	}
	return nil
}

func rejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrExecutionCorrupt
	}
	return nil
}

func rejectExistingSymlink(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrExecutionCorrupt
	}
	return nil
}

func (summary ExecutionSummary) String() string {
	return fmt.Sprintf("%s %s", summary.SourceLabel, summary.Resumability)
}
