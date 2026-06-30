package reddit

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
	"github.com/itsmeares/vanish/internal/platform"
)

func TestBuildCleanupPlanMapsSelectedRedditItems(t *testing.T) {
	result, err := BuildCleanupPlan(redditPlanBuildRequest(redditPlanTestItems()[:2]))
	if err != nil {
		t.Fatalf("BuildCleanupPlan returned error: %v", err)
	}

	if result.SelectedCount != 2 || len(result.Skipped) != 0 {
		t.Fatalf("unexpected selection/skips: %#v", result)
	}
	if result.Counts.DeleteComment != 1 || result.Counts.DeletePost != 1 {
		t.Fatalf("unexpected counts: %#v", result.Counts)
	}
	if result.Plan.Platform != domain.PlatformReddit || result.Plan.Mode != domain.PlanModeDryRun {
		t.Fatalf("unexpected plan: %#v", result.Plan)
	}
	requireRedditAction(t, result.Plan.Actions, "comment-1", domain.ActionRedditDeleteComment)
	requireRedditAction(t, result.Plan.Actions, "post-1", domain.ActionRedditDeletePost)
	for _, action := range result.Plan.Actions {
		if action.Metadata["reddit_plan_only"] != "true" {
			t.Fatalf("expected plan-only metadata, got %#v", action)
		}
	}
}

func TestBuildCleanupPlanSkipsUnsupportedRedditItems(t *testing.T) {
	result, err := BuildCleanupPlan(redditPlanBuildRequest(redditPlanTestItems()))
	if err != nil {
		t.Fatalf("BuildCleanupPlan returned error: %v", err)
	}

	if len(result.Plan.Actions) != 2 || len(result.Skipped) != 1 {
		t.Fatalf("expected 2 actions and 1 skip, got %#v", result)
	}
	if result.Skipped[0].SourceActivityItemID != "like-1" || !strings.Contains(result.Skipped[0].Reason, "unsupported") {
		t.Fatalf("unexpected skip: %#v", result.Skipped[0])
	}
}

func TestBuildCleanupPlanNoSelectionMessage(t *testing.T) {
	result, err := BuildCleanupPlan(redditPlanBuildRequest(nil))
	if err != nil {
		t.Fatalf("BuildCleanupPlan returned error: %v", err)
	}
	if result.SelectedCount != 0 || !strings.Contains(result.Message, "Select at least one Reddit item") {
		t.Fatalf("unexpected no-selection result: %#v", result)
	}
}

func TestBuildCleanupPlanJSONDoesNotPersistSafeText(t *testing.T) {
	items := redditPlanTestItems()[:1]
	items[0].Text = &domain.SafeTextReference{Hash: "sha256:comment", Preview: "comment preview"}

	result, err := BuildCleanupPlan(redditPlanBuildRequest(items))
	if err != nil {
		t.Fatalf("BuildCleanupPlan returned error: %v", err)
	}

	var buf bytes.Buffer
	if err := domain.WritePlanJSON(&buf, result.Plan); err != nil {
		t.Fatalf("WritePlanJSON returned error: %v", err)
	}
	text := buf.String()
	for _, forbidden := range []string{"safe_text", "sha256:comment", "comment preview"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("plan JSON leaked safe text %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, "reddit_delete_comment") {
		t.Fatalf("plan JSON missing reddit action: %s", text)
	}
}

func TestBuildCleanupPlanUsesStableIDs(t *testing.T) {
	req := redditPlanBuildRequest(redditPlanTestItems()[:2])
	first, err := BuildCleanupPlan(req)
	if err != nil {
		t.Fatalf("first BuildCleanupPlan returned error: %v", err)
	}
	second, err := BuildCleanupPlan(req)
	if err != nil {
		t.Fatalf("second BuildCleanupPlan returned error: %v", err)
	}
	if first.Plan.ID != second.Plan.ID || first.Plan.Actions[0].ID != second.Plan.Actions[0].ID {
		t.Fatalf("expected stable IDs, first=%#v second=%#v", first.Plan, second.Plan)
	}
}

func redditPlanBuildRequest(items []domain.ActivityItem) platform.BuildPlanRequest {
	return platform.BuildPlanRequest{
		PlanID:     "ignored-by-reddit-builder",
		Platform:   domain.PlatformReddit,
		SourceName: redditSourceName,
		CreatedAt:  time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC),
		Items:      items,
	}
}

func redditPlanTestItems() []domain.ActivityItem {
	return []domain.ActivityItem{
		{
			ID:        "comment-1",
			Platform:  domain.PlatformReddit,
			Type:      domain.ItemTypeComment,
			TargetURL: "https://www.reddit.com/r/test/comments/p/comment/c1/",
			TargetID:  "t1_c1",
		},
		{
			ID:        "post-1",
			Platform:  domain.PlatformReddit,
			Type:      domain.ItemTypePost,
			TargetURL: "https://www.reddit.com/r/test/comments/p/post/",
			TargetID:  "t3_p1",
		},
		{
			ID:       "like-1",
			Platform: domain.PlatformReddit,
			Type:     domain.ItemTypeLike,
			TargetID: "t3_liked",
		},
	}
}

func requireRedditAction(t *testing.T, actions []domain.CleanupAction, sourceItemID string, actionType domain.ActionType) {
	t.Helper()

	for _, action := range actions {
		if action.SourceActivityItemID == sourceItemID {
			if action.Type != actionType {
				t.Fatalf("action type for %s = %q, want %q", sourceItemID, action.Type, actionType)
			}
			return
		}
	}
	t.Fatalf("missing action for %s in %#v", sourceItemID, actions)
}
