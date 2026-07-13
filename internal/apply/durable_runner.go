package apply

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

func (runner Runner) Start(ctx context.Context, plan domain.CleanupPlan, mode ExecutionMode) (Execution, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	preview := runner.Preview(plan, mode)
	execution := Execution{Plan: cloneExecutionPlan(plan), Preview: preview, State: ExecutionStatePending}
	execution.Counts = CountsForPlan(execution.Plan)
	if !preview.CanApply {
		execution.State = ExecutionStateFailed
		return execution, errors.New("execution preview is blocked")
	}
	if runner.Store == nil {
		execution.State = ExecutionStateFailed
		execution.BlockReason = RuntimeErrorMessage(ErrExecutionStoreUnavailable)
		return execution, ErrExecutionStoreUnavailable
	}
	provider, err := runner.Providers.Resolve(plan.Platform, mode)
	if err != nil || provider.Executor() == nil {
		execution.State = ExecutionStateFailed
		return execution, ErrProviderUnavailable
	}
	id, err := NewExecutionID()
	if err != nil {
		return execution, err
	}
	manifest, err := newExecutionManifest(id, plan, mode, provider.ExecutorID(), runner.Policy, runner.now())
	if err != nil {
		return execution, err
	}
	writer, _, err := runner.Store.Create(manifest, manifest.CreatedAt)
	if err != nil {
		var existing ExistingExecutionError
		if errors.As(err, &existing) {
			execution.ID = existing.Summary.ExecutionID
			execution.Resumability = existing.Summary.Resumability
			execution.BlockReason = RuntimeErrorMessage(err)
		}
		return execution, err
	}
	defer writer.Close()
	execution.ID = manifest.ExecutionID
	execution.Plan = cloneExecutionPlan(manifest.Plan)
	execution.State = ExecutionStateRunning
	execution.Resumability = ResumabilityResumable
	execution.Events = append(execution.Events, ExecutionEvent{
		Type: EventExecutionStarted, PlanID: manifest.PlanID, Platform: manifest.Platform,
		State: ExecutionStateRunning, Counts: CountsForPlan(execution.Plan), Mode: manifest.Mode,
		Executor: manifest.Executor, ExecutionID: manifest.ExecutionID, Sequence: 1,
	})
	return runner.executeDurable(ctx, writer, execution, provider, make(map[string]int))
}

func (runner Runner) Resume(ctx context.Context, id ExecutionID) (Execution, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if runner.Store == nil {
		return Execution{}, ErrExecutionStoreUnavailable
	}
	writer, view, err := runner.Store.OpenWriter(id)
	if err != nil {
		return Execution{}, err
	}
	defer writer.Close()
	if view.Resumability == ResumabilityTerminal {
		return executionFromView(view), ErrExecutionTerminal
	}
	if view.Resumability == ResumabilityResolution {
		return executionFromView(view), ErrExecutionResolutionRequired
	}
	if view.Resumability == ResumabilityCorrupt {
		return executionFromView(view), ErrExecutionCorrupt
	}
	now := runner.now()
	if !view.RetryNotBefore.IsZero() && now.Before(view.RetryNotBefore) {
		execution := executionFromView(view)
		execution.BlockReason = "Retry time has not arrived."
		return execution, ErrExecutionNotReady
	}
	provider, err := runner.Providers.Resolve(view.Manifest.Platform, view.Manifest.Mode)
	if err != nil || provider.Executor() == nil || validateResumeIdentity(view, provider, runner.Policy) != nil {
		return executionFromView(view), ErrExecutionIdentityMismatch
	}
	prerequisites := provider.Prerequisites(view.Plan, runner.State)
	if hasBlockingPrerequisites(prerequisites) {
		execution := executionFromView(view)
		execution.Resumability = ResumabilityWaitingProvider
		if len(prerequisites) > 0 {
			execution.BlockReason = prerequisites[0].Message
		}
		return execution, ErrExecutionNotReady
	}
	execution := executionFromView(view)
	execution.Preview = runner.Preview(view.Manifest.Plan, view.Manifest.Mode)
	execution.Events = nil
	execution.State = ExecutionStateRunning
	execution.Resumability = ResumabilityResumable
	summary := summaryForRuntime(view.Manifest, execution.Counts, ExecutionStateRunning, ResumabilityResumable, "", execution.HaltReason, now)
	resumed, err := writer.Append(JournalEvent{Timestamp: now, Kind: JournalExecutionResumed}, summary)
	if err != nil {
		return execution, err
	}
	execution.Events = append(execution.Events, ExecutionEvent{
		Type: EventExecutionResumed, PlanID: view.Manifest.PlanID, Platform: view.Manifest.Platform,
		State: ExecutionStateRunning, Counts: CountsForPlan(execution.Plan), Mode: view.Manifest.Mode,
		Executor: view.Manifest.Executor, ExecutionID: view.Manifest.ExecutionID, Sequence: resumed.Sequence,
	})
	return runner.executeDurable(ctx, writer, execution, provider, cloneAttemptCounts(view.LastAttempts))
}

func (runner Runner) Abandon(id ExecutionID) (Execution, error) {
	if runner.Store == nil {
		return Execution{}, ErrExecutionStoreUnavailable
	}
	view, err := runner.Store.Abandon(id, runner.now())
	if err != nil {
		return Execution{}, err
	}
	execution := executionFromView(view)
	execution.Events = []ExecutionEvent{{
		Type: EventExecutionAbandoned, PlanID: view.Manifest.PlanID, Platform: view.Manifest.Platform,
		State: ExecutionStateAbandoned, Counts: view.Counts, Mode: view.Manifest.Mode,
		Executor: view.Manifest.Executor, ExecutionID: view.Manifest.ExecutionID, Sequence: view.LastSequence,
	}}
	return execution, nil
}

func (runner Runner) executeDurable(ctx context.Context, writer *ExecutionWriter, execution Execution, provider Provider, attempts map[string]int) (Execution, error) {
	policy := writer.manifest.Policy
	executor := provider.Executor()
	// A durable cancelled result is authoritative even if the process stopped
	// before it could append the execution-level cancellation record. Finalize
	// untouched actions without invoking the executor again.
	if execution.Counts.Cancelled > 0 {
		return runner.finishDurable(writer, &execution, JournalExecutionCancelled, ExecutionStateCancelled, "")
	}
	if shouldStopAfterDurableFailure(execution, attempts, policy) {
		return runner.finishDurable(writer, &execution, JournalExecutionFailed, ExecutionStateFailed, "")
	}
	for index := 0; index < len(execution.Plan.Actions); index++ {
		action := &execution.Plan.Actions[index]
		if action.Status != domain.ActionStatusPending && !(action.Status == domain.ActionStatusFailed && attempts[action.ID] > 0) {
			continue
		}
		for {
			attempt := attempts[action.ID] + 1
			if attempt > policy.MaxAttemptsPerAction {
				break
			}
			if err := ctx.Err(); err != nil {
				return runner.finishDurable(writer, &execution, JournalExecutionCancelled, ExecutionStateCancelled, "")
			}
			previousStatus := action.Status
			transitionResultCount(&execution.Counts, previousStatus, domain.ActionStatusRunning)
			action.Status = domain.ActionStatusRunning
			startedSummary := summaryForRuntime(writer.manifest, execution.Counts, ExecutionStateRunning, ResumabilityResolution, "Action result is pending.", "", runner.now())
			started, err := writer.Append(JournalEvent{
				Timestamp: runner.now(), Kind: JournalAttemptStarted, ActionID: action.ID,
				ActionType: action.Type, Platform: action.Platform, Attempt: attempt,
			}, startedSummary)
			if err != nil {
				if started.Sequence == 0 {
					transitionResultCount(&execution.Counts, domain.ActionStatusRunning, previousStatus)
					action.Status = previousStatus
					execution.Resumability = ResumabilityResumable
				} else if view, replayErr := writer.store.Replay(writer.manifest.ExecutionID); replayErr == nil {
					execution = mergeExecutionView(execution, view)
				} else {
					execution.Resumability = ResumabilityResolution
				}
				execution.BlockReason = RuntimeErrorMessage(err)
				return execution, err
			}
			attempts[action.ID] = attempt
			providerResult, executeErr := executor.Execute(ctx, *action)
			result := normalizeProviderResult(ctx, *action, attempt, providerResult, executeErr)
			transitionResultCount(&execution.Counts, domain.ActionStatusRunning, result.Status)
			action.Status = result.Status
			resultState, resultResume, resultBlock := stateAfterResult(result, attempt, policy)
			resultSummary := summaryForRuntime(writer.manifest, execution.Counts, resultState, resultResume, resultBlock, result.Outcome, runner.now())
			recorded, appendErr := writer.Append(journalEventForResult(result, runner.now()), resultSummary)
			if recorded.Sequence != 0 {
				execution.Results = append(execution.Results, result)
				publicEvent := eventForActionResult(writer.manifest.PlanID, result, writer.manifest.Mode, writer.manifest.Executor)
				publicEvent.ExecutionID = writer.manifest.ExecutionID
				publicEvent.Sequence = recorded.Sequence
				execution.Events = append(execution.Events, publicEvent)
			}
			if appendErr != nil {
				execution.Resumability = ResumabilityResolution
				execution.BlockReason = RuntimeErrorMessage(appendErr)
				if view, replayErr := writer.store.Replay(writer.manifest.ExecutionID); replayErr == nil {
					execution = mergeExecutionView(execution, view)
				}
				_ = started
				return execution, appendErr
			}
			if ctx.Err() != nil {
				return runner.finishDurable(writer, &execution, JournalExecutionCancelled, ExecutionStateCancelled, "")
			}
			switch result.Outcome {
			case OutcomeSucceeded, OutcomeAlreadySatisfied:
				break
			case OutcomeRetryableFailure:
				if result.RetryAfter > 0 {
					return runner.finishDurable(writer, &execution, JournalExecutionHalted, ExecutionStateHalted, result.Outcome)
				}
				if attempt < policy.MaxAttemptsPerAction {
					continue
				}
				if policy.StopAfterFinalFailure {
					return runner.finishDurable(writer, &execution, JournalExecutionFailed, ExecutionStateFailed, "")
				}
			case OutcomePermanentFailure:
				if policy.StopAfterFinalFailure {
					return runner.finishDurable(writer, &execution, JournalExecutionFailed, ExecutionStateFailed, "")
				}
			case OutcomeRateLimited, OutcomeAuthenticationRequired:
				return runner.finishDurable(writer, &execution, JournalExecutionHalted, ExecutionStateHalted, result.Outcome)
			case OutcomeStopped:
				return runner.finishDurable(writer, &execution, JournalExecutionStopped, ExecutionStateStopped, "")
			case OutcomeCancelled:
				return runner.finishDurable(writer, &execution, JournalExecutionCancelled, ExecutionStateCancelled, "")
			}
			break
		}
	}
	if execution.Counts.Failed > 0 {
		return runner.finishDurable(writer, &execution, JournalExecutionFailed, ExecutionStateFailed, "")
	}
	return runner.finishDurable(writer, &execution, JournalExecutionCompleted, stateForCounts(execution.Counts), "")
}

func (runner Runner) finishDurable(writer *ExecutionWriter, execution *Execution, kind JournalEventKind, state ExecutionState, haltReason ActionOutcome) (Execution, error) {
	now := runner.now()
	if kind == JournalExecutionCancelled {
		CancelPending(&execution.Plan, "Execution cancelled.")
		execution.Counts = CountsForPlan(execution.Plan)
	}
	resumability := ResumabilityTerminal
	blockReason := terminalReason(kind)
	if kind == JournalExecutionHalted {
		resumability = ResumabilityResumable
		blockReason = "Execution paused. Resume is explicit."
		if haltReason == OutcomeAuthenticationRequired {
			resumability = ResumabilityWaitingProvider
			blockReason = "Reconnect the account before resuming."
		} else if len(execution.Results) > 0 && execution.Results[len(execution.Results)-1].RetryAfter > 0 {
			resumability = ResumabilityWaitingRetry
			blockReason = "Retry time has not arrived."
		}
	} else if kind == JournalExecutionStopped {
		if execution.Counts.Pending > 0 {
			resumability = ResumabilityResumable
			blockReason = "Execution was stopped."
		}
	}
	summary := summaryForRuntime(writer.manifest, execution.Counts, state, resumability, blockReason, haltReason, now)
	journalEvent := JournalEvent{Timestamp: now, Kind: kind, HaltReason: haltReason}
	committed, err := writer.Append(journalEvent, summary)
	if err != nil {
		execution.State = state
		execution.Resumability = resumability
		execution.BlockReason = RuntimeErrorMessage(err)
		return *execution, err
	}
	execution.State = state
	execution.HaltReason = haltReason
	execution.Resumability = resumability
	execution.BlockReason = blockReason
	execution.Events = append(execution.Events, executionFinishedEvent(execution.Plan, state, execution.Counts, writer.manifest.Mode, writer.manifest.Executor, haltReason))
	execution.Events[len(execution.Events)-1].ExecutionID = writer.manifest.ExecutionID
	execution.Events[len(execution.Events)-1].Sequence = committed.Sequence
	view, replayErr := writer.store.Replay(writer.manifest.ExecutionID)
	if replayErr != nil {
		return *execution, replayErr
	}
	if refreshErr := writer.store.RefreshSummary(view); refreshErr != nil {
		return *execution, refreshErr
	}
	return mergeExecutionView(*execution, view), nil
}

func journalEventForResult(result ActionResult, at time.Time) JournalEvent {
	return JournalEvent{
		Timestamp: at, Kind: JournalResultRecorded, ActionID: result.ActionID,
		ActionType: result.Type, Platform: result.Platform, Attempt: result.Attempt,
		Outcome: result.Outcome, Status: result.Status, RetryAfterMillis: durationMilliseconds(result.RetryAfter),
		ProviderCode: result.ProviderCode, MessageID: result.MessageID,
	}
}

func stateAfterResult(result ActionResult, attempt int, policy RunPolicy) (ExecutionState, Resumability, string) {
	switch result.Outcome {
	case OutcomeAuthenticationRequired:
		return ExecutionStateHalted, ResumabilityWaitingProvider, "Reconnect the account before resuming."
	case OutcomeRateLimited:
		if result.RetryAfter > 0 {
			return ExecutionStateHalted, ResumabilityWaitingRetry, "Retry time has not arrived."
		}
		return ExecutionStateHalted, ResumabilityResumable, "Execution paused. Resume is explicit."
	case OutcomeRetryableFailure:
		if result.RetryAfter > 0 {
			return ExecutionStateHalted, ResumabilityWaitingRetry, "Retry time has not arrived."
		}
		if attempt < policy.MaxAttemptsPerAction {
			return ExecutionStateRunning, ResumabilityResumable, ""
		}
	case OutcomeStopped:
		return ExecutionStateStopped, ResumabilityResumable, "Execution was stopped."
	case OutcomeCancelled:
		return ExecutionStateCancelled, ResumabilityTerminal, "Execution was cancelled."
	}
	return ExecutionStateRunning, ResumabilityResumable, ""
}

func summaryForRuntime(manifest ExecutionManifest, counts ResultCounts, state ExecutionState, resumability Resumability, blockReason string, haltReason ActionOutcome, at time.Time) ExecutionSummary {
	_ = haltReason
	return ExecutionSummary{
		FormatVersion: ExecutionJournalFormatVersion,
		ExecutionID:   manifest.ExecutionID,
		Fingerprint:   manifest.Fingerprint,
		CreatedAt:     manifest.CreatedAt,
		UpdatedAt:     at.UTC(),
		SourceLabel:   manifest.Summary.SourceLabel,
		Platform:      manifest.Platform,
		Mode:          manifest.Mode,
		State:         state,
		Resumability:  resumability,
		BlockReason:   blockReason,
		Counts:        counts,
	}
}

func transitionResultCount(counts *ResultCounts, from, to domain.ActionStatus) {
	if counts == nil || from == to {
		return
	}
	adjustResultCount(counts, from, -1)
	adjustResultCount(counts, to, 1)
}

func adjustResultCount(counts *ResultCounts, status domain.ActionStatus, delta int) {
	switch status {
	case domain.ActionStatusPending:
		counts.Pending += delta
	case domain.ActionStatusRunning:
		counts.Running += delta
	case domain.ActionStatusDone:
		counts.Done += delta
	case domain.ActionStatusFailed:
		counts.Failed += delta
	case domain.ActionStatusSkipped:
		counts.Skipped += delta
	case domain.ActionStatusStopped:
		counts.Stopped += delta
	case domain.ActionStatusCancelled:
		counts.Cancelled += delta
	}
}

func shouldStopAfterDurableFailure(execution Execution, attempts map[string]int, policy RunPolicy) bool {
	if !policy.StopAfterFinalFailure || execution.Counts.Failed == 0 {
		return false
	}
	lastResults := make(map[string]ActionResult, execution.Counts.Failed)
	for _, result := range execution.Results {
		lastResults[result.ActionID] = result
	}
	for _, action := range execution.Plan.Actions {
		if action.Status != domain.ActionStatusFailed {
			continue
		}
		result, ok := lastResults[action.ID]
		if ok && safeResumeOutcome(result.Outcome) && attempts[action.ID] < policy.MaxAttemptsPerAction {
			continue
		}
		return true
	}
	return false
}

func executionFromView(view ExecutionView) Execution {
	execution := Execution{
		ID:              view.Manifest.ExecutionID,
		Plan:            cloneExecutionPlan(view.Plan),
		State:           view.State,
		Counts:          view.Counts,
		HaltReason:      view.HaltReason,
		Resumability:    view.Resumability,
		BlockReason:     view.BlockReason,
		RecoveryWarning: view.RecoveryWarning,
	}
	for _, action := range view.Manifest.Plan.Actions {
		for _, attempt := range view.AttemptHistory[action.ID] {
			execution.Results = append(execution.Results, attempt.Result)
		}
	}
	return execution
}

func mergeExecutionView(execution Execution, view ExecutionView) Execution {
	execution.ID = view.Manifest.ExecutionID
	execution.Plan = cloneExecutionPlan(view.Plan)
	execution.State = view.State
	execution.Counts = view.Counts
	execution.HaltReason = view.HaltReason
	execution.Resumability = view.Resumability
	execution.BlockReason = view.BlockReason
	execution.RecoveryWarning = view.RecoveryWarning
	return execution
}

func cloneAttemptCounts(input map[string]int) map[string]int {
	result := make(map[string]int, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func durationMilliseconds(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}
	milliseconds := int64(duration / time.Millisecond)
	if duration%time.Millisecond != 0 {
		milliseconds++
	}
	return milliseconds
}

func (runner Runner) now() time.Time {
	if runner.Now != nil {
		return runner.Now().UTC()
	}
	return time.Now().UTC()
}

func validateResumeIdentity(view ExecutionView, provider Provider, policy RunPolicy) error {
	if provider == nil || provider.ExecutorID() != view.Manifest.Executor || provider.Mode() != view.Manifest.Mode || provider.Platform() != view.Manifest.Platform || policy.normalized() != view.Manifest.Policy {
		return fmt.Errorf("%w: route or policy changed", ErrExecutionIdentityMismatch)
	}
	return nil
}
