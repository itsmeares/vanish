package apply

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

const testPlatform domain.PlatformName = "test-provider"

type fakeExecutor struct {
	results map[string]ProviderResult
	calls   []string
}

type scriptedStep struct {
	result ProviderResult
	err    error
}

type scriptedCall struct {
	actionID string
	attempt  int
}

type scriptedExecutor struct {
	scripts   map[string][]scriptedStep
	positions map[string]int
	calls     []scriptedCall
}

func (executor *scriptedExecutor) Execute(_ context.Context, action domain.CleanupAction) (ProviderResult, error) {
	if executor.positions == nil {
		executor.positions = make(map[string]int)
	}
	position := executor.positions[action.ID]
	executor.positions[action.ID] = position + 1
	executor.calls = append(executor.calls, scriptedCall{actionID: action.ID, attempt: position + 1})
	steps := executor.scripts[action.ID]
	if position >= len(steps) {
		return ProviderResult{}, errors.New("script exhausted")
	}
	return steps[position].result, steps[position].err
}

func (executor *fakeExecutor) Execute(_ context.Context, action domain.CleanupAction) (ProviderResult, error) {
	executor.calls = append(executor.calls, action.ID)
	if result, ok := executor.results[action.ID]; ok {
		return result, nil
	}
	return ProviderResult{Outcome: OutcomeSucceeded}, nil
}

type fakeProvider struct {
	platform        domain.PlatformName
	mode            ExecutionMode
	executorID      ExecutorID
	supported       map[domain.ActionType]bool
	prerequisites   func(domain.CleanupPlan, RuntimeState) []Prerequisite
	executor        Executor
	executorFactory func() Executor
}

func (provider fakeProvider) Platform() domain.PlatformName { return provider.platform }
func (provider fakeProvider) Mode() ExecutionMode           { return provider.mode }
func (provider fakeProvider) ExecutorID() ExecutorID        { return provider.executorID }
func (provider fakeProvider) Supports(action domain.ActionType) bool {
	return provider.supported[action]
}
func (provider fakeProvider) Prerequisites(plan domain.CleanupPlan, state RuntimeState) []Prerequisite {
	if provider.prerequisites == nil {
		return nil
	}
	return provider.prerequisites(plan, state)
}
func (provider fakeProvider) Executor() Executor {
	if provider.executorFactory != nil {
		return provider.executorFactory()
	}
	return provider.executor
}

func TestPreviewRoutesSupportedPlanToProvider(t *testing.T) {
	executor := &fakeExecutor{}
	runner := testRunner(t, testProvider(executor), RuntimeState{})
	plan := applyTestPlan(testPlatform, []domain.CleanupAction{
		applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
		applyTestAction("action-2", testPlatform, domain.ActionDeleteComment, domain.ActionStatusPending),
	})

	preview := runner.Preview(plan, ExecutionModeSimulation)

	if !preview.CanApply || !preview.ProviderReady || preview.Executor != "test-simulation" || preview.Mode != ExecutionModeSimulation {
		t.Fatalf("unexpected preview route: %#v", preview)
	}
	if preview.PendingCount != 2 || preview.UnsupportedCount != 0 {
		t.Fatalf("unexpected preview counts: %#v", preview)
	}
}

func TestPreviewFailsClosedForInvalidUnsupportedAndMissingRoutes(t *testing.T) {
	runner := testRunner(t, testProvider(&fakeExecutor{}), RuntimeState{})

	invalid := applyTestPlan(testPlatform, []domain.CleanupAction{applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusPending)})
	invalid.ID = ""
	if preview := runner.Preview(invalid, ExecutionModeSimulation); preview.CanApply || !hasBlocker(preview, "plan_invalid") {
		t.Fatalf("expected invalid plan blocker, got %#v", preview)
	}

	noPending := applyTestPlan(testPlatform, []domain.CleanupAction{applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusDone)})
	if preview := runner.Preview(noPending, ExecutionModeSimulation); preview.CanApply || !hasBlocker(preview, "no_pending_actions") {
		t.Fatalf("expected no pending blocker, got %#v", preview)
	}

	unsupported := applyTestPlan(testPlatform, []domain.CleanupAction{applyTestAction("action-1", testPlatform, domain.ActionDeletePost, domain.ActionStatusPending)})
	if preview := runner.Preview(unsupported, ExecutionModeSimulation); preview.CanApply || preview.UnsupportedCount != 1 || !hasBlocker(preview, "unsupported_actions") {
		t.Fatalf("expected unsupported action blocker, got %#v", preview)
	}

	mixed := applyTestPlan(testPlatform, []domain.CleanupAction{applyTestAction("action-1", "other", domain.ActionUnlike, domain.ActionStatusPending)})
	if preview := runner.Preview(mixed, ExecutionModeSimulation); preview.CanApply || preview.Unsupported[0].Reason != "action platform does not match plan platform" {
		t.Fatalf("expected mixed platform blocker, got %#v", preview)
	}

	missingProvider := applyTestPlan("missing", []domain.CleanupAction{applyTestAction("action-1", "missing", domain.ActionUnlike, domain.ActionStatusPending)})
	if preview := runner.Preview(missingProvider, ExecutionModeSimulation); preview.CanApply || preview.Executor != "" || !hasBlocker(preview, "provider_unavailable") {
		t.Fatalf("expected missing provider blocker, got %#v", preview)
	}

	if preview := runner.Preview(applyTestPlan(testPlatform, []domain.CleanupAction{applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusPending)}), ExecutionMode("automatic")); preview.CanApply || preview.Executor != "" || !hasBlocker(preview, "execution_mode_unavailable") {
		t.Fatalf("expected missing mode blocker, got %#v", preview)
	}
}

func TestPreviewRejectsDuplicateActionIDsBeforeProviderUse(t *testing.T) {
	executor := &fakeExecutor{}
	prerequisiteCalls := 0
	provider := testProvider(executor)
	provider.prerequisites = func(domain.CleanupPlan, RuntimeState) []Prerequisite {
		prerequisiteCalls++
		return nil
	}
	runner := testRunner(t, provider, RuntimeState{})
	plan := applyTestPlan(testPlatform, []domain.CleanupAction{
		applyTestAction("duplicate", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
		applyTestAction("duplicate", testPlatform, domain.ActionDeleteComment, domain.ActionStatusPending),
	})

	preview := runner.Preview(plan, ExecutionModeSimulation)
	if preview.CanApply || preview.Executor != "" || !hasBlocker(preview, "plan_invalid") || prerequisiteCalls != 0 {
		t.Fatalf("duplicate IDs reached provider preview: preview=%#v prerequisite_calls=%d", preview, prerequisiteCalls)
	}
	if len(preview.Blockers) == 0 || !strings.Contains(preview.Blockers[0].Message, `duplicate id "duplicate"`) {
		t.Fatalf("duplicate ID error was not clear: %#v", preview.Blockers)
	}

	execution := runner.Run(context.Background(), plan, ExecutionModeSimulation)
	if execution.State != ExecutionStateFailed || len(execution.Results) != 0 || len(executor.calls) != 0 || prerequisiteCalls != 0 {
		t.Fatalf("duplicate IDs reached execution: execution=%#v calls=%#v prerequisites=%d", execution, executor.calls, prerequisiteCalls)
	}
}

func TestProviderPrerequisiteUsesMatchingRuntimeConnection(t *testing.T) {
	provider := testProvider(&fakeExecutor{})
	provider.prerequisites = func(_ domain.CleanupPlan, state RuntimeState) []Prerequisite {
		if state.Connected(testPlatform) {
			return nil
		}
		return []Prerequisite{{Code: "connection_required", Message: "Connect provider.", Blocking: true}}
	}
	plan := applyTestPlan(testPlatform, []domain.CleanupAction{applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusPending)})

	disconnected := testRunner(t, provider, NewRuntimeState(map[domain.PlatformName]ConnectionState{"other": {Connected: true}})).Preview(plan, ExecutionModeSimulation)
	if disconnected.CanApply || disconnected.ProviderReady || !hasBlocker(disconnected, "connection_required") {
		t.Fatalf("expected unrelated connection not to satisfy provider, got %#v", disconnected)
	}
	connected := testRunner(t, provider, NewRuntimeState(map[domain.PlatformName]ConnectionState{testPlatform: {Connected: true}})).Preview(plan, ExecutionModeSimulation)
	if !connected.CanApply || !connected.ProviderReady {
		t.Fatalf("expected matching connection to satisfy provider, got %#v", connected)
	}
}

func TestRuntimeStateCopiesConnectionInput(t *testing.T) {
	connections := map[domain.PlatformName]ConnectionState{testPlatform: {Connected: true}}
	state := NewRuntimeState(connections)
	connections[testPlatform] = ConnectionState{}
	if !state.Connected(testPlatform) {
		t.Fatal("runtime state changed through caller-owned map")
	}
}

func TestRunnerExecutesSequentiallyAndReportsSelectedIdentity(t *testing.T) {
	executor := &fakeExecutor{results: map[string]ProviderResult{}}
	runner := testRunner(t, testProvider(executor), RuntimeState{})
	plan := applyTestPlan(testPlatform, []domain.CleanupAction{
		applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
		applyTestAction("action-2", testPlatform, domain.ActionDeleteComment, domain.ActionStatusPending),
	})

	execution := runner.Run(context.Background(), plan, ExecutionModeSimulation)

	if execution.State != ExecutionStateDone || execution.Counts.Done != 2 || len(executor.calls) != 2 {
		t.Fatalf("unexpected execution: %#v calls=%#v", execution, executor.calls)
	}
	for _, event := range execution.Events {
		if event.Executor != "test-simulation" || event.Mode != ExecutionModeSimulation {
			t.Fatalf("event missing selected identity: %#v", event)
		}
	}
	if !hasEvent(execution, EventExecutionStarted) || !hasEvent(execution, EventActionResult) || !hasEvent(execution, EventExecutionFinished) {
		t.Fatalf("expected lifecycle events, got %#v", execution.Events)
	}
}

func TestRunnerPreservesFailureCancellationSkipAndStopStates(t *testing.T) {
	t.Run("failure", func(t *testing.T) {
		executor := &fakeExecutor{results: map[string]ProviderResult{"action-1": {Outcome: OutcomePermanentFailure}}}
		execution := testRunner(t, testProvider(executor), RuntimeState{}).Run(context.Background(), singleActionPlan(), ExecutionModeSimulation)
		if execution.State != ExecutionStateFailed || execution.Counts.Failed != 1 || execution.Results[0].Message != "Action failed." {
			t.Fatalf("expected failed execution, got %#v", execution)
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		executor := &fakeExecutor{}
		execution := testRunner(t, testProvider(executor), RuntimeState{}).Run(ctx, applyTestPlan(testPlatform, []domain.CleanupAction{
			applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
			applyTestAction("action-2", testPlatform, domain.ActionDeleteComment, domain.ActionStatusPending),
		}), ExecutionModeSimulation)
		if execution.State != ExecutionStateCancelled || execution.Counts.Cancelled != 2 || !hasEvent(execution, EventExecutionCancelled) || len(executor.calls) != 0 {
			t.Fatalf("expected cancellation before executor call, execution=%#v calls=%#v", execution, executor.calls)
		}
	})

	t.Run("expired context", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		executor := &fakeExecutor{}
		execution := testRunner(t, testProvider(executor), RuntimeState{}).Run(ctx, applyTestPlan(testPlatform, []domain.CleanupAction{
			applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
			applyTestAction("action-2", testPlatform, domain.ActionDeleteComment, domain.ActionStatusPending),
		}), ExecutionModeSimulation)
		if execution.State != ExecutionStateCancelled || execution.Counts.Cancelled != 2 || len(executor.calls) != 0 {
			t.Fatalf("expected deadline cancellation before executor call, execution=%#v calls=%#v", execution, executor.calls)
		}
	})

	t.Run("already satisfied", func(t *testing.T) {
		executor := &fakeExecutor{results: map[string]ProviderResult{"action-1": {Outcome: OutcomeAlreadySatisfied}}}
		execution := testRunner(t, testProvider(executor), RuntimeState{}).Run(context.Background(), singleActionPlan(), ExecutionModeSimulation)
		if execution.State != ExecutionStateDone || execution.Counts.Done != 1 || execution.Results[0].Outcome != OutcomeAlreadySatisfied {
			t.Fatalf("expected already-satisfied execution, got %#v", execution)
		}
	})

	t.Run("stopped", func(t *testing.T) {
		executor := &fakeExecutor{results: map[string]ProviderResult{"action-1": {Outcome: OutcomeStopped}}}
		plan := applyTestPlan(testPlatform, []domain.CleanupAction{
			applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
			applyTestAction("action-2", testPlatform, domain.ActionDeleteComment, domain.ActionStatusPending),
		})
		execution := testRunner(t, testProvider(executor), RuntimeState{}).Run(context.Background(), plan, ExecutionModeSimulation)
		if execution.State != ExecutionStateStopped || execution.Counts.Stopped != 2 || len(executor.calls) != 1 {
			t.Fatalf("expected stopped execution, got %#v calls=%#v", execution, executor.calls)
		}
	})
}

func TestBlockedRunNeverExecutesOrFallsBack(t *testing.T) {
	executor := &fakeExecutor{}
	registry, err := NewProviderRegistry()
	if err != nil {
		t.Fatal(err)
	}
	execution := (Runner{Providers: registry}).Run(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if execution.State != ExecutionStateFailed || len(executor.calls) != 0 || !hasBlocker(execution.Preview, "provider_unavailable") {
		t.Fatalf("expected blocked execution without fallback, got %#v", execution)
	}
}

func TestSkipRetryStopCancelPrimitives(t *testing.T) {
	plan := applyTestPlan(testPlatform, []domain.CleanupAction{
		applyTestAction("pending", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
		applyTestAction("failed", testPlatform, domain.ActionDeleteComment, domain.ActionStatusFailed),
		applyTestAction("done", testPlatform, domain.ActionUnfollow, domain.ActionStatusDone),
	})

	edit, err := SkipAction(&plan, "pending", "user skipped")
	if err != nil || edit.PlanID != plan.ID || edit.ActionID != "pending" || edit.Status != domain.ActionStatusSkipped || plan.Actions[0].Status != domain.ActionStatusSkipped {
		t.Fatalf("expected standalone skipped plan edit, edit=%#v err=%v", edit, err)
	}
	retryEdit, err := RetryAction(&plan, "failed")
	if err != nil || retryEdit.Status != domain.ActionStatusPending || retryEdit.ActionID != "failed" || plan.Actions[1].Status != domain.ActionStatusPending {
		t.Fatalf("expected failed action retried, edit=%#v err=%v plan=%#v", retryEdit, err, plan)
	}
	if _, err := RetryAction(&plan, "done"); err == nil {
		t.Fatal("expected done action retry to fail")
	}
	if _, err := SkipAction(&plan, "done", ""); err == nil {
		t.Fatal("expected done action skip to fail")
	}
	if stopped := StopPending(&plan, "stop"); stopped != 1 || plan.Actions[1].Status != domain.ActionStatusStopped {
		t.Fatalf("expected one stopped action, got %d %#v", stopped, plan)
	}
	cancelPlan := singleActionPlan()
	if cancelled := CancelPending(&cancelPlan, "cancel"); cancelled != 1 || cancelPlan.Actions[0].Status != domain.ActionStatusCancelled {
		t.Fatalf("expected cancelled action, got %d %#v", cancelled, cancelPlan)
	}
}

func TestProviderRegistryRejectsInvalidAndDuplicateRoutes(t *testing.T) {
	provider := testProvider(&fakeExecutor{})
	if _, err := NewProviderRegistry(provider, provider); err == nil {
		t.Fatal("expected duplicate provider route rejection")
	}
}

func TestRunnerResolvesOneStableExecutorInstance(t *testing.T) {
	first := &fakeExecutor{}
	second := &fakeExecutor{}
	calls := 0
	provider := testProvider(nil)
	provider.executorFactory = func() Executor {
		calls++
		if calls == 1 {
			return first
		}
		return second
	}
	runner := testRunner(t, provider, RuntimeState{})
	if calls != 0 {
		t.Fatalf("registry construction resolved executor %d times", calls)
	}
	plan := applyTestPlan(testPlatform, []domain.CleanupAction{
		applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
		applyTestAction("action-2", testPlatform, domain.ActionDeleteComment, domain.ActionStatusPending),
	})

	execution := runner.Run(context.Background(), plan, ExecutionModeSimulation)

	if execution.State != ExecutionStateDone || calls != 1 || len(first.calls) != 2 || len(second.calls) != 0 {
		t.Fatalf("expected one stable executor, execution=%#v calls=%d first=%#v second=%#v", execution, calls, first.calls, second.calls)
	}
}

func TestRunnerFailsClosedWhenProviderHasNoExecutor(t *testing.T) {
	execution := testRunner(t, testProvider(nil), RuntimeState{}).Run(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if execution.State != ExecutionStateFailed || execution.Preview.CanApply || execution.Preview.ProviderReady || !hasBlocker(execution.Preview, "executor_unavailable") || len(execution.Results) != 0 {
		t.Fatalf("expected missing executor to fail closed, got %#v", execution)
	}
	if len(execution.Events) != 1 || execution.Events[0].Executor != "test-simulation" || execution.Events[0].Mode != ExecutionModeSimulation {
		t.Fatalf("missing executor finish event lost route identity: %#v", execution.Events)
	}
}

func TestGenericApplyPackageContainsNoProviderSpecificReadiness(t *testing.T) {
	for _, name := range []string{"engine.go", "provider.go"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, forbidden := range []string{"PlatformReddit", "RedditConnected", "internal/reddit"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s contains provider-specific symbol %q", name, forbidden)
			}
		}
	}
}

func TestNoopExecutorHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NoopExecutor{}.Execute(ctx, applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusPending))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestNormalizeProviderOutcomes(t *testing.T) {
	tests := []struct {
		outcome   ActionOutcome
		status    domain.ActionStatus
		retryable bool
	}{
		{OutcomeSucceeded, domain.ActionStatusDone, false},
		{OutcomeAlreadySatisfied, domain.ActionStatusDone, false},
		{OutcomeRetryableFailure, domain.ActionStatusFailed, true},
		{OutcomePermanentFailure, domain.ActionStatusFailed, false},
		{OutcomeRateLimited, domain.ActionStatusFailed, false},
		{OutcomeAuthenticationRequired, domain.ActionStatusFailed, false},
		{OutcomeStopped, domain.ActionStatusStopped, false},
		{OutcomeCancelled, domain.ActionStatusCancelled, false},
	}
	action := applyTestAction("owned-action", testPlatform, domain.ActionUnlike, domain.ActionStatusRunning)
	for _, test := range tests {
		t.Run(string(test.outcome), func(t *testing.T) {
			result := normalizeProviderResult(context.Background(), action, 1, ProviderResult{Outcome: test.outcome}, nil)
			if result.Status != test.status || result.Outcome != test.outcome || result.Retryable() != test.retryable {
				t.Fatalf("unexpected normalized result: %#v", result)
			}
			if result.ActionID != action.ID || result.Platform != action.Platform || result.Type != action.Type || result.Attempt != 1 {
				t.Fatalf("runner-owned identity changed: %#v", result)
			}
		})
	}
}

func TestNormalizeProviderResultFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		result ProviderResult
	}{
		{name: "empty outcome", result: ProviderResult{}},
		{name: "unknown outcome", result: ProviderResult{Outcome: "mystery"}},
		{name: "negative retry after", result: ProviderResult{Outcome: OutcomeRetryableFailure, RetryAfter: -time.Second}},
		{name: "contradictory retry after", result: ProviderResult{Outcome: OutcomeSucceeded, RetryAfter: time.Second}},
		{name: "secret-like provider code", result: ProviderResult{Outcome: OutcomePermanentFailure, ProviderCode: "session_token"}},
		{name: "unsafe provider code", result: ProviderResult{Outcome: OutcomePermanentFailure, ProviderCode: "https://example.test/raw"}},
	}
	action := applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusRunning)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := normalizeProviderResult(context.Background(), action, 3, test.result, nil)
			if result.Outcome != OutcomePermanentFailure || result.Status != domain.ActionStatusFailed || result.Attempt != 3 {
				t.Fatalf("invalid result did not fail closed: %#v", result)
			}
			if result.Message != "Executor returned an invalid result." || result.ProviderCode != "" || result.RetryAfter != 0 {
				t.Fatalf("invalid metadata leaked: %#v", result)
			}
		})
	}

	valid := normalizeProviderResult(context.Background(), action, 1, ProviderResult{
		Outcome:      OutcomeSucceeded,
		Message:      ProviderMessageNoopCompleted,
		ProviderCode: "  http_503  ",
	}, nil)
	if valid.Message != "No-op apply completed." || valid.ProviderCode != "http_503" {
		t.Fatalf("safe provider metadata not normalized: %#v", valid)
	}
}

func TestNormalizeUnsafeProviderMessagesUsesRuntimeDefaults(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{name: "authorization", message: "Authorization: Basic dXNlcjpwYXNz"},
		{name: "bearer", message: "Bearer eyJhbGciOiJIUzI1NiJ9"},
		{name: "token", message: "access_token=private-value"},
		{name: "cookie", message: "Cookie: sid=private-value"},
		{name: "session", message: "session_id=private-value"},
		{name: "password", message: "password=private-value"},
		{name: "secret", message: "client_secret=private-value"},
		{name: "credential", message: "credential=private-value"},
		{name: "raw json", message: `{"error":"private response"}`},
		{name: "raw html", message: "<html><body>private response</body></html>"},
		{name: "credential URL query", message: "https://example.test/callback?code=private-value"},
		{name: "credential URL fragment", message: "https://example.test/#private-value"},
		{name: "multiline", message: "raw\nresponse"},
		{name: "terminal control", message: "private\x1b[31mresponse"},
	}
	action := applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusRunning)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := normalizeProviderResult(context.Background(), action, 2, ProviderResult{
				Outcome:      OutcomeRateLimited,
				Message:      ProviderMessage(test.message),
				RetryAfter:   30 * time.Second,
				ProviderCode: "safe_code",
			}, nil)
			if result.Outcome != OutcomeRateLimited || result.Status != domain.ActionStatusFailed || result.RetryAfter != 30*time.Second {
				t.Fatalf("unsafe message changed typed outcome: %#v", result)
			}
			if result.Message != "Provider rate limit reached." || result.ProviderCode != "" || strings.Contains(result.Message, test.message) {
				t.Fatalf("unsafe provider text survived normalization: %#v", result)
			}
		})
	}
}

func TestNormalizeExecutorErrorsWithoutRawDetails(t *testing.T) {
	action := applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusRunning)
	rawErr := errors.New("raw platform response with private content")

	failed := normalizeProviderResult(context.Background(), action, 1, ProviderResult{}, rawErr)
	if failed.Outcome != OutcomePermanentFailure || failed.Message != "Executor failed unexpectedly." || strings.Contains(failed.Message, rawErr.Error()) {
		t.Fatalf("unexpected error did not fail safely: %#v", failed)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled := normalizeProviderResult(ctx, action, 2, ProviderResult{Outcome: OutcomeSucceeded}, rawErr)
	if cancelled.Outcome != OutcomeCancelled || cancelled.Status != domain.ActionStatusCancelled || cancelled.Message != "Execution cancelled." {
		t.Fatalf("context cancellation not normalized: %#v", cancelled)
	}
}

func TestNormalizeCancellationUsesRunnerContextOnly(t *testing.T) {
	action := applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusRunning)
	for _, executorErr := range []error{context.Canceled, context.DeadlineExceeded} {
		result := normalizeProviderResult(context.Background(), action, 1, ProviderResult{}, executorErr)
		if result.Outcome != OutcomePermanentFailure || result.Status != domain.ActionStatusFailed || result.Message != "Executor failed unexpectedly." {
			t.Fatalf("active runner context misclassified executor error %v: %#v", executorErr, result)
		}
	}

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled := normalizeProviderResult(cancelledCtx, action, 1, ProviderResult{}, errors.New("provider error"))
	if cancelled.Outcome != OutcomeCancelled || cancelled.Status != domain.ActionStatusCancelled {
		t.Fatalf("cancelled runner context not classified: %#v", cancelled)
	}

	expiredCtx, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	expired := normalizeProviderResult(expiredCtx, action, 1, ProviderResult{}, errors.New("provider error"))
	if expired.Outcome != OutcomeCancelled || expired.Status != domain.ActionStatusCancelled {
		t.Fatalf("expired runner context not classified: %#v", expired)
	}
}

func TestRunnerBoundedRetryPolicy(t *testing.T) {
	retryThenSuccess := func() *scriptedExecutor {
		return &scriptedExecutor{scripts: map[string][]scriptedStep{
			"action-1": {
				{result: ProviderResult{Outcome: OutcomeRetryableFailure, ProviderCode: "temporary_failure"}},
				{result: ProviderResult{Outcome: OutcomeSucceeded}},
			},
		}}
	}

	t.Run("default performs one attempt", func(t *testing.T) {
		executor := retryThenSuccess()
		execution := testRunner(t, testProvider(executor), RuntimeState{}).Run(context.Background(), singleActionPlan(), ExecutionModeSimulation)
		if len(executor.calls) != 1 || len(execution.Results) != 1 || execution.Results[0].Attempt != 1 || execution.State != ExecutionStateFailed {
			t.Fatalf("default retry policy was not one attempt: execution=%#v calls=%#v", execution, executor.calls)
		}
	})

	t.Run("configured retry succeeds", func(t *testing.T) {
		executor := retryThenSuccess()
		runner := testRunner(t, testProvider(executor), RuntimeState{})
		runner.Policy = RunPolicy{MaxAttemptsPerAction: 2}
		execution := runner.Run(context.Background(), singleActionPlan(), ExecutionModeSimulation)
		if execution.State != ExecutionStateDone || execution.Counts.Done != 1 || len(executor.calls) != 2 || len(execution.Results) != 2 {
			t.Fatalf("safe retry did not succeed: execution=%#v calls=%#v", execution, executor.calls)
		}
		if execution.Results[0].Attempt != 1 || execution.Results[1].Attempt != 2 || execution.Results[1].Outcome != OutcomeSucceeded {
			t.Fatalf("attempt history incorrect: %#v", execution.Results)
		}
	})

	t.Run("retry exhaustion stops at maximum", func(t *testing.T) {
		executor := &scriptedExecutor{scripts: map[string][]scriptedStep{"action-1": {
			{result: ProviderResult{Outcome: OutcomeRetryableFailure}},
			{result: ProviderResult{Outcome: OutcomeRetryableFailure}},
		}}}
		runner := testRunner(t, testProvider(executor), RuntimeState{})
		runner.Policy = RunPolicy{MaxAttemptsPerAction: 2}
		execution := runner.Run(context.Background(), singleActionPlan(), ExecutionModeSimulation)
		if len(executor.calls) != 2 || execution.State != ExecutionStateFailed || execution.Counts.Failed != 1 || execution.Results[1].Attempt != 2 {
			t.Fatalf("retry exhaustion incorrect: execution=%#v calls=%#v", execution, executor.calls)
		}
	})

	t.Run("permanent failure never retries", func(t *testing.T) {
		executor := &scriptedExecutor{scripts: map[string][]scriptedStep{"action-1": {
			{result: ProviderResult{Outcome: OutcomePermanentFailure}},
			{result: ProviderResult{Outcome: OutcomeSucceeded}},
		}}}
		runner := testRunner(t, testProvider(executor), RuntimeState{})
		runner.Policy = RunPolicy{MaxAttemptsPerAction: 3}
		execution := runner.Run(context.Background(), singleActionPlan(), ExecutionModeSimulation)
		if len(executor.calls) != 1 || execution.State != ExecutionStateFailed {
			t.Fatalf("permanent failure retried: execution=%#v calls=%#v", execution, executor.calls)
		}
	})
}

func TestRunnerFailureContinuationPolicy(t *testing.T) {
	newPlan := func() domain.CleanupPlan {
		return applyTestPlan(testPlatform, []domain.CleanupAction{
			applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
			applyTestAction("action-2", testPlatform, domain.ActionDeleteComment, domain.ActionStatusPending),
		})
	}
	newExecutor := func() *scriptedExecutor {
		return &scriptedExecutor{scripts: map[string][]scriptedStep{
			"action-1": {{result: ProviderResult{Outcome: OutcomePermanentFailure}}},
			"action-2": {{result: ProviderResult{Outcome: OutcomeSucceeded}}},
		}}
	}

	continued := newExecutor()
	continuedExecution := testRunner(t, testProvider(continued), RuntimeState{}).Run(context.Background(), newPlan(), ExecutionModeSimulation)
	if len(continued.calls) != 2 || continuedExecution.Counts.Failed != 1 || continuedExecution.Counts.Done != 1 || continuedExecution.State != ExecutionStateFailed {
		t.Fatalf("default policy did not continue: execution=%#v calls=%#v", continuedExecution, continued.calls)
	}

	stopped := newExecutor()
	runner := testRunner(t, testProvider(stopped), RuntimeState{})
	runner.Policy = RunPolicy{MaxAttemptsPerAction: 1, StopAfterFinalFailure: true}
	stoppedExecution := runner.Run(context.Background(), newPlan(), ExecutionModeSimulation)
	if len(stopped.calls) != 1 || stoppedExecution.Counts.Failed != 1 || stoppedExecution.Counts.Pending != 1 || stoppedExecution.State != ExecutionStateFailed {
		t.Fatalf("stop-after-failure policy incorrect: execution=%#v calls=%#v", stoppedExecution, stopped.calls)
	}
}

func TestRunPolicyNormalizesAttemptLimits(t *testing.T) {
	tests := []struct {
		name string
		max  int
		want int
	}{
		{name: "zero", max: 0, want: 1},
		{name: "negative", max: -4, want: 1},
		{name: "normal", max: 3, want: 3},
		{name: "boundary", max: MaxAutomaticAttemptsPerAction, want: MaxAutomaticAttemptsPerAction},
		{name: "excessive", max: MaxAutomaticAttemptsPerAction + 1000, want: MaxAutomaticAttemptsPerAction},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := (RunPolicy{MaxAttemptsPerAction: test.max}).normalized().MaxAttemptsPerAction; got != test.want {
				t.Fatalf("normalized attempts = %d, want %d", got, test.want)
			}
		})
	}

	steps := make([]scriptedStep, MaxAutomaticAttemptsPerAction+1)
	for i := range steps {
		steps[i] = scriptedStep{result: ProviderResult{Outcome: OutcomeRetryableFailure}}
	}
	executor := &scriptedExecutor{scripts: map[string][]scriptedStep{"action-1": steps}}
	runner := testRunner(t, testProvider(executor), RuntimeState{})
	runner.Policy = RunPolicy{MaxAttemptsPerAction: MaxAutomaticAttemptsPerAction + 1000}
	execution := runner.Run(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if len(executor.calls) != MaxAutomaticAttemptsPerAction || execution.Results[len(execution.Results)-1].Attempt != MaxAutomaticAttemptsPerAction || execution.State != ExecutionStateFailed {
		t.Fatalf("excessive policy exceeded hard maximum: execution=%#v calls=%#v", execution, executor.calls)
	}
}

func TestRunnerUnexpectedExecutorErrorFailsClosedAndContinues(t *testing.T) {
	plan := applyTestPlan(testPlatform, []domain.CleanupAction{
		applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
		applyTestAction("action-2", testPlatform, domain.ActionDeleteComment, domain.ActionStatusPending),
	})
	executor := &scriptedExecutor{scripts: map[string][]scriptedStep{
		"action-1": {{err: errors.New("raw response with private content")}},
		"action-2": {{result: ProviderResult{Outcome: OutcomeSucceeded}}},
	}}
	execution := testRunner(t, testProvider(executor), RuntimeState{}).Run(context.Background(), plan, ExecutionModeSimulation)
	if len(executor.calls) != 2 || execution.Counts.Failed != 1 || execution.Counts.Done != 1 || execution.Results[0].Message != "Executor failed unexpectedly." {
		t.Fatalf("unexpected error lifecycle incorrect: execution=%#v calls=%#v", execution, executor.calls)
	}
	for _, event := range execution.Events {
		if strings.Contains(event.Message, "raw response") {
			t.Fatalf("raw executor error entered event: %#v", event)
		}
	}
}

func TestRunnerInternalContextErrorsFollowFailurePolicy(t *testing.T) {
	for _, executorErr := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(executorErr.Error(), func(t *testing.T) {
			newPlan := func() domain.CleanupPlan {
				return applyTestPlan(testPlatform, []domain.CleanupAction{
					applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
					applyTestAction("action-2", testPlatform, domain.ActionDeleteComment, domain.ActionStatusPending),
				})
			}
			newExecutor := func() *scriptedExecutor {
				return &scriptedExecutor{scripts: map[string][]scriptedStep{
					"action-1": {{err: executorErr}},
					"action-2": {{result: ProviderResult{Outcome: OutcomeSucceeded}}},
				}}
			}

			continued := newExecutor()
			continuedExecution := testRunner(t, testProvider(continued), RuntimeState{}).Run(context.Background(), newPlan(), ExecutionModeSimulation)
			if continuedExecution.State != ExecutionStateFailed || continuedExecution.Counts.Failed != 1 || continuedExecution.Counts.Done != 1 || continuedExecution.Counts.Cancelled != 0 || len(continued.calls) != 2 {
				t.Fatalf("internal timeout cancelled continued run: execution=%#v calls=%#v", continuedExecution, continued.calls)
			}

			stopped := newExecutor()
			runner := testRunner(t, testProvider(stopped), RuntimeState{})
			runner.Policy = RunPolicy{StopAfterFinalFailure: true}
			stoppedExecution := runner.Run(context.Background(), newPlan(), ExecutionModeSimulation)
			if stoppedExecution.State != ExecutionStateFailed || stoppedExecution.Counts.Failed != 1 || stoppedExecution.Counts.Pending != 1 || stoppedExecution.Counts.Cancelled != 0 || len(stopped.calls) != 1 {
				t.Fatalf("internal timeout ignored stop policy: execution=%#v calls=%#v", stoppedExecution, stopped.calls)
			}
		})
	}
}

func TestRunnerProviderHaltsLeaveLaterActionsPending(t *testing.T) {
	tests := []struct {
		name       string
		result     ProviderResult
		haltReason ActionOutcome
		retryAfter time.Duration
	}{
		{name: "rate limit", result: ProviderResult{Outcome: OutcomeRateLimited, RetryAfter: 30 * time.Second, ProviderCode: "rate_limit"}, haltReason: OutcomeRateLimited, retryAfter: 30 * time.Second},
		{name: "authentication required", result: ProviderResult{Outcome: OutcomeAuthenticationRequired}, haltReason: OutcomeAuthenticationRequired},
		{name: "retry after", result: ProviderResult{Outcome: OutcomeRetryableFailure, RetryAfter: 2 * time.Second}, haltReason: OutcomeRetryableFailure, retryAfter: 2 * time.Second},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := applyTestPlan(testPlatform, []domain.CleanupAction{
				applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
				applyTestAction("action-2", testPlatform, domain.ActionDeleteComment, domain.ActionStatusPending),
			})
			executor := &scriptedExecutor{scripts: map[string][]scriptedStep{"action-1": {{result: test.result}}}}
			runner := testRunner(t, testProvider(executor), RuntimeState{})
			runner.Policy = RunPolicy{MaxAttemptsPerAction: 3}
			execution := runner.Run(context.Background(), plan, ExecutionModeSimulation)
			if execution.State != ExecutionStateHalted || execution.HaltReason != test.haltReason || execution.Counts.Failed != 1 || execution.Counts.Pending != 1 || len(executor.calls) != 1 {
				t.Fatalf("provider halt corrupted plan: execution=%#v calls=%#v", execution, executor.calls)
			}
			if execution.Results[0].RetryAfter != test.retryAfter || execution.Events[len(execution.Events)-1].HaltReason != test.haltReason {
				t.Fatalf("halt metadata lost: %#v", execution)
			}
		})
	}
}

func TestRunnerActionEventsPreserveOutcomeAttemptAndRoute(t *testing.T) {
	executor := &scriptedExecutor{scripts: map[string][]scriptedStep{"action-1": {
		{result: ProviderResult{Outcome: OutcomeRetryableFailure, ProviderCode: "temporary_failure"}},
		{result: ProviderResult{Outcome: OutcomeSucceeded}},
	}}}
	runner := testRunner(t, testProvider(executor), RuntimeState{})
	runner.Policy = RunPolicy{MaxAttemptsPerAction: 2}
	execution := runner.Run(context.Background(), singleActionPlan(), ExecutionModeSimulation)

	var actionEvents []ExecutionEvent
	for _, event := range execution.Events {
		if event.Type == EventActionResult {
			actionEvents = append(actionEvents, event)
		}
		if event.Executor == "" || event.Mode == "" {
			t.Fatalf("routed event lost identity: %#v", event)
		}
	}
	if len(actionEvents) != 2 || actionEvents[0].Outcome != OutcomeRetryableFailure || actionEvents[0].Attempt != 1 || !actionEvents[0].Retryable || actionEvents[0].ProviderCode != "temporary_failure" || actionEvents[1].Attempt != 2 {
		t.Fatalf("action event metadata incorrect: %#v", actionEvents)
	}
}

func TestRuntimeMetadataDoesNotEnterCleanupPlanJSON(t *testing.T) {
	executor := &scriptedExecutor{scripts: map[string][]scriptedStep{"action-1": {
		{result: ProviderResult{Outcome: OutcomeRetryableFailure}},
		{result: ProviderResult{Outcome: OutcomeSucceeded, ProviderCode: "done"}},
	}}}
	runner := testRunner(t, testProvider(executor), RuntimeState{})
	runner.Policy = RunPolicy{MaxAttemptsPerAction: 2}
	execution := runner.Run(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	encoded, err := json.Marshal(execution.Plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"outcome", "attempt", "retry_after", "provider_code"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("runtime metadata %q entered plan JSON: %s", forbidden, encoded)
		}
	}
}

func TestExecutionEventTypeNamesRemainStable(t *testing.T) {
	want := map[EventType]string{
		EventPreviewed:          "apply_previewed",
		EventConfirmed:          "apply_confirmed",
		EventExecutionStarted:   "apply_execution_started",
		EventActionResult:       "apply_action_result",
		EventActionSkipped:      "apply_action_skipped",
		EventExecutionStopped:   "apply_execution_stopped",
		EventExecutionCancelled: "apply_execution_cancelled",
		EventExecutionFinished:  "apply_execution_finished",
	}
	for eventType, value := range want {
		if string(eventType) != value {
			t.Fatalf("event type changed: got %q want %q", eventType, value)
		}
	}
}

func TestRetryActionValidatesEveryEligibleStatus(t *testing.T) {
	for _, status := range []domain.ActionStatus{
		domain.ActionStatusFailed,
		domain.ActionStatusSkipped,
		domain.ActionStatusStopped,
		domain.ActionStatusCancelled,
	} {
		t.Run("accept_"+string(status), func(t *testing.T) {
			plan := applyTestPlan(testPlatform, []domain.CleanupAction{applyTestAction("action-1", testPlatform, domain.ActionUnlike, status)})
			edit, err := RetryAction(&plan, "action-1")
			if err != nil || edit.Status != domain.ActionStatusPending || plan.Actions[0].Status != domain.ActionStatusPending || edit.Message != "Action queued for retry." {
				t.Fatalf("eligible retry failed: edit=%#v err=%v plan=%#v", edit, err, plan)
			}
		})
	}
	for _, status := range []domain.ActionStatus{
		domain.ActionStatusPending,
		domain.ActionStatusRunning,
		domain.ActionStatusDone,
	} {
		t.Run("reject_"+string(status), func(t *testing.T) {
			plan := applyTestPlan(testPlatform, []domain.CleanupAction{applyTestAction("action-1", testPlatform, domain.ActionUnlike, status)})
			if _, err := RetryAction(&plan, "action-1"); err == nil || plan.Actions[0].Status != status {
				t.Fatalf("ineligible retry changed action: err=%v plan=%#v", err, plan)
			}
		})
	}
}

func testProvider(executor Executor) fakeProvider {
	return fakeProvider{
		platform:   testPlatform,
		mode:       ExecutionModeSimulation,
		executorID: "test-simulation",
		supported: map[domain.ActionType]bool{
			domain.ActionUnlike:        true,
			domain.ActionDeleteComment: true,
			domain.ActionUnfollow:      true,
		},
		executor: executor,
	}
}

func testRunner(t *testing.T, provider Provider, state RuntimeState) Runner {
	t.Helper()
	registry, err := NewProviderRegistry(provider)
	if err != nil {
		t.Fatal(err)
	}
	return Runner{Providers: registry, State: state}
}

func singleActionPlan() domain.CleanupPlan {
	return applyTestPlan(testPlatform, []domain.CleanupAction{applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusPending)})
}

func applyTestPlan(platform domain.PlatformName, actions []domain.CleanupAction) domain.CleanupPlan {
	return domain.NewCleanupPlan("plan-1", platform, "test-source", applyTestTime(), actions)
}

func applyTestAction(id string, platform domain.PlatformName, actionType domain.ActionType, status domain.ActionStatus) domain.CleanupAction {
	return domain.CleanupAction{ID: id, Platform: platform, Type: actionType, TargetURL: "https://example.test/" + id, TargetID: "target-" + id, SourceActivityItemID: "item-" + id, Status: status, CreatedAt: applyTestTime()}
}

func applyTestTime() time.Time {
	return time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
}

func hasBlocker(preview Preview, code string) bool {
	for _, blocker := range preview.Blockers {
		if blocker.Code == code && blocker.Blocking {
			return true
		}
	}
	return false
}

func hasEvent(execution Execution, eventType EventType) bool {
	for _, event := range execution.Events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
