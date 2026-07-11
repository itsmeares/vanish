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
	plan.Actions[0].Metadata = map[string]string{"batch": "alpha"}
	plan.Actions[1].Status = domain.ActionStatusStopped
	plan.Actions[1].CreatedAt = now.Add(time.Minute)
	plan.Actions[3].Status = domain.ActionStatusFailed
	plan.Actions[3].CreatedAt = now.Add(3 * time.Minute)
	original := clonePlan(plan)
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
	if !reflect.DeepEqual(loaded.OriginalPlan(), original) {
		t.Fatalf("loaded plan snapshot differs:\ngot:  %#v\nwant: %#v", loaded.OriginalPlan(), original)
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

	manifestPath, _ := store.paths(plan.ID)
	manifestBefore, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest before start over: %v", err)
	}
	restarted, err := store.StartOver(plan.ID, now.Add(6*time.Minute))
	if err != nil {
		t.Fatalf("StartOver: %v", err)
	}
	manifestAfter, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest after start over: %v", err)
	}
	if !reflect.DeepEqual(manifestBefore, manifestAfter) {
		t.Fatal("start over changed immutable session manifest")
	}
	reset, ok, err := store.Load(plan.ID)
	if err != nil || !ok || reset.ID != restarted.ID || reset.ID != "manual-one" || reset.CurrentPosition != 0 || reset.State != StateActive {
		t.Fatalf("reset session=%#v ok=%t err=%v", reset, ok, err)
	}
	if !reflect.DeepEqual(reset.OriginalPlan(), original) || !reflect.DeepEqual(plan, original) {
		t.Fatal("session lifecycle mutated original plan")
	}
}

func TestStoreLoadRejectsMismatchedEmbeddedPlanID(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	plan, items := testPlanAndItems(now)
	session, _, err := New("manual-mismatch", plan, items, now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	store := NewStore(t.TempDir())
	if err := store.Start(session); err != nil {
		t.Fatalf("Start: %v", err)
	}
	manifestPath, _ := store.paths(plan.ID)
	session.Manifest.PlanID = "different-plan"
	session.Manifest.PlanSnapshot.ID = "different-plan"
	encoded, err := json.Marshal(session.Manifest)
	if err != nil {
		t.Fatalf("marshal tampered manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, encoded, 0o600); err != nil {
		t.Fatalf("write tampered manifest: %v", err)
	}
	loaded, ok, err := store.Load(plan.ID)
	if err == nil || ok || loaded.ID != "" {
		t.Fatalf("mismatched load session=%#v ok=%t err=%v", loaded, ok, err)
	}
}

func TestReplayRejectsMismatchedOutcomeActionAndPosition(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	plan, items := testPlanAndItems(now)
	tests := []struct {
		name  string
		event progressEvent
	}{
		{name: "session ID", event: progressEvent{ID: "other", At: now.Add(time.Minute), Kind: "done", ActionID: "unfollow-1", Outcome: OutcomeDone, Position: 1, State: StateActive}},
		{name: "action ID", event: progressEvent{ID: "manual-replay", At: now.Add(time.Minute), Kind: "done", ActionID: "unlike-1", Outcome: OutcomeDone, Position: 1, State: StateActive}},
		{name: "position", event: progressEvent{ID: "manual-replay", At: now.Add(time.Minute), Kind: "done", ActionID: "unfollow-1", Outcome: OutcomeDone, Position: 2, State: StateActive}},
		{name: "outcome", event: progressEvent{ID: "manual-replay", At: now.Add(time.Minute), Kind: "done", ActionID: "unfollow-1", Outcome: OutcomeSkipped, Position: 1, State: StateActive}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session, _, err := New("manual-replay", plan, items, now)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			store := NewStore(t.TempDir())
			if err := store.Start(session); err != nil {
				t.Fatalf("Start: %v", err)
			}
			_, progressPath := store.paths(plan.ID)
			writeProgressEvents(t, progressPath,
				progressEvent{ID: session.ID, At: now, Kind: "started", Position: 0, State: StateActive},
				test.event,
			)
			loaded, ok, err := store.Load(plan.ID)
			if err == nil || ok || loaded.ID != "" {
				t.Fatalf("corrupt replay session=%#v ok=%t err=%v", loaded, ok, err)
			}
		})
	}
}

func TestManifestBindsAssistedTargetsToPlanSnapshot(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	plan, items := testPlanAndItems(now)
	plan.Actions[1].TargetURL = "https://instagram.com/p/POSTA/?utm_source=export#fragment"
	plan.Actions[1].TargetID = "ignored-export-id"
	session, _, err := New("manual-target-binding", plan, items, now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := validateManifest(session.Manifest); err != nil {
		t.Fatalf("unchanged canonical target rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{
			name: "snapshot post A assisted post B",
			mutate: func(manifest *Manifest) {
				manifest.Actions[1].TargetURL = "https://www.instagram.com/p/POSTB/"
				manifest.Actions[1].TargetID = "POSTB"
				manifest.Actions[1].TargetKind = "post"
			},
		},
		{
			name: "snapshot profile A assisted profile B",
			mutate: func(manifest *Manifest) {
				manifest.Actions[0].TargetURL = "https://www.instagram.com/other_user/"
				manifest.Actions[0].TargetID = "other_user"
				manifest.Actions[0].TargetKind = "profile"
			},
		},
		{
			name: "changed target ID with unchanged URL",
			mutate: func(manifest *Manifest) {
				manifest.Actions[1].TargetID = "POSTB"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := session.Manifest
			manifest.Actions = append([]Action(nil), session.Actions...)
			test.mutate(&manifest)
			if err := validateManifest(manifest); err == nil {
				t.Fatal("tampered assisted target accepted")
			}
		})
	}
}

func TestStoreEnforcesSessionStateTransitions(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	plan, items := testPlanAndItems(now)

	t.Run("Start Stop Mark fails", func(t *testing.T) {
		session, _, err := New("manual-stop-mark", plan, items, now)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		store := NewStore(t.TempDir())
		if err := store.Start(session); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if err := store.Stop(&session, now.Add(time.Minute)); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		if _, err := store.Mark(&session, OutcomeDone, now.Add(2*time.Minute)); err == nil {
			t.Fatal("stopped session accepted Mark")
		}
		loaded, ok, err := store.Load(plan.ID)
		if err != nil || !ok || loaded.State != StateStopped || loaded.CurrentPosition != 0 {
			t.Fatalf("failed Mark changed progress: loaded=%#v ok=%t err=%v", loaded, ok, err)
		}
	})

	t.Run("active interrupted session resumes", func(t *testing.T) {
		session, _, err := New("manual-active-resume", plan, items, now)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		store := NewStore(t.TempDir())
		if err := store.Start(session); err != nil {
			t.Fatalf("Start: %v", err)
		}
		loaded, ok, err := store.Load(plan.ID)
		if err != nil || !ok {
			t.Fatalf("Load ok=%t err=%v", ok, err)
		}
		if err := store.Resume(&loaded, now.Add(time.Minute)); err != nil || loaded.State != StateActive {
			t.Fatalf("Resume state=%q err=%v", loaded.State, err)
		}
	})

	t.Run("completed session rejects Mark and Stop", func(t *testing.T) {
		session, _, err := New("manual-completed", plan, items, now)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		store := NewStore(t.TempDir())
		if err := store.Start(session); err != nil {
			t.Fatalf("Start: %v", err)
		}
		for range session.Actions {
			if _, err := store.Mark(&session, OutcomeDone, now.Add(time.Minute)); err != nil {
				t.Fatalf("Mark: %v", err)
			}
		}
		if _, err := store.Mark(&session, OutcomeDone, now.Add(2*time.Minute)); err == nil {
			t.Fatal("completed session accepted Mark")
		}
		if err := store.Stop(&session, now.Add(3*time.Minute)); err == nil {
			t.Fatal("completed session accepted Stop")
		}
		loaded, ok, err := store.Load(plan.ID)
		if err != nil || !ok || loaded.State != StateCompleted || loaded.CurrentPosition != len(loaded.Actions) {
			t.Fatalf("failed completed transition changed progress: loaded=%#v ok=%t err=%v", loaded, ok, err)
		}
	})
}

func TestReplayEnforcesStoppedSessionTransitions(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	plan, items := testPlanAndItems(now)
	tests := []struct {
		name    string
		events  func(Session) []progressEvent
		wantErr bool
	}{
		{
			name: "Stop then Done rejected",
			events: func(session Session) []progressEvent {
				return []progressEvent{
					{ID: session.ID, At: now, Kind: "started", Position: 0, State: StateActive},
					{ID: session.ID, At: now.Add(time.Minute), Kind: "stopped", Position: 0, State: StateStopped},
					{ID: session.ID, At: now.Add(2 * time.Minute), Kind: "done", ActionID: session.Actions[0].ActionID, Outcome: OutcomeDone, Position: 1, State: StateActive},
				}
			},
			wantErr: true,
		},
		{
			name: "Stop Resume Done succeeds",
			events: func(session Session) []progressEvent {
				return []progressEvent{
					{ID: session.ID, At: now, Kind: "started", Position: 0, State: StateActive},
					{ID: session.ID, At: now.Add(time.Minute), Kind: "stopped", Position: 0, State: StateStopped},
					{ID: session.ID, At: now.Add(2 * time.Minute), Kind: "resumed", Position: 0, State: StateActive},
					{ID: session.ID, At: now.Add(3 * time.Minute), Kind: "done", ActionID: session.Actions[0].ActionID, Outcome: OutcomeDone, Position: 1, State: StateActive},
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session, _, err := New("manual-journal-state", plan, items, now)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			store := NewStore(t.TempDir())
			if err := store.Start(session); err != nil {
				t.Fatalf("Start: %v", err)
			}
			_, progressPath := store.paths(plan.ID)
			writeProgressEvents(t, progressPath, test.events(session)...)
			loaded, ok, err := store.Load(plan.ID)
			if test.wantErr {
				if err == nil || ok || loaded.ID != "" {
					t.Fatalf("invalid journal loaded=%#v ok=%t err=%v", loaded, ok, err)
				}
				return
			}
			if err != nil || !ok || loaded.State != StateActive || loaded.CurrentPosition != 1 || loaded.Outcomes[0] != OutcomeDone {
				t.Fatalf("valid journal loaded=%#v ok=%t err=%v", loaded, ok, err)
			}
		})
	}
}

func TestSessionFilesNeverPersistRawCommentText(t *testing.T) {
	const raw = "private synthetic comment body"
	const invalidActor = "bad actor\x1b[31m"
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	plan, items := testPlanAndItems(now)
	items[2].Text = &domain.SafeTextReference{Hash: "sha256:abc", Preview: raw}
	items[2].Actor = invalidActor
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
		if strings.Contains(string(content), raw) || strings.Contains(string(content), invalidActor) {
			t.Fatalf("unsafe display text persisted in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	encoded, err := json.Marshal(session.Manifest)
	if err != nil || strings.Contains(string(encoded), raw) || strings.Contains(string(encoded), invalidActor) {
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

func writeProgressEvents(t *testing.T, path string, events ...progressEvent) {
	t.Helper()
	var lines []byte
	for _, event := range events {
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal progress event: %v", err)
		}
		lines = append(lines, encoded...)
		lines = append(lines, '\n')
	}
	if err := os.WriteFile(path, lines, 0o600); err != nil {
		t.Fatalf("write progress events: %v", err)
	}
}
