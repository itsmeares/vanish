package apply

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

func (store *ExecutionStore) Replay(id ExecutionID) (ExecutionView, error) {
	if store == nil {
		return ExecutionView{}, ErrExecutionStoreUnavailable
	}
	if err := store.validateRoot(); err != nil {
		return ExecutionView{}, ErrExecutionCorrupt
	}
	dir, err := store.executionDir(id)
	if err != nil {
		return ExecutionView{}, err
	}
	if err := rejectSymlink(dir); err != nil {
		return ExecutionView{}, ErrExecutionCorrupt
	}
	manifest, err := loadExecutionManifest(filepath.Join(dir, manifestFileName))
	if err != nil {
		return ExecutionView{}, ErrExecutionCorrupt
	}
	if manifest.ExecutionID != id {
		return ExecutionView{}, ErrExecutionIdentityMismatch
	}
	view := ExecutionView{
		Manifest:       manifest,
		Plan:           cloneExecutionPlan(manifest.Plan),
		State:          ExecutionStatePending,
		AttemptHistory: make(map[string][]AttemptRecord, len(manifest.Plan.Actions)),
		LastAttempts:   make(map[string]int, len(manifest.Plan.Actions)),
		UpdatedAt:      manifest.CreatedAt,
	}
	view.Counts = CountsForPlan(view.Plan)
	actionIndex := make(map[string]int, len(view.Plan.Actions))
	for index, action := range view.Plan.Actions {
		actionIndex[action.ID] = index
	}
	journalPath := filepath.Join(dir, journalFileName)
	if err := rejectSymlink(journalPath); err != nil {
		return ExecutionView{}, ErrExecutionCorrupt
	}
	file, err := os.Open(journalPath)
	if err != nil {
		return ExecutionView{}, ErrExecutionCorrupt
	}
	defer file.Close()

	reader := bufio.NewReaderSize(file, 64*1024)
	expectedSequence := int64(1)
	started := false
	terminal := false
	lastActionIndex := -1
	lastResult := make(map[string]ActionResult, len(view.Plan.Actions))
	var inFlight *JournalEvent
	var lastKind JournalEventKind
	var lastOutcome ActionOutcome
	var completeAt int64

	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			terminated := line[len(line)-1] == '\n'
			if !terminated {
				if errors.Is(readErr, io.EOF) {
					view.RecoveryWarning = "An interrupted final journal write was ignored."
					view.journalCompleteAt = completeAt
					view.ignoredPartialTail = true
					break
				}
				return ExecutionView{}, ErrExecutionCorrupt
			}
			line = bytes.TrimSuffix(line, []byte{'\n'})
			if len(line) == 0 {
				return ExecutionView{}, ErrExecutionCorrupt
			}
			event, decodeErr := decodeJournalEvent(line)
			if decodeErr != nil || event.ExecutionID != manifest.ExecutionID || event.Fingerprint != manifest.Fingerprint || event.Sequence != expectedSequence || event.Timestamp.IsZero() || (expectedSequence > 1 && event.Timestamp.Before(view.UpdatedAt)) {
				return ExecutionView{}, ErrExecutionCorrupt
			}
			if terminal {
				return ExecutionView{}, ErrExecutionCorrupt
			}
			if err := applyJournalEvent(&view, event, actionIndex, &started, &terminal, &lastActionIndex, lastResult, &inFlight, lastKind, &lastOutcome); err != nil {
				return ExecutionView{}, ErrExecutionCorrupt
			}
			expectedSequence++
			view.LastSequence = event.Sequence
			view.UpdatedAt = event.Timestamp.UTC()
			lastKind = event.Kind
			completeAt += int64(len(line) + 1)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return ExecutionView{}, ErrExecutionCorrupt
		}
	}
	if !view.ignoredPartialTail {
		view.journalCompleteAt = completeAt
	}
	if !started {
		return ExecutionView{}, ErrExecutionCorrupt
	}
	if inFlight != nil && !terminal {
		view.Unresolved = &UnresolvedAttempt{
			ActionID:   inFlight.ActionID,
			ActionType: inFlight.ActionType,
			Platform:   inFlight.Platform,
			Attempt:    inFlight.Attempt,
			StartedAt:  inFlight.Timestamp,
		}
	}
	classifyExecutionView(&view, lastResult, terminal, lastKind)
	return view, nil
}

func decodeJournalEvent(line []byte) (JournalEvent, error) {
	var event JournalEvent
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return JournalEvent{}, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return JournalEvent{}, ErrExecutionCorrupt
	}
	if !knownJournalKind(event.Kind) {
		return JournalEvent{}, ErrExecutionCorrupt
	}
	return event, nil
}

func applyJournalEvent(
	view *ExecutionView,
	event JournalEvent,
	actionIndex map[string]int,
	started *bool,
	terminal *bool,
	lastActionIndex *int,
	lastResult map[string]ActionResult,
	inFlight **JournalEvent,
	lastKind JournalEventKind,
	lastOutcome *ActionOutcome,
) error {
	switch event.Kind {
	case JournalExecutionStarted:
		if *started || event.Sequence != 1 || hasActionFields(event) || event.HaltReason != "" {
			return ErrExecutionCorrupt
		}
		*started = true
		view.State = ExecutionStateRunning
	case JournalAttemptStarted:
		if !*started || *inFlight != nil || view.State != ExecutionStateRunning || event.HaltReason != "" || event.Outcome != "" || event.Status != "" || event.RetryAfterMillis != 0 || event.ProviderCode != "" || event.MessageID != "" {
			return ErrExecutionCorrupt
		}
		index, ok := actionIndex[event.ActionID]
		if !ok || index < *lastActionIndex || event.Attempt != view.LastAttempts[event.ActionID]+1 || event.Attempt > view.Manifest.Policy.MaxAttemptsPerAction {
			return ErrExecutionCorrupt
		}
		action := &view.Plan.Actions[index]
		if action.Platform != event.Platform || action.Type != event.ActionType {
			return ErrExecutionCorrupt
		}
		if event.Attempt == 1 {
			if action.Status != domain.ActionStatusPending {
				return ErrExecutionCorrupt
			}
		} else {
			previous, ok := lastResult[event.ActionID]
			if !ok || !safeResumeOutcome(previous.Outcome) || action.Status != domain.ActionStatusFailed {
				return ErrExecutionCorrupt
			}
			if (previous.Outcome == OutcomeRateLimited || previous.Outcome == OutcomeAuthenticationRequired || previous.RetryAfter > 0) && lastKind != JournalExecutionResumed {
				return ErrExecutionCorrupt
			}
			if previous.RetryAfter > 0 {
				previousHistory := view.AttemptHistory[event.ActionID]
				if len(previousHistory) == 0 || event.Timestamp.Before(previousHistory[len(previousHistory)-1].ResultAt.Add(previous.RetryAfter)) {
					return ErrExecutionCorrupt
				}
			}
		}
		transitionResultCount(&view.Counts, action.Status, domain.ActionStatusRunning)
		action.Status = domain.ActionStatusRunning
		view.LastAttempts[event.ActionID] = event.Attempt
		*lastActionIndex = index
		copied := event
		*inFlight = &copied
		view.State = ExecutionStateRunning
	case JournalResultRecorded:
		if !*started || *inFlight == nil || event.HaltReason != "" {
			return ErrExecutionCorrupt
		}
		startedEvent := *inFlight
		if event.ActionID != startedEvent.ActionID || event.ActionType != startedEvent.ActionType || event.Platform != startedEvent.Platform || event.Attempt != startedEvent.Attempt {
			return ErrExecutionCorrupt
		}
		status, ok := statusForOutcome(event.Outcome)
		if !ok || status != event.Status || event.RetryAfterMillis < 0 || !event.ProviderCode.Known() || !event.MessageID.Known() {
			return ErrExecutionCorrupt
		}
		if event.MessageID != "" {
			if _, valid := runtimeProviderMessage(event.Outcome, event.MessageID); !valid {
				return ErrExecutionCorrupt
			}
		}
		if event.RetryAfterMillis > 0 && event.Outcome != OutcomeRetryableFailure && event.Outcome != OutcomeRateLimited {
			return ErrExecutionCorrupt
		}
		duration := journalRetryDuration(event.RetryAfterMillis)
		if event.RetryAfterMillis > 0 && duration == 0 {
			return ErrExecutionCorrupt
		}
		message, _ := runtimeProviderMessage(event.Outcome, event.MessageID)
		result := ActionResult{
			ActionID:     event.ActionID,
			Platform:     event.Platform,
			Type:         event.ActionType,
			Status:       event.Status,
			Outcome:      event.Outcome,
			Attempt:      event.Attempt,
			RetryAfter:   duration,
			ProviderCode: event.ProviderCode,
			MessageID:    event.MessageID,
			Message:      message,
		}
		index := actionIndex[event.ActionID]
		transitionResultCount(&view.Counts, view.Plan.Actions[index].Status, event.Status)
		view.Plan.Actions[index].Status = event.Status
		view.AttemptHistory[event.ActionID] = append(view.AttemptHistory[event.ActionID], AttemptRecord{StartedAt: startedEvent.Timestamp, ResultAt: event.Timestamp, Result: result})
		lastResult[event.ActionID] = result
		*lastOutcome = result.Outcome
		*inFlight = nil
	case JournalExecutionResumed:
		if !*started || *inFlight != nil || hasActionFields(event) || event.HaltReason != "" || lastKind == JournalExecutionResumed {
			return ErrExecutionCorrupt
		}
		view.State = ExecutionStateRunning
		view.HaltReason = ""
	case JournalExecutionHalted:
		if !*started || *inFlight != nil || hasActionFields(event) || !safeResumeOutcome(event.HaltReason) || lastKind != JournalResultRecorded || *lastOutcome != event.HaltReason {
			return ErrExecutionCorrupt
		}
		view.State = ExecutionStateHalted
		view.HaltReason = event.HaltReason
	case JournalExecutionStopped:
		if !*started || *inFlight != nil || hasActionFields(event) || event.HaltReason != "" || lastKind != JournalResultRecorded || *lastOutcome != OutcomeStopped {
			return ErrExecutionCorrupt
		}
		view.State = ExecutionStateStopped
		if view.Counts.Pending == 0 {
			view.TerminalKind = event.Kind
			*terminal = true
		}
	case JournalExecutionCancelled:
		if !*started || *inFlight != nil || hasActionFields(event) || event.HaltReason != "" {
			return ErrExecutionCorrupt
		}
		CancelPending(&view.Plan, "Execution cancelled.")
		view.Counts = CountsForPlan(view.Plan)
		view.State = ExecutionStateCancelled
		view.TerminalKind = event.Kind
		*terminal = true
	case JournalExecutionFailed:
		if !*started || *inFlight != nil || hasActionFields(event) || event.HaltReason != "" || (lastKind != JournalResultRecorded && lastKind != JournalExecutionResumed && lastKind != JournalExecutionStarted) {
			return ErrExecutionCorrupt
		}
		if view.Counts.Failed == 0 {
			return ErrExecutionCorrupt
		}
		view.State = ExecutionStateFailed
		view.TerminalKind = event.Kind
		*terminal = true
	case JournalExecutionCompleted:
		if !*started || *inFlight != nil || hasActionFields(event) || event.HaltReason != "" {
			return ErrExecutionCorrupt
		}
		counts := view.Counts
		if counts.Pending > 0 || counts.Running > 0 || counts.Failed > 0 || counts.Cancelled > 0 {
			return ErrExecutionCorrupt
		}
		view.State = stateForCounts(counts)
		view.TerminalKind = event.Kind
		*terminal = true
	case JournalExecutionAbandoned:
		if !*started || hasActionFields(event) || event.HaltReason != "" {
			return ErrExecutionCorrupt
		}
		view.State = ExecutionStateAbandoned
		view.TerminalKind = event.Kind
		*terminal = true
	default:
		return ErrExecutionCorrupt
	}
	return nil
}

func classifyExecutionView(view *ExecutionView, lastResult map[string]ActionResult, terminal bool, lastKind JournalEventKind) {
	view.Counts = CountsForPlan(view.Plan)
	if terminal {
		view.Resumability = ResumabilityTerminal
		view.BlockReason = terminalReason(view.TerminalKind)
		return
	}
	if view.Unresolved != nil {
		view.Resumability = ResumabilityResolution
		view.BlockReason = "A previous action has an unknown result."
		return
	}
	var gatedResult ActionResult
	var gatedAt time.Time
	for actionID, result := range lastResult {
		if !safeResumeOutcome(result.Outcome) {
			continue
		}
		history := view.AttemptHistory[actionID]
		if len(history) == 0 || (!gatedAt.IsZero() && !history[len(history)-1].ResultAt.After(gatedAt)) {
			continue
		}
		gatedResult = result
		gatedAt = history[len(history)-1].ResultAt
	}
	if gatedResult.Outcome == OutcomeAuthenticationRequired {
		view.Resumability = ResumabilityWaitingProvider
		view.BlockReason = "Reconnect the account before resuming."
	} else if gatedResult.RetryAfter > 0 {
		view.RetryNotBefore = gatedAt.Add(gatedResult.RetryAfter)
		view.Resumability = ResumabilityWaitingRetry
		view.BlockReason = "Retry time has not arrived."
	}

	var retryAction string
	var retryAttempt int
	for index := range view.Plan.Actions {
		action := view.Plan.Actions[index]
		result, ok := lastResult[action.ID]
		if ok && safeResumeOutcome(result.Outcome) && view.LastAttempts[action.ID] < view.Manifest.Policy.MaxAttemptsPerAction {
			retryAction = action.ID
			retryAttempt = view.LastAttempts[action.ID] + 1
			if result.RetryAfter > 0 {
				history := view.AttemptHistory[action.ID]
				view.RetryNotBefore = history[len(history)-1].ResultAt.Add(result.RetryAfter)
			}
			if result.Outcome == OutcomeAuthenticationRequired {
				view.Resumability = ResumabilityWaitingProvider
				view.BlockReason = "Reconnect the account before resuming."
			} else if !view.RetryNotBefore.IsZero() {
				view.Resumability = ResumabilityWaitingRetry
				view.BlockReason = "Retry time has not arrived."
			} else if view.Resumability == "" {
				view.Resumability = ResumabilityResumable
			}
			break
		}
	}
	if retryAction != "" {
		view.NextActionID = retryAction
		view.NextAttempt = retryAttempt
		return
	}
	for _, action := range view.Plan.Actions {
		if action.Status == domain.ActionStatusPending {
			view.NextActionID = action.ID
			view.NextAttempt = view.LastAttempts[action.ID] + 1
			if view.NextAttempt == 0 {
				view.NextAttempt = 1
			}
			if view.Resumability == "" {
				view.Resumability = ResumabilityResumable
			}
			return
		}
	}
	if lastKind == JournalExecutionStopped {
		view.Resumability = ResumabilityTerminal
		view.BlockReason = "Execution was stopped with no remaining work."
		return
	}
	view.Resumability = ResumabilityResumable
	view.NeedsFinalization = true
}

func knownJournalKind(kind JournalEventKind) bool {
	switch kind {
	case JournalExecutionStarted, JournalAttemptStarted, JournalResultRecorded, JournalExecutionResumed, JournalExecutionHalted, JournalExecutionStopped, JournalExecutionCancelled, JournalExecutionFailed, JournalExecutionCompleted, JournalExecutionAbandoned:
		return true
	default:
		return false
	}
}

func safeResumeOutcome(outcome ActionOutcome) bool {
	switch outcome {
	case OutcomeRetryableFailure, OutcomeRateLimited, OutcomeAuthenticationRequired:
		return true
	default:
		return false
	}
}

func hasActionFields(event JournalEvent) bool {
	return strings.TrimSpace(event.ActionID) != "" || event.ActionType != "" || event.Platform != "" || event.Attempt != 0 || event.Outcome != "" || event.Status != "" || event.RetryAfterMillis != 0 || event.ProviderCode != "" || event.MessageID != ""
}

func terminalReason(kind JournalEventKind) string {
	switch kind {
	case JournalExecutionCompleted:
		return "Execution completed."
	case JournalExecutionCancelled:
		return "Execution was cancelled."
	case JournalExecutionStopped:
		return "Execution was stopped with no remaining work."
	case JournalExecutionFailed:
		return "Execution failed."
	case JournalExecutionAbandoned:
		return "Execution was abandoned."
	default:
		return "Execution ended."
	}
}
