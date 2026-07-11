package manualcleanup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

func TestSessionMappingPersistenceResumeAndStartOver(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	plan, items := testPlanAndItems(now)
	original := plan
	session, unavailable, err := New("manual-one", plan, items, now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(session.Actions) != 3 || len(unavailable) != 1 {
		t.Fatalf("actions=%d unavailable=%#v", len(session.Actions), unavailable)
	}
	if !reflect.DeepEqual(plan, original) {
		t.Fatal("session creation mutated original plan")
	}

	store := NewStore(t.TempDir())
	if err := store.Start(session); err != nil {
		t.Fatalf("Start: %v", err)
	}
	completed, err := store.Mark(&session, OutcomeDone, now.Add(time.Minute))
	if err != nil || completed || session.CurrentPosition != 1 {
		t.Fatalf("first mark completed=%t position=%d err=%v", completed, session.CurrentPosition, err)
	}
	if err := store.Stop(&session, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	loaded, ok, err := store.Load(plan.ID)
	if err != nil || !ok {
		t.Fatalf("Load ok=%t err=%v", ok, err)
	}
	if loaded.State != StateStopped || loaded.CurrentPosition != 1 || loaded.Outcomes[0] != OutcomeDone {
		t.Fatalf("loaded session = %#v", loaded)
	}
	if err := store.Resume(&loaded, now.Add(3*time.Minute)); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if _, err := store.Mark(&loaded, OutcomeSkipped, now.Add(4*time.Minute)); err != nil {
		t.Fatalf("skip: %v", err)
	}
	completed, err = store.Mark(&loaded, OutcomeDone, now.Add(5*time.Minute))
	if err != nil || !completed || loaded.State != StateCompleted {
		t.Fatalf("complete=%t state=%q err=%v", completed, loaded.State, err)
	}
	done, skipped, pending := loaded.Counts()
	if done != 2 || skipped != 1 || pending != 0 {
		t.Fatalf("counts done=%d skipped=%d pending=%d", done, skipped, pending)
	}

	restarted, _, err := New("manual-two", plan, items, now.Add(6*time.Minute))
	if err != nil {
		t.Fatalf("New start-over: %v", err)
	}
	if err := store.Start(restarted); err != nil {
		t.Fatalf("Start over: %v", err)
	}
	reset, ok, err := store.Load(plan.ID)
	if err != nil || !ok || reset.ID != "manual-two" || reset.CurrentPosition != 0 || reset.State != StateActive {
		t.Fatalf("reset session=%#v ok=%t err=%v", reset, ok, err)
	}
	if !reflect.DeepEqual(plan, original) {
		t.Fatal("session lifecycle mutated original plan")
	}
}

func TestSessionFilesNeverPersistRawCommentText(t *testing.T) {
	const raw = "private synthetic comment body"
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	plan, items := testPlanAndItems(now)
	items[2].Text = &domain.SafeTextReference{Hash: "sha256:abc", Preview: raw}
	session, _, err := New("privacy-test", plan, items, now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	root := t.TempDir()
	store := NewStore(root)
	if err := store.Start(session); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := store.Mark(&session, OutcomeDone, now.Add(time.Minute)); err != nil {
		t.Fatalf("Mark: %v", err)
	}
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(content), raw) {
			t.Fatalf("raw comment persisted in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	encoded, err := json.Marshal(session.Manifest)
	if err != nil || strings.Contains(string(encoded), raw) {
		t.Fatalf("manifest privacy failure: %s err=%v", encoded, err)
	}
}

func testPlanAndItems(now time.Time) (domain.CleanupPlan, []domain.ActivityItem) {
	actions := []domain.CleanupAction{
		{ID: "unfollow-1", Platform: domain.PlatformInstagram, Type: domain.ActionUnfollow, TargetURL: "https://www.instagram.com/demo_user/", TargetID: "demo_user", SourceActivityItemID: "follow-item", Status: domain.ActionStatusPending, CreatedAt: now},
		{ID: "unlike-1", Platform: domain.PlatformInstagram, Type: domain.ActionUnlike, TargetURL: "https://www.instagram.com/reel/REEL1/", TargetID: "REEL1", SourceActivityItemID: "like-item", Status: domain.ActionStatusPending, CreatedAt: now},
		{ID: "comment-1", Platform: domain.PlatformInstagram, Type: domain.ActionDeleteComment, TargetURL: "https://www.instagram.com/p/POST1/", TargetID: "POST1", SourceActivityItemID: "comment-item", Status: domain.ActionStatusPending, CreatedAt: now},
		{ID: "unsupported-1", Platform: domain.PlatformInstagram, Type: domain.ActionDeletePost, TargetURL: "https://www.instagram.com/p/POST2/", TargetID: "POST2", SourceActivityItemID: "post-item", Status: domain.ActionStatusPending, CreatedAt: now},
	}
	plan := domain.NewCleanupPlan("plan-one", domain.PlatformInstagram, "synthetic", now, actions)
	items := []domain.ActivityItem{
		{ID: "follow-item", Platform: domain.PlatformInstagram, Type: domain.ItemTypeFollow, TargetURL: actions[0].TargetURL, TargetID: "demo_user", Actor: "demo_user", Metadata: map[string]string{"relationship": "following"}},
		{ID: "like-item", Platform: domain.PlatformInstagram, Type: domain.ItemTypeLike, TargetURL: actions[1].TargetURL, TargetID: "REEL1", Actor: "demo_owner", OccurredAt: &now},
		{ID: "comment-item", Platform: domain.PlatformInstagram, Type: domain.ItemTypeComment, TargetURL: actions[2].TargetURL, TargetID: "POST1", Actor: "post_owner", OccurredAt: &now, Text: &domain.SafeTextReference{Hash: "sha256:abc", Preview: "synthetic preview"}},
	}
	return plan, items
}
