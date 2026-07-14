package apply

import (
	"context"
	"testing"
)

func TestActionIdempotencyKeyIsStableAndScoped(t *testing.T) {
	executionOne := ExecutionID("exec-11111111111111111111111111111111")
	executionTwo := ExecutionID("exec-22222222222222222222222222222222")

	first := actionIdempotencyKey(executionOne, "action-1")
	if !first.valid() {
		t.Fatalf("invalid key %q", first)
	}
	if repeated := actionIdempotencyKey(executionOne, "action-1"); repeated != first {
		t.Fatalf("same identity changed key: %q != %q", repeated, first)
	}
	if otherAction := actionIdempotencyKey(executionOne, "action-2"); otherAction == first {
		t.Fatalf("different actions shared key %q", first)
	}
	if otherExecution := actionIdempotencyKey(executionTwo, "action-1"); otherExecution == first {
		t.Fatalf("different executions shared key %q", first)
	}
}

func TestDurableResumeReusesActionIdempotencyKey(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	firstExecutor := &scriptedExecutor{scripts: map[string][]scriptedStep{
		"action-1": {{result: ProviderResult{Outcome: OutcomeRateLimited}}},
	}}
	firstRunner := durableTestRunner(t, store, firstExecutor, RunPolicy{MaxAttemptsPerAction: 2}, applyTestTime())
	first, err := firstRunner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if err != nil || len(firstExecutor.calls) != 1 {
		t.Fatalf("Start execution=%#v err=%v calls=%v", first, err, firstExecutor.calls)
	}

	secondExecutor := &scriptedExecutor{scripts: map[string][]scriptedStep{
		"action-1": {{result: ProviderResult{Outcome: OutcomeSucceeded}}},
	}}
	secondRunner := durableTestRunner(t, store, secondExecutor, RunPolicy{MaxAttemptsPerAction: 2}, applyTestTime())
	resumed, err := secondRunner.Resume(context.Background(), first.ID)
	if err != nil || resumed.State != ExecutionStateDone || len(secondExecutor.calls) != 1 {
		t.Fatalf("Resume execution=%#v err=%v calls=%v", resumed, err, secondExecutor.calls)
	}
	firstKey := firstExecutor.calls[0].idempotencyKey
	secondKey := secondExecutor.calls[0].idempotencyKey
	if !firstKey.valid() || firstKey != secondKey {
		t.Fatalf("resume changed idempotency key: first=%q second=%q", firstKey, secondKey)
	}
	if replayed, replayErr := store.Replay(first.ID); replayErr != nil || replayed.LastAttempts["action-1"] != 2 {
		t.Fatalf("Replay view=%#v err=%v", replayed, replayErr)
	}
}
