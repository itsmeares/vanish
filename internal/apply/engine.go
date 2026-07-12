package apply

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/itsmeares/vanish/internal/domain"
)

type ExecutionState string

const (
	ExecutionStatePending   ExecutionState = "pending"
	ExecutionStateRunning   ExecutionState = "running"
	ExecutionStateDone      ExecutionState = "done"
	ExecutionStateFailed    ExecutionState = "failed"
	ExecutionStateSkipped   ExecutionState = "skipped"
	ExecutionStateStopped   ExecutionState = "stopped"
	ExecutionStateCancelled ExecutionState = "cancelled"
)

type EventType string

const (
	EventPreviewed          EventType = "apply_previewed"
	EventConfirmed          EventType = "apply_confirmed"
	EventExecutionStarted   EventType = "apply_execution_started"
	EventActionResult       EventType = "apply_action_result"
	EventActionSkipped      EventType = "apply_action_skipped"
	EventExecutionStopped   EventType = "apply_execution_stopped"
	EventExecutionCancelled EventType = "apply_execution_cancelled"
	EventExecutionFinished  EventType = "apply_execution_finished"
)

type Prerequisite struct {
	Code     string
	Message  string
	Blocking bool
}

type UnsupportedAction struct {
	ActionID string
	Platform domain.PlatformName
	Type     domain.ActionType
	Status   domain.ActionStatus
	Reason   string
}

type Preview struct {
	PlanID           string
	Platform         domain.PlatformName
	Mode             ExecutionMode
	Executor         ExecutorID
	Summary          domain.CleanupPlanSummary
	PendingCount     int
	UnsupportedCount int
	ProviderReady    bool
	CanApply         bool
	Blockers         []Prerequisite
	Unsupported      []UnsupportedAction
}

type ActionResult struct {
	ActionID string
	Platform domain.PlatformName
	Type     domain.ActionType
	Status   domain.ActionStatus
	Message  string
}

type ResultCounts struct {
	Pending   int
	Running   int
	Done      int
	Failed    int
	Skipped   int
	Stopped   int
	Cancelled int
}

type ExecutionEvent struct {
	Type       EventType
	PlanID     string
	Platform   domain.PlatformName
	ActionID   string
	ActionType domain.ActionType
	Status     domain.ActionStatus
	State      ExecutionState
	Message    string
	Counts     ResultCounts
	Mode       ExecutionMode
	Executor   ExecutorID
}

type Execution struct {
	Plan    domain.CleanupPlan
	Preview Preview
	State   ExecutionState
	Results []ActionResult
	Events  []ExecutionEvent
	Counts  ResultCounts
}

type Executor interface {
	Execute(context.Context, domain.CleanupAction) (ActionResult, error)
}

type NoopExecutor struct{}

func (NoopExecutor) Execute(ctx context.Context, action domain.CleanupAction) (ActionResult, error) {
	if err := ctx.Err(); err != nil {
		return ActionResult{}, err
	}
	return ActionResult{
		ActionID: action.ID,
		Platform: action.Platform,
		Type:     action.Type,
		Status:   domain.ActionStatusDone,
		Message:  "No-op apply completed.",
	}, nil
}

type Runner struct {
	Providers ProviderRegistry
	State     RuntimeState
}

func (runner Runner) Preview(plan domain.CleanupPlan, mode ExecutionMode) Preview {
	summary := domain.SummarizeCleanupPlan(plan)
	preview := Preview{
		PlanID:       plan.ID,
		Platform:     plan.Platform,
		Mode:         mode,
		Summary:      summary,
		PendingCount: summary.StatusCounts[domain.ActionStatusPending],
	}

	if err := plan.Validate(); err != nil {
		preview.Blockers = append(preview.Blockers, Prerequisite{
			Code:     "plan_invalid",
			Message:  err.Error(),
			Blocking: true,
		})
		preview.CanApply = false
		return preview
	}

	provider, err := runner.Providers.Resolve(plan.Platform, mode)
	if err != nil {
		code := "provider_unavailable"
		message := "This plan's platform is unavailable for simulation."
		if errors.Is(err, ErrExecutionModeUnavailable) {
			code = "execution_mode_unavailable"
			message = "The requested execution mode is unavailable for this platform."
		}
		preview.Blockers = append(preview.Blockers, Prerequisite{Code: code, Message: message, Blocking: true})
		return preview
	}
	preview.Executor = provider.ExecutorID()
	preview.ProviderReady = true

	if preview.PendingCount == 0 {
		preview.Blockers = append(preview.Blockers, Prerequisite{
			Code:     "no_pending_actions",
			Message:  "Plan has no pending actions.",
			Blocking: true,
		})
	}

	for _, action := range plan.Actions {
		if action.Platform != plan.Platform {
			preview.Unsupported = append(preview.Unsupported, UnsupportedAction{
				ActionID: action.ID,
				Platform: action.Platform,
				Type:     action.Type,
				Status:   action.Status,
				Reason:   "action platform does not match plan platform",
			})
			continue
		}
		if !provider.Supports(action.Type) {
			preview.Unsupported = append(preview.Unsupported, UnsupportedAction{
				ActionID: action.ID,
				Platform: action.Platform,
				Type:     action.Type,
				Status:   action.Status,
				Reason:   fmt.Sprintf("%s is not supported by %s", action.Type, provider.ExecutorID()),
			})
		}
	}
	preview.UnsupportedCount = len(preview.Unsupported)
	if preview.UnsupportedCount > 0 {
		preview.Blockers = append(preview.Blockers, Prerequisite{
			Code:     "unsupported_actions",
			Message:  "Plan includes unsupported actions.",
			Blocking: true,
		})
	}

	providerPrerequisites := provider.Prerequisites(plan, runner.State)
	preview.Blockers = append(preview.Blockers, providerPrerequisites...)
	if hasBlockingPrerequisites(providerPrerequisites) {
		preview.ProviderReady = false
	}

	preview.CanApply = !hasBlockingPrerequisites(preview.Blockers)
	return preview
}

func (runner Runner) Run(ctx context.Context, plan domain.CleanupPlan, mode ExecutionMode) Execution {
	if ctx == nil {
		ctx = context.Background()
	}

	preview := runner.Preview(plan, mode)
	execution := Execution{
		Plan:    plan,
		Preview: preview,
		State:   ExecutionStatePending,
	}
	if !preview.CanApply {
		execution.State = ExecutionStateFailed
		execution.Counts = CountsForPlan(plan)
		execution.Events = append(execution.Events, executionFinishedEvent(plan, execution.State, execution.Counts, preview.Mode, preview.Executor))
		return execution
	}
	provider, err := runner.Providers.Resolve(plan.Platform, mode)
	if err != nil || provider.Executor() == nil {
		execution.State = ExecutionStateFailed
		execution.Counts = CountsForPlan(plan)
		execution.Events = append(execution.Events, executionFinishedEvent(plan, execution.State, execution.Counts, preview.Mode, preview.Executor))
		return execution
	}
	executor := provider.Executor()

	execution.State = ExecutionStateRunning
	execution.Events = append(execution.Events, ExecutionEvent{
		Type:     EventExecutionStarted,
		PlanID:   plan.ID,
		Platform: plan.Platform,
		State:    ExecutionStateRunning,
		Counts:   CountsForPlan(execution.Plan),
		Mode:     mode,
		Executor: provider.ExecutorID(),
	})

	for i := range execution.Plan.Actions {
		action := &execution.Plan.Actions[i]
		if action.Status != domain.ActionStatusPending {
			continue
		}
		if err := ctx.Err(); err != nil {
			result := setActionResult(action, domain.ActionStatusCancelled, err.Error())
			execution.Results = append(execution.Results, result)
			execution.Events = append(execution.Events, eventForActionResult(plan.ID, result, mode, provider.ExecutorID()))
			CancelPending(&execution.Plan, "Execution cancelled.")
			execution.State = ExecutionStateCancelled
			break
		}

		action.Status = domain.ActionStatusRunning
		result, err := executor.Execute(ctx, *action)
		if err != nil {
			result = ActionResult{
				ActionID: action.ID,
				Platform: action.Platform,
				Type:     action.Type,
				Status:   domain.ActionStatusFailed,
				Message:  err.Error(),
			}
		}
		result = normalizeActionResult(*action, result)
		action.Status = result.Status
		execution.Results = append(execution.Results, result)
		execution.Events = append(execution.Events, eventForActionResult(plan.ID, result, mode, provider.ExecutorID()))

		switch result.Status {
		case domain.ActionStatusStopped:
			StopPending(&execution.Plan, "Execution stopped.")
			execution.State = ExecutionStateStopped
		case domain.ActionStatusCancelled:
			CancelPending(&execution.Plan, "Execution cancelled.")
			execution.State = ExecutionStateCancelled
		}
		if execution.State == ExecutionStateStopped || execution.State == ExecutionStateCancelled {
			break
		}
	}

	execution.Counts = CountsForPlan(execution.Plan)
	if execution.State == ExecutionStateRunning {
		execution.State = stateForCounts(execution.Counts)
	}
	execution.Events = append(execution.Events, executionFinishedEvent(execution.Plan, execution.State, execution.Counts, mode, provider.ExecutorID()))
	return execution
}

func RetryAction(plan *domain.CleanupPlan, actionID string) error {
	action, err := findAction(plan, actionID)
	if err != nil {
		return err
	}
	switch action.Status {
	case domain.ActionStatusFailed, domain.ActionStatusSkipped, domain.ActionStatusStopped, domain.ActionStatusCancelled:
		action.Status = domain.ActionStatusPending
		return nil
	default:
		return fmt.Errorf("action %q with status %q cannot be retried", actionID, action.Status)
	}
}

func SkipAction(plan *domain.CleanupPlan, actionID, reason string) (ExecutionEvent, error) {
	action, err := findAction(plan, actionID)
	if err != nil {
		return ExecutionEvent{}, err
	}
	switch action.Status {
	case domain.ActionStatusPending, domain.ActionStatusFailed:
		action.Status = domain.ActionStatusSkipped
	default:
		return ExecutionEvent{}, fmt.Errorf("action %q with status %q cannot be skipped", actionID, action.Status)
	}
	result := ActionResult{
		ActionID: action.ID,
		Platform: action.Platform,
		Type:     action.Type,
		Status:   action.Status,
		Message:  cleanMessage(reason, "Action skipped."),
	}
	return eventForActionResult(plan.ID, result, "", ""), nil
}

func StopPending(plan *domain.CleanupPlan, reason string) int {
	return setPendingStatus(plan, domain.ActionStatusStopped, cleanMessage(reason, "Execution stopped."))
}

func CancelPending(plan *domain.CleanupPlan, reason string) int {
	return setPendingStatus(plan, domain.ActionStatusCancelled, cleanMessage(reason, "Execution cancelled."))
}

func CountsForPlan(plan domain.CleanupPlan) ResultCounts {
	summary := domain.SummarizeCleanupPlan(plan)
	return ResultCounts{
		Pending:   summary.StatusCounts[domain.ActionStatusPending],
		Running:   summary.StatusCounts[domain.ActionStatusRunning],
		Done:      summary.StatusCounts[domain.ActionStatusDone],
		Failed:    summary.StatusCounts[domain.ActionStatusFailed],
		Skipped:   summary.StatusCounts[domain.ActionStatusSkipped],
		Stopped:   summary.StatusCounts[domain.ActionStatusStopped],
		Cancelled: summary.StatusCounts[domain.ActionStatusCancelled],
	}
}

func hasBlockingPrerequisites(prerequisites []Prerequisite) bool {
	for _, prerequisite := range prerequisites {
		if prerequisite.Blocking {
			return true
		}
	}
	return false
}

func findAction(plan *domain.CleanupPlan, actionID string) (*domain.CleanupAction, error) {
	if plan == nil {
		return nil, errors.New("plan is required")
	}
	actionID = strings.TrimSpace(actionID)
	if actionID == "" {
		return nil, errors.New("action id is required")
	}
	for i := range plan.Actions {
		if plan.Actions[i].ID == actionID {
			return &plan.Actions[i], nil
		}
	}
	return nil, fmt.Errorf("action %q was not found", actionID)
}

func setPendingStatus(plan *domain.CleanupPlan, status domain.ActionStatus, _ string) int {
	if plan == nil {
		return 0
	}
	changed := 0
	for i := range plan.Actions {
		if plan.Actions[i].Status == domain.ActionStatusPending {
			plan.Actions[i].Status = status
			changed++
		}
	}
	return changed
}

func setActionResult(action *domain.CleanupAction, status domain.ActionStatus, message string) ActionResult {
	action.Status = status
	return ActionResult{
		ActionID: action.ID,
		Platform: action.Platform,
		Type:     action.Type,
		Status:   status,
		Message:  message,
	}
}

func normalizeActionResult(action domain.CleanupAction, result ActionResult) ActionResult {
	if strings.TrimSpace(result.ActionID) == "" {
		result.ActionID = action.ID
	}
	if result.Platform == "" {
		result.Platform = action.Platform
	}
	if result.Type == "" {
		result.Type = action.Type
	}
	if !isTerminalStatus(result.Status) {
		result.Status = domain.ActionStatusDone
	}
	if strings.TrimSpace(result.Message) == "" {
		result.Message = "No-op apply completed."
	}
	return result
}

func isTerminalStatus(status domain.ActionStatus) bool {
	switch status {
	case domain.ActionStatusDone, domain.ActionStatusFailed, domain.ActionStatusSkipped, domain.ActionStatusStopped, domain.ActionStatusCancelled:
		return true
	default:
		return false
	}
}

func eventForActionResult(planID string, result ActionResult, mode ExecutionMode, executor ExecutorID) ExecutionEvent {
	eventType := EventActionResult
	switch result.Status {
	case domain.ActionStatusSkipped:
		eventType = EventActionSkipped
	case domain.ActionStatusStopped:
		eventType = EventExecutionStopped
	case domain.ActionStatusCancelled:
		eventType = EventExecutionCancelled
	}
	return ExecutionEvent{
		Type:       eventType,
		PlanID:     planID,
		Platform:   result.Platform,
		ActionID:   result.ActionID,
		ActionType: result.Type,
		Status:     result.Status,
		Message:    result.Message,
		Mode:       mode,
		Executor:   executor,
	}
}

func executionFinishedEvent(plan domain.CleanupPlan, state ExecutionState, counts ResultCounts, mode ExecutionMode, executor ExecutorID) ExecutionEvent {
	return ExecutionEvent{
		Type:     EventExecutionFinished,
		PlanID:   plan.ID,
		Platform: plan.Platform,
		State:    state,
		Counts:   counts,
		Mode:     mode,
		Executor: executor,
	}
}

func stateForCounts(counts ResultCounts) ExecutionState {
	switch {
	case counts.Cancelled > 0:
		return ExecutionStateCancelled
	case counts.Stopped > 0:
		return ExecutionStateStopped
	case counts.Failed > 0:
		return ExecutionStateFailed
	case counts.Done == 0 && counts.Skipped > 0:
		return ExecutionStateSkipped
	case counts.Done > 0 || counts.Skipped > 0:
		return ExecutionStateDone
	default:
		return ExecutionStateSkipped
	}
}

func cleanMessage(message, fallback string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return fallback
	}
	return message
}
