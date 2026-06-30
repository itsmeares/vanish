package apply

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

type fakeExecutor struct {
	results map[string]ActionResult
	calls   []string
}

func (executor *fakeExecutor) Execute(_ context.Context, action domain.CleanupAction) (ActionResult, error) {
	executor.calls = append(executor.calls, action.ID)
	if result, ok := executor.results[action.ID]; ok {
		return result, nil
	}
	return ActionResult{
		ActionID: action.ID,
		Platform: action.Platform,
		Type:     action.Type,
		Status:   domain.ActionStatusDone,
		Message:  "fake done",
	}, nil
}

func TestPreviewAllowsSupportedPendingInstagramPlan(t *testing.T) {
	plan := applyTestPlan(domain.PlatformInstagram, []domain.CleanupAction{
		applyTestAction("action-1", domain.PlatformInstagram, domain.ActionUnlike, domain.ActionStatusPending),
		applyTestAction("action-2", domain.PlatformInstagram, domain.ActionDeleteComment, domain.ActionStatusPending),
	})

	preview := Runner{}.Preview(plan)

	if !preview.CanApply || preview.PendingCount != 2 || preview.UnsupportedCount != 0 || !preview.AccountReady {
		t.Fatalf("unexpected preview: %#v", preview)
	}
}

func TestPreviewBlocksInvalidPlanNoPendingUnsupportedAndRedditPrerequisite(t *testing.T) {
	invalid := applyTestPlan(domain.PlatformInstagram, []domain.CleanupAction{
		applyTestAction("action-1", domain.PlatformInstagram, domain.ActionUnlike, domain.ActionStatusPending),
	})
	invalid.ID = ""
	if preview := (Runner{}).Preview(invalid); preview.CanApply || !hasBlocker(preview, "plan_invalid") {
		t.Fatalf("expected invalid plan blocker, got %#v", preview)
	}

	noPending := applyTestPlan(domain.PlatformInstagram, []domain.CleanupAction{
		applyTestAction("action-1", domain.PlatformInstagram, domain.ActionUnlike, domain.ActionStatusDone),
	})
	if preview := (Runner{}).Preview(noPending); preview.CanApply || !hasBlocker(preview, "no_pending_actions") {
		t.Fatalf("expected no pending blocker, got %#v", preview)
	}

	unsupported := applyTestPlan(domain.PlatformInstagram, []domain.CleanupAction{
		applyTestAction("action-1", domain.PlatformInstagram, domain.ActionDeletePost, domain.ActionStatusPending),
	})
	if preview := (Runner{}).Preview(unsupported); preview.CanApply || preview.UnsupportedCount != 1 || !hasBlocker(preview, "unsupported_actions") {
		t.Fatalf("expected unsupported action blocker, got %#v", preview)
	}

	mixed := applyTestPlan(domain.PlatformInstagram, []domain.CleanupAction{
		applyTestAction("action-1", domain.PlatformReddit, domain.ActionRedditDeleteComment, domain.ActionStatusPending),
	})
	if preview := (Runner{Accounts: AccountState{RedditConnected: true}}).Preview(mixed); preview.CanApply || preview.UnsupportedCount != 1 || preview.Unsupported[0].Reason != "action platform does not match plan platform" {
		t.Fatalf("expected mixed platform blocker, got %#v", preview)
	}

	redditPlan := applyTestPlan(domain.PlatformReddit, []domain.CleanupAction{
		applyTestAction("action-1", domain.PlatformReddit, domain.ActionRedditDeleteComment, domain.ActionStatusPending),
	})
	if preview := (Runner{}).Preview(redditPlan); preview.CanApply || preview.AccountReady || !hasBlocker(preview, "reddit_account_required") {
		t.Fatalf("expected reddit account blocker, got %#v", preview)
	}
	if preview := (Runner{Accounts: AccountState{RedditConnected: true}}).Preview(redditPlan); !preview.CanApply || !preview.AccountReady {
		t.Fatalf("expected reddit account ready preview, got %#v", preview)
	}
}

func TestRunnerRunsOneActionAtATimeAndRecordsEvents(t *testing.T) {
	plan := applyTestPlan(domain.PlatformInstagram, []domain.CleanupAction{
		applyTestAction("action-1", domain.PlatformInstagram, domain.ActionUnlike, domain.ActionStatusPending),
		applyTestAction("action-2", domain.PlatformInstagram, domain.ActionDeleteComment, domain.ActionStatusPending),
	})
	executor := &fakeExecutor{results: map[string]ActionResult{}}

	execution := (Runner{Executor: executor}).Run(context.Background(), plan)

	if execution.State != ExecutionStateDone || execution.Counts.Done != 2 || execution.Counts.Pending != 0 {
		t.Fatalf("unexpected execution: %#v", execution)
	}
	if len(executor.calls) != 2 || executor.calls[0] != "action-1" || executor.calls[1] != "action-2" {
		t.Fatalf("expected sequential calls, got %#v", executor.calls)
	}
	if len(execution.Results) != 2 || execution.Plan.Actions[0].Status != domain.ActionStatusDone || execution.Plan.Actions[1].Status != domain.ActionStatusDone {
		t.Fatalf("expected done action results, got %#v", execution)
	}
	if !hasEvent(execution, EventExecutionStarted) || !hasEvent(execution, EventActionResult) || !hasEvent(execution, EventExecutionFinished) {
		t.Fatalf("expected lifecycle events, got %#v", execution.Events)
	}
}

func TestRunnerRecordsFailuresAndCancellation(t *testing.T) {
	plan := applyTestPlan(domain.PlatformInstagram, []domain.CleanupAction{
		applyTestAction("action-1", domain.PlatformInstagram, domain.ActionUnlike, domain.ActionStatusPending),
	})
	executor := &fakeExecutor{results: map[string]ActionResult{
		"action-1": {
			Status:  domain.ActionStatusFailed,
			Message: "fake failure",
		},
	}}

	execution := (Runner{Executor: executor}).Run(context.Background(), plan)

	if execution.State != ExecutionStateFailed || execution.Counts.Failed != 1 || execution.Results[0].Message != "fake failure" {
		t.Fatalf("expected failed execution, got %#v", execution)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled := (Runner{}).Run(ctx, applyTestPlan(domain.PlatformInstagram, []domain.CleanupAction{
		applyTestAction("action-1", domain.PlatformInstagram, domain.ActionUnlike, domain.ActionStatusPending),
		applyTestAction("action-2", domain.PlatformInstagram, domain.ActionDeleteComment, domain.ActionStatusPending),
	}))
	if cancelled.State != ExecutionStateCancelled || cancelled.Counts.Cancelled != 2 || !hasEvent(cancelled, EventExecutionCancelled) {
		t.Fatalf("expected cancelled execution, got %#v", cancelled)
	}
}

func TestRunnerReportsAllSkippedState(t *testing.T) {
	plan := applyTestPlan(domain.PlatformInstagram, []domain.CleanupAction{
		applyTestAction("action-1", domain.PlatformInstagram, domain.ActionUnlike, domain.ActionStatusPending),
	})
	executor := &fakeExecutor{results: map[string]ActionResult{
		"action-1": {
			Status: domain.ActionStatusSkipped,
		},
	}}

	execution := (Runner{Executor: executor}).Run(context.Background(), plan)

	if execution.State != ExecutionStateSkipped || execution.Counts.Skipped != 1 {
		t.Fatalf("expected skipped execution, got %#v", execution)
	}
}

func TestRunnerStopsWhenExecutorReturnsStopped(t *testing.T) {
	plan := applyTestPlan(domain.PlatformInstagram, []domain.CleanupAction{
		applyTestAction("action-1", domain.PlatformInstagram, domain.ActionUnlike, domain.ActionStatusPending),
		applyTestAction("action-2", domain.PlatformInstagram, domain.ActionDeleteComment, domain.ActionStatusPending),
	})
	executor := &fakeExecutor{results: map[string]ActionResult{
		"action-1": {
			Status:  domain.ActionStatusStopped,
			Message: "stop requested",
		},
	}}

	execution := (Runner{Executor: executor}).Run(context.Background(), plan)

	if execution.State != ExecutionStateStopped || execution.Counts.Stopped != 2 || len(executor.calls) != 1 {
		t.Fatalf("expected stopped execution before second action, got execution=%#v calls=%#v", execution, executor.calls)
	}
}

func TestSkipRetryStopCancelPrimitives(t *testing.T) {
	plan := applyTestPlan(domain.PlatformInstagram, []domain.CleanupAction{
		applyTestAction("pending", domain.PlatformInstagram, domain.ActionUnlike, domain.ActionStatusPending),
		applyTestAction("failed", domain.PlatformInstagram, domain.ActionDeleteComment, domain.ActionStatusFailed),
		applyTestAction("done", domain.PlatformInstagram, domain.ActionUnfollow, domain.ActionStatusDone),
	})

	event, err := SkipAction(&plan, "pending", "user skipped")
	if err != nil || event.Type != EventActionSkipped || plan.Actions[0].Status != domain.ActionStatusSkipped {
		t.Fatalf("expected skipped pending action, event=%#v err=%v plan=%#v", event, err, plan)
	}
	if err := RetryAction(&plan, "failed"); err != nil || plan.Actions[1].Status != domain.ActionStatusPending {
		t.Fatalf("expected failed action retried, err=%v plan=%#v", err, plan)
	}
	if err := RetryAction(&plan, "done"); err == nil {
		t.Fatalf("expected done action retry to fail")
	}
	if _, err := SkipAction(&plan, "done", ""); err == nil {
		t.Fatalf("expected done action skip to fail")
	}

	stopped := StopPending(&plan, "stop")
	if stopped != 1 || plan.Actions[1].Status != domain.ActionStatusStopped {
		t.Fatalf("expected one stopped pending action, stopped=%d plan=%#v", stopped, plan)
	}

	cancelPlan := applyTestPlan(domain.PlatformInstagram, []domain.CleanupAction{
		applyTestAction("a", domain.PlatformInstagram, domain.ActionUnlike, domain.ActionStatusPending),
	})
	if cancelled := CancelPending(&cancelPlan, "cancel"); cancelled != 1 || cancelPlan.Actions[0].Status != domain.ActionStatusCancelled {
		t.Fatalf("expected cancelled pending action, got %d %#v", cancelled, cancelPlan)
	}
}

func TestRunWithBlockedPreviewDoesNotExecute(t *testing.T) {
	plan := applyTestPlan(domain.PlatformReddit, []domain.CleanupAction{
		applyTestAction("action-1", domain.PlatformReddit, domain.ActionRedditDeletePost, domain.ActionStatusPending),
	})
	executor := &fakeExecutor{}

	execution := (Runner{Executor: executor}).Run(context.Background(), plan)

	if execution.State != ExecutionStateFailed || len(executor.calls) != 0 || !hasBlocker(execution.Preview, "reddit_account_required") {
		t.Fatalf("expected blocked execution without calls, execution=%#v calls=%#v", execution, executor.calls)
	}
}

func TestNoopExecutorHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NoopExecutor{}.Execute(ctx, applyTestAction("action-1", domain.PlatformInstagram, domain.ActionUnlike, domain.ActionStatusPending))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func applyTestPlan(platform domain.PlatformName, actions []domain.CleanupAction) domain.CleanupPlan {
	return domain.NewCleanupPlan("plan-1", platform, "test-source", applyTestTime(), actions)
}

func applyTestAction(id string, platform domain.PlatformName, actionType domain.ActionType, status domain.ActionStatus) domain.CleanupAction {
	return domain.CleanupAction{
		ID:                   id,
		Platform:             platform,
		Type:                 actionType,
		TargetURL:            "https://example.test/" + id,
		TargetID:             "target-" + id,
		SourceActivityItemID: "item-" + id,
		Status:               status,
		CreatedAt:            applyTestTime(),
	}
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
