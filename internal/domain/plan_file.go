package domain

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

var ErrUnsupportedPlanMode = errors.New("unsupported cleanup plan mode")

// CleanupPlanSummary contains read-only counts for inspecting a saved plan.
type CleanupPlanSummary struct {
	TotalActions int
	ActionCounts map[ActionType]int
	StatusCounts map[ActionStatus]int
}

// LoadPlanJSONFile reads and validates a local dry-run cleanup plan.
func LoadPlanJSONFile(path string) (CleanupPlan, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return CleanupPlan{}, errors.New("plan path is required")
	}

	file, err := os.Open(path)
	if err != nil {
		return CleanupPlan{}, err
	}
	defer file.Close()

	plan, err := ReadPlanJSON(file)
	if err != nil {
		return CleanupPlan{}, err
	}
	if plan.Mode != PlanModeDryRun {
		return CleanupPlan{}, fmt.Errorf("%w: %q", ErrUnsupportedPlanMode, plan.Mode)
	}
	return plan, nil
}

// SummarizeCleanupPlan counts actions by type and status without changing the plan.
func SummarizeCleanupPlan(plan CleanupPlan) CleanupPlanSummary {
	summary := CleanupPlanSummary{
		TotalActions: len(plan.Actions),
		ActionCounts: make(map[ActionType]int),
		StatusCounts: map[ActionStatus]int{
			ActionStatusPending: 0,
			ActionStatusRunning: 0,
			ActionStatusDone:    0,
			ActionStatusFailed:  0,
			ActionStatusSkipped: 0,
		},
	}

	for _, action := range plan.Actions {
		summary.ActionCounts[action.Type]++
		summary.StatusCounts[action.Status]++
	}
	return summary
}
