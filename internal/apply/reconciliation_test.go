package apply

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

type reconciliationStep struct {
	outcome ReconciliationOutcome
	err     error
}

type scriptedReconciler struct {
	steps    []reconciliationStep
	requests []ReconciliationRequest
	order    *[]string
}

func (reconciler *scriptedReconciler) Reconcile(_ context.Context, request ReconciliationRequest) (ReconciliationOutcome, error) {
	reconciler.requests = append(reconciler.requests, request)
	if reconciler.order != nil {
		*reconciler.order = append(*reconciler.order, "reconciler")
	}
	position := len(reconciler.requests) - 1
	if position >= len(reconciler.steps) {
		return "", errors.New("reconciliation script exhausted")
	}
	return reconciler.steps[position].outcome, reconciler.steps[position].err
}

func TestReconciliationAppliedResolvesWithoutExecutor(t *testing.T) {
	reconciler := &scriptedReconciler{steps: []reconciliationStep{{outcome: ReconciliationAlreadyApplied}}}
	store, runner, executor, first := unresolvedReconciliationExecution(t, DefaultRunPolicy(), reconciler)

	resolved, err := runner.Reconcile(context.Background(), first.ID)
	if err != nil || resolved.State != ExecutionStateDone || resolved.Resumability != ResumabilityTerminal || resolved.Counts.Done != 1 {
		t.Fatalf("Reconcile execution=%#v err=%v", resolved, err)
	}
	if len(executor.calls) != 1 || len(reconciler.requests) != 1 {
		t.Fatalf("calls executor=%v reconciler=%v", executor.calls, reconciler.requests)
	}
	if reconciler.requests[0].IdempotencyKey != executor.requests[0].IdempotencyKey || !reconciler.requests[0].IdempotencyKey.valid() {
		t.Fatalf("identity executor=%q reconciler=%q", executor.requests[0].IdempotencyKey, reconciler.requests[0].IdempotencyKey)
	}
	if len(resolved.Reconciliations) != 1 || resolved.Reconciliations[0].Outcome != ReconciliationAlreadyApplied {
		t.Fatalf("reconciliation history=%#v", resolved.Reconciliations)
	}
	if len(resolved.Results) != 0 {
		t.Fatalf("reconciliation synthesized executor results: %#v", resolved.Results)
	}
	view, replayErr := store.Replay(first.ID)
	if replayErr != nil || view.Counts.Done != 1 || view.Resumability != ResumabilityTerminal || view.Unresolved != nil {
		t.Fatalf("Replay view=%#v err=%v", view, replayErr)
	}
}

func TestReconciliationNotAppliedRequiresExplicitBoundedResume(t *testing.T) {
	reconciler := &scriptedReconciler{steps: []reconciliationStep{{outcome: ReconciliationNotApplied}}}
	_, runner, executor, first := unresolvedReconciliationExecution(t, DefaultRunPolicy(), reconciler)

	resolved, err := runner.Reconcile(context.Background(), first.ID)
	if err != nil || resolved.State != ExecutionStateHalted || resolved.Resumability != ResumabilityResumable || resolved.Counts.Failed != 1 || len(executor.calls) != 1 {
		t.Fatalf("Reconcile execution=%#v err=%v calls=%v", resolved, err, executor.calls)
	}
	resumed, err := runner.Resume(context.Background(), first.ID)
	if err != nil || resumed.State != ExecutionStateDone || len(executor.calls) != 2 {
		t.Fatalf("Resume execution=%#v err=%v calls=%v", resumed, err, executor.calls)
	}
	if executor.requests[0].IdempotencyKey != executor.requests[1].IdempotencyKey {
		t.Fatalf("retry changed key: %#v", executor.requests)
	}

	t.Run("hard attempt ceiling remains final", func(t *testing.T) {
		reconciler := &scriptedReconciler{steps: []reconciliationStep{{outcome: ReconciliationNotApplied}}}
		store := NewExecutionStore(t.TempDir())
		store.hooks.beforeAppend = func(event JournalEvent) error {
			if event.Kind == JournalResultRecorded && event.Attempt == MaxAutomaticAttemptsPerAction {
				return errors.New("synthetic unknown final executor result")
			}
			return nil
		}
		steps := make([]scriptedStep, MaxAutomaticAttemptsPerAction)
		for index := range steps {
			steps[index].result.Outcome = OutcomeRetryableFailure
		}
		steps[len(steps)-1].result.Outcome = OutcomeSucceeded
		executor := &scriptedExecutor{scripts: map[string][]scriptedStep{"action-1": steps}}
		runner := reconciliationTestRunner(t, store, executor, RunPolicy{MaxAttemptsPerAction: MaxAutomaticAttemptsPerAction}, reconciler)
		first, startErr := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
		if startErr == nil || len(executor.calls) != MaxAutomaticAttemptsPerAction {
			t.Fatalf("Start execution=%#v err=%v calls=%v", first, startErr, executor.calls)
		}
		store.hooks = executionStoreHooks{}
		resolved, err := runner.Reconcile(context.Background(), first.ID)
		if err != nil || resolved.State != ExecutionStateFailed || resolved.Resumability != ResumabilityTerminal || len(executor.calls) != MaxAutomaticAttemptsPerAction {
			t.Fatalf("Reconcile execution=%#v err=%v calls=%v", resolved, err, executor.calls)
		}
	})
}

func TestReconciliationEvidenceIsScopedToExactUnresolvedAttempt(t *testing.T) {
	reconciler := &scriptedReconciler{steps: []reconciliationStep{{outcome: ReconciliationNotApplied}}}
	store, runner, executor, first := unresolvedReconciliationExecution(t, RunPolicy{MaxAttemptsPerAction: 3}, reconciler)
	if _, err := runner.Reconcile(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	store.hooks.beforeAppend = func(event JournalEvent) error {
		if event.Kind == JournalResultRecorded && event.Attempt == 2 {
			return errors.New("synthetic unknown second executor result")
		}
		return nil
	}
	if _, err := runner.Resume(context.Background(), first.ID); err == nil || len(executor.calls) != 2 {
		t.Fatalf("Resume err=%v calls=%v", err, executor.calls)
	}
	store.hooks = executionStoreHooks{}
	view, err := store.Replay(first.ID)
	if err != nil || view.Resumability != ResumabilityResolution || view.Unresolved == nil || view.Unresolved.Attempt != 2 {
		t.Fatalf("Replay view=%#v err=%v", view, err)
	}
	if view.BlockReason != "A previous action has an unknown result." || view.LastReconciliation["action-1"].Attempt != 1 {
		t.Fatalf("stale reconciliation affected attempt 2: reason=%q reconciliation=%#v", view.BlockReason, view.LastReconciliation["action-1"])
	}

	writer, view, err := store.OpenWriter(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	summary := summaryForRuntime(view.Manifest, view.Counts, ExecutionStateRunning, ResumabilityResumable, "", "", runner.now())
	for _, event := range []JournalEvent{
		{
			Timestamp: runner.now(), Kind: JournalResultRecorded,
			ActionID: "action-1", ActionType: view.Unresolved.ActionType, Platform: view.Unresolved.Platform, Attempt: 2,
			Outcome: OutcomePermanentFailure, Status: domain.ActionStatusFailed,
		},
		{Timestamp: runner.now(), Kind: JournalExecutionResumed},
		{
			Timestamp: runner.now(), Kind: JournalAttemptStarted,
			ActionID: "action-1", ActionType: view.Unresolved.ActionType, Platform: view.Unresolved.Platform, Attempt: 3,
		},
	} {
		if _, err := writer.Append(event, summary); err != nil {
			_ = writer.Close()
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replay(first.ID); !errors.Is(err, ErrExecutionCorrupt) {
		t.Fatalf("stale reconciliation validated attempt 3: err=%v", err)
	}
}

func TestStaleReconciliationDoesNotRetryPermanentFailureBeforeProviderHalt(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	failed := false
	store.hooks.beforeAppend = func(event JournalEvent) error {
		if event.Kind == JournalResultRecorded && !failed {
			failed = true
			return errors.New("synthetic unknown first executor result")
		}
		return nil
	}
	plan := applyTestPlan(testPlatform, []domain.CleanupAction{
		applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
		applyTestAction("action-2", testPlatform, domain.ActionDeleteComment, domain.ActionStatusPending),
	})
	executor := &scriptedExecutor{scripts: map[string][]scriptedStep{
		"action-1": {
			{result: ProviderResult{Outcome: OutcomeSucceeded}},
			{result: ProviderResult{Outcome: OutcomePermanentFailure}},
		},
		"action-2": {{result: ProviderResult{Outcome: OutcomeAuthenticationRequired}}},
	}}
	reconciler := &scriptedReconciler{steps: []reconciliationStep{{outcome: ReconciliationNotApplied}}}
	runner := reconciliationTestRunner(t, store, executor, RunPolicy{MaxAttemptsPerAction: 3}, reconciler)
	first, err := runner.Start(context.Background(), plan, ExecutionModeSimulation)
	if err == nil || len(executor.calls) != 1 {
		t.Fatalf("Start execution=%#v err=%v calls=%v", first, err, executor.calls)
	}
	store.hooks = executionStoreHooks{}
	if _, err := runner.Reconcile(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	halted, err := runner.Resume(context.Background(), first.ID)
	if err != nil || halted.Resumability != ResumabilityWaitingProvider || len(executor.calls) != 3 {
		t.Fatalf("Resume execution=%#v err=%v calls=%v", halted, err, executor.calls)
	}
	view, err := store.Replay(first.ID)
	if err != nil || view.NextActionID != "action-2" || view.NextAttempt != 2 || view.Plan.Actions[0].Status != domain.ActionStatusFailed {
		t.Fatalf("stale reconciliation selected retry: view=%#v err=%v", view, err)
	}
}

func TestBlockedReconciliationOutcomesNeverEnableExecution(t *testing.T) {
	tests := []struct {
		name       string
		reconciler Reconciler
		want       ReconciliationOutcome
	}{
		{name: "conflict", reconciler: &scriptedReconciler{steps: []reconciliationStep{{outcome: ReconciliationConflictingState}}}, want: ReconciliationConflictingState},
		{name: "unknown", reconciler: &scriptedReconciler{steps: []reconciliationStep{{outcome: ReconciliationUnknown}}}, want: ReconciliationUnknown},
		{name: "temporarily unavailable", reconciler: &scriptedReconciler{steps: []reconciliationStep{{outcome: ReconciliationTemporarilyUnavailable}}}, want: ReconciliationTemporarilyUnavailable},
		{name: "provider error", reconciler: &scriptedReconciler{steps: []reconciliationStep{{err: errors.New("private provider error")}}}, want: ReconciliationTemporarilyUnavailable},
		{name: "invalid", reconciler: &scriptedReconciler{steps: []reconciliationStep{{outcome: ReconciliationOutcome("provider-private-value")}}}, want: ReconciliationInvalid},
		{name: "unsupported", reconciler: nil, want: ReconciliationUnsupported},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, runner, executor, first := unresolvedReconciliationExecution(t, RunPolicy{MaxAttemptsPerAction: 2}, test.reconciler)
			blocked, err := runner.Reconcile(context.Background(), first.ID)
			if err != nil || blocked.Resumability != ResumabilityResolution || blocked.State != ExecutionStateRunning || len(executor.calls) != 1 {
				t.Fatalf("Reconcile execution=%#v err=%v calls=%v", blocked, err, executor.calls)
			}
			if len(blocked.Reconciliations) != 1 || blocked.Reconciliations[0].Outcome != test.want || strings.Contains(blocked.BlockReason, "private") {
				t.Fatalf("normalized history=%#v reason=%q", blocked.Reconciliations, blocked.BlockReason)
			}
			if _, err := runner.Resume(context.Background(), first.ID); !errors.Is(err, ErrExecutionResolutionRequired) || len(executor.calls) != 1 {
				t.Fatalf("blocked Resume err=%v calls=%v", err, executor.calls)
			}
			view, replayErr := store.Replay(first.ID)
			if replayErr != nil || view.Unresolved == nil || view.Resumability != ResumabilityResolution {
				t.Fatalf("Replay view=%#v err=%v", view, replayErr)
			}
		})
	}
}

func TestRepeatedReconciliationUsesStableRequestAndHistory(t *testing.T) {
	reconciler := &scriptedReconciler{steps: []reconciliationStep{
		{outcome: ReconciliationUnknown},
		{outcome: ReconciliationUnknown},
	}}
	store, runner, executor, first := unresolvedReconciliationExecution(t, RunPolicy{MaxAttemptsPerAction: 2}, reconciler)
	for attempt := 1; attempt <= 2; attempt++ {
		blocked, err := runner.Reconcile(context.Background(), first.ID)
		if err != nil || blocked.Resumability != ResumabilityResolution || len(executor.calls) != 1 {
			t.Fatalf("attempt %d execution=%#v err=%v calls=%v", attempt, blocked, err, executor.calls)
		}
	}
	if len(reconciler.requests) != 2 || reconciler.requests[0].IdempotencyKey != reconciler.requests[1].IdempotencyKey || reconciler.requests[0].Attempt != reconciler.requests[1].Attempt {
		t.Fatalf("requests changed: %#v", reconciler.requests)
	}
	view, err := store.Replay(first.ID)
	if err != nil || len(view.ReconciliationHistory["action-1"]) != 2 || view.ReconciliationHistory["action-1"][0].ReconciliationAttempt != 1 || view.ReconciliationHistory["action-1"][1].ReconciliationAttempt != 2 {
		t.Fatalf("Replay view=%#v err=%v", view, err)
	}
}

func TestReconciliationDurabilityOrderingAndWriteFailures(t *testing.T) {
	t.Run("ordering", func(t *testing.T) {
		var order []string
		reconciler := &scriptedReconciler{steps: []reconciliationStep{{outcome: ReconciliationUnknown}}, order: &order}
		store, runner, _, first := unresolvedReconciliationExecution(t, RunPolicy{MaxAttemptsPerAction: 2}, reconciler)
		store.hooks.onAppend = func(event JournalEvent) { order = append(order, string(event.Kind)) }
		if _, err := runner.Reconcile(context.Background(), first.ID); err != nil {
			t.Fatal(err)
		}
		want := []string{"action_reconciliation_started", "reconciler", "action_reconciliation_result_recorded"}
		if !slices.Equal(order, want) {
			t.Fatalf("order=%v want=%v", order, want)
		}
	})

	t.Run("start write failure prevents provider", func(t *testing.T) {
		reconciler := &scriptedReconciler{steps: []reconciliationStep{{outcome: ReconciliationAlreadyApplied}}}
		store, runner, executor, first := unresolvedReconciliationExecution(t, RunPolicy{MaxAttemptsPerAction: 2}, reconciler)
		store.hooks.beforeAppend = func(event JournalEvent) error {
			if event.Kind == JournalReconciliationStarted {
				return errors.New("synthetic reconciliation-start failure")
			}
			return nil
		}
		if _, err := runner.Reconcile(context.Background(), first.ID); err == nil || len(reconciler.requests) != 0 || len(executor.calls) != 1 {
			t.Fatalf("Reconcile err=%v provider=%v executor=%v", err, reconciler.requests, executor.calls)
		}
		view, err := store.Replay(first.ID)
		if err != nil || view.Resumability != ResumabilityResolution || view.ReconciliationAttempts["action-1"] != 0 {
			t.Fatalf("Replay view=%#v err=%v", view, err)
		}
	})

	t.Run("result write failure remains unresolved", func(t *testing.T) {
		reconciler := &scriptedReconciler{steps: []reconciliationStep{{outcome: ReconciliationAlreadyApplied}}}
		store, runner, executor, first := unresolvedReconciliationExecution(t, RunPolicy{MaxAttemptsPerAction: 2}, reconciler)
		store.hooks.beforeAppend = func(event JournalEvent) error {
			if event.Kind == JournalReconciliationResult {
				return errors.New("synthetic reconciliation-result failure")
			}
			return nil
		}
		if _, err := runner.Reconcile(context.Background(), first.ID); err == nil || len(reconciler.requests) != 1 || len(executor.calls) != 1 {
			t.Fatalf("Reconcile err=%v provider=%v executor=%v", err, reconciler.requests, executor.calls)
		}
		view, err := store.Replay(first.ID)
		if err != nil || view.Resumability != ResumabilityResolution || view.Unresolved == nil || len(view.ReconciliationHistory["action-1"]) != 0 || !strings.Contains(view.BlockReason, "interrupted") {
			t.Fatalf("Replay view=%#v err=%v", view, err)
		}
	})

	t.Run("durable result survives summary failure", func(t *testing.T) {
		reconciler := &scriptedReconciler{steps: []reconciliationStep{{outcome: ReconciliationAlreadyApplied}}}
		store, runner, executor, first := unresolvedReconciliationExecution(t, RunPolicy{MaxAttemptsPerAction: 2}, reconciler)
		failSummary := false
		store.hooks.onAppend = func(event JournalEvent) {
			if event.Kind == JournalReconciliationResult {
				failSummary = true
			}
		}
		store.hooks.beforeSummary = func() error {
			if failSummary {
				failSummary = false
				return errors.New("synthetic summary failure")
			}
			return nil
		}
		if _, err := runner.Reconcile(context.Background(), first.ID); err == nil || len(executor.calls) != 1 {
			t.Fatalf("Reconcile err=%v calls=%v", err, executor.calls)
		}
		store.hooks = executionStoreHooks{}
		view, err := store.Replay(first.ID)
		if err != nil || view.Unresolved != nil || view.Counts.Done != 1 || !view.NeedsFinalization {
			t.Fatalf("Replay view=%#v err=%v", view, err)
		}
		finished, err := runner.Resume(context.Background(), first.ID)
		if err != nil || finished.State != ExecutionStateDone || len(executor.calls) != 1 {
			t.Fatalf("Resume execution=%#v err=%v calls=%v", finished, err, executor.calls)
		}
	})
}

func TestReconcileHonorsCancellationAfterDurableStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reconciler := &scriptedReconciler{steps: []reconciliationStep{{outcome: ReconciliationAlreadyApplied}}}
	store, runner, executor, first := unresolvedReconciliationExecution(t, DefaultRunPolicy(), reconciler)
	store.hooks.onAppend = func(event JournalEvent) {
		if event.Kind == JournalReconciliationStarted {
			cancel()
		}
	}
	execution, err := runner.Reconcile(ctx, first.ID)
	if !errors.Is(err, context.Canceled) || len(reconciler.requests) != 0 || len(executor.calls) != 1 {
		t.Fatalf("Reconcile execution=%#v err=%v provider=%v executor=%v", execution, err, reconciler.requests, executor.calls)
	}
	store.hooks = executionStoreHooks{}
	view, replayErr := store.Replay(first.ID)
	if replayErr != nil || view.Resumability != ResumabilityResolution || view.Unresolved == nil || !view.Unresolved.ReconciliationStarted || len(view.ReconciliationHistory["action-1"]) != 0 {
		t.Fatalf("Replay view=%#v err=%v", view, replayErr)
	}
}

func TestReconcileRejectsNonResolutionWorkWithoutProviderCall(t *testing.T) {
	reconciler := &scriptedReconciler{steps: []reconciliationStep{{outcome: ReconciliationAlreadyApplied}}}
	store := NewExecutionStore(t.TempDir())
	executor := &fakeExecutor{}
	runner := reconciliationTestRunner(t, store, executor, DefaultRunPolicy(), reconciler)
	execution, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Replay(execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Reconcile(context.Background(), execution.ID); !errors.Is(err, ErrExecutionReconciliationUnavailable) || len(reconciler.requests) != 0 || len(executor.calls) != 1 {
		t.Fatalf("Reconcile err=%v provider=%v executor=%v", err, reconciler.requests, executor.calls)
	}
	after, err := store.Replay(execution.ID)
	if err != nil || after.LastSequence != before.LastSequence {
		t.Fatalf("journal changed before=%#v after=%#v err=%v", before, after, err)
	}
}

func TestReplayRejectsReconciliationResultWithoutDurableStart(t *testing.T) {
	store, _, _, first := unresolvedReconciliationExecution(t, RunPolicy{MaxAttemptsPerAction: 2}, nil)
	writer, view, err := store.OpenWriter(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	summary := summaryForRuntime(view.Manifest, view.Counts, ExecutionStateRunning, ResumabilityResolution, reconciliationBlockReason(ReconciliationUnknown), "", applyTestTime())
	_, appendErr := writer.Append(JournalEvent{
		Timestamp: applyTestTime(), Kind: JournalReconciliationResult,
		ActionID: view.Unresolved.ActionID, ActionType: view.Unresolved.ActionType,
		Platform: view.Unresolved.Platform, Attempt: view.Unresolved.Attempt,
		ReconciliationAttempt: 1, ReconciliationOutcome: ReconciliationUnknown,
	}, summary)
	if closeErr := writer.Close(); appendErr != nil || closeErr != nil {
		t.Fatalf("append=%v close=%v", appendErr, closeErr)
	}
	if _, err := store.Replay(first.ID); !errors.Is(err, ErrExecutionCorrupt) {
		t.Fatalf("malformed reconciliation replay err=%v", err)
	}
}

func TestReplayRejectsExecutorResultAfterReconciliationLifecycle(t *testing.T) {
	reconciler := &scriptedReconciler{steps: []reconciliationStep{{outcome: ReconciliationUnknown}}}
	store, runner, _, first := unresolvedReconciliationExecution(t, DefaultRunPolicy(), reconciler)
	if _, err := runner.Reconcile(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	writer, view, err := store.OpenWriter(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	result := JournalEvent{
		Timestamp: runner.now(), Kind: JournalResultRecorded,
		ActionID: view.Unresolved.ActionID, ActionType: view.Unresolved.ActionType,
		Platform: view.Unresolved.Platform, Attempt: view.Unresolved.Attempt,
		Outcome: OutcomeSucceeded, Status: domain.ActionStatusDone,
	}
	summary := summaryForRuntime(view.Manifest, view.Counts, ExecutionStateRunning, ResumabilityResolution, view.BlockReason, "", runner.now())
	_, appendErr := writer.Append(result, summary)
	closeErr := writer.Close()
	if appendErr != nil || closeErr != nil {
		t.Fatalf("append=%v close=%v", appendErr, closeErr)
	}
	if _, err := store.Replay(first.ID); !errors.Is(err, ErrExecutionCorrupt) {
		t.Fatalf("result after reconciliation replay err=%v", err)
	}
}

func unresolvedReconciliationExecution(t *testing.T, policy RunPolicy, reconciler Reconciler) (*ExecutionStore, Runner, *fakeExecutor, Execution) {
	t.Helper()
	store := NewExecutionStore(t.TempDir())
	failed := false
	store.hooks.beforeAppend = func(event JournalEvent) error {
		if event.Kind == JournalResultRecorded && !failed {
			failed = true
			return errors.New("synthetic unknown executor result")
		}
		return nil
	}
	executor := &fakeExecutor{}
	runner := reconciliationTestRunner(t, store, executor, policy, reconciler)
	first, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if err == nil || len(executor.calls) != 1 {
		t.Fatalf("Start execution=%#v err=%v calls=%v", first, err, executor.calls)
	}
	store.hooks = executionStoreHooks{}
	view, replayErr := store.Replay(first.ID)
	if replayErr != nil || view.Resumability != ResumabilityResolution || view.Unresolved == nil {
		t.Fatalf("Replay view=%#v err=%v", view, replayErr)
	}
	return store, runner, executor, first
}

func reconciliationTestRunner(t *testing.T, store *ExecutionStore, executor Executor, policy RunPolicy, reconciler Reconciler) Runner {
	t.Helper()
	provider := testProvider(executor)
	provider.reconciler = reconciler
	registry, err := NewProviderRegistry(provider)
	if err != nil {
		t.Fatal(err)
	}
	return Runner{Providers: registry, Policy: policy, Store: store, Now: func() time.Time { return applyTestTime() }}
}
