package apply

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

type runtimeScenarioSpec struct {
	Plan                domain.CleanupPlan
	Policy              RunPolicy
	ExecutorSteps       map[string][]scenarioExecutorStep
	ReconciliationSteps []scenarioReconciliationStep
}

type scenarioExecutorStep struct {
	Result       ProviderResult
	Err          error
	BeforeReturn func()
}

type scenarioReconciliationStep struct {
	Outcome ReconciliationOutcome
	Err     error
}

type scenarioObservationKind string

const (
	scenarioObservedRestart          scenarioObservationKind = "restart"
	scenarioObservedJournalAppend    scenarioObservationKind = "journal_append"
	scenarioObservedExecutorEntry    scenarioObservationKind = "executor_entry"
	scenarioObservedExecutorReturn   scenarioObservationKind = "executor_return"
	scenarioObservedReconcilerEntry  scenarioObservationKind = "reconciler_entry"
	scenarioObservedReconcilerReturn scenarioObservationKind = "reconciler_return"
	scenarioObservedFault            scenarioObservationKind = "fault"
)

type scenarioObservation struct {
	Kind                  scenarioObservationKind
	JournalKind           JournalEventKind
	Sequence              int64
	ActionID              string
	Attempt               int
	ReconciliationAttempt int
	Outcome               ActionOutcome
	ReconciliationOutcome ReconciliationOutcome
	IdempotencyKey        ActionIdempotencyKey
	Fault                 scenarioFaultPoint
}

type scenarioFaultPoint string

const (
	scenarioFaultAfterAttemptStarted        scenarioFaultPoint = "after_attempt_started"
	scenarioFaultAfterExecutorReturn        scenarioFaultPoint = "after_executor_return"
	scenarioFaultAfterReconciliationStarted scenarioFaultPoint = "after_reconciliation_started"
	scenarioFaultBeforeReconciliationResult scenarioFaultPoint = "before_reconciliation_result"
	scenarioFaultAfterReconciliationResult  scenarioFaultPoint = "after_reconciliation_result"
	scenarioFaultBeforeTerminalEvent        scenarioFaultPoint = "before_terminal_event"
	scenarioFaultAfterTerminalEvent         scenarioFaultPoint = "after_terminal_event"
)

type armedScenarioFault struct {
	occurrence int
	seen       int
}

type runtimeScenarioState struct {
	executorSteps       map[string][]scenarioExecutorStep
	executorPositions   map[string]int
	reconciliationSteps []scenarioReconciliationStep
	reconciliationPos   int
	currentAttempts     map[string]int
	currentReconciles   map[string]int
	observations        []scenarioObservation
	faults              map[scenarioFaultPoint]*armedScenarioFault
	lastAppended        JournalEvent
	executorReturned    bool
	providerReady       bool
	reconcilerAvailable bool
}

type runtimeScenario struct {
	t         *testing.T
	workspace string
	spec      runtimeScenarioSpec
	clock     *scenarioClock
	state     *runtimeScenarioState
	store     *ExecutionStore
	runner    Runner
}

type scenarioClock struct {
	now time.Time
}

func newRuntimeScenario(t *testing.T, spec runtimeScenarioSpec) *runtimeScenario {
	t.Helper()
	if err := spec.Plan.Validate(); err != nil {
		t.Fatalf("scenario plan: %v", err)
	}
	state := &runtimeScenarioState{
		executorSteps:       cloneScenarioExecutorSteps(spec.ExecutorSteps),
		executorPositions:   make(map[string]int),
		reconciliationSteps: slices.Clone(spec.ReconciliationSteps),
		currentAttempts:     make(map[string]int),
		currentReconciles:   make(map[string]int),
		faults:              make(map[scenarioFaultPoint]*armedScenarioFault),
		providerReady:       true,
		reconcilerAvailable: true,
	}
	scenario := &runtimeScenario{
		t:         t,
		workspace: t.TempDir(),
		spec:      spec,
		clock:     &scenarioClock{now: applyTestTime()},
		state:     state,
	}
	scenario.restart(false)
	return scenario
}

func cloneScenarioExecutorSteps(input map[string][]scenarioExecutorStep) map[string][]scenarioExecutorStep {
	cloned := make(map[string][]scenarioExecutorStep, len(input))
	for actionID, steps := range input {
		cloned[actionID] = slices.Clone(steps)
	}
	return cloned
}

func (scenario *runtimeScenario) Restart() {
	scenario.t.Helper()
	scenario.restart(true)
}

func (scenario *runtimeScenario) restart(observe bool) {
	scenario.t.Helper()
	scenario.store = NewExecutionStore(scenario.workspace)
	if scenario.store == nil {
		scenario.t.Fatal("scenario store is unavailable")
	}
	scenario.installStoreHooks()
	provider := scenarioProvider{state: scenario.state}
	registry, err := NewProviderRegistry(provider)
	if err != nil {
		scenario.t.Fatalf("scenario registry: %v", err)
	}
	scenario.runner = Runner{
		Providers: registry,
		Policy:    scenario.spec.Policy,
		Store:     scenario.store,
		Now:       scenario.clock.Now,
	}
	if observe {
		scenario.state.observe(scenarioObservation{Kind: scenarioObservedRestart})
	}
}

func (scenario *runtimeScenario) Start(ctx context.Context) (Execution, error) {
	scenario.t.Helper()
	return scenario.runner.Start(ctx, scenario.spec.Plan, ExecutionModeSimulation)
}

func (scenario *runtimeScenario) Resume(ctx context.Context, id ExecutionID) (Execution, error) {
	scenario.t.Helper()
	return scenario.runner.Resume(ctx, id)
}

func (scenario *runtimeScenario) Reconcile(ctx context.Context, id ExecutionID) (Execution, error) {
	scenario.t.Helper()
	return scenario.runner.Reconcile(ctx, id)
}

func (scenario *runtimeScenario) Replay(id ExecutionID) (ExecutionView, error) {
	scenario.t.Helper()
	return scenario.store.Replay(id)
}

func (scenario *runtimeScenario) Advance(duration time.Duration) {
	scenario.t.Helper()
	scenario.clock.Advance(duration)
}

func (scenario *runtimeScenario) SetProviderReady(ready bool) {
	scenario.t.Helper()
	scenario.state.providerReady = ready
}

func (scenario *runtimeScenario) SetReconcilerAvailable(available bool) {
	scenario.t.Helper()
	scenario.state.reconcilerAvailable = available
}

func (scenario *runtimeScenario) ArmFault(point scenarioFaultPoint, occurrence int) {
	scenario.t.Helper()
	if occurrence <= 0 {
		scenario.t.Fatalf("fault %q occurrence must be positive", point)
	}
	scenario.state.faults[point] = &armedScenarioFault{occurrence: occurrence}
}

func (scenario *runtimeScenario) Observations() []scenarioObservation {
	scenario.t.Helper()
	return slices.Clone(scenario.state.observations)
}

func (scenario *runtimeScenario) installStoreHooks() {
	scenario.store.hooks.beforeAppend = func(event JournalEvent) error {
		switch {
		case event.Kind == JournalResultRecorded && scenario.state.executorReturned:
			scenario.state.executorReturned = false
			return scenario.state.trip(scenarioFaultAfterExecutorReturn)
		case event.Kind == JournalReconciliationResult:
			return scenario.state.trip(scenarioFaultBeforeReconciliationResult)
		case scenarioTerminalJournalKind(event.Kind):
			return scenario.state.trip(scenarioFaultBeforeTerminalEvent)
		default:
			return nil
		}
	}
	scenario.store.hooks.onAppend = func(event JournalEvent) {
		scenario.state.lastAppended = event
		if event.Kind == JournalAttemptStarted {
			scenario.state.currentAttempts[event.ActionID] = event.Attempt
		}
		if event.Kind == JournalReconciliationStarted {
			scenario.state.currentReconciles[event.ActionID] = event.ReconciliationAttempt
		}
		scenario.state.observe(scenarioObservation{
			Kind:                  scenarioObservedJournalAppend,
			JournalKind:           event.Kind,
			Sequence:              event.Sequence,
			ActionID:              event.ActionID,
			Attempt:               event.Attempt,
			ReconciliationAttempt: event.ReconciliationAttempt,
			Outcome:               event.Outcome,
			ReconciliationOutcome: event.ReconciliationOutcome,
		})
	}
	scenario.store.hooks.beforeSummary = func() error {
		event := scenario.state.lastAppended
		scenario.state.lastAppended = JournalEvent{}
		switch {
		case event.Kind == JournalAttemptStarted:
			return scenario.state.trip(scenarioFaultAfterAttemptStarted)
		case event.Kind == JournalReconciliationStarted:
			return scenario.state.trip(scenarioFaultAfterReconciliationStarted)
		case event.Kind == JournalReconciliationResult:
			return scenario.state.trip(scenarioFaultAfterReconciliationResult)
		case scenarioTerminalJournalKind(event.Kind):
			return scenario.state.trip(scenarioFaultAfterTerminalEvent)
		default:
			return nil
		}
	}
}

func (state *runtimeScenarioState) observe(observation scenarioObservation) {
	state.observations = append(state.observations, observation)
}

func (state *runtimeScenarioState) trip(point scenarioFaultPoint) error {
	fault, ok := state.faults[point]
	if !ok {
		return nil
	}
	fault.seen++
	if fault.seen != fault.occurrence {
		return nil
	}
	delete(state.faults, point)
	state.observe(scenarioObservation{Kind: scenarioObservedFault, Fault: point})
	return fmt.Errorf("runtime scenario fault: %s", point)
}

func scenarioTerminalJournalKind(kind JournalEventKind) bool {
	switch kind {
	case JournalExecutionCompleted, JournalExecutionFailed, JournalExecutionCancelled, JournalExecutionAbandoned:
		return true
	default:
		return false
	}
}

func (clock *scenarioClock) Now() time.Time {
	value := clock.now
	clock.now = clock.now.Add(time.Millisecond)
	return value
}

func (clock *scenarioClock) Advance(duration time.Duration) {
	clock.now = clock.now.Add(duration)
}

type scenarioProvider struct {
	state *runtimeScenarioState
}

func (scenarioProvider) Platform() domain.PlatformName { return testPlatform }
func (scenarioProvider) Mode() ExecutionMode           { return ExecutionModeSimulation }
func (scenarioProvider) ExecutorID() ExecutorID        { return "runtime-scenario" }
func (scenarioProvider) Supports(domain.ActionType) bool {
	return true
}
func (provider scenarioProvider) Prerequisites(domain.CleanupPlan, RuntimeState) []Prerequisite {
	if provider.state.providerReady {
		return nil
	}
	return []Prerequisite{{Code: "scenario_provider_unavailable", Message: "Reconnect the account before resuming.", Blocking: true}}
}
func (provider scenarioProvider) Executor() Executor {
	return &scenarioExecutor{state: provider.state}
}
func (provider scenarioProvider) Reconciler() Reconciler {
	if !provider.state.reconcilerAvailable {
		return nil
	}
	return &scenarioReconciler{state: provider.state}
}

type scenarioExecutor struct {
	state *runtimeScenarioState
}

func (executor *scenarioExecutor) Execute(_ context.Context, request ActionRequest) (ProviderResult, error) {
	actionID := request.Action.ID
	attempt := executor.state.currentAttempts[actionID]
	executor.state.observe(scenarioObservation{
		Kind:           scenarioObservedExecutorEntry,
		ActionID:       actionID,
		Attempt:        attempt,
		IdempotencyKey: request.IdempotencyKey,
	})
	position := executor.state.executorPositions[actionID]
	executor.state.executorPositions[actionID] = position + 1
	steps := executor.state.executorSteps[actionID]
	if position >= len(steps) {
		err := errors.New("runtime scenario executor script exhausted")
		executor.state.observe(scenarioObservation{
			Kind:           scenarioObservedExecutorReturn,
			ActionID:       actionID,
			Attempt:        attempt,
			IdempotencyKey: request.IdempotencyKey,
		})
		executor.state.executorReturned = true
		return ProviderResult{}, err
	}
	step := steps[position]
	if step.BeforeReturn != nil {
		step.BeforeReturn()
	}
	executor.state.observe(scenarioObservation{
		Kind:           scenarioObservedExecutorReturn,
		ActionID:       actionID,
		Attempt:        attempt,
		Outcome:        step.Result.Outcome,
		IdempotencyKey: request.IdempotencyKey,
	})
	executor.state.executorReturned = true
	return step.Result, step.Err
}

type scenarioReconciler struct {
	state *runtimeScenarioState
}

func (reconciler *scenarioReconciler) Reconcile(_ context.Context, request ReconciliationRequest) (ReconciliationOutcome, error) {
	reconciliationAttempt := reconciler.state.currentReconciles[request.Action.ID]
	reconciler.state.observe(scenarioObservation{
		Kind:                  scenarioObservedReconcilerEntry,
		ActionID:              request.Action.ID,
		Attempt:               request.Attempt,
		ReconciliationAttempt: reconciliationAttempt,
		IdempotencyKey:        request.IdempotencyKey,
	})
	position := reconciler.state.reconciliationPos
	reconciler.state.reconciliationPos++
	if position >= len(reconciler.state.reconciliationSteps) {
		err := errors.New("runtime scenario reconciliation script exhausted")
		reconciler.state.observe(scenarioObservation{
			Kind:                  scenarioObservedReconcilerReturn,
			ActionID:              request.Action.ID,
			Attempt:               request.Attempt,
			ReconciliationAttempt: reconciliationAttempt,
			IdempotencyKey:        request.IdempotencyKey,
		})
		return "", err
	}
	step := reconciler.state.reconciliationSteps[position]
	reconciler.state.observe(scenarioObservation{
		Kind:                  scenarioObservedReconcilerReturn,
		ActionID:              request.Action.ID,
		Attempt:               request.Attempt,
		ReconciliationAttempt: reconciliationAttempt,
		ReconciliationOutcome: step.Outcome,
		IdempotencyKey:        request.IdempotencyKey,
	})
	return step.Outcome, step.Err
}

func scenarioObservationsOfKind(observations []scenarioObservation, kind scenarioObservationKind) []scenarioObservation {
	return slices.DeleteFunc(slices.Clone(observations), func(observation scenarioObservation) bool {
		return observation.Kind != kind
	})
}

func scenarioObservationLabels(observations []scenarioObservation) []string {
	labels := make([]string, 0, len(observations))
	for _, observation := range observations {
		switch observation.Kind {
		case scenarioObservedJournalAppend:
			labels = append(labels, string(observation.JournalKind))
		case scenarioObservedFault:
			labels = append(labels, "fault:"+string(observation.Fault))
		default:
			labels = append(labels, string(observation.Kind))
		}
	}
	return labels
}
