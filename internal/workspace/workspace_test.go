package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultDirUsesAppDirOverride(t *testing.T) {
	t.Setenv(appDirEnv, filepath.Join(t.TempDir(), "custom"))

	got, err := DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir returned error: %v", err)
	}
	if got != filepath.Clean(os.Getenv(appDirEnv)) {
		t.Fatalf("DefaultDir = %q, want override %q", got, os.Getenv(appDirEnv))
	}
}

func TestOpenInitializesDefaultsAndTelemetryDisabled(t *testing.T) {
	w, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}

	for _, name := range []string{configFileName, recentImportsFileName, recentPlansFileName, auditFileName} {
		if _, err := os.Stat(filepath.Join(w.Dir(), name)); err != nil {
			t.Fatalf("default file %s was not created: %v", name, err)
		}
	}

	config, err := w.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if config.Version != ConfigVersion {
		t.Fatalf("config version = %d, want %d", config.Version, ConfigVersion)
	}
	if config.Telemetry.Enabled {
		t.Fatal("telemetry should be disabled by default")
	}
	if config.CreatedAt.IsZero() {
		t.Fatal("default config CreatedAt should be set")
	}
	if config.UpdatedAt.IsZero() {
		t.Fatal("default config UpdatedAt should be set")
	}
	if config.DefaultPlanExportPath != DefaultPlanExportPath {
		t.Fatalf("default plan export path = %q, want %q", config.DefaultPlanExportPath, DefaultPlanExportPath)
	}
}

func TestConfigRoundTripPreservesCreatedAt(t *testing.T) {
	w, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	original, err := w.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	want := original
	want.Telemetry.Enabled = true
	want.DefaultPlanExportPath = filepath.Join("plans", "next.json")
	want.LastOpenedPlanPath = filepath.Join("plans", "loaded.json")
	want.CreatedAt = original.CreatedAt.Add(24 * time.Hour)

	if err := w.SaveConfig(want); err != nil {
		t.Fatalf("SaveConfig returned error: %v", err)
	}
	got, err := w.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if got.Version != ConfigVersion || !got.Telemetry.Enabled || got.DefaultPlanExportPath != want.DefaultPlanExportPath || got.LastOpenedPlanPath != want.LastOpenedPlanPath {
		t.Fatalf("config round trip = %#v, want %#v", got, want)
	}
	if !got.CreatedAt.Equal(original.CreatedAt) {
		t.Fatalf("CreatedAt = %s, want preserved %s", got.CreatedAt, original.CreatedAt)
	}
	if got.UpdatedAt.IsZero() || got.UpdatedAt.Before(original.UpdatedAt) {
		t.Fatalf("UpdatedAt = %s, want refreshed after %s", got.UpdatedAt, original.UpdatedAt)
	}
}

func TestMalformedConfigReturnsError(t *testing.T) {
	w, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(w.Dir(), configFileName), []byte(`{"version":`), 0o600); err != nil {
		t.Fatalf("write malformed config: %v", err)
	}

	if _, err := w.LoadConfig(); err == nil {
		t.Fatal("LoadConfig returned nil error for malformed config")
	}
}

func TestRecentImportsDedupedAndBounded(t *testing.T) {
	w, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}

	for i := 0; i < MaxRecentImports+5; i++ {
		entry := RecentImport{
			SourceLabel:    "export",
			SourcePath:     filepath.Join("imports", string(rune('a'+i))),
			Platform:       "instagram",
			ImportedAt:     time.Date(2026, 6, 28, 0, i, 0, 0, time.UTC),
			ItemCount:      i,
			LikeCount:      i + 1,
			CommentCount:   i + 2,
			FollowingCount: i + 3,
			FollowerCount:  i + 4,
			WarningCount:   i % 3,
			SkippedCount:   i % 2,
		}
		if err := w.UpsertRecentImport(entry); err != nil {
			t.Fatalf("UpsertRecentImport %d returned error: %v", i, err)
		}
	}
	replacement := RecentImport{
		SourceLabel:    "new label",
		SourcePath:     filepath.Join("imports", "y"),
		Platform:       "instagram",
		ImportedAt:     time.Date(2026, 6, 28, 3, 0, 0, 0, time.UTC),
		ItemCount:      99,
		LikeCount:      11,
		CommentCount:   12,
		FollowingCount: 13,
		FollowerCount:  14,
	}
	if err := w.UpsertRecentImport(replacement); err != nil {
		t.Fatalf("UpsertRecentImport replacement returned error: %v", err)
	}

	got, err := w.RecentImports()
	if err != nil {
		t.Fatalf("RecentImports returned error: %v", err)
	}
	if len(got) != MaxRecentImports {
		t.Fatalf("recent import count = %d, want %d", len(got), MaxRecentImports)
	}
	if got[0].SourceLabel != replacement.SourceLabel || got[0].ItemCount != replacement.ItemCount || got[0].LikeCount != replacement.LikeCount {
		t.Fatalf("replacement was not moved to front: %#v", got[0])
	}
	for i := 1; i < len(got); i++ {
		if filepath.Clean(got[i].SourcePath) == filepath.Clean(replacement.SourcePath) {
			t.Fatalf("duplicate recent import remained at index %d", i)
		}
	}
}

func TestRecentImportRoundTripPreservesPerTypeCounts(t *testing.T) {
	w, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	want := RecentImport{
		SourceLabel:    "instagram-export.zip",
		SourcePath:     filepath.Join("imports", "instagram-export.zip"),
		Platform:       "instagram",
		ImportedAt:     time.Date(2026, 6, 28, 7, 0, 0, 0, time.UTC),
		Demo:           true,
		ItemCount:      10,
		LikeCount:      4,
		CommentCount:   3,
		FollowingCount: 2,
		FollowerCount:  1,
		WarningCount:   5,
		SkippedCount:   6,
	}
	if err := w.UpsertRecentImport(want); err != nil {
		t.Fatalf("UpsertRecentImport returned error: %v", err)
	}

	got, err := w.RecentImports()
	if err != nil {
		t.Fatalf("RecentImports returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("recent import count = %d, want 1", len(got))
	}
	if got[0].LikeCount != want.LikeCount || got[0].CommentCount != want.CommentCount || got[0].FollowingCount != want.FollowingCount || got[0].FollowerCount != want.FollowerCount {
		t.Fatalf("recent import counts = %#v, want %#v", got[0], want)
	}
}

func TestRecentPlansDedupedAndBounded(t *testing.T) {
	w, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}

	for i := 0; i < MaxRecentPlans+4; i++ {
		entry := RecentPlan{
			ID:            "plan-" + string(rune('a'+i)),
			Path:          filepath.Join("plans", string(rune('a'+i))+".json"),
			Mode:          "dry-run",
			SourceName:    "instagram-export",
			PlanCreatedAt: time.Date(2026, 6, 28, 1, i, 0, 0, time.UTC),
			LastUsedAt:    time.Date(2026, 6, 28, 2, i, 0, 0, time.UTC),
			LastOperation: "exported",
			ActionCounts:  map[string]int{"unlike": i},
			StatusCounts:  map[string]int{"pending": i},
		}
		if err := w.UpsertRecentPlan(entry); err != nil {
			t.Fatalf("UpsertRecentPlan %d returned error: %v", i, err)
		}
	}
	replacement := RecentPlan{
		ID:            "plan-x",
		Path:          "other-location.json",
		Mode:          "dry-run",
		SourceName:    "updated",
		PlanCreatedAt: time.Date(2026, 6, 28, 4, 0, 0, 0, time.UTC),
		LastUsedAt:    time.Date(2026, 6, 28, 5, 0, 0, 0, time.UTC),
		LastOperation: "loaded",
		ActionCounts:  map[string]int{"unfollow": 2},
		StatusCounts:  map[string]int{"done": 2},
	}
	if err := w.UpsertRecentPlan(replacement); err != nil {
		t.Fatalf("UpsertRecentPlan replacement returned error: %v", err)
	}

	got, err := w.RecentPlans()
	if err != nil {
		t.Fatalf("RecentPlans returned error: %v", err)
	}
	if len(got) != MaxRecentPlans {
		t.Fatalf("recent plan count = %d, want %d", len(got), MaxRecentPlans)
	}
	if got[0].SourceName != replacement.SourceName || got[0].LastOperation != "loaded" || got[0].ActionCounts["unfollow"] != 2 {
		t.Fatalf("replacement was not moved to front: %#v", got[0])
	}
	for i := 1; i < len(got); i++ {
		if got[i].ID == replacement.ID {
			t.Fatalf("duplicate recent plan remained at index %d", i)
		}
	}
}

func TestRecentPlanRoundTripPreservesUsageFields(t *testing.T) {
	w, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	want := RecentPlan{
		ID:            "plan-1",
		Path:          filepath.Join("plans", "plan-1.json"),
		Mode:          "dry-run",
		SourceName:    "instagram-export",
		PlanCreatedAt: time.Date(2026, 6, 28, 8, 0, 0, 0, time.UTC),
		LastUsedAt:    time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC),
		LastOperation: "exported",
		ActionCounts:  map[string]int{"unlike": 2},
		StatusCounts:  map[string]int{"pending": 2},
	}
	if err := w.UpsertRecentPlan(want); err != nil {
		t.Fatalf("UpsertRecentPlan returned error: %v", err)
	}

	got, err := w.RecentPlans()
	if err != nil {
		t.Fatalf("RecentPlans returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("recent plan count = %d, want 1", len(got))
	}
	if !got[0].PlanCreatedAt.Equal(want.PlanCreatedAt) || !got[0].LastUsedAt.Equal(want.LastUsedAt) || got[0].LastOperation != want.LastOperation {
		t.Fatalf("recent plan usage fields = %#v, want %#v", got[0], want)
	}

	data, err := os.ReadFile(filepath.Join(w.Dir(), recentPlansFileName))
	if err != nil {
		t.Fatalf("read recent plans file: %v", err)
	}
	text := string(data)
	if strings.Contains(text, `"created_at"`) {
		t.Fatalf("new recent plan schema should not write legacy created_at, got:\n%s", text)
	}
	if !strings.Contains(text, `"plan_created_at"`) || !strings.Contains(text, `"last_used_at"`) {
		t.Fatalf("new recent plan schema missing usage fields, got:\n%s", text)
	}
}

func TestRecentPlansLegacyCreatedAtCompatibility(t *testing.T) {
	w, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	createdAt := time.Date(2026, 6, 27, 10, 30, 0, 0, time.UTC)
	content := `[{
		"id": "legacy-plan",
		"path": "plans/legacy.json",
		"mode": "dry-run",
		"source_name": "instagram-export",
		"created_at": "` + createdAt.Format(time.RFC3339) + `",
		"last_operation": "loaded",
		"action_counts": {"unlike": 2},
		"status_counts": {"pending": 2}
	}]`
	if err := os.WriteFile(filepath.Join(w.Dir(), recentPlansFileName), []byte(content), 0o600); err != nil {
		t.Fatalf("write legacy recent plans: %v", err)
	}

	got, err := w.RecentPlans()
	if err != nil {
		t.Fatalf("RecentPlans returned error for legacy created_at: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("recent plan count = %d, want 1", len(got))
	}
	if !got[0].PlanCreatedAt.Equal(createdAt) || !got[0].LastUsedAt.Equal(createdAt) {
		t.Fatalf("legacy created_at did not migrate into usage fields: %#v", got[0])
	}
	if got[0].ActionCounts["unlike"] != 2 || got[0].StatusCounts["pending"] != 2 {
		t.Fatalf("legacy counts not preserved: %#v", got[0])
	}
}

func TestAuditAppendReadAndMalformedLines(t *testing.T) {
	w, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	event := AuditEvent{
		Type:      "plan_written",
		Timestamp: time.Date(2026, 6, 28, 5, 0, 0, 0, time.UTC),
		Fields: map[string]any{
			"action_count": float64(3),
			"demo":         true,
			"source_label": "sample export",
		},
	}
	if err := w.AppendAudit(event); err != nil {
		t.Fatalf("AppendAudit returned error: %v", err)
	}
	file, err := os.OpenFile(filepath.Join(w.Dir(), auditFileName), os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open audit file: %v", err)
	}
	if _, err := file.WriteString("{malformed\n{}\n"); err != nil {
		t.Fatalf("append malformed audit lines: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close audit file: %v", err)
	}

	got, err := w.ReadAudit()
	if err != nil {
		t.Fatalf("ReadAudit returned error: %v", err)
	}
	if len(got.Events) != 1 {
		t.Fatalf("event count = %d, want 1", len(got.Events))
	}
	if got.MalformedLines != 2 {
		t.Fatalf("malformed lines = %d, want 2", got.MalformedLines)
	}
	if got.Events[0].Type != event.Type || got.Events[0].Fields["source_label"] != "sample export" {
		t.Fatalf("audit event = %#v, want %#v", got.Events[0], event)
	}
}

func TestAuditRejectsSecretLikeFields(t *testing.T) {
	w, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}

	err = w.AppendAudit(AuditEvent{
		Type:      "import_complete",
		Timestamp: time.Date(2026, 6, 28, 6, 0, 0, 0, time.UTC),
		Fields: map[string]any{
			"session_token": "do-not-store",
		},
	})
	if err == nil {
		t.Fatal("AppendAudit returned nil error for secret-like field")
	}
}

func TestWipeRecreatesDefaultsAndPreservesExternalPlan(t *testing.T) {
	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	externalPlan := filepath.Join(root, "vanish-plan.json")
	if err := os.WriteFile(externalPlan, []byte(`{"id":"outside"}`), 0o600); err != nil {
		t.Fatalf("write external plan: %v", err)
	}

	w, err := Open(appDir)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	if err := w.UpsertRecentPlan(RecentPlan{ID: "outside", Path: externalPlan, Mode: "dry-run", PlanCreatedAt: time.Date(2026, 6, 28, 11, 0, 0, 0, time.UTC), LastOperation: "exported"}); err != nil {
		t.Fatalf("UpsertRecentPlan returned error: %v", err)
	}
	if err := w.Wipe(); err != nil {
		t.Fatalf("Wipe returned error: %v", err)
	}

	if _, err := os.Stat(externalPlan); err != nil {
		t.Fatalf("external plan was touched by wipe: %v", err)
	}
	for _, name := range []string{configFileName, recentImportsFileName, recentPlansFileName, auditFileName} {
		if _, err := os.Stat(filepath.Join(w.Dir(), name)); err != nil {
			t.Fatalf("default file %s was not recreated: %v", name, err)
		}
	}
	plans, err := w.RecentPlans()
	if err != nil {
		t.Fatalf("RecentPlans returned error: %v", err)
	}
	if len(plans) != 0 {
		t.Fatalf("recent plans after wipe = %d, want 0", len(plans))
	}
}
