package apply_test

import (
	"context"
	"testing"
	"time"

	"github.com/itsmeares/vanish/internal/apply"
	"github.com/itsmeares/vanish/internal/domain"
	"github.com/itsmeares/vanish/internal/instagram"
	"github.com/itsmeares/vanish/internal/reddit"
)

func TestBuiltInSimulationProvidersPreserveNoopPlans(t *testing.T) {
	registry, err := apply.NewProviderRegistry(instagram.SimulationProvider(), reddit.SimulationProvider())
	if err != nil {
		t.Fatal(err)
	}

	instagramPlan := integrationPlan(domain.PlatformInstagram, domain.ActionUnlike)
	store := apply.NewExecutionStore(t.TempDir())
	instagramExecution, err := (apply.Runner{Providers: registry, Policy: apply.DefaultRunPolicy(), Store: store}).Start(context.Background(), instagramPlan, apply.ExecutionModeSimulation)
	if err != nil {
		t.Fatal(err)
	}
	if instagramExecution.State != apply.ExecutionStateDone || instagramExecution.Preview.Executor != instagram.SimulationExecutorID || instagramExecution.Results[0].Message != "No-op apply completed." || instagramExecution.Results[0].Outcome != apply.OutcomeSucceeded || instagramExecution.Results[0].Attempt != 1 {
		t.Fatalf("unexpected Instagram simulation: %#v", instagramExecution)
	}

	redditPlan := integrationPlan(domain.PlatformReddit, domain.ActionRedditDeleteComment)
	connected := apply.NewRuntimeState(map[domain.PlatformName]apply.ConnectionState{domain.PlatformReddit: {Connected: true}})
	redditExecution, err := (apply.Runner{Providers: registry, State: connected, Policy: apply.DefaultRunPolicy(), Store: store}).Start(context.Background(), redditPlan, apply.ExecutionModeSimulation)
	if err != nil {
		t.Fatal(err)
	}
	if redditExecution.State != apply.ExecutionStateDone || redditExecution.Preview.Executor != reddit.SimulationExecutorID || redditExecution.Results[0].Message != "No-op apply completed." || redditExecution.Results[0].Outcome != apply.OutcomeSucceeded || redditExecution.Results[0].Attempt != 1 {
		t.Fatalf("unexpected Reddit simulation: %#v", redditExecution)
	}
	for _, event := range redditExecution.Events {
		if event.Executor != reddit.SimulationExecutorID {
			t.Fatalf("Reddit event used wrong executor: %#v", event)
		}
	}
	for _, platform := range []domain.PlatformName{domain.PlatformInstagram, domain.PlatformReddit} {
		provider, resolveErr := registry.Resolve(platform, apply.ExecutionModeSimulation)
		if resolveErr != nil || provider.Reconciler() != nil {
			t.Fatalf("production simulation provider exposes reconciliation: platform=%s err=%v", platform, resolveErr)
		}
	}
}

func TestRedditSimulationReadinessUsesOnlyRedditConnection(t *testing.T) {
	registry, err := apply.NewProviderRegistry(instagram.SimulationProvider(), reddit.SimulationProvider())
	if err != nil {
		t.Fatal(err)
	}
	plan := integrationPlan(domain.PlatformReddit, domain.ActionRedditDeletePost)

	for name, state := range map[string]apply.RuntimeState{
		"disconnected": apply.NewRuntimeState(nil),
		"unrelated":    apply.NewRuntimeState(map[domain.PlatformName]apply.ConnectionState{domain.PlatformInstagram: {Connected: true}}),
	} {
		t.Run(name, func(t *testing.T) {
			preview := (apply.Runner{Providers: registry, State: state}).Preview(plan, apply.ExecutionModeSimulation)
			if preview.CanApply || preview.ProviderReady || !integrationHasBlocker(preview, "reddit_account_required") {
				t.Fatalf("expected Reddit readiness blocker, got %#v", preview)
			}
		})
	}
}

func integrationPlan(platform domain.PlatformName, actionType domain.ActionType) domain.CleanupPlan {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	return domain.NewCleanupPlan("plan-1", platform, "test", now, []domain.CleanupAction{{
		ID: "action-1", Platform: platform, Type: actionType, TargetID: "target-1", SourceActivityItemID: "item-1", Status: domain.ActionStatusPending, CreatedAt: now,
	}})
}

func integrationHasBlocker(preview apply.Preview, code string) bool {
	for _, blocker := range preview.Blockers {
		if blocker.Blocking && blocker.Code == code {
			return true
		}
	}
	return false
}
