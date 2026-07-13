package apply

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

func TestOpenWriterRepairsOnlyIgnoredPartialTail(t *testing.T) {
	store, writer, manifest := startedTestExecution(t)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	dir, _ := store.executionDir(manifest.ExecutionID)
	journalPath := filepath.Join(dir, journalFileName)
	original, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	appendJournalBytes(t, store, manifest.ExecutionID, []byte(`{"execution_id":"partial"`))
	repairSyncs := 0
	store.hooks.beforeDirectorySync = func(path string) error {
		if path != dir {
			t.Fatalf("repair synced %q, want %q", path, dir)
		}
		repairSyncs++
		return nil
	}

	readOnly, err := store.Replay(manifest.ExecutionID)
	if err != nil || readOnly.RecoveryWarning == "" || !readOnly.ignoredPartialTail || readOnly.journalCompleteAt != int64(len(original)) {
		t.Fatalf("read-only replay=%#v err=%v", readOnly, err)
	}

	repairedWriter, repaired, err := store.OpenWriter(manifest.ExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.RecoveryWarning != "" || repaired.ignoredPartialTail || repaired.LastSequence != readOnly.LastSequence {
		t.Fatalf("repaired replay=%#v", repaired)
	}
	if repairSyncs != 1 {
		t.Fatalf("repair directory syncs=%d", repairSyncs)
	}
	if err := repairedWriter.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, original) {
		t.Fatalf("repair changed complete records: got %d bytes want %d", len(after), len(original))
	}
	if replayed, err := store.Replay(manifest.ExecutionID); err != nil || replayed.RecoveryWarning != "" {
		t.Fatalf("post-repair replay=%#v err=%v", replayed, err)
	}
}

func TestResumeAppendsValidEventsAfterPartialTailRepair(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	executor := &scriptedExecutor{scripts: map[string][]scriptedStep{
		"action-1": {
			{result: ProviderResult{Outcome: OutcomeRateLimited}},
			{result: ProviderResult{Outcome: OutcomeSucceeded}},
		},
	}}
	runner := durableTestRunner(t, store, executor, RunPolicy{MaxAttemptsPerAction: 2}, applyTestTime())
	first, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if err != nil {
		t.Fatal(err)
	}
	appendJournalBytes(t, store, first.ID, []byte(`{"kind":"partial"`))
	if view, err := store.Replay(first.ID); err != nil || view.RecoveryWarning == "" {
		t.Fatalf("partial replay=%#v err=%v", view, err)
	}

	resumed, err := runner.Resume(context.Background(), first.ID)
	if err != nil || resumed.State != ExecutionStateDone || len(executor.calls) != 2 {
		t.Fatalf("resumed=%#v err=%v calls=%v", resumed, err, executor.calls)
	}
	view, err := store.Replay(first.ID)
	if err != nil || view.State != ExecutionStateDone || view.RecoveryWarning != "" || view.LastSequence < 7 {
		t.Fatalf("final replay=%#v err=%v", view, err)
	}
}

func TestOpenWriterNeverRepairsTerminatedOrInteriorCorruption(t *testing.T) {
	for name, corrupt := range map[string][]byte{
		"terminated": []byte("not-json\n"),
		"interior":   []byte("not-json\n{}\n"),
	} {
		t.Run(name, func(t *testing.T) {
			store, writer, manifest := startedTestExecution(t)
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			appendJournalBytes(t, store, manifest.ExecutionID, corrupt)
			dir, _ := store.executionDir(manifest.ExecutionID)
			journalPath := filepath.Join(dir, journalFileName)
			before, err := os.ReadFile(journalPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := store.OpenWriter(manifest.ExecutionID); !errors.Is(err, ErrExecutionCorrupt) {
				t.Fatalf("OpenWriter error=%v", err)
			}
			after, err := os.ReadFile(journalPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) {
				t.Fatal("corrupt terminated data was truncated")
			}
		})
	}
}

func TestJournalRepairFailurePreventsExecutorCalls(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	executor := &scriptedExecutor{scripts: map[string][]scriptedStep{
		"action-1": {{result: ProviderResult{Outcome: OutcomeRateLimited}}},
	}}
	runner := durableTestRunner(t, store, executor, RunPolicy{MaxAttemptsPerAction: 2}, applyTestTime())
	first, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if err != nil {
		t.Fatal(err)
	}
	appendJournalBytes(t, store, first.ID, []byte(`{"partial":`))
	store.hooks.beforeJournalRepair = func() error { return errors.New("synthetic repair failure") }
	before := len(executor.calls)
	if _, err := runner.Resume(context.Background(), first.ID); err == nil || len(executor.calls) != before {
		t.Fatalf("Resume err=%v calls=%v", err, executor.calls)
	}
	if view, err := store.Replay(first.ID); err != nil || view.RecoveryWarning == "" {
		t.Fatalf("failed repair changed journal: view=%#v err=%v", view, err)
	}
}

func TestFirstJournalCreationSyncsExecutionDirectory(t *testing.T) {
	t.Run("sync once", func(t *testing.T) {
		store := NewExecutionStore(t.TempDir())
		syncCalls := 0
		store.hooks.beforeDirectorySync = func(string) error {
			syncCalls++
			return nil
		}
		id, err := NewExecutionID()
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := newExecutionManifest(id, singleActionPlan(), ExecutionModeSimulation, "test-executor", DefaultRunPolicy(), applyTestTime())
		if err != nil {
			t.Fatal(err)
		}
		writer, summary, err := store.Create(manifest, manifest.CreatedAt)
		if err != nil {
			t.Fatal(err)
		}
		if syncCalls != 1 {
			t.Fatalf("first creation sync calls=%d", syncCalls)
		}
		if _, err := writer.Append(JournalEvent{Timestamp: applyTestTime(), Kind: JournalExecutionResumed}, summary); err != nil {
			t.Fatal(err)
		}
		if syncCalls != 1 {
			t.Fatalf("ordinary append synced directory: calls=%d", syncCalls)
		}
		_ = writer.Close()
	})

	t.Run("sync failure stops before executor", func(t *testing.T) {
		store := NewExecutionStore(t.TempDir())
		store.hooks.beforeDirectorySync = func(string) error { return errors.New("synthetic directory sync failure") }
		executor := &fakeExecutor{}
		runner := durableTestRunner(t, store, executor, DefaultRunPolicy(), applyTestTime())
		if _, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation); err == nil || len(executor.calls) != 0 {
			t.Fatalf("Start err=%v calls=%v", err, executor.calls)
		}
	})
}

func TestDurableRunnerResolvesOneExecutorPerInvocation(t *testing.T) {
	t.Run("start uses one instance for all actions", func(t *testing.T) {
		factoryCalls := 0
		var instances []int
		provider := testProvider(nil)
		provider.executorFactory = func() Executor {
			factoryCalls++
			return &instanceTrackingExecutor{instance: factoryCalls, calls: &instances, outcome: OutcomeSucceeded}
		}
		registry, err := NewProviderRegistry(provider)
		if err != nil {
			t.Fatal(err)
		}
		if factoryCalls != 0 {
			t.Fatalf("registry construction called factory %d times", factoryCalls)
		}
		plan := applyTestPlan(testPlatform, []domain.CleanupAction{
			applyTestAction("first", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
			applyTestAction("second", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
		})
		runner := Runner{Providers: registry, Policy: DefaultRunPolicy(), Store: NewExecutionStore(t.TempDir()), Now: func() time.Time { return applyTestTime() }}
		execution, err := runner.Start(context.Background(), plan, ExecutionModeSimulation)
		if err != nil || execution.State != ExecutionStateDone || factoryCalls != 1 || len(instances) != 2 || instances[0] != 1 || instances[1] != 1 {
			t.Fatalf("execution=%#v err=%v factory=%d instances=%v", execution, err, factoryCalls, instances)
		}
	})

	t.Run("resume resolves one new stable instance", func(t *testing.T) {
		factoryCalls := 0
		var instances []int
		provider := testProvider(nil)
		provider.executorFactory = func() Executor {
			factoryCalls++
			outcome := OutcomeSucceeded
			if factoryCalls == 1 {
				outcome = OutcomeRateLimited
			}
			return &instanceTrackingExecutor{instance: factoryCalls, calls: &instances, outcome: outcome}
		}
		registry, err := NewProviderRegistry(provider)
		if err != nil {
			t.Fatal(err)
		}
		runner := Runner{Providers: registry, Policy: RunPolicy{MaxAttemptsPerAction: 2}, Store: NewExecutionStore(t.TempDir()), Now: func() time.Time { return applyTestTime() }}
		first, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
		if err != nil || first.State != ExecutionStateHalted || factoryCalls != 1 {
			t.Fatalf("first=%#v err=%v factory=%d", first, err, factoryCalls)
		}
		resumed, err := runner.Resume(context.Background(), first.ID)
		if err != nil || resumed.State != ExecutionStateDone || factoryCalls != 2 || len(instances) != 2 || instances[0] != 1 || instances[1] != 2 {
			t.Fatalf("resumed=%#v err=%v factory=%d instances=%v", resumed, err, factoryCalls, instances)
		}
	})

	t.Run("missing executor fails before journal", func(t *testing.T) {
		factoryCalls := 0
		provider := testProvider(nil)
		provider.executorFactory = func() Executor {
			factoryCalls++
			return nil
		}
		registry, err := NewProviderRegistry(provider)
		if err != nil {
			t.Fatal(err)
		}
		store := NewExecutionStore(t.TempDir())
		runner := Runner{Providers: registry, Policy: DefaultRunPolicy(), Store: store}
		if _, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation); !errors.Is(err, ErrProviderUnavailable) || factoryCalls != 1 {
			t.Fatalf("Start err=%v factory=%d", err, factoryCalls)
		}
		if summaries, err := store.List(); err != nil || len(summaries) != 0 {
			t.Fatalf("missing executor created journal: summaries=%v err=%v", summaries, err)
		}
	})

	t.Run("missing resume executor fails before journal", func(t *testing.T) {
		factoryCalls := 0
		executor := &scriptedExecutor{scripts: map[string][]scriptedStep{
			"action-1": {{result: ProviderResult{Outcome: OutcomeRateLimited}}},
		}}
		provider := testProvider(nil)
		provider.executorFactory = func() Executor {
			factoryCalls++
			if factoryCalls > 1 {
				return nil
			}
			return executor
		}
		registry, err := NewProviderRegistry(provider)
		if err != nil {
			t.Fatal(err)
		}
		store := NewExecutionStore(t.TempDir())
		runner := Runner{Providers: registry, Policy: RunPolicy{MaxAttemptsPerAction: 2}, Store: store, Now: func() time.Time { return applyTestTime() }}
		first, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
		if err != nil || first.State != ExecutionStateHalted || factoryCalls != 1 || len(executor.calls) != 1 {
			t.Fatalf("first=%#v err=%v factory=%d calls=%v", first, err, factoryCalls, executor.calls)
		}
		before, err := store.Replay(first.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := runner.Resume(context.Background(), first.ID); !errors.Is(err, ErrExecutionIdentityMismatch) || factoryCalls != 2 || len(executor.calls) != 1 {
			t.Fatalf("Resume err=%v factory=%d calls=%v", err, factoryCalls, executor.calls)
		}
		after, err := store.Replay(first.ID)
		if err != nil || after.LastSequence != before.LastSequence {
			t.Fatalf("missing executor journaled work: before=%#v after=%#v err=%v", before, after, err)
		}
	})
}

func TestCancellationAfterAttemptStartPersistsSafeTerminalResult(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.hooks.onAppend = func(event JournalEvent) {
		if event.Kind == JournalAttemptStarted {
			cancel()
		}
	}
	plan := applyTestPlan(testPlatform, []domain.CleanupAction{
		applyTestAction("first", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
		applyTestAction("second", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
	})
	executor := &fakeExecutor{}
	runner := durableTestRunner(t, store, executor, DefaultRunPolicy(), applyTestTime())
	execution, err := runner.Start(ctx, plan, ExecutionModeSimulation)
	if err != nil || len(executor.calls) != 0 || execution.State != ExecutionStateCancelled || execution.Resumability != ResumabilityTerminal || execution.Counts.Cancelled != 2 {
		t.Fatalf("execution=%#v err=%v calls=%v", execution, err, executor.calls)
	}
	if len(execution.Results) != 1 || execution.Results[0].Outcome != OutcomeCancelled || execution.Results[0].Attempt != 1 {
		t.Fatalf("cancelled result=%#v", execution.Results)
	}
	view, err := store.Replay(execution.ID)
	if err != nil || view.State != ExecutionStateCancelled || view.Resumability != ResumabilityTerminal || view.Unresolved != nil || view.Counts.Cancelled != 2 {
		t.Fatalf("replay=%#v err=%v", view, err)
	}
	if _, err := runner.Resume(context.Background(), execution.ID); !errors.Is(err, ErrExecutionTerminal) {
		t.Fatalf("terminal resume err=%v", err)
	}
}

func TestCancellationResultWriteFailureRemainsResolutionRequired(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.hooks.onAppend = func(event JournalEvent) {
		if event.Kind == JournalAttemptStarted {
			cancel()
		}
	}
	store.hooks.beforeAppend = func(event JournalEvent) error {
		if event.Kind == JournalResultRecorded {
			return errors.New("synthetic cancellation result failure")
		}
		return nil
	}
	executor := &fakeExecutor{}
	runner := durableTestRunner(t, store, executor, DefaultRunPolicy(), applyTestTime())
	execution, err := runner.Start(ctx, singleActionPlan(), ExecutionModeSimulation)
	if err == nil || len(executor.calls) != 0 || execution.Resumability != ResumabilityResolution {
		t.Fatalf("execution=%#v err=%v calls=%v", execution, err, executor.calls)
	}
	view, replayErr := store.Replay(execution.ID)
	if replayErr != nil || view.Resumability != ResumabilityResolution || view.Unresolved == nil || view.Unresolved.Attempt != 1 {
		t.Fatalf("replay=%#v err=%v", view, replayErr)
	}
}

type instanceTrackingExecutor struct {
	instance int
	calls    *[]int
	outcome  ActionOutcome
}

func (executor *instanceTrackingExecutor) Execute(_ context.Context, _ domain.CleanupAction) (ProviderResult, error) {
	*executor.calls = append(*executor.calls, executor.instance)
	return ProviderResult{Outcome: executor.outcome}, nil
}
