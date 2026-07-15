package apply

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

func TestRuntimeSimulatorRecordsDeterministicExecutionOrder(t *testing.T) {
	scenario := newRuntimeScenario(t, runtimeScenarioSpec{
		Plan:   singleActionPlan(),
		Policy: DefaultRunPolicy(),
		ExecutorSteps: map[string][]scenarioExecutorStep{
			"action-1": {{Result: ProviderResult{Outcome: OutcomeSucceeded}}},
		},
	})

	execution, err := scenario.Start(context.Background())
	if err != nil || execution.State != ExecutionStateDone || execution.Resumability != ResumabilityTerminal {
		t.Fatalf("Start execution=%#v err=%v", execution, err)
	}
	wantOrder := []string{
		"execution_started",
		"action_attempt_started",
		"executor_entry",
		"executor_return",
		"action_result_recorded",
		"execution_completed",
	}
	if got := scenarioObservationLabels(scenario.Observations()); !slices.Equal(got, wantOrder) {
		t.Fatalf("observation order\n got: %v\nwant: %v", got, wantOrder)
	}
	executorEntries := scenarioObservationsOfKind(scenario.Observations(), scenarioObservedExecutorEntry)
	if len(executorEntries) != 1 || executorEntries[0].Attempt != 1 || !executorEntries[0].IdempotencyKey.valid() {
		t.Fatalf("executor observations=%#v", executorEntries)
	}
	journal := scenarioObservationsOfKind(scenario.Observations(), scenarioObservedJournalAppend)
	for index, observation := range journal {
		if observation.Sequence != int64(index+1) {
			t.Fatalf("journal sequence at %d = %d", index, observation.Sequence)
		}
	}
}

func TestRuntimeSimulatorScriptsEveryActionOutcome(t *testing.T) {
	tests := []struct {
		name             string
		result           ProviderResult
		policy           RunPolicy
		wantState        ExecutionState
		wantResumability Resumability
		wantStatus       domain.ActionStatus
	}{
		{name: "success", result: ProviderResult{Outcome: OutcomeSucceeded}, wantState: ExecutionStateDone, wantResumability: ResumabilityTerminal, wantStatus: domain.ActionStatusDone},
		{name: "already satisfied", result: ProviderResult{Outcome: OutcomeAlreadySatisfied}, wantState: ExecutionStateDone, wantResumability: ResumabilityTerminal, wantStatus: domain.ActionStatusDone},
		{name: "retryable failure", result: ProviderResult{Outcome: OutcomeRetryableFailure}, wantState: ExecutionStateFailed, wantResumability: ResumabilityTerminal, wantStatus: domain.ActionStatusFailed},
		{name: "permanent failure", result: ProviderResult{Outcome: OutcomePermanentFailure}, wantState: ExecutionStateFailed, wantResumability: ResumabilityTerminal, wantStatus: domain.ActionStatusFailed},
		{name: "rate limit", result: ProviderResult{Outcome: OutcomeRateLimited, RetryAfter: time.Hour}, policy: RunPolicy{MaxAttemptsPerAction: 2}, wantState: ExecutionStateHalted, wantResumability: ResumabilityWaitingRetry, wantStatus: domain.ActionStatusFailed},
		{name: "authentication required", result: ProviderResult{Outcome: OutcomeAuthenticationRequired}, policy: RunPolicy{MaxAttemptsPerAction: 2}, wantState: ExecutionStateHalted, wantResumability: ResumabilityWaitingProvider, wantStatus: domain.ActionStatusFailed},
		{name: "stopped", result: ProviderResult{Outcome: OutcomeStopped}, wantState: ExecutionStateStopped, wantResumability: ResumabilityTerminal, wantStatus: domain.ActionStatusStopped},
		{name: "cancelled", result: ProviderResult{Outcome: OutcomeCancelled}, wantState: ExecutionStateCancelled, wantResumability: ResumabilityTerminal, wantStatus: domain.ActionStatusCancelled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := test.policy
			if policy == (RunPolicy{}) {
				policy = DefaultRunPolicy()
			}
			scenario := newRuntimeScenario(t, runtimeScenarioSpec{
				Plan:   singleActionPlan(),
				Policy: policy,
				ExecutorSteps: map[string][]scenarioExecutorStep{
					"action-1": {{Result: test.result}},
				},
			})
			execution, err := scenario.Start(context.Background())
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			if execution.State != test.wantState || execution.Resumability != test.wantResumability || len(execution.Results) != 1 {
				t.Fatalf("execution=%#v", execution)
			}
			if execution.Results[0].Outcome != test.result.Outcome || execution.Results[0].Status != test.wantStatus {
				t.Fatalf("result=%#v", execution.Results[0])
			}
		})
	}

	t.Run("unknown in-flight result", func(t *testing.T) {
		scenario := newRuntimeScenario(t, runtimeScenarioSpec{
			Plan:   singleActionPlan(),
			Policy: DefaultRunPolicy(),
			ExecutorSteps: map[string][]scenarioExecutorStep{
				"action-1": {{Result: ProviderResult{Outcome: OutcomeSucceeded}}},
			},
		})
		scenario.ArmFault(scenarioFaultAfterExecutorReturn, 1)
		execution, err := scenario.Start(context.Background())
		if err == nil {
			t.Fatal("unknown result fault did not stop execution")
		}
		scenario.Restart()
		view, replayErr := scenario.Replay(execution.ID)
		if replayErr != nil || view.Resumability != ResumabilityResolution || view.Unresolved == nil || view.Unresolved.Attempt != 1 {
			t.Fatalf("Replay view=%#v err=%v", view, replayErr)
		}
		if entries := scenarioObservationsOfKind(scenario.Observations(), scenarioObservedExecutorEntry); len(entries) != 1 {
			t.Fatalf("executor entries=%#v", entries)
		}
	})
}

func TestRuntimeSimulatorResumeRetryAndCancellationScenarios(t *testing.T) {
	t.Run("bounded retry preserves identity", func(t *testing.T) {
		scenario := newRuntimeScenario(t, runtimeScenarioSpec{
			Plan:   singleActionPlan(),
			Policy: RunPolicy{MaxAttemptsPerAction: 2},
			ExecutorSteps: map[string][]scenarioExecutorStep{
				"action-1": {
					{Result: ProviderResult{Outcome: OutcomeRetryableFailure}},
					{Result: ProviderResult{Outcome: OutcomeSucceeded}},
				},
			},
		})
		execution, err := scenario.Start(context.Background())
		entries := scenarioObservationsOfKind(scenario.Observations(), scenarioObservedExecutorEntry)
		if err != nil || execution.State != ExecutionStateDone || len(entries) != 2 || entries[0].Attempt != 1 || entries[1].Attempt != 2 {
			t.Fatalf("execution=%#v err=%v entries=%#v", execution, err, entries)
		}
		if entries[0].IdempotencyKey != entries[1].IdempotencyKey {
			t.Fatalf("retry changed key: %#v", entries)
		}
	})

	t.Run("rate limit waits for explicit fake-clock advance", func(t *testing.T) {
		scenario := newRuntimeScenario(t, runtimeScenarioSpec{
			Plan:   singleActionPlan(),
			Policy: RunPolicy{MaxAttemptsPerAction: 2},
			ExecutorSteps: map[string][]scenarioExecutorStep{
				"action-1": {
					{Result: ProviderResult{Outcome: OutcomeRateLimited, RetryAfter: time.Hour}},
					{Result: ProviderResult{Outcome: OutcomeSucceeded}},
				},
			},
		})
		first, err := scenario.Start(context.Background())
		if err != nil || first.Resumability != ResumabilityWaitingRetry {
			t.Fatalf("Start execution=%#v err=%v", first, err)
		}
		scenario.Restart()
		if _, err := scenario.Resume(context.Background(), first.ID); !errors.Is(err, ErrExecutionNotReady) {
			t.Fatalf("early Resume err=%v", err)
		}
		scenario.Advance(2 * time.Hour)
		scenario.Restart()
		resumed, err := scenario.Resume(context.Background(), first.ID)
		entries := scenarioObservationsOfKind(scenario.Observations(), scenarioObservedExecutorEntry)
		if err != nil || resumed.State != ExecutionStateDone || len(entries) != 2 || entries[0].IdempotencyKey != entries[1].IdempotencyKey {
			t.Fatalf("Resume execution=%#v err=%v entries=%#v", resumed, err, entries)
		}
	})

	t.Run("authentication waits for provider readiness", func(t *testing.T) {
		scenario := newRuntimeScenario(t, runtimeScenarioSpec{
			Plan:   singleActionPlan(),
			Policy: RunPolicy{MaxAttemptsPerAction: 2},
			ExecutorSteps: map[string][]scenarioExecutorStep{
				"action-1": {
					{Result: ProviderResult{Outcome: OutcomeAuthenticationRequired}},
					{Result: ProviderResult{Outcome: OutcomeSucceeded}},
				},
			},
		})
		first, err := scenario.Start(context.Background())
		if err != nil || first.Resumability != ResumabilityWaitingProvider {
			t.Fatalf("Start execution=%#v err=%v", first, err)
		}
		scenario.SetProviderReady(false)
		scenario.Restart()
		if _, err := scenario.Resume(context.Background(), first.ID); !errors.Is(err, ErrExecutionNotReady) {
			t.Fatalf("disconnected Resume err=%v", err)
		}
		if entries := scenarioObservationsOfKind(scenario.Observations(), scenarioObservedExecutorEntry); len(entries) != 1 {
			t.Fatalf("blocked Resume entered executor: %#v", entries)
		}
		scenario.SetProviderReady(true)
		scenario.Restart()
		resumed, err := scenario.Resume(context.Background(), first.ID)
		if err != nil || resumed.State != ExecutionStateDone {
			t.Fatalf("ready Resume execution=%#v err=%v", resumed, err)
		}
	})

	t.Run("hard ceiling survives unknown result and reconciliation", func(t *testing.T) {
		steps := make([]scenarioExecutorStep, MaxAutomaticAttemptsPerAction)
		for index := range steps {
			steps[index] = scenarioExecutorStep{Result: ProviderResult{Outcome: OutcomeRetryableFailure}}
		}
		steps[len(steps)-1] = scenarioExecutorStep{Result: ProviderResult{Outcome: OutcomeSucceeded}}
		scenario := newRuntimeScenario(t, runtimeScenarioSpec{
			Plan:                singleActionPlan(),
			Policy:              RunPolicy{MaxAttemptsPerAction: MaxAutomaticAttemptsPerAction + 50},
			ExecutorSteps:       map[string][]scenarioExecutorStep{"action-1": steps},
			ReconciliationSteps: []scenarioReconciliationStep{{Outcome: ReconciliationNotApplied}},
		})
		scenario.ArmFault(scenarioFaultAfterExecutorReturn, MaxAutomaticAttemptsPerAction)
		first, err := scenario.Start(context.Background())
		if err == nil {
			t.Fatal("final unknown result did not stop execution")
		}
		scenario.Restart()
		resolved, err := scenario.Reconcile(context.Background(), first.ID)
		entries := scenarioObservationsOfKind(scenario.Observations(), scenarioObservedExecutorEntry)
		if err != nil || resolved.State != ExecutionStateFailed || resolved.Resumability != ResumabilityTerminal || len(entries) != MaxAutomaticAttemptsPerAction {
			t.Fatalf("Reconcile execution=%#v err=%v entries=%d", resolved, err, len(entries))
		}
	})

	t.Run("definitive result wins cancellation race", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		plan := applyTestPlan(testPlatform, []domain.CleanupAction{
			applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
			applyTestAction("action-2", testPlatform, domain.ActionDeleteComment, domain.ActionStatusPending),
		})
		scenario := newRuntimeScenario(t, runtimeScenarioSpec{
			Plan:   plan,
			Policy: DefaultRunPolicy(),
			ExecutorSteps: map[string][]scenarioExecutorStep{
				"action-1": {{Result: ProviderResult{Outcome: OutcomeSucceeded}, BeforeReturn: cancel}},
				"action-2": {{Result: ProviderResult{Outcome: OutcomeSucceeded}}},
			},
		})
		execution, err := scenario.Start(ctx)
		if err != nil || execution.State != ExecutionStateCancelled || execution.Counts.Done != 1 || execution.Counts.Cancelled != 1 {
			t.Fatalf("Start execution=%#v err=%v", execution, err)
		}
		entries := scenarioObservationsOfKind(scenario.Observations(), scenarioObservedExecutorEntry)
		if len(entries) != 1 || entries[0].ActionID != "action-1" || execution.Plan.Actions[0].Status != domain.ActionStatusDone || execution.Plan.Actions[1].Status != domain.ActionStatusCancelled {
			t.Fatalf("cancellation observations=%#v execution=%#v", entries, execution)
		}
	})
}

func TestRuntimeSimulatorScriptsEveryReconciliationOutcome(t *testing.T) {
	tests := []struct {
		name                  string
		step                  scenarioReconciliationStep
		reconcilerAvailable   bool
		want                  ReconciliationOutcome
		wantResumability      Resumability
		wantState             ExecutionState
		wantReconcilerEntries int
	}{
		{name: "already applied", step: scenarioReconciliationStep{Outcome: ReconciliationAlreadyApplied}, reconcilerAvailable: true, want: ReconciliationAlreadyApplied, wantResumability: ResumabilityTerminal, wantState: ExecutionStateDone, wantReconcilerEntries: 1},
		{name: "not applied", step: scenarioReconciliationStep{Outcome: ReconciliationNotApplied}, reconcilerAvailable: true, want: ReconciliationNotApplied, wantResumability: ResumabilityResumable, wantState: ExecutionStateHalted, wantReconcilerEntries: 1},
		{name: "conflicting state", step: scenarioReconciliationStep{Outcome: ReconciliationConflictingState}, reconcilerAvailable: true, want: ReconciliationConflictingState, wantResumability: ResumabilityResolution, wantState: ExecutionStateRunning, wantReconcilerEntries: 1},
		{name: "unknown", step: scenarioReconciliationStep{Outcome: ReconciliationUnknown}, reconcilerAvailable: true, want: ReconciliationUnknown, wantResumability: ResumabilityResolution, wantState: ExecutionStateRunning, wantReconcilerEntries: 1},
		{name: "temporarily unavailable", step: scenarioReconciliationStep{Outcome: ReconciliationTemporarilyUnavailable}, reconcilerAvailable: true, want: ReconciliationTemporarilyUnavailable, wantResumability: ResumabilityResolution, wantState: ExecutionStateRunning, wantReconcilerEntries: 1},
		{name: "provider error", step: scenarioReconciliationStep{Err: errors.New("private reconciliation response")}, reconcilerAvailable: true, want: ReconciliationTemporarilyUnavailable, wantResumability: ResumabilityResolution, wantState: ExecutionStateRunning, wantReconcilerEntries: 1},
		{name: "invalid", step: scenarioReconciliationStep{Outcome: ReconciliationOutcome("private-provider-value")}, reconcilerAvailable: true, want: ReconciliationInvalid, wantResumability: ResumabilityResolution, wantState: ExecutionStateRunning, wantReconcilerEntries: 1},
		{name: "unsupported", reconcilerAvailable: false, want: ReconciliationUnsupported, wantResumability: ResumabilityResolution, wantState: ExecutionStateRunning},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scenario, first := unknownResultScenario(t, []scenarioReconciliationStep{test.step})
			scenario.SetReconcilerAvailable(test.reconcilerAvailable)
			resolved, err := scenario.Reconcile(context.Background(), first.ID)
			if err != nil || resolved.Resumability != test.wantResumability || resolved.State != test.wantState || len(resolved.Reconciliations) != 1 {
				t.Fatalf("Reconcile execution=%#v err=%v", resolved, err)
			}
			if resolved.Reconciliations[0].Outcome != test.want || strings.Contains(resolved.BlockReason, "private") {
				t.Fatalf("reconciliation=%#v reason=%q", resolved.Reconciliations[0], resolved.BlockReason)
			}
			reconcilerEntries := scenarioObservationsOfKind(scenario.Observations(), scenarioObservedReconcilerEntry)
			if len(reconcilerEntries) != test.wantReconcilerEntries {
				t.Fatalf("reconciler entries=%#v", reconcilerEntries)
			}
			executorEntries := scenarioObservationsOfKind(scenario.Observations(), scenarioObservedExecutorEntry)
			if len(executorEntries) != 1 {
				t.Fatalf("Reconcile entered executor: %#v", executorEntries)
			}
			if test.want == ReconciliationNotApplied {
				scenario.Restart()
				resumed, resumeErr := scenario.Resume(context.Background(), first.ID)
				executorEntries = scenarioObservationsOfKind(scenario.Observations(), scenarioObservedExecutorEntry)
				if resumeErr != nil || resumed.State != ExecutionStateDone || len(executorEntries) != 2 || executorEntries[0].IdempotencyKey != executorEntries[1].IdempotencyKey {
					t.Fatalf("Resume execution=%#v err=%v entries=%#v", resumed, resumeErr, executorEntries)
				}
			} else if test.wantResumability == ResumabilityResolution {
				if _, resumeErr := scenario.Resume(context.Background(), first.ID); !errors.Is(resumeErr, ErrExecutionResolutionRequired) {
					t.Fatalf("blocked Resume err=%v", resumeErr)
				}
			}
		})
	}
}

func TestRuntimeSimulatorRecoversAtEveryDurableBoundary(t *testing.T) {
	t.Run("after attempt start", func(t *testing.T) {
		scenario := newRuntimeScenario(t, runtimeScenarioSpec{
			Plan:                singleActionPlan(),
			Policy:              RunPolicy{MaxAttemptsPerAction: 2},
			ExecutorSteps:       map[string][]scenarioExecutorStep{"action-1": {{Result: ProviderResult{Outcome: OutcomeSucceeded}}}},
			ReconciliationSteps: []scenarioReconciliationStep{{Outcome: ReconciliationNotApplied}},
		})
		scenario.ArmFault(scenarioFaultAfterAttemptStarted, 1)
		first, err := scenario.Start(context.Background())
		if err == nil || len(scenarioObservationsOfKind(scenario.Observations(), scenarioObservedExecutorEntry)) != 0 {
			t.Fatalf("Start execution=%#v err=%v", first, err)
		}
		scenario.Restart()
		view, replayErr := scenario.Replay(first.ID)
		if replayErr != nil || view.Unresolved == nil || view.Unresolved.Attempt != 1 {
			t.Fatalf("Replay view=%#v err=%v", view, replayErr)
		}
		if _, err := scenario.Reconcile(context.Background(), first.ID); err != nil {
			t.Fatal(err)
		}
		scenario.Restart()
		resumed, err := scenario.Resume(context.Background(), first.ID)
		entries := scenarioObservationsOfKind(scenario.Observations(), scenarioObservedExecutorEntry)
		if err != nil || resumed.State != ExecutionStateDone || len(entries) != 1 || entries[0].Attempt != 2 {
			t.Fatalf("Resume execution=%#v err=%v entries=%#v", resumed, err, entries)
		}
	})

	t.Run("after executor return", func(t *testing.T) {
		scenario, first := unknownResultScenario(t, []scenarioReconciliationStep{{Outcome: ReconciliationAlreadyApplied}})
		resolved, err := scenario.Reconcile(context.Background(), first.ID)
		entries := scenarioObservationsOfKind(scenario.Observations(), scenarioObservedExecutorEntry)
		if err != nil || resolved.State != ExecutionStateDone || len(entries) != 1 {
			t.Fatalf("Reconcile execution=%#v err=%v entries=%#v", resolved, err, entries)
		}
		reconcilerEntries := scenarioObservationsOfKind(scenario.Observations(), scenarioObservedReconcilerEntry)
		if len(reconcilerEntries) != 1 || reconcilerEntries[0].IdempotencyKey != entries[0].IdempotencyKey {
			t.Fatalf("identity executor=%#v reconciler=%#v", entries, reconcilerEntries)
		}
	})

	t.Run("after reconciliation start", func(t *testing.T) {
		scenario, first := unknownResultScenario(t, []scenarioReconciliationStep{{Outcome: ReconciliationAlreadyApplied}})
		scenario.ArmFault(scenarioFaultAfterReconciliationStarted, 1)
		if _, err := scenario.Reconcile(context.Background(), first.ID); err == nil {
			t.Fatal("reconciliation-start fault did not stop execution")
		}
		if entries := scenarioObservationsOfKind(scenario.Observations(), scenarioObservedReconcilerEntry); len(entries) != 0 {
			t.Fatalf("fault entered reconciler: %#v", entries)
		}
		scenario.Restart()
		resolved, err := scenario.Reconcile(context.Background(), first.ID)
		entries := scenarioObservationsOfKind(scenario.Observations(), scenarioObservedReconcilerEntry)
		if err != nil || resolved.State != ExecutionStateDone || len(entries) != 1 || entries[0].ReconciliationAttempt != 2 {
			t.Fatalf("Reconcile execution=%#v err=%v entries=%#v", resolved, err, entries)
		}
	})

	t.Run("before reconciliation result commit", func(t *testing.T) {
		scenario, first := unknownResultScenario(t, []scenarioReconciliationStep{
			{Outcome: ReconciliationAlreadyApplied},
			{Outcome: ReconciliationAlreadyApplied},
		})
		scenario.ArmFault(scenarioFaultBeforeReconciliationResult, 1)
		if _, err := scenario.Reconcile(context.Background(), first.ID); err == nil {
			t.Fatal("reconciliation-result fault did not stop execution")
		}
		scenario.Restart()
		view, replayErr := scenario.Replay(first.ID)
		if replayErr != nil || view.Unresolved == nil || len(view.ReconciliationHistory["action-1"]) != 0 {
			t.Fatalf("Replay view=%#v err=%v", view, replayErr)
		}
		resolved, err := scenario.Reconcile(context.Background(), first.ID)
		entries := scenarioObservationsOfKind(scenario.Observations(), scenarioObservedReconcilerEntry)
		if err != nil || resolved.State != ExecutionStateDone || len(entries) != 2 || entries[0].IdempotencyKey != entries[1].IdempotencyKey {
			t.Fatalf("Reconcile execution=%#v err=%v entries=%#v", resolved, err, entries)
		}
	})

	t.Run("after reconciliation result commit", func(t *testing.T) {
		scenario, first := unknownResultScenario(t, []scenarioReconciliationStep{{Outcome: ReconciliationAlreadyApplied}})
		scenario.ArmFault(scenarioFaultAfterReconciliationResult, 1)
		if _, err := scenario.Reconcile(context.Background(), first.ID); err == nil {
			t.Fatal("post-result fault did not stop execution")
		}
		scenario.Restart()
		view, replayErr := scenario.Replay(first.ID)
		if replayErr != nil || view.Unresolved != nil || view.Counts.Done != 1 || !view.NeedsFinalization {
			t.Fatalf("Replay view=%#v err=%v", view, replayErr)
		}
		finished, err := scenario.Resume(context.Background(), first.ID)
		if err != nil || finished.State != ExecutionStateDone || len(scenarioObservationsOfKind(scenario.Observations(), scenarioObservedExecutorEntry)) != 1 || len(scenarioObservationsOfKind(scenario.Observations(), scenarioObservedReconcilerEntry)) != 1 {
			t.Fatalf("Resume execution=%#v err=%v observations=%#v", finished, err, scenario.Observations())
		}
	})

	t.Run("before terminal event commit", func(t *testing.T) {
		scenario := successfulRuntimeScenario(t)
		scenario.ArmFault(scenarioFaultBeforeTerminalEvent, 1)
		first, err := scenario.Start(context.Background())
		if err == nil {
			t.Fatal("terminal fault did not stop execution")
		}
		scenario.Restart()
		view, replayErr := scenario.Replay(first.ID)
		if replayErr != nil || !view.NeedsFinalization || view.Counts.Done != 1 || view.Resumability == ResumabilityTerminal {
			t.Fatalf("Replay view=%#v err=%v", view, replayErr)
		}
		finished, err := scenario.Resume(context.Background(), first.ID)
		if err != nil || finished.State != ExecutionStateDone || len(scenarioObservationsOfKind(scenario.Observations(), scenarioObservedExecutorEntry)) != 1 {
			t.Fatalf("Resume execution=%#v err=%v", finished, err)
		}
	})

	t.Run("after terminal event commit", func(t *testing.T) {
		scenario := successfulRuntimeScenario(t)
		scenario.ArmFault(scenarioFaultAfterTerminalEvent, 1)
		first, err := scenario.Start(context.Background())
		if err == nil {
			t.Fatal("post-terminal fault did not stop execution")
		}
		scenario.Restart()
		view, replayErr := scenario.Replay(first.ID)
		if replayErr != nil || view.State != ExecutionStateDone || view.Resumability != ResumabilityTerminal {
			t.Fatalf("Replay view=%#v err=%v", view, replayErr)
		}
		if _, err := scenario.Resume(context.Background(), first.ID); !errors.Is(err, ErrExecutionTerminal) {
			t.Fatalf("terminal Resume err=%v", err)
		}
		if entries := scenarioObservationsOfKind(scenario.Observations(), scenarioObservedExecutorEntry); len(entries) != 1 {
			t.Fatalf("terminal recovery repeated executor: %#v", entries)
		}
	})
}

func TestRuntimeSimulatorKeepsProviderDataOutOfDurableAudit(t *testing.T) {
	const privateError = "private reconciliation response token=secret"
	scenario, first := unknownResultScenario(t, []scenarioReconciliationStep{{Err: errors.New(privateError)}})
	blocked, err := scenario.Reconcile(context.Background(), first.ID)
	if err != nil || blocked.Resumability != ResumabilityResolution || strings.Contains(blocked.BlockReason, privateError) {
		t.Fatalf("Reconcile execution=%#v err=%v", blocked, err)
	}
	executorEntries := scenarioObservationsOfKind(scenario.Observations(), scenarioObservedExecutorEntry)
	if len(executorEntries) != 1 {
		t.Fatalf("executor entries=%#v", executorEntries)
	}
	dir := filepath.Join(scenario.store.Root(), executionStoreKey(first.ID))
	journal, journalErr := os.ReadFile(filepath.Join(dir, journalFileName))
	summary, summaryErr := os.ReadFile(filepath.Join(dir, summaryFileName))
	if journalErr != nil || summaryErr != nil {
		t.Fatalf("read durable audit journal=%v summary=%v", journalErr, summaryErr)
	}
	durable := string(journal) + string(summary)
	if strings.Contains(durable, privateError) || strings.Contains(durable, string(executorEntries[0].IdempotencyKey)) {
		t.Fatalf("private provider data entered durable audit: %q", durable)
	}
}

func successfulRuntimeScenario(t *testing.T) *runtimeScenario {
	t.Helper()
	return newRuntimeScenario(t, runtimeScenarioSpec{
		Plan:   singleActionPlan(),
		Policy: DefaultRunPolicy(),
		ExecutorSteps: map[string][]scenarioExecutorStep{
			"action-1": {{Result: ProviderResult{Outcome: OutcomeSucceeded}}},
		},
	})
}

func unknownResultScenario(t *testing.T, reconciliationSteps []scenarioReconciliationStep) (*runtimeScenario, Execution) {
	t.Helper()
	scenario := newRuntimeScenario(t, runtimeScenarioSpec{
		Plan:   singleActionPlan(),
		Policy: RunPolicy{MaxAttemptsPerAction: 2},
		ExecutorSteps: map[string][]scenarioExecutorStep{
			"action-1": {
				{Result: ProviderResult{Outcome: OutcomeSucceeded}},
				{Result: ProviderResult{Outcome: OutcomeSucceeded}},
			},
		},
		ReconciliationSteps: reconciliationSteps,
	})
	scenario.ArmFault(scenarioFaultAfterExecutorReturn, 1)
	first, err := scenario.Start(context.Background())
	if err == nil {
		t.Fatal("unknown-result setup did not fail")
	}
	scenario.Restart()
	view, replayErr := scenario.Replay(first.ID)
	if replayErr != nil || view.Resumability != ResumabilityResolution || view.Unresolved == nil {
		t.Fatalf("unknown-result Replay view=%#v err=%v", view, replayErr)
	}
	return scenario, first
}
