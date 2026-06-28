package workspace

import (
	"os"
	"path/filepath"
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
}

func TestConfigRoundTrip(t *testing.T) {
	w, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	updatedAt := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	want := Config{
		Version:   ConfigVersion,
		Telemetry: TelemetryConfig{Enabled: true},
		UpdatedAt: updatedAt,
	}

	if err := w.SaveConfig(want); err != nil {
		t.Fatalf("SaveConfig returned error: %v", err)
	}
	got, err := w.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if got.Version != want.Version || got.Telemetry.Enabled != want.Telemetry.Enabled || !got.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("config round trip = %#v, want %#v", got, want)
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
			SourceLabel:  "export",
			SourcePath:   filepath.Join("imports", string(rune('a'+i))),
			Platform:     "instagram",
			ImportedAt:   time.Date(2026, 6, 28, 0, i, 0, 0, time.UTC),
			ItemCount:    i,
			WarningCount: i % 3,
			SkippedCount: i % 2,
		}
		if err := w.UpsertRecentImport(entry); err != nil {
			t.Fatalf("UpsertRecentImport %d returned error: %v", i, err)
		}
	}
	replacement := RecentImport{
		SourceLabel: "new label",
		SourcePath:  filepath.Join("imports", "y"),
		Platform:    "instagram",
		ImportedAt:  time.Date(2026, 6, 28, 3, 0, 0, 0, time.UTC),
		ItemCount:   99,
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
	if got[0].SourceLabel != replacement.SourceLabel || got[0].ItemCount != replacement.ItemCount {
		t.Fatalf("replacement was not moved to front: %#v", got[0])
	}
	for i := 1; i < len(got); i++ {
		if filepath.Clean(got[i].SourcePath) == filepath.Clean(replacement.SourcePath) {
			t.Fatalf("duplicate recent import remained at index %d", i)
		}
	}
}

func TestRecentPlansDedupedAndBounded(t *testing.T) {
	w, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}

	for i := 0; i < MaxRecentPlans+4; i++ {
		entry := RecentPlan{
			ID:           "plan-" + string(rune('a'+i)),
			Path:         filepath.Join("plans", string(rune('a'+i))+".json"),
			Mode:         "dry-run",
			SourceName:   "instagram-export",
			CreatedAt:    time.Date(2026, 6, 28, 1, i, 0, 0, time.UTC),
			ActionCounts: map[string]int{"unlike": i},
			StatusCounts: map[string]int{"pending": i},
		}
		if err := w.UpsertRecentPlan(entry); err != nil {
			t.Fatalf("UpsertRecentPlan %d returned error: %v", i, err)
		}
	}
	replacement := RecentPlan{
		ID:           "plan-x",
		Path:         "other-location.json",
		Mode:         "dry-run",
		SourceName:   "updated",
		CreatedAt:    time.Date(2026, 6, 28, 4, 0, 0, 0, time.UTC),
		ActionCounts: map[string]int{"unfollow": 2},
		StatusCounts: map[string]int{"done": 2},
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
	if got[0].SourceName != replacement.SourceName || got[0].ActionCounts["unfollow"] != 2 {
		t.Fatalf("replacement was not moved to front: %#v", got[0])
	}
	for i := 1; i < len(got); i++ {
		if got[i].ID == replacement.ID {
			t.Fatalf("duplicate recent plan remained at index %d", i)
		}
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
	if err := w.UpsertRecentPlan(RecentPlan{ID: "outside", Path: externalPlan, Mode: "dry-run"}); err != nil {
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
