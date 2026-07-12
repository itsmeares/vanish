package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var errValidation = errors.New("validation failed")

// NewCleanupPlan returns a v1 dry-run plan. Constructors are useful in Go when
// a struct has defaults that callers should not have to remember.
func NewCleanupPlan(id string, platform PlatformName, sourceName string, createdAt time.Time, actions []CleanupAction) CleanupPlan {
	return CleanupPlan{
		FormatVersion: PlanFormatVersion,
		ID:            id,
		Platform:      platform,
		CreatedAt:     createdAt,
		SourceName:    sourceName,
		Mode:          PlanModeDryRun,
		Actions:       actions,
	}
}

// NewCleanupActionFromItem creates a pending action from a selected activity
// item. Returning an error is idiomatic Go: the caller gets the useful value and
// a separate signal that explains why it could not be built.
func NewCleanupActionFromItem(id string, item ActivityItem, actionType ActionType, createdAt time.Time) (CleanupAction, error) {
	if err := item.Validate(); err != nil {
		return CleanupAction{}, err
	}

	action := CleanupAction{
		ID:                   id,
		Platform:             item.Platform,
		Type:                 actionType,
		TargetURL:            item.TargetURL,
		TargetID:             item.TargetID,
		SourceActivityItemID: item.ID,
		Status:               ActionStatusPending,
		CreatedAt:            createdAt,
	}

	if err := action.Validate(); err != nil {
		return CleanupAction{}, err
	}

	return action, nil
}

// Validate checks that the item has enough safe information to review and turn
// into an action. Methods in Go are functions with a receiver before the name;
// here, "item ActivityItem" is the value being validated.
func (item ActivityItem) Validate() error {
	if item.ID == "" {
		return validationError("activity item id is required")
	}
	if item.Platform == "" {
		return validationError("activity item platform is required")
	}
	if item.Type == "" {
		return validationError("activity item type is required")
	}
	if item.TargetURL == "" && item.TargetID == "" {
		return validationError("activity item target_url or target_id is required")
	}
	if err := validateMetadata("activity item metadata", item.Metadata); err != nil {
		return err
	}
	return item.Source.validate()
}

// Validate checks that an action is complete enough for a plan file.
func (action CleanupAction) Validate() error {
	if action.ID == "" {
		return validationError("cleanup action id is required")
	}
	if action.Platform == "" {
		return validationError("cleanup action platform is required")
	}
	if action.Type == "" {
		return validationError("cleanup action type is required")
	}
	if action.TargetURL == "" && action.TargetID == "" {
		return validationError("cleanup action target_url or target_id is required")
	}
	if action.SourceActivityItemID == "" {
		return validationError("cleanup action source_activity_item_id is required")
	}
	if !isKnownActionStatus(action.Status) {
		return validationError("cleanup action status %q is not supported", action.Status)
	}
	if action.CreatedAt.IsZero() {
		return validationError("cleanup action created_at is required")
	}
	return validateMetadata("cleanup action metadata", action.Metadata)
}

// Validate checks that a cleanup plan is a valid v1 dry-run/apply document.
func (plan CleanupPlan) Validate() error {
	if plan.FormatVersion != PlanFormatVersion {
		return validationError("cleanup plan format_version must be %d", PlanFormatVersion)
	}
	if plan.ID == "" {
		return validationError("cleanup plan id is required")
	}
	if plan.Platform == "" {
		return validationError("cleanup plan platform is required")
	}
	if plan.CreatedAt.IsZero() {
		return validationError("cleanup plan created_at is required")
	}
	if plan.SourceName == "" {
		return validationError("cleanup plan source_name is required")
	}
	if !isKnownPlanMode(plan.Mode) {
		return validationError("cleanup plan mode %q is not supported", plan.Mode)
	}
	seenActionIDs := make(map[string]int, len(plan.Actions))
	for i, action := range plan.Actions {
		if err := action.Validate(); err != nil {
			return validationError("cleanup plan action %d: %v", i, err)
		}
		if firstIndex, exists := seenActionIDs[action.ID]; exists {
			return validationError("cleanup plan action %d has duplicate id %q; first used by action %d", i, action.ID, firstIndex)
		}
		seenActionIDs[action.ID] = i
	}
	return nil
}

func (source SourceMetadata) validate() error {
	return validateMetadata("source metadata", source.Metadata)
}

func validationError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errValidation, fmt.Sprintf(format, args...))
}

func isKnownActionStatus(status ActionStatus) bool {
	switch status {
	case ActionStatusPending, ActionStatusRunning, ActionStatusDone, ActionStatusFailed, ActionStatusSkipped, ActionStatusStopped, ActionStatusCancelled:
		return true
	default:
		return false
	}
}

func isKnownPlanMode(mode PlanMode) bool {
	switch mode {
	case PlanModeDryRun, PlanModeApply:
		return true
	default:
		return false
	}
}

func validateMetadata(label string, metadata map[string]string) error {
	for key := range metadata {
		if looksSecretLike(key) {
			return validationError("%s key %q looks secret-like and must not be persisted", label, key)
		}
	}
	return nil
}

func looksSecretLike(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")

	for _, part := range forbiddenMetadataKeyParts {
		if strings.Contains(normalized, part) {
			return true
		}
	}
	return false
}

var forbiddenMetadataKeyParts = []string{
	"password",
	"passwd",
	"cookie",
	"token",
	"session",
	"secret",
	"credential",
	"authorization",
	"private_message",
	"direct_message",
	"dm_body",
	"message_body",
	"full_message",
	"raw_message",
}
