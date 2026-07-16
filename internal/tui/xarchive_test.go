package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/itsmeares/vanish/internal/platform"
	"github.com/itsmeares/vanish/internal/workspace"
	"github.com/itsmeares/vanish/internal/xarchive"
)

func TestXArchiveOnboardingActionsAndUnavailableState(t *testing.T) {
	m := NewModel()
	m.openPlatformDetail(1)
	if m.selectedPlatformID != platform.PlatformXArchive || m.current != screenPlatformDetail {
		t.Fatalf("expected X platform detail, screen=%v id=%q", m.current, m.selectedPlatformID)
	}
	m.platformActionCursor = platformActionIndex(t, m.selectedPlatform(), platform.ActionChooseXArchiveZIP)
	plain := stripANSI(m.View().Content)
	for _, want := range []string{"Request archive", "Choose archive ZIP", "Try demo archive", "Back", "Local data is unavailable"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected X onboarding to contain %q, got:\n%s", want, plain)
		}
	}

	next := updateModel(t, m, keyPress("enter"))
	if next.current != screenPlatformDetail {
		t.Fatalf("unavailable X import should remain on detail, got %v", next.current)
	}
}

func TestXRequestArchiveUsesOfficialSettingsURL(t *testing.T) {
	m := NewModel()
	m.openPlatformDetail(1)
	m.platformActionCursor = platformActionIndex(t, m.selectedPlatform(), platform.ActionRequestXArchive)
	var opened string
	previous := openExternalURL
	openExternalURL = func(rawURL string) error { opened = rawURL; return nil }
	t.Cleanup(func() { openExternalURL = previous })
	updated, cmd := m.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("expected request-archive browser command")
	}
	next := requireModel(t, updated)
	message := cmd()
	next = requireModel(t, mustUpdate(t, next, message))
	if opened != xArchiveSettingsURL || next.xError != "" || next.xStatus == "" {
		t.Fatalf("official X archive request mismatch: opened=%q error=%q status=%q", opened, next.xError, next.xStatus)
	}
}

func TestXDetailWrappingPreservesLongWords(t *testing.T) {
	value := strings.Repeat("a", 200)
	lines := wrapPlainText(value, 17)
	if strings.Join(lines, "") != value {
		t.Fatalf("long post text was truncated: got %d bytes, want %d", len(strings.Join(lines, "")), len(value))
	}
	for _, line := range lines {
		if len(line) > 17 {
			t.Fatalf("wrapped line exceeded width: %q", line)
		}
	}
}

func TestXDemoImportResultBrowserAndDetailAreReadOnly(t *testing.T) {
	w, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := xarchive.NewStore(w.Dir())
	result, err := store.ImportDemo()
	if err != nil {
		t.Fatal(err)
	}
	m := NewModelWithWorkspace(w, nil)
	next := updateModel(t, m, xImportFinishedMsg{result: result})
	if next.current != screenXImportResult {
		t.Fatalf("expected X result, got %v", next.current)
	}
	view := stripANSI(next.View().Content)
	for _, want := range []string{"Posts: 3", "Replies: 1", "Quote posts: 1", "Reposts: 1", "Media: 2", "Browse posts"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected X result to contain %q, got:\n%s", want, view)
		}
	}

	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenXBrowser || !next.xSelectedLoaded {
		t.Fatalf("expected X browser with cached selection, screen=%v loaded=%v", next.current, next.xSelectedLoaded)
	}
	browser := stripANSI(next.View().Content)
	for _, unwanted := range []string{"Toggle selected", "Generate dry-run plan", "Filters"} {
		if strings.Contains(browser, unwanted) {
			t.Fatalf("read-only X browser exposed %q:\n%s", unwanted, browser)
		}
	}

	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenXDetail || next.xDetailTab != 0 {
		t.Fatalf("expected full-text detail, screen=%v tab=%d", next.current, next.xDetailTab)
	}
	next = updateModel(t, next, keyPress("tab"))
	if next.xDetailTab != 1 || !strings.Contains(stripANSI(next.View().Content), "Media actions") {
		t.Fatalf("expected explicit media-actions tab, got:\n%s", stripANSI(next.View().Content))
	}
}

func TestReviewReopensLatestXDatasetAfterRestart(t *testing.T) {
	w, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := xarchive.NewStore(w.Dir()).ImportDemo(); err != nil {
		t.Fatal(err)
	}
	restarted := NewModelWithWorkspace(w, nil)
	updated, _ := restarted.activateTab("Review")
	next := requireModel(t, updated)
	if next.current != screenXBrowser || next.xDataset == nil || !next.xSelectedLoaded {
		t.Fatalf("expected restart Review to open latest X dataset, screen=%v", next.current)
	}
}

func TestLocalDataListsXArchivesAndWipeClearsThem(t *testing.T) {
	w, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := xarchive.NewStore(w.Dir())
	if _, err := store.ImportDemo(); err != nil {
		t.Fatal(err)
	}
	m := NewModelWithWorkspace(w, nil)
	m.openLocalDataOverview()
	if !strings.Contains(stripANSI(m.View().Content), "X archives: 1") {
		t.Fatalf("expected Local Data X count, got:\n%s", stripANSI(m.View().Content))
	}
	m.localDataCursor = localDataXArchives
	next := updateModel(t, m, keyPress("enter"))
	if next.current != screenXArchives || len(next.xDatasets) != 1 {
		t.Fatalf("expected X archive history, screen=%v count=%d", next.current, len(next.xDatasets))
	}

	next.wipeLocalData()
	if len(next.xDatasets) != 0 || next.xDataset != nil {
		t.Fatalf("wipe retained X in-memory state")
	}
	if _, err := os.Stat(store.Root()); !os.IsNotExist(err) {
		t.Fatalf("wipe retained X files: %v", err)
	}
}

func TestXMediaActionOpensOnlyResolvedLocalFile(t *testing.T) {
	w, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := xarchive.NewStore(w.Dir())
	result, err := store.ImportDemo()
	if err != nil {
		t.Fatal(err)
	}
	m := NewModelWithWorkspace(w, nil)
	m.xDataset = result.Dataset
	for index := 0; index < result.Dataset.Len(); index++ {
		activity, readErr := result.Dataset.ActivityAt(index)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(activity.Media) > 0 {
			m.xCursor = index
			m.refreshXSelected()
			break
		}
	}
	m.current = screenXDetail
	m.xDetailTab = 1
	var opened string
	previous := openLocalMedia
	openLocalMedia = func(path string) error { opened = path; return nil }
	t.Cleanup(func() { openLocalMedia = previous })
	updated, cmd := m.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("expected local media open command")
	}
	_ = requireModel(t, updated)
	message := cmd()
	if result := requireModel(t, mustUpdate(t, m, message)); result.xError != "" {
		t.Fatalf("unexpected media-open error: %s", result.xError)
	}
	if opened == "" || !filepath.IsAbs(opened) || !strings.HasPrefix(filepath.Clean(opened), filepath.Clean(store.Root())) {
		t.Fatalf("media action opened non-dataset path %q", opened)
	}
}

func mustUpdate(t *testing.T, model Model, message any) tea.Model {
	t.Helper()
	updated, _ := model.Update(message)
	return updated
}
