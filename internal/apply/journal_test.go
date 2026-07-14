package apply

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

func TestExecutionManifestIdentityIsUniqueImmutableAndDeterministic(t *testing.T) {
	plan := singleActionPlan()
	plan.Actions[0].Metadata = map[string]string{"safe": "original"}
	id1, err := NewExecutionID()
	if err != nil {
		t.Fatal(err)
	}
	id2, err := NewExecutionID()
	if err != nil {
		t.Fatal(err)
	}
	if id1 == id2 {
		t.Fatal("execution IDs were not unique")
	}
	now := applyTestTime()
	manifest, err := newExecutionManifest(id1, plan, ExecutionModeSimulation, "test-executor", DefaultRunPolicy(), now)
	if err != nil {
		t.Fatal(err)
	}
	wantFingerprint, err := executionFingerprint(plan, ExecutionModeSimulation, "test-executor", DefaultRunPolicy())
	if err != nil || manifest.Fingerprint != wantFingerprint {
		t.Fatalf("fingerprint mismatch got=%q want=%q err=%v", manifest.Fingerprint, wantFingerprint, err)
	}
	plan.Actions[0].ID = "changed"
	plan.Actions[0].Metadata["safe"] = "changed"
	if manifest.Plan.Actions[0].ID == "changed" || manifest.Plan.Actions[0].Metadata["safe"] != "original" {
		t.Fatal("manifest retained caller-owned plan data")
	}
	changed := cloneExecutionPlan(manifest.Plan)
	changed.Actions[0].TargetID = "different"
	changedFingerprint, err := executionFingerprint(changed, manifest.Mode, manifest.Executor, manifest.Policy)
	if err != nil || changedFingerprint == manifest.Fingerprint {
		t.Fatal("changed plan content retained the same fingerprint")
	}
	changedFingerprint, _ = executionFingerprint(manifest.Plan, manifest.Mode, "other-executor", manifest.Policy)
	if changedFingerprint == manifest.Fingerprint {
		t.Fatal("changed route retained the same fingerprint")
	}
}

func TestDurableRunnerCommitsOrderingAndTerminalState(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	var order []string
	store.hooks.onAppend = func(event JournalEvent) { order = append(order, string(event.Kind)) }
	executor := &orderingExecutor{order: &order}
	runner := durableTestRunner(t, store, executor, DefaultRunPolicy(), applyTestTime())

	execution, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	want := []string{"execution_started", "action_attempt_started", "executor", "action_result_recorded", "execution_completed"}
	if !slices.Equal(order, want) {
		t.Fatalf("durable order mismatch\n got: %v\nwant: %v", order, want)
	}
	if execution.State != ExecutionStateDone || execution.Resumability != ResumabilityTerminal || len(executor.calls) != 1 {
		t.Fatalf("unexpected execution: %#v calls=%v", execution, executor.calls)
	}
	view, err := store.Replay(execution.ID)
	if err != nil || view.State != ExecutionStateDone || view.Resumability != ResumabilityTerminal || view.Counts.Done != 1 {
		t.Fatalf("Replay view=%#v err=%v", view, err)
	}
	if _, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation); !errors.Is(err, ErrExecutionExists) {
		t.Fatalf("identical terminal execution restarted: %v", err)
	}
}

func TestAttemptStartFailurePreventsExecutor(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	store.hooks.beforeAppend = func(event JournalEvent) error {
		if event.Kind == JournalAttemptStarted {
			return errors.New("synthetic attempt-start failure")
		}
		return nil
	}
	executor := &fakeExecutor{}
	runner := durableTestRunner(t, store, executor, DefaultRunPolicy(), applyTestTime())
	execution, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if err == nil || len(executor.calls) != 0 {
		t.Fatalf("attempt-start failure err=%v calls=%v", err, executor.calls)
	}
	view, replayErr := store.Replay(execution.ID)
	if replayErr != nil || view.Resumability != ResumabilityResumable || view.Counts.Pending != 1 {
		t.Fatalf("safe pending replay view=%#v err=%v", view, replayErr)
	}
}

func TestResultWriteFailureStopsAndRequiresResolution(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	store.hooks.beforeAppend = func(event JournalEvent) error {
		if event.Kind == JournalResultRecorded {
			return errors.New("synthetic result failure")
		}
		return nil
	}
	plan := applyTestPlan(testPlatform, []domain.CleanupAction{
		applyTestAction("first", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
		applyTestAction("second", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
	})
	executor := &fakeExecutor{}
	runner := durableTestRunner(t, store, executor, DefaultRunPolicy(), applyTestTime())
	execution, err := runner.Start(context.Background(), plan, ExecutionModeSimulation)
	if err == nil || len(executor.calls) != 1 || executor.calls[0] != "first" {
		t.Fatalf("result failure err=%v calls=%v", err, executor.calls)
	}
	view, replayErr := store.Replay(execution.ID)
	if replayErr != nil || view.Resumability != ResumabilityResolution || view.Unresolved == nil || view.Unresolved.ActionID != "first" {
		t.Fatalf("resolution replay view=%#v err=%v", view, replayErr)
	}
	before := len(executor.calls)
	if _, err := runner.Resume(context.Background(), execution.ID); !errors.Is(err, ErrExecutionResolutionRequired) || len(executor.calls) != before {
		t.Fatalf("resolution resume err=%v calls=%v", err, executor.calls)
	}
	abandoned, err := runner.Abandon(execution.ID)
	if err != nil || abandoned.State != ExecutionStateAbandoned || len(executor.calls) != before {
		t.Fatalf("abandon execution=%#v err=%v calls=%v", abandoned, err, executor.calls)
	}
}

func TestSafeResumeContinuesAttemptsAndNeverRepeatsCompletedActions(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	executor := &scriptedExecutor{scripts: map[string][]scriptedStep{
		"action-1": {
			{result: ProviderResult{Outcome: OutcomeRateLimited}},
			{result: ProviderResult{Outcome: OutcomeSucceeded, Message: ProviderMessageNoopCompleted}},
		},
	}}
	policy := RunPolicy{MaxAttemptsPerAction: 2}
	runner := durableTestRunner(t, store, executor, policy, applyTestTime())
	first, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if err != nil || first.State != ExecutionStateHalted || len(executor.calls) != 1 {
		t.Fatalf("first execution=%#v err=%v calls=%v", first, err, executor.calls)
	}
	resumed, err := runner.Resume(context.Background(), first.ID)
	if err != nil || resumed.State != ExecutionStateDone || len(executor.calls) != 2 {
		t.Fatalf("resumed execution=%#v err=%v calls=%v", resumed, err, executor.calls)
	}
	if len(resumed.Results) != 2 || resumed.Results[0].Attempt != 1 || resumed.Results[1].Attempt != 2 {
		t.Fatalf("attempt continuity lost: %#v", resumed.Results)
	}
	if _, err := runner.Resume(context.Background(), first.ID); !errors.Is(err, ErrExecutionTerminal) || len(executor.calls) != 2 {
		t.Fatalf("terminal resume err=%v calls=%v", err, executor.calls)
	}
}

func TestRetryNotBeforeUsesInjectedClockWithoutSleeping(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	now := applyTestTime()
	executor := &scriptedExecutor{scripts: map[string][]scriptedStep{
		"action-1": {
			{result: ProviderResult{Outcome: OutcomeRateLimited, RetryAfter: time.Minute}},
			{result: ProviderResult{Outcome: OutcomeSucceeded}},
		},
	}}
	runner := durableTestRunner(t, store, executor, RunPolicy{MaxAttemptsPerAction: 2}, now)
	runner.Now = func() time.Time { return now }
	first, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Resume(context.Background(), first.ID); !errors.Is(err, ErrExecutionNotReady) || len(executor.calls) != 1 {
		t.Fatalf("early resume err=%v calls=%v", err, executor.calls)
	}
	now = now.Add(time.Minute)
	resumed, err := runner.Resume(context.Background(), first.ID)
	if err != nil || resumed.State != ExecutionStateDone || len(executor.calls) != 2 {
		t.Fatalf("ready resume execution=%#v err=%v calls=%v", resumed, err, executor.calls)
	}
}

func TestReplayTrailingRecordPolicy(t *testing.T) {
	t.Run("unterminated final record is ignored", func(t *testing.T) {
		store, writer, manifest := startedTestExecution(t)
		writer.Close()
		appendJournalBytes(t, store, manifest.ExecutionID, []byte(`{"execution_id":"partial"`))
		view, err := store.Replay(manifest.ExecutionID)
		if err != nil || view.RecoveryWarning == "" || view.Resumability != ResumabilityResumable {
			t.Fatalf("view=%#v err=%v", view, err)
		}
	})
	t.Run("malformed terminated record is corrupt", func(t *testing.T) {
		store, writer, manifest := startedTestExecution(t)
		writer.Close()
		appendJournalBytes(t, store, manifest.ExecutionID, []byte("not-json\n"))
		if _, err := store.Replay(manifest.ExecutionID); !errors.Is(err, ErrExecutionCorrupt) {
			t.Fatalf("malformed journal error=%v", err)
		}
	})
}

func TestSecondWriterFailsAndCleanCloseReleasesLock(t *testing.T) {
	store, writer, manifest := startedTestExecution(t)
	if _, _, err := store.OpenWriter(manifest.ExecutionID); !errors.Is(err, ErrExecutionLocked) {
		t.Fatalf("second writer error=%v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	next, _, err := store.OpenWriter(manifest.ExecutionID)
	if err != nil {
		t.Fatalf("clean close did not release lock: %v", err)
	}
	next.Close()
}

func TestListingUsesSummaryWithoutLoadingLargeManifest(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	runner := durableTestRunner(t, store, &fakeExecutor{}, DefaultRunPolicy(), applyTestTime())
	execution, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := store.executionDir(execution.ID)
	if err := os.Remove(filepath.Join(dir, manifestFileName)); err != nil {
		t.Fatal(err)
	}
	summaries, err := store.List()
	if err != nil || len(summaries) != 1 || summaries[0].ExecutionID != execution.ID {
		t.Fatalf("summary listing summaries=%#v err=%v", summaries, err)
	}
}

func TestExecutionStoreUsesRestrictivePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX permission bits consistently")
	}
	store, writer, manifest := startedTestExecution(t)
	writer.Close()
	dir, _ := store.executionDir(manifest.ExecutionID)
	for _, test := range []struct {
		path string
		want os.FileMode
	}{{store.root, 0o700}, {dir, 0o700}, {filepath.Join(dir, manifestFileName), 0o600}, {filepath.Join(dir, journalFileName), 0o600}, {filepath.Join(dir, summaryFileName), 0o600}} {
		info, err := os.Stat(test.path)
		if err != nil || info.Mode().Perm() != test.want {
			t.Fatalf("%s mode=%v want=%v err=%v", test.path, info.Mode().Perm(), test.want, err)
		}
	}
}

type orderingExecutor struct {
	order *[]string
	calls []string
}

func (executor *orderingExecutor) Execute(_ context.Context, request ActionRequest) (ProviderResult, error) {
	*executor.order = append(*executor.order, "executor")
	executor.calls = append(executor.calls, request.Action.ID)
	return ProviderResult{Outcome: OutcomeSucceeded, Message: ProviderMessageNoopCompleted}, nil
}

func durableTestRunner(t *testing.T, store *ExecutionStore, executor Executor, policy RunPolicy, now time.Time) Runner {
	t.Helper()
	provider := testProvider(executor)
	registry, err := NewProviderRegistry(provider)
	if err != nil {
		t.Fatal(err)
	}
	return Runner{Providers: registry, Policy: policy, Store: store, Now: func() time.Time { return now }}
}

func startedTestExecution(t *testing.T) (*ExecutionStore, *ExecutionWriter, ExecutionManifest) {
	t.Helper()
	store := NewExecutionStore(t.TempDir())
	id, err := NewExecutionID()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := newExecutionManifest(id, singleActionPlan(), ExecutionModeSimulation, "test-executor", DefaultRunPolicy(), applyTestTime())
	if err != nil {
		t.Fatal(err)
	}
	writer, _, err := store.Create(manifest, manifest.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	return store, writer, manifest
}

func appendJournalBytes(t *testing.T, store *ExecutionStore, id ExecutionID, data []byte) {
	t.Helper()
	dir, err := store.executionDir(id)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(dir, journalFileName), os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
