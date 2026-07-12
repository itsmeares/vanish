package apply

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

const testPlatform domain.PlatformName = "test-provider"

type fakeExecutor struct {
	results map[string]ActionResult
	calls   []string
}

func (executor *fakeExecutor) Execute(_ context.Context, action domain.CleanupAction) (ActionResult, error) {
	executor.calls = append(executor.calls, action.ID)
	if result, ok := executor.results[action.ID]; ok {
		return result, nil
	}
	return ActionResult{ActionID: action.ID, Platform: action.Platform, Type: action.Type, Status: domain.ActionStatusDone, Message: "fake done"}, nil
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
	executor := &fakeExecutor{results: map[string]ActionResult{}}
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
		executor := &fakeExecutor{results: map[string]ActionResult{"action-1": {Status: domain.ActionStatusFailed, Message: "fake failure"}}}
		execution := testRunner(t, testProvider(executor), RuntimeState{}).Run(context.Background(), singleActionPlan(), ExecutionModeSimulation)
		if execution.State != ExecutionStateFailed || execution.Counts.Failed != 1 || execution.Results[0].Message != "fake failure" {
			t.Fatalf("expected failed execution, got %#v", execution)
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		execution := testRunner(t, testProvider(&fakeExecutor{}), RuntimeState{}).Run(ctx, applyTestPlan(testPlatform, []domain.CleanupAction{
			applyTestAction("action-1", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
			applyTestAction("action-2", testPlatform, domain.ActionDeleteComment, domain.ActionStatusPending),
		}), ExecutionModeSimulation)
		if execution.State != ExecutionStateCancelled || execution.Counts.Cancelled != 2 || !hasEvent(execution, EventExecutionCancelled) {
			t.Fatalf("expected cancelled execution, got %#v", execution)
		}
	})

	t.Run("all skipped", func(t *testing.T) {
		executor := &fakeExecutor{results: map[string]ActionResult{"action-1": {Status: domain.ActionStatusSkipped}}}
		execution := testRunner(t, testProvider(executor), RuntimeState{}).Run(context.Background(), singleActionPlan(), ExecutionModeSimulation)
		if execution.State != ExecutionStateSkipped || execution.Counts.Skipped != 1 {
			t.Fatalf("expected skipped execution, got %#v", execution)
		}
		foundSkipped := false
		for _, event := range execution.Events {
			if event.Type == EventActionSkipped && (event.Mode != ExecutionModeSimulation || event.Executor != "test-simulation") {
				t.Fatalf("skipped execution event lost route identity: %#v", event)
			}
			if event.Type == EventActionSkipped {
				foundSkipped = true
			}
		}
		if !foundSkipped {
			t.Fatalf("expected skipped execution event, got %#v", execution.Events)
		}
	})

	t.Run("stopped", func(t *testing.T) {
		executor := &fakeExecutor{results: map[string]ActionResult{"action-1": {Status: domain.ActionStatusStopped, Message: "stop requested"}}}
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
	if err := RetryAction(&plan, "failed"); err != nil || plan.Actions[1].Status != domain.ActionStatusPending {
		t.Fatalf("expected failed action retried, err=%v plan=%#v", err, plan)
	}
	if err := RetryAction(&plan, "done"); err == nil {
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
