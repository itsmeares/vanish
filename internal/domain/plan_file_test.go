package domain

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLoadPlanJSONFileLoadsValidPlan(t *testing.T) {
	createdAt := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	plan := NewCleanupPlan("plan-1", PlatformInstagram, "instagram-export", createdAt, []CleanupAction{
		validAction(createdAt),
	})
	path := writeTestPlan(t, plan)

	loaded, err := LoadPlanJSONFile(path)
	if err != nil {
		t.Fatalf("expected valid plan to load, got error: %v", err)
	}
	if loaded.ID != plan.ID || loaded.Platform != PlatformInstagram || len(loaded.Actions) != 1 {
		t.Fatalf("loaded plan mismatch: %#v", loaded)
	}
}

func TestLoadPlanJSONFileMalformedJSONFailsCleanly(t *testing.T) {
	path := writeRawPlanFile(t, "{not json")

	_, err := LoadPlanJSONFile(path)
	if err == nil {
		t.Fatalf("expected malformed JSON to fail")
	}
	if !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("expected JSON parse error, got %v", err)
	}
}

func TestLoadPlanJSONFileUnknownFieldFails(t *testing.T) {
	path := writeRawPlanFile(t, `{
		"format_version": 1,
		"id": "plan-1",
		"platform": "instagram",
		"created_at": "2026-06-26T12:00:00Z",
		"source_name": "instagram-export",
		"mode": "dry-run",
		"actions": [],
		"surprise": true
	}`)

	_, err := LoadPlanJSONFile(path)
	if err == nil {
		t.Fatalf("expected unknown field to fail")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestLoadPlanJSONFileInvalidPlanFailsValidation(t *testing.T) {
	path := writeRawPlanFile(t, `{
		"format_version": 1,
		"id": "",
		"platform": "instagram",
		"created_at": "2026-06-26T12:00:00Z",
		"source_name": "instagram-export",
		"mode": "dry-run",
		"actions": []
	}`)

	_, err := LoadPlanJSONFile(path)
	if err == nil {
		t.Fatalf("expected invalid plan to fail")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestSummarizeCleanupPlanCountsTypesAndStatuses(t *testing.T) {
	createdAt := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	actions := []CleanupAction{
		validActionWithTypeAndStatus(createdAt, "action-1", ActionUnlike, ActionStatusPending),
		validActionWithTypeAndStatus(createdAt, "action-2", ActionDeleteComment, ActionStatusDone),
		validActionWithTypeAndStatus(createdAt, "action-3", ActionDeleteComment, ActionStatusFailed),
		validActionWithTypeAndStatus(createdAt, "action-4", ActionUnfollow, ActionStatusSkipped),
		validActionWithTypeAndStatus(createdAt, "action-5", ActionUnfollow, ActionStatusRunning),
		validActionWithTypeAndStatus(createdAt, "action-6", ActionUnfollow, ActionStatusStopped),
		validActionWithTypeAndStatus(createdAt, "action-7", ActionUnfollow, ActionStatusCancelled),
	}
	plan := NewCleanupPlan("plan-1", PlatformInstagram, "instagram-export", createdAt, actions)

	summary := SummarizeCleanupPlan(plan)

	if summary.TotalActions != 7 {
		t.Fatalf("expected 7 actions, got %d", summary.TotalActions)
	}
	if summary.ActionCounts[ActionUnlike] != 1 || summary.ActionCounts[ActionDeleteComment] != 2 || summary.ActionCounts[ActionUnfollow] != 4 {
		t.Fatalf("unexpected action counts: %#v", summary.ActionCounts)
	}
	if summary.StatusCounts[ActionStatusPending] != 1 ||
		summary.StatusCounts[ActionStatusRunning] != 1 ||
		summary.StatusCounts[ActionStatusDone] != 1 ||
		summary.StatusCounts[ActionStatusFailed] != 1 ||
		summary.StatusCounts[ActionStatusSkipped] != 1 ||
		summary.StatusCounts[ActionStatusStopped] != 1 ||
		summary.StatusCounts[ActionStatusCancelled] != 1 {
		t.Fatalf("unexpected status counts: %#v", summary.StatusCounts)
	}
}

func TestSummarizeCleanupPlanHandlesEmptyActions(t *testing.T) {
	createdAt := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	plan := NewCleanupPlan("plan-1", PlatformInstagram, "instagram-export", createdAt, nil)

	summary := SummarizeCleanupPlan(plan)

	if summary.TotalActions != 0 || len(summary.ActionCounts) != 0 {
		t.Fatalf("expected empty action counts, got %#v", summary)
	}
	if summary.StatusCounts[ActionStatusPending] != 0 ||
		summary.StatusCounts[ActionStatusRunning] != 0 ||
		summary.StatusCounts[ActionStatusDone] != 0 ||
		summary.StatusCounts[ActionStatusFailed] != 0 ||
		summary.StatusCounts[ActionStatusSkipped] != 0 ||
		summary.StatusCounts[ActionStatusStopped] != 0 ||
		summary.StatusCounts[ActionStatusCancelled] != 0 {
		t.Fatalf("expected zero status counts, got %#v", summary.StatusCounts)
	}
}

func TestLoadPlanJSONFileRejectsSecretLikeMetadata(t *testing.T) {
	createdAt := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	action := validAction(createdAt)
	action.Metadata = map[string]string{"session_id": "unsafe"}
	plan := NewCleanupPlan("plan-1", PlatformInstagram, "instagram-export", createdAt, []CleanupAction{action})
	path := writeRawPlanFile(t, rawPlanJSON(t, plan))

	_, err := LoadPlanJSONFile(path)
	if err == nil {
		t.Fatalf("expected secret-like metadata to fail")
	}
	if !strings.Contains(err.Error(), "secret-like") {
		t.Fatalf("expected secret-like error, got %v", err)
	}
}

func TestLoadPlanJSONFileRejectsApplyMode(t *testing.T) {
	createdAt := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	plan := NewCleanupPlan("plan-1", PlatformInstagram, "instagram-export", createdAt, []CleanupAction{
		validAction(createdAt),
	})
	plan.Mode = PlanModeApply
	path := writeRawPlanFile(t, rawPlanJSON(t, plan))

	_, err := LoadPlanJSONFile(path)
	if err == nil {
		t.Fatalf("expected apply mode to fail")
	}
	if !strings.Contains(err.Error(), ErrUnsupportedPlanMode.Error()) {
		t.Fatalf("expected unsupported mode error, got %v", err)
	}
}

func writeTestPlan(t *testing.T, plan CleanupPlan) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "vanish-plan.json")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test plan: %v", err)
	}
	defer file.Close()

	if err := WritePlanJSON(file, plan); err != nil {
		t.Fatalf("write test plan: %v", err)
	}
	return path
}

func writeRawPlanFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "vanish-plan.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write raw plan: %v", err)
	}
	return path
}

func rawPlanJSON(t *testing.T, plan CleanupPlan) string {
	t.Helper()

	return `{
		"format_version": ` + strconv.Itoa(plan.FormatVersion) + `,
		"id": "` + plan.ID + `",
		"platform": "` + string(plan.Platform) + `",
		"created_at": "` + plan.CreatedAt.Format(time.RFC3339) + `",
		"source_name": "` + plan.SourceName + `",
		"mode": "` + string(plan.Mode) + `",
		"actions": [{
			"id": "` + plan.Actions[0].ID + `",
			"platform": "` + string(plan.Actions[0].Platform) + `",
			"type": "` + string(plan.Actions[0].Type) + `",
			"target_url": "` + plan.Actions[0].TargetURL + `",
			"target_id": "` + plan.Actions[0].TargetID + `",
			"source_activity_item_id": "` + plan.Actions[0].SourceActivityItemID + `",
			"status": "` + string(plan.Actions[0].Status) + `",
			"created_at": "` + plan.Actions[0].CreatedAt.Format(time.RFC3339) + `",
			"metadata": ` + metadataJSON(plan.Actions[0].Metadata) + `
		}]
	}`
}

func metadataJSON(metadata map[string]string) string {
	if len(metadata) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(metadata))
	for key, value := range metadata {
		parts = append(parts, `"`+key+`":"`+value+`"`)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func validActionWithTypeAndStatus(createdAt time.Time, id string, actionType ActionType, status ActionStatus) CleanupAction {
	action := validAction(createdAt)
	action.ID = id
	action.Type = actionType
	action.Status = status
	action.SourceActivityItemID = id + "-item"
	return action
}
