package instagram

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
	"github.com/itsmeares/vanish/internal/platform"
)

func TestBuildCleanupPlanMapsSelectedInstagramItems(t *testing.T) {
	result, err := BuildCleanupPlan(planBuildRequest(planTestItems()[:3]))
	if err != nil {
		t.Fatalf("expected plan build, got error: %v", err)
	}

	if result.SelectedCount != 3 {
		t.Fatalf("expected selected count 3, got %d", result.SelectedCount)
	}
	if len(result.Skipped) != 0 {
		t.Fatalf("expected no skipped items, got %#v", result.Skipped)
	}
	if result.Counts.Unlike != 1 || result.Counts.DeleteComment != 1 || result.Counts.Unfollow != 1 {
		t.Fatalf("expected one action of each supported type, got %#v", result.Counts)
	}
	if result.Plan.Mode != domain.PlanModeDryRun {
		t.Fatalf("expected dry-run plan, got %q", result.Plan.Mode)
	}
	requireAction(t, result.Plan.Actions, "like-1", domain.ActionUnlike)
	requireAction(t, result.Plan.Actions, "comment-1", domain.ActionDeleteComment)
	requireAction(t, result.Plan.Actions, "following-1", domain.ActionUnfollow)
}

func TestBuildCleanupPlanSkipsUnsupportedFollower(t *testing.T) {
	result, err := BuildCleanupPlan(planBuildRequest([]domain.ActivityItem{planTestItems()[3]}))
	if err != nil {
		t.Fatalf("expected plan build, got error: %v", err)
	}

	if len(result.Plan.Actions) != 0 {
		t.Fatalf("expected no supported actions, got %#v", result.Plan.Actions)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected one skipped item, got %#v", result.Skipped)
	}
	skip := result.Skipped[0]
	if skip.SourceActivityItemID != "follower-1" || !strings.Contains(skip.Reason, "unsupported") {
		t.Fatalf("expected unsupported follower skip, got %#v", skip)
	}
}

func TestBuildCleanupPlanMixedItemsCountsAndValidation(t *testing.T) {
	result, err := BuildCleanupPlan(planBuildRequest(planTestItems()))
	if err != nil {
		t.Fatalf("expected plan build, got error: %v", err)
	}

	if result.SelectedCount != 4 {
		t.Fatalf("expected selected count 4, got %d", result.SelectedCount)
	}
	if len(result.Plan.Actions) != 3 || len(result.Skipped) != 1 {
		t.Fatalf("expected 3 actions and 1 skip, got actions=%#v skipped=%#v", result.Plan.Actions, result.Skipped)
	}
	if err := result.Plan.Validate(); err != nil {
		t.Fatalf("expected generated plan to validate: %v", err)
	}
}

func TestBuildCleanupPlanNoSelectedItemsReturnsFriendlyMessage(t *testing.T) {
	result, err := BuildCleanupPlan(planBuildRequest(nil))
	if err != nil {
		t.Fatalf("expected plan build, got error: %v", err)
	}

	if result.SelectedCount != 0 || result.Message == "" {
		t.Fatalf("expected no-selection message, got %#v", result)
	}
	if !strings.Contains(result.Message, "Select at least one item") {
		t.Fatalf("expected friendly no-selection message, got %q", result.Message)
	}
}

func TestBuildCleanupPlanJSONRoundTripAndPrivacy(t *testing.T) {
	items := planTestItems()
	items[1].Text = &domain.SafeTextReference{
		Hash:    "sha256:123456",
		Preview: "raw private comment body",
	}

	result, err := BuildCleanupPlan(planBuildRequest(items[:2]))
	if err != nil {
		t.Fatalf("expected plan build, got error: %v", err)
	}

	var buf bytes.Buffer
	if err := domain.WritePlanJSON(&buf, result.Plan); err != nil {
		t.Fatalf("expected plan JSON write, got error: %v", err)
	}

	jsonText := buf.String()
	if !strings.Contains(jsonText, "\n  \"format_version\"") {
		t.Fatalf("expected pretty JSON, got %s", jsonText)
	}
	for _, forbidden := range []string{"raw private comment body", "safe_text", "sha256:123456"} {
		if strings.Contains(jsonText, forbidden) {
			t.Fatalf("expected sensitive text data not to be in plan JSON, found %q in %s", forbidden, jsonText)
		}
	}

	loaded, err := domain.ReadPlanJSON(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("expected plan JSON read, got error: %v", err)
	}
	if len(loaded.Actions) != 2 || loaded.Actions[0].SourceActivityItemID != "like-1" || loaded.Actions[1].SourceActivityItemID != "comment-1" {
		t.Fatalf("expected source item IDs after roundtrip, got %#v", loaded.Actions)
	}
}

func TestBuildCleanupPlanUnsupportedItemsDoNotCrash(t *testing.T) {
	items := []domain.ActivityItem{
		{
			ID:       "other-1",
			Platform: domain.PlatformReddit,
			Type:     domain.ItemTypeLike,
			TargetID: "target-1",
		},
		{
			ID:       "follow-unknown",
			Platform: domain.PlatformInstagram,
			Type:     domain.ItemTypeFollow,
			TargetID: "target-2",
		},
	}

	result, err := BuildCleanupPlan(planBuildRequest(items))
	if err != nil {
		t.Fatalf("expected unsupported items not to crash, got error: %v", err)
	}
	if len(result.Plan.Actions) != 0 || len(result.Skipped) != 2 {
		t.Fatalf("expected unsupported items to be skipped, got actions=%#v skipped=%#v", result.Plan.Actions, result.Skipped)
	}
}

func TestBuildCleanupPlanUsesStableIDs(t *testing.T) {
	req := planBuildRequest(planTestItems()[:2])
	first, err := BuildCleanupPlan(req)
	if err != nil {
		t.Fatalf("expected first build, got error: %v", err)
	}
	second, err := BuildCleanupPlan(req)
	if err != nil {
		t.Fatalf("expected second build, got error: %v", err)
	}

	if first.Plan.ID != second.Plan.ID {
		t.Fatalf("expected stable plan ID, got %q then %q", first.Plan.ID, second.Plan.ID)
	}
	if first.Plan.Actions[0].ID != second.Plan.Actions[0].ID {
		t.Fatalf("expected stable action ID, got %q then %q", first.Plan.Actions[0].ID, second.Plan.Actions[0].ID)
	}
}

func planBuildRequest(items []domain.ActivityItem) platform.BuildPlanRequest {
	return platform.BuildPlanRequest{
		PlanID:     "ignored-by-instagram-builder",
		Platform:   domain.PlatformInstagram,
		SourceName: "demo instagram export",
		CreatedAt:  time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC),
		Items:      items,
	}
}

func planTestItems() []domain.ActivityItem {
	return []domain.ActivityItem{
		{
			ID:        "like-1",
			Platform:  domain.PlatformInstagram,
			Type:      domain.ItemTypeLike,
			TargetURL: "https://www.instagram.com/p/one/",
			TargetID:  "demo_artist",
		},
		{
			ID:       "comment-1",
			Platform: domain.PlatformInstagram,
			Type:     domain.ItemTypeComment,
			TargetID: "comment-target",
		},
		{
			ID:       "following-1",
			Platform: domain.PlatformInstagram,
			Type:     domain.ItemTypeFollow,
			TargetID: "demo_following",
			Metadata: map[string]string{"relationship": "following"},
		},
		{
			ID:       "follower-1",
			Platform: domain.PlatformInstagram,
			Type:     domain.ItemTypeFollow,
			TargetID: "demo_follower",
			Metadata: map[string]string{"relationship": "follower"},
		},
	}
}

func requireAction(t *testing.T, actions []domain.CleanupAction, sourceItemID string, actionType domain.ActionType) {
	t.Helper()

	for _, action := range actions {
		if action.SourceActivityItemID == sourceItemID {
			if action.Type != actionType {
				t.Fatalf("expected %s action for %s, got %s", actionType, sourceItemID, action.Type)
			}
			return
		}
	}
	t.Fatalf("expected action for source item %s, got %#v", sourceItemID, actions)
}
