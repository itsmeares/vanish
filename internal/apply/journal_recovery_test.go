package apply

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

func TestDuplicateDetectionUsesAuthoritativeManifest(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	executor := &fakeExecutor{}
	runner := durableTestRunner(t, store, executor, DefaultRunPolicy(), applyTestTime())
	execution, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := store.executionDir(execution.ID)
	summary, err := loadExecutionSummary(filepath.Join(dir, summaryFileName))
	if err != nil {
		t.Fatal(err)
	}
	summary.Fingerprint = string(bytes.Repeat([]byte{'0'}, 64))
	if err := writeJSONAtomic(filepath.Join(dir, summaryFileName), summary); err != nil {
		t.Fatal(err)
	}
	before := len(executor.calls)
	duplicate, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if !errors.Is(err, ErrExecutionExists) || duplicate.ID != execution.ID || len(executor.calls) != before {
		t.Fatalf("duplicate=%#v err=%v calls=%v", duplicate, err, executor.calls)
	}
}

func TestCorruptManifestFailsClosedBeforeNewExecutorCall(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	executor := &fakeExecutor{}
	runner := durableTestRunner(t, store, executor, DefaultRunPolicy(), applyTestTime())
	execution, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := store.executionDir(execution.ID)
	if err := os.WriteFile(filepath.Join(dir, manifestFileName), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	other := singleActionPlan()
	other.Actions[0].TargetID = "different-target"
	before := len(executor.calls)
	if _, err := runner.Start(context.Background(), other, ExecutionModeSimulation); !errors.Is(err, ErrExecutionCorrupt) || len(executor.calls) != before {
		t.Fatalf("corrupt identity scan err=%v calls=%v", err, executor.calls)
	}
}

func TestDurableResultSurvivesSummaryFailureAndFinalizesWithoutRepeat(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	resultCommitted := false
	failOnce := true
	store.hooks.onAppend = func(event JournalEvent) {
		if event.Kind == JournalResultRecorded {
			resultCommitted = true
		}
	}
	store.hooks.beforeSummary = func() error {
		if resultCommitted && failOnce {
			failOnce = false
			return errors.New("synthetic summary failure")
		}
		return nil
	}
	executor := &fakeExecutor{}
	runner := durableTestRunner(t, store, executor, DefaultRunPolicy(), applyTestTime())
	execution, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if err == nil || len(executor.calls) != 1 {
		t.Fatalf("execution=%#v err=%v calls=%v", execution, err, executor.calls)
	}
	view, replayErr := store.Replay(execution.ID)
	if replayErr != nil || view.Unresolved != nil || !view.NeedsFinalization || view.Counts.Done != 1 {
		t.Fatalf("view=%#v err=%v", view, replayErr)
	}
	resumed, err := runner.Resume(context.Background(), execution.ID)
	if err != nil || resumed.State != ExecutionStateDone || len(executor.calls) != 1 {
		t.Fatalf("resumed=%#v err=%v calls=%v", resumed, err, executor.calls)
	}
}

func TestAttemptStartSummaryFailureRequiresResolutionWithoutExecutor(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	attemptCommitted := false
	store.hooks.onAppend = func(event JournalEvent) {
		if event.Kind == JournalAttemptStarted {
			attemptCommitted = true
		}
	}
	store.hooks.beforeSummary = func() error {
		if attemptCommitted {
			return errors.New("synthetic attempt summary failure")
		}
		return nil
	}
	executor := &fakeExecutor{}
	runner := durableTestRunner(t, store, executor, DefaultRunPolicy(), applyTestTime())
	execution, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if err == nil || len(executor.calls) != 0 || execution.Resumability != ResumabilityResolution {
		t.Fatalf("execution=%#v err=%v calls=%v", execution, err, executor.calls)
	}
	view, replayErr := store.Replay(execution.ID)
	if replayErr != nil || view.Unresolved == nil || view.Unresolved.ActionID != "action-1" {
		t.Fatalf("view=%#v err=%v", view, replayErr)
	}
}

func TestCancelledResultRecoveryFinalizesWithoutCallingNextAction(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	resultCommitted := false
	failOnce := true
	store.hooks.onAppend = func(event JournalEvent) {
		if event.Kind == JournalResultRecorded {
			resultCommitted = true
		}
	}
	store.hooks.beforeSummary = func() error {
		if resultCommitted && failOnce {
			failOnce = false
			return errors.New("synthetic post-result crash")
		}
		return nil
	}
	plan := applyTestPlan(testPlatform, []domain.CleanupAction{
		applyTestAction("first", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
		applyTestAction("second", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
	})
	executor := &fakeExecutor{results: map[string]ProviderResult{"first": {Outcome: OutcomeCancelled}}}
	runner := durableTestRunner(t, store, executor, DefaultRunPolicy(), applyTestTime())
	execution, err := runner.Start(context.Background(), plan, ExecutionModeSimulation)
	if err == nil || len(executor.calls) != 1 {
		t.Fatalf("execution=%#v err=%v calls=%v", execution, err, executor.calls)
	}
	resumed, err := runner.Resume(context.Background(), execution.ID)
	if err != nil || resumed.State != ExecutionStateCancelled || resumed.Counts.Cancelled != 2 || len(executor.calls) != 1 {
		t.Fatalf("resumed=%#v err=%v calls=%v", resumed, err, executor.calls)
	}
}

func TestFinalFailureRecoveryHonorsStopPolicyBeforeNextAction(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	resultCommitted := false
	failOnce := true
	store.hooks.onAppend = func(event JournalEvent) {
		if event.Kind == JournalResultRecorded {
			resultCommitted = true
		}
	}
	store.hooks.beforeSummary = func() error {
		if resultCommitted && failOnce {
			failOnce = false
			return errors.New("synthetic post-failure crash")
		}
		return nil
	}
	plan := applyTestPlan(testPlatform, []domain.CleanupAction{
		applyTestAction("first", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
		applyTestAction("second", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
	})
	executor := &fakeExecutor{results: map[string]ProviderResult{"first": {Outcome: OutcomePermanentFailure}}}
	policy := RunPolicy{MaxAttemptsPerAction: 1, StopAfterFinalFailure: true}
	runner := durableTestRunner(t, store, executor, policy, applyTestTime())
	execution, err := runner.Start(context.Background(), plan, ExecutionModeSimulation)
	if err == nil || len(executor.calls) != 1 {
		t.Fatalf("execution=%#v err=%v calls=%v", execution, err, executor.calls)
	}
	resumed, err := runner.Resume(context.Background(), execution.ID)
	if err != nil || resumed.State != ExecutionStateFailed || resumed.Counts.Pending != 1 || len(executor.calls) != 1 {
		t.Fatalf("resumed=%#v err=%v calls=%v", resumed, err, executor.calls)
	}
}

func TestStoppedExecutionCanResumeUntouchedActions(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	plan := applyTestPlan(testPlatform, []domain.CleanupAction{
		applyTestAction("first", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
		applyTestAction("second", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
	})
	executor := &fakeExecutor{results: map[string]ProviderResult{"first": {Outcome: OutcomeStopped}}}
	runner := durableTestRunner(t, store, executor, DefaultRunPolicy(), applyTestTime())
	first, err := runner.Start(context.Background(), plan, ExecutionModeSimulation)
	if err != nil || first.State != ExecutionStateStopped || first.Resumability != ResumabilityResumable || len(executor.calls) != 1 {
		t.Fatalf("first=%#v err=%v calls=%v", first, err, executor.calls)
	}
	resumed, err := runner.Resume(context.Background(), first.ID)
	if err != nil || resumed.State != ExecutionStateStopped || resumed.Resumability != ResumabilityTerminal || len(executor.calls) != 2 || executor.calls[1] != "second" {
		t.Fatalf("resumed=%#v err=%v calls=%v", resumed, err, executor.calls)
	}
}

func TestAuthenticationResumeRechecksProviderReadiness(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	executor := &scriptedExecutor{scripts: map[string][]scriptedStep{
		"action-1": {
			{result: ProviderResult{Outcome: OutcomeAuthenticationRequired}},
			{result: ProviderResult{Outcome: OutcomeSucceeded}},
		},
	}}
	provider := testProvider(executor)
	provider.prerequisites = func(_ domain.CleanupPlan, state RuntimeState) []Prerequisite {
		if state.Connected(testPlatform) {
			return nil
		}
		return []Prerequisite{{Code: "connection_required", Message: "Connect provider.", Blocking: true}}
	}
	registry, err := NewProviderRegistry(provider)
	if err != nil {
		t.Fatal(err)
	}
	connected := NewRuntimeState(map[domain.PlatformName]ConnectionState{testPlatform: {Connected: true}})
	runner := Runner{Providers: registry, State: connected, Policy: RunPolicy{MaxAttemptsPerAction: 2}, Store: store, Now: func() time.Time { return applyTestTime() }}
	first, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if err != nil || first.Resumability != ResumabilityWaitingProvider {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	runner.State = RuntimeState{}
	if _, err := runner.Resume(context.Background(), first.ID); !errors.Is(err, ErrExecutionNotReady) || len(executor.calls) != 1 {
		t.Fatalf("disconnected resume err=%v calls=%v", err, executor.calls)
	}
	runner.State = connected
	resumed, err := runner.Resume(context.Background(), first.ID)
	if err != nil || resumed.State != ExecutionStateDone || len(executor.calls) != 2 {
		t.Fatalf("resumed=%#v err=%v calls=%v", resumed, err, executor.calls)
	}
}

func TestRetryGateAppliesAfterAttemptBudgetIsExhausted(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	now := applyTestTime()
	plan := applyTestPlan(testPlatform, []domain.CleanupAction{
		applyTestAction("first", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
		applyTestAction("second", testPlatform, domain.ActionUnlike, domain.ActionStatusPending),
	})
	executor := &scriptedExecutor{scripts: map[string][]scriptedStep{
		"first":  {{result: ProviderResult{Outcome: OutcomeRateLimited, RetryAfter: time.Minute}}},
		"second": {{result: ProviderResult{Outcome: OutcomeSucceeded}}},
	}}
	runner := durableTestRunner(t, store, executor, DefaultRunPolicy(), now)
	runner.Now = func() time.Time { return now }
	first, err := runner.Start(context.Background(), plan, ExecutionModeSimulation)
	if err != nil || first.Resumability != ResumabilityWaitingRetry {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	if _, err := runner.Resume(context.Background(), first.ID); !errors.Is(err, ErrExecutionNotReady) || len(executor.calls) != 1 {
		t.Fatalf("early resume err=%v calls=%v", err, executor.calls)
	}
	now = now.Add(time.Minute)
	resumed, err := runner.Resume(context.Background(), first.ID)
	if err != nil || resumed.State != ExecutionStateFailed || len(executor.calls) != 2 || executor.calls[1].actionID != "second" {
		t.Fatalf("resumed=%#v err=%v calls=%v", resumed, err, executor.calls)
	}
}

func TestConcurrentStartsCreateOneExecutionIdentity(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	executor := &blockingExecutor{entered: make(chan struct{}), release: make(chan struct{})}
	runner := durableTestRunner(t, store, executor, DefaultRunPolicy(), applyTestTime())
	type result struct {
		execution Execution
		err       error
	}
	firstResult := make(chan result, 1)
	go func() {
		execution, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
		firstResult <- result{execution: execution, err: err}
	}()
	select {
	case <-executor.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first executor did not start")
	}
	second, secondErr := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	close(executor.release)
	first := <-firstResult
	if first.err != nil || !errors.Is(secondErr, ErrExecutionExists) || second.ID != first.execution.ID || executor.CallCount() != 1 {
		t.Fatalf("first=%#v firstErr=%v second=%#v secondErr=%v calls=%d", first.execution, first.err, second, secondErr, executor.CallCount())
	}
}

func TestReplayRejectsInvalidEventSequencesAndPayloads(t *testing.T) {
	store, writer, manifest := startedTestExecution(t)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	start := testJournalEvent(manifest, 1, JournalExecutionStarted, 0)
	attempt := testJournalEvent(manifest, 2, JournalAttemptStarted, time.Second)
	attempt.ActionID = manifest.Plan.Actions[0].ID
	attempt.ActionType = manifest.Plan.Actions[0].Type
	attempt.Platform = manifest.Plan.Actions[0].Platform
	attempt.Attempt = 1
	result := testJournalEvent(manifest, 3, JournalResultRecorded, 2*time.Second)
	result.ActionID = attempt.ActionID
	result.ActionType = attempt.ActionType
	result.Platform = attempt.Platform
	result.Attempt = 1
	result.Outcome = OutcomeSucceeded
	result.Status = domain.ActionStatusDone
	stoppedResult := result
	stoppedResult.Outcome = OutcomeStopped
	stoppedResult.Status = domain.ActionStatusStopped
	stoppedTerminal := testJournalEvent(manifest, 4, JournalExecutionStopped, 3*time.Second)

	tests := map[string][]JournalEvent{
		"sequence gap":                {start, withSequence(attempt, 3)},
		"result without attempt":      {start, withSequence(result, 2)},
		"route mismatch":              {start, withPlatform(attempt, "other")},
		"unknown outcome":             {start, attempt, withOutcome(result, "invented")},
		"duplicate result":            {start, attempt, result, withSequenceAndTime(result, 4, 3*time.Second)},
		"event after terminal":        {start, testJournalEvent(manifest, 2, JournalExecutionAbandoned, time.Second), testJournalEvent(manifest, 3, JournalExecutionResumed, 2*time.Second)},
		"unknown event kind":          {start, testJournalEvent(manifest, 2, "invented", time.Second)},
		"failed after success":        {start, attempt, result, testJournalEvent(manifest, 4, JournalExecutionFailed, 3*time.Second)},
		"completed with pending work": {start, testJournalEvent(manifest, 2, JournalExecutionCompleted, time.Second)},
		"event after terminal stop":   {start, attempt, stoppedResult, stoppedTerminal, testJournalEvent(manifest, 5, JournalExecutionResumed, 4*time.Second)},
	}
	for name, events := range tests {
		t.Run(name, func(t *testing.T) {
			rewriteJournal(t, store, manifest.ExecutionID, events)
			if _, err := store.Replay(manifest.ExecutionID); !errors.Is(err, ErrExecutionCorrupt) {
				t.Fatalf("Replay error=%v", err)
			}
		})
	}
}

func TestStoreRejectsSymlinkAndTraversalDeletion(t *testing.T) {
	t.Run("root symlink", func(t *testing.T) {
		workspace := t.TempDir()
		target := t.TempDir()
		store := NewExecutionStore(workspace)
		if err := os.Symlink(target, store.root); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if _, err := store.List(); !errors.Is(err, ErrExecutionCorrupt) {
			t.Fatalf("List error=%v", err)
		}
		runner := durableTestRunner(t, store, &fakeExecutor{}, DefaultRunPolicy(), applyTestTime())
		if _, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation); !errors.Is(err, ErrExecutionCorrupt) {
			t.Fatalf("Start error=%v", err)
		}
	})

	t.Run("session symlink deletion", func(t *testing.T) {
		workspace := t.TempDir()
		outside := t.TempDir()
		sentinel := filepath.Join(outside, "sentinel")
		if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
		store := NewExecutionStore(workspace)
		if err := store.ensureRoots(); err != nil {
			t.Fatal(err)
		}
		key := string(bytes.Repeat([]byte{'a'}, 64))
		if err := os.Symlink(outside, filepath.Join(store.root, key)); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if err := store.Delete(ExecutionSummary{Resumability: ResumabilityCorrupt, storeKey: key}); !errors.Is(err, ErrExecutionCorrupt) {
			t.Fatalf("Delete error=%v", err)
		}
		if _, err := os.Stat(sentinel); err != nil {
			t.Fatalf("outside data changed: %v", err)
		}
	})

	t.Run("invalid store key", func(t *testing.T) {
		store := NewExecutionStore(t.TempDir())
		if err := store.Delete(ExecutionSummary{Resumability: ResumabilityCorrupt, storeKey: "../outside"}); !errors.Is(err, ErrExecutionCorrupt) {
			t.Fatalf("Delete error=%v", err)
		}
		path, err := store.executionDir("../../outside")
		if err != nil || filepath.Dir(path) != store.root {
			t.Fatalf("hashed path escaped root: path=%q err=%v", path, err)
		}
	})
}

func TestDurableTerminalOutcomesReplay(t *testing.T) {
	tests := []struct {
		name    string
		outcome ActionOutcome
		state   ExecutionState
	}{
		{name: "permanent failure", outcome: OutcomePermanentFailure, state: ExecutionStateFailed},
		{name: "stopped", outcome: OutcomeStopped, state: ExecutionStateStopped},
		{name: "cancelled", outcome: OutcomeCancelled, state: ExecutionStateCancelled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := NewExecutionStore(t.TempDir())
			executor := &fakeExecutor{results: map[string]ProviderResult{"action-1": {Outcome: test.outcome}}}
			runner := durableTestRunner(t, store, executor, RunPolicy{MaxAttemptsPerAction: 1, StopAfterFinalFailure: true}, applyTestTime())
			execution, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
			if err != nil {
				t.Fatal(err)
			}
			view, replayErr := store.Replay(execution.ID)
			if replayErr != nil || view.State != test.state || view.Resumability != ResumabilityTerminal {
				t.Fatalf("execution=%#v view=%#v err=%v", execution, view, replayErr)
			}
		})
	}
}

func TestReplayHandles100KActionManifest(t *testing.T) {
	if testing.Short() {
		t.Skip("large manifest")
	}
	store := NewExecutionStore(t.TempDir())
	id, err := NewExecutionID()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := newExecutionManifest(id, largeExecutionPlan(100_000), ExecutionModeSimulation, "test-executor", DefaultRunPolicy(), applyTestTime())
	if err != nil {
		t.Fatal(err)
	}
	writer, _, err := store.Create(manifest, manifest.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	view, err := store.Replay(id)
	if err != nil || len(view.Plan.Actions) != 100_000 || view.Counts.Pending != 100_000 || view.NextActionID != "action-000000" {
		t.Fatalf("actions=%d counts=%#v next=%q err=%v", len(view.Plan.Actions), view.Counts, view.NextActionID, err)
	}
}

func BenchmarkReplayManifestSizes(b *testing.B) {
	for _, size := range []int{1_000, 10_000, 100_000} {
		b.Run(fmt.Sprintf("actions_%d", size), func(b *testing.B) {
			store := NewExecutionStore(b.TempDir())
			id, _ := NewExecutionID()
			manifest, err := newExecutionManifest(id, largeExecutionPlan(size), ExecutionModeSimulation, "test-executor", DefaultRunPolicy(), applyTestTime())
			if err != nil {
				b.Fatal(err)
			}
			writer, _, err := store.Create(manifest, manifest.CreatedAt)
			if err != nil {
				b.Fatal(err)
			}
			_ = writer.Close()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				if _, err := store.Replay(id); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkJournalAppendByHistorySize(b *testing.B) {
	for _, historyBytes := range []int64{0, 64 << 20} {
		b.Run(fmt.Sprintf("history_%d", historyBytes), func(b *testing.B) {
			store, writer, manifest := startedBenchmarkExecution(b)
			dir, _ := store.executionDir(manifest.ExecutionID)
			if historyBytes > 0 {
				journal, err := os.OpenFile(filepath.Join(dir, journalFileName), os.O_WRONLY, 0o600)
				if err != nil {
					b.Fatal(err)
				}
				if err := journal.Truncate(historyBytes); err != nil {
					journal.Close()
					b.Fatal(err)
				}
				if err := journal.Sync(); err != nil {
					journal.Close()
					b.Fatal(err)
				}
				if err := journal.Close(); err != nil {
					b.Fatal(err)
				}
			}
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				summary := initialExecutionSummary(manifest, applyTestTime())
				if _, err := writer.Append(JournalEvent{Timestamp: applyTestTime().Add(time.Duration(index+1) * time.Second), Kind: JournalExecutionResumed}, summary); err != nil {
					b.Fatal(err)
				}
			}
			_ = writer.Close()
		})
	}
}

type blockingExecutor struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	calls   int
}

func (executor *blockingExecutor) Execute(_ context.Context, _ ActionRequest) (ProviderResult, error) {
	executor.mu.Lock()
	executor.calls++
	executor.mu.Unlock()
	executor.once.Do(func() { close(executor.entered) })
	<-executor.release
	return ProviderResult{Outcome: OutcomeSucceeded}, nil
}

func (executor *blockingExecutor) CallCount() int {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	return executor.calls
}

func testJournalEvent(manifest ExecutionManifest, sequence int64, kind JournalEventKind, offset time.Duration) JournalEvent {
	return JournalEvent{ExecutionID: manifest.ExecutionID, Fingerprint: manifest.Fingerprint, Sequence: sequence, Timestamp: manifest.CreatedAt.Add(offset), Kind: kind}
}

func withSequence(event JournalEvent, sequence int64) JournalEvent {
	event.Sequence = sequence
	return event
}

func withPlatform(event JournalEvent, platform domain.PlatformName) JournalEvent {
	event.Platform = platform
	return event
}

func withOutcome(event JournalEvent, outcome ActionOutcome) JournalEvent {
	event.Outcome = outcome
	return event
}

func withSequenceAndTime(event JournalEvent, sequence int64, offset time.Duration) JournalEvent {
	event.Sequence = sequence
	event.Timestamp = event.Timestamp.Add(offset - 2*time.Second)
	return event
}

func rewriteJournal(t *testing.T, store *ExecutionStore, id ExecutionID, events []JournalEvent) {
	t.Helper()
	var content bytes.Buffer
	encoder := json.NewEncoder(&content)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			t.Fatal(err)
		}
	}
	dir, _ := store.executionDir(id)
	if err := os.WriteFile(filepath.Join(dir, journalFileName), content.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func largeExecutionPlan(size int) domain.CleanupPlan {
	actions := make([]domain.CleanupAction, size)
	for index := range actions {
		id := fmt.Sprintf("action-%06d", index)
		actions[index] = domain.CleanupAction{ID: id, Platform: testPlatform, Type: domain.ActionUnlike, TargetID: "target-" + id, SourceActivityItemID: "item-" + id, Status: domain.ActionStatusPending, CreatedAt: applyTestTime()}
	}
	return domain.NewCleanupPlan("large-plan", testPlatform, "large-source", applyTestTime(), actions)
}

func startedBenchmarkExecution(b *testing.B) (*ExecutionStore, *ExecutionWriter, ExecutionManifest) {
	b.Helper()
	store := NewExecutionStore(b.TempDir())
	id, err := NewExecutionID()
	if err != nil {
		b.Fatal(err)
	}
	manifest, err := newExecutionManifest(id, singleActionPlan(), ExecutionModeSimulation, "test-executor", DefaultRunPolicy(), applyTestTime())
	if err != nil {
		b.Fatal(err)
	}
	writer, _, err := store.Create(manifest, manifest.CreatedAt)
	if err != nil {
		b.Fatal(err)
	}
	return store, writer, manifest
}
