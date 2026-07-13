package apply

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/itsmeares/vanish/internal/localdata"
	"github.com/itsmeares/vanish/internal/workspace"
)

func TestDeletePreservesDuplicateExecutionGuard(t *testing.T) {
	tests := []struct {
		name     string
		executor Executor
		policy   RunPolicy
		abandon  bool
	}{
		{name: "completed", executor: &fakeExecutor{}, policy: DefaultRunPolicy()},
		{
			name: "abandoned",
			executor: &scriptedExecutor{scripts: map[string][]scriptedStep{
				"action-1": {{result: ProviderResult{Outcome: OutcomeRateLimited}}},
			}},
			policy:  RunPolicy{MaxAttemptsPerAction: 2},
			abandon: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := NewExecutionStore(t.TempDir())
			runner := durableTestRunner(t, store, test.executor, test.policy, applyTestTime())
			first, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
			if err != nil {
				t.Fatal(err)
			}
			if test.abandon {
				if first.State != ExecutionStateHalted {
					t.Fatalf("first state=%s", first.State)
				}
				if _, err := runner.Abandon(first.ID); err != nil {
					t.Fatal(err)
				}
			}
			summaries, err := store.List()
			if err != nil || len(summaries) != 1 || summaries[0].Resumability != ResumabilityTerminal {
				t.Fatalf("summaries=%#v err=%v", summaries, err)
			}
			fingerprint := summaries[0].Fingerprint
			if err := store.Delete(summaries[0]); err != nil {
				t.Fatal(err)
			}
			if summaries, err := store.List(); err != nil || len(summaries) != 0 {
				t.Fatalf("deleted summaries=%#v err=%v", summaries, err)
			}
			guard, err := os.ReadFile(store.identityGuardPath(fingerprint))
			if err != nil || string(guard) != identityGuardMarker {
				t.Fatalf("identity guard=%q err=%v", guard, err)
			}
			callsBefore := executorCallCount(test.executor)
			if _, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation); !errors.Is(err, ErrExecutionExists) {
				t.Fatalf("second Start error=%v", err)
			}
			if callsAfter := executorCallCount(test.executor); callsAfter != callsBefore {
				t.Fatalf("executor calls before=%d after=%d", callsBefore, callsAfter)
			}
		})
	}
}

func TestWorkspaceWipeCoordinatesWithDurableWriters(t *testing.T) {
	root := t.TempDir()
	w, err := workspace.Open(filepath.Join(root, "app"))
	if err != nil {
		t.Fatal(err)
	}
	store := NewExecutionStore(w.Dir())
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
	if err := w.Wipe(); !errors.Is(err, workspace.ErrWorkspaceActive) {
		_ = writer.Close()
		t.Fatalf("active wipe error=%v", err)
	}
	if _, err := store.Replay(id); err != nil {
		_ = writer.Close()
		t.Fatalf("failed wipe damaged execution: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	wipeLease, err := localdata.TryWipe(w.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.OpenWriter(id); !errors.Is(err, ErrExecutionLocked) {
		_ = wipeLease.Close()
		t.Fatalf("writer opened during wipe lease: %v", err)
	}
	if err := wipeLease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Wipe(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.Root()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("execution store survived wipe: %v", err)
	}
}

func TestJournalClockRollbackRemainsValid(t *testing.T) {
	t.Run("append clamps logical time before executor", func(t *testing.T) {
		store := NewExecutionStore(t.TempDir())
		executor := &fakeExecutor{}
		now := applyTestTime()
		nowCalls := 0
		runner := durableTestRunner(t, store, executor, DefaultRunPolicy(), now)
		runner.Now = func() time.Time {
			nowCalls++
			if nowCalls == 1 {
				return now
			}
			return now.Add(-time.Hour)
		}
		execution, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
		if err != nil || execution.State != ExecutionStateDone || len(executor.calls) != 1 {
			t.Fatalf("execution=%#v err=%v calls=%v", execution, err, executor.calls)
		}
		view, err := store.Replay(execution.ID)
		if err != nil || view.Resumability != ResumabilityTerminal || view.UpdatedAt.Before(now) {
			t.Fatalf("rollback replay=%#v err=%v", view, err)
		}
	})

	t.Run("replay accepts decreasing wall clock", func(t *testing.T) {
		store := NewExecutionStore(t.TempDir())
		runner := durableTestRunner(t, store, &fakeExecutor{}, DefaultRunPolicy(), applyTestTime())
		execution, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
		if err != nil {
			t.Fatal(err)
		}
		dir, err := store.executionDir(execution.ID)
		if err != nil {
			t.Fatal(err)
		}
		journalPath := filepath.Join(dir, journalFileName)
		data, err := os.ReadFile(journalPath)
		if err != nil {
			t.Fatal(err)
		}
		lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
		var rewritten bytes.Buffer
		for index, line := range lines {
			var event JournalEvent
			if err := json.Unmarshal(line, &event); err != nil {
				t.Fatal(err)
			}
			event.Timestamp = applyTestTime().Add(-time.Duration(index) * time.Minute)
			encoded, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			rewritten.Write(encoded)
			rewritten.WriteByte('\n')
		}
		if err := os.WriteFile(journalPath, rewritten.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		view, err := store.Replay(execution.ID)
		if err != nil || view.State != ExecutionStateDone || view.LastSequence != int64(len(lines)) || !view.UpdatedAt.Equal(applyTestTime()) {
			t.Fatalf("decreasing replay=%#v err=%v", view, err)
		}
	})

	t.Run("rollback cannot bypass retry deadline", func(t *testing.T) {
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
		if err != nil || first.State != ExecutionStateHalted || len(executor.calls) != 1 {
			t.Fatalf("first=%#v err=%v calls=%v", first, err, executor.calls)
		}
		now = now.Add(-time.Hour)
		if _, err := runner.Resume(context.Background(), first.ID); !errors.Is(err, ErrExecutionNotReady) || len(executor.calls) != 1 {
			t.Fatalf("rollback Resume err=%v calls=%v", err, executor.calls)
		}
		if view, err := store.Replay(first.ID); err != nil || view.Resumability != ResumabilityWaitingRetry {
			t.Fatalf("rollback replay=%#v err=%v", view, err)
		}
		now = applyTestTime().Add(time.Minute)
		if resumed, err := runner.Resume(context.Background(), first.ID); err != nil || resumed.State != ExecutionStateDone || len(executor.calls) != 2 {
			t.Fatalf("ready Resume=%#v err=%v calls=%v", resumed, err, executor.calls)
		}
	})
}

func TestExecutionDirectorySyncFailureStopsBeforeExecutor(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	store.hooks.beforeDirectorySync = func(path string) error {
		if path == store.root {
			return errors.New("synthetic execution directory sync failure")
		}
		return nil
	}
	executor := &fakeExecutor{}
	runner := durableTestRunner(t, store, executor, DefaultRunPolicy(), applyTestTime())
	if _, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation); err == nil || len(executor.calls) != 0 {
		t.Fatalf("Start err=%v calls=%v", err, executor.calls)
	}
	entries, err := os.ReadDir(store.root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() && validStoreKey(entry.Name()) {
			t.Fatalf("failed session directory survived: %s", entry.Name())
		}
	}
	store.hooks.beforeDirectorySync = nil
	if execution, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation); err != nil || execution.State != ExecutionStateDone || len(executor.calls) != 1 {
		t.Fatalf("retry execution=%#v err=%v calls=%v", execution, err, executor.calls)
	}
}

func TestApplyExistingExecutionPreservesLockedClassification(t *testing.T) {
	store := NewExecutionStore(t.TempDir())
	executor := &fakeExecutor{}
	runner := durableTestRunner(t, store, executor, DefaultRunPolicy(), applyTestTime())
	id, err := NewExecutionID()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := newExecutionManifest(id, singleActionPlan(), ExecutionModeSimulation, "test-simulation", DefaultRunPolicy(), applyTestTime())
	if err != nil {
		t.Fatal(err)
	}
	writer, _, err := store.Create(manifest, manifest.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	execution, err := runner.Start(context.Background(), singleActionPlan(), ExecutionModeSimulation)
	if !errors.Is(err, ErrExecutionExists) || execution.ID != id || execution.Resumability != ResumabilityLocked || execution.BlockReason != "Execution is active in another Vanish process." || len(executor.calls) != 0 {
		t.Fatalf("execution=%#v err=%v calls=%v", execution, err, executor.calls)
	}
}

func executorCallCount(executor Executor) int {
	switch value := executor.(type) {
	case *fakeExecutor:
		return len(value.calls)
	case *scriptedExecutor:
		return len(value.calls)
	default:
		return -1
	}
}
