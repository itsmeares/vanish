package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/itsmeares/vanish/internal/domain"
	"github.com/itsmeares/vanish/internal/instagram"
)

func TestInitialViewContainsSelectableHomeMenu(t *testing.T) {
	m := NewModel()
	view := m.View().Content

	if !strings.Contains(view, "Vanish") {
		t.Fatalf("expected initial view to contain app name")
	}
	for _, want := range []string{
		"Import Instagram export ZIP",
		"Load cleanup plan",
		"Demo import with fake local data",
		"Quit",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected initial view to show %q, got:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{
		"i  Import Instagram export ZIP",
		"d  Demo import with fake local data",
	} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("expected shortcut row %q to be removed, got:\n%s", unwanted, view)
		}
	}
}

func TestHomeMenuNavigationUsesArrowAndJK(t *testing.T) {
	m := NewModel()

	next := updateModel(t, m, keyPress("down"))
	if next.homeCursor != homeLoadPlan {
		t.Fatalf("expected down to select load plan, got %d", next.homeCursor)
	}

	next = updateModel(t, next, keyPress("j"))
	if next.homeCursor != homeDemo {
		t.Fatalf("expected j to select demo, got %d", next.homeCursor)
	}

	next = updateModel(t, next, keyPress("j"))
	if next.homeCursor != homeQuit {
		t.Fatalf("expected j to select quit, got %d", next.homeCursor)
	}

	next = updateModel(t, next, keyPress("j"))
	if next.homeCursor != homeQuit {
		t.Fatalf("expected cursor to stay at bottom, got %d", next.homeCursor)
	}

	next = updateModel(t, next, keyPress("up"))
	if next.homeCursor != homeDemo {
		t.Fatalf("expected up to select demo, got %d", next.homeCursor)
	}

	next = updateModel(t, next, keyPress("k"))
	if next.homeCursor != homeLoadPlan {
		t.Fatalf("expected k to select load plan, got %d", next.homeCursor)
	}

	next = updateModel(t, next, keyPress("k"))
	if next.homeCursor != homeImportZip {
		t.Fatalf("expected k to select import, got %d", next.homeCursor)
	}

	next = updateModel(t, next, keyPress("k"))
	if next.homeCursor != homeImportZip {
		t.Fatalf("expected cursor to stay at top, got %d", next.homeCursor)
	}
}

func TestEnterOnDemoStartsDemoImport(t *testing.T) {
	m := NewModel()
	m.homeCursor = homeDemo

	updated, cmd := m.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatalf("expected demo import to return a command")
	}

	next := requireModel(t, updated)
	if next.current != screenImporting {
		t.Fatalf("expected demo import to move to importing screen, got %v", next.current)
	}
	if next.importSource != "demo instagram export" {
		t.Fatalf("expected demo source, got %q", next.importSource)
	}
}

func TestQuitKeysOpenConfirmationBeforeQuit(t *testing.T) {
	for _, keyName := range []string{"ctrl+c", "ctrl+q"} {
		m := NewModel()

		updated, cmd := m.Update(keyPress(keyName))
		if cmd != nil {
			t.Fatalf("expected %s not to quit immediately", keyName)
		}

		next := requireModel(t, updated)
		if next.current != screenQuitConfirm {
			t.Fatalf("expected %s to open quit confirmation, got %v", keyName, next.current)
		}
		view := next.View().Content
		for _, want := range []string{"Quit Vanish?", "Quit Vanish", "Cancel"} {
			if !strings.Contains(view, want) {
				t.Fatalf("expected quit confirmation to contain %q, got:\n%s", want, view)
			}
		}

		next = updateModel(t, next, keyPress("up"))
		updated, cmd = next.Update(keyPress("enter"))
		if cmd == nil {
			t.Fatalf("expected confirmed quit to return command")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("expected confirmed quit command to return tea.QuitMsg")
		}
	}
}

func TestQNoLongerQuits(t *testing.T) {
	m := NewModel()

	updated, cmd := m.Update(keyPress("q"))
	if cmd != nil {
		t.Fatalf("expected q not to return quit command")
	}
	next := requireModel(t, updated)
	if next.current != screenHome {
		t.Fatalf("expected q to leave screen unchanged, got %v", next.current)
	}
}

func TestQuitConfirmationEscReturnsToPreviousScreen(t *testing.T) {
	m := importedModel(t, fakeImportResult())
	next := updateModel(t, m, keyPress("enter"))
	if next.current != screenItemsBrowser {
		t.Fatalf("expected test setup to open items browser")
	}

	next = updateModel(t, next, keyPress("ctrl+c"))
	if next.current != screenQuitConfirm {
		t.Fatalf("expected quit confirmation, got %v", next.current)
	}

	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenItemsBrowser {
		t.Fatalf("expected esc to return to previous screen, got %v", next.current)
	}
}

func TestHomeQuitMenuOpensConfirmation(t *testing.T) {
	m := NewModel()
	m.homeCursor = homeQuit

	updated, cmd := m.Update(keyPress("enter"))
	if cmd != nil {
		t.Fatalf("expected home quit menu not to quit immediately")
	}
	next := requireModel(t, updated)
	if next.current != screenQuitConfirm {
		t.Fatalf("expected home quit menu to open confirmation, got %v", next.current)
	}
}

func TestImportPathScreenAcceptsTypedPathAndEscReturnsHome(t *testing.T) {
	m := NewModel()

	next := updateModel(t, m, keyPress("enter"))
	if next.current != screenImportPath {
		t.Fatalf("expected import path screen, got %v", next.current)
	}

	for _, keyName := range []string{"a", "b", "c"} {
		next = updateModel(t, next, keyPress(keyName))
	}
	if next.pathInput.Value() != "abc" {
		t.Fatalf("expected typed path to be captured, got %q", next.pathInput.Value())
	}

	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenHome {
		t.Fatalf("expected esc to return home, got %v", next.current)
	}
}

func TestPlanLoadPathShowsFriendlyMissingFileError(t *testing.T) {
	m := NewModel()
	m.homeCursor = homeLoadPlan

	next := updateModel(t, m, keyPress("enter"))
	if next.current != screenPlanLoadPath {
		t.Fatalf("expected plan load path screen, got %v", next.current)
	}
	if !strings.Contains(next.View().Content, "Load Cleanup Plan") {
		t.Fatalf("expected load screen, got:\n%s", next.View().Content)
	}

	missingPath := filepath.Join(t.TempDir(), "missing-plan.json")
	next.planPathInput.SetValue(missingPath)
	updated, cmd := next.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatalf("expected plan load command")
	}
	next = requireModel(t, updated)
	next = updateModel(t, next, cmd())

	view := next.View().Content
	if next.current != screenPlanLoadPath {
		t.Fatalf("expected to stay on load path after error, got %v", next.current)
	}
	if !strings.Contains(view, "Plan file not found") {
		t.Fatalf("expected friendly missing file error, got:\n%s", view)
	}
}

func TestPlanLoadSuccessShowsSummaryAndActionsBrowser(t *testing.T) {
	plan := fakeCleanupPlan()
	path := writeTUIPlan(t, plan)

	m := NewModel()
	m.homeCursor = homeLoadPlan
	next := updateModel(t, m, keyPress("enter"))
	next.planPathInput.SetValue(path)
	updated, cmd := next.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatalf("expected plan load command")
	}
	next = requireModel(t, updated)
	next = updateModel(t, next, cmd())

	if next.current != screenLoadedPlanSummary {
		t.Fatalf("expected loaded plan summary, got %v", next.current)
	}
	view := next.View().Content
	for _, want := range []string{
		"Loaded Cleanup Plan",
		"Plan ID: plan-loaded",
		"Format version: 1",
		"Platform: instagram",
		"Source name: instagram-export",
		"Mode: dry-run",
		"Total actions: 3",
		"delete_comment: 1",
		"unfollow: 1",
		"unlike: 1",
		"pending: 1",
		"done: 1",
		"skipped: 1",
		"View actions",
		"Back home",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected summary to contain %q, got:\n%s", want, view)
		}
	}

	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenLoadedPlanActions {
		t.Fatalf("expected actions browser, got %v", next.current)
	}
	view = next.View().Content
	for _, want := range []string{
		"Plan Actions",
		"unlike | pending | https://instagram.example/p/1",
		"Type: unlike",
		"Status: pending",
		"Target URL: https://instagram.example/p/1",
		"Target ID: target-1",
		"Source activity item ID: item-1",
		"Created at: 2026-06-26T12:00:00Z",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected actions browser to contain %q, got:\n%s", want, view)
		}
	}

	next = updateModel(t, next, keyPress("j"))
	if next.loadedActionCursor != 1 {
		t.Fatalf("expected j to move action cursor, got %d", next.loadedActionCursor)
	}
	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenLoadedPlanSummary {
		t.Fatalf("expected esc to return to summary, got %v", next.current)
	}
	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenHome {
		t.Fatalf("expected esc to return home, got %v", next.current)
	}
}

func TestLoadedPlanQuitConfirmation(t *testing.T) {
	plan := fakeCleanupPlan()
	path := writeTUIPlan(t, plan)

	m := NewModel()
	m.homeCursor = homeLoadPlan
	next := updateModel(t, m, keyPress("enter"))
	next.planPathInput.SetValue(path)
	updated, cmd := next.Update(keyPress("enter"))
	next = requireModel(t, updated)
	next = updateModel(t, next, cmd())

	next = updateModel(t, next, keyPress("ctrl+q"))
	if next.current != screenQuitConfirm {
		t.Fatalf("expected ctrl+q to open quit confirmation, got %v", next.current)
	}
	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenLoadedPlanSummary {
		t.Fatalf("expected esc to return to loaded summary, got %v", next.current)
	}
}

func TestImportResultViewShowsSummaryCountsAndActions(t *testing.T) {
	m := NewModel()
	result := instagram.ImportResult{
		Summary: instagram.ImportSummary{
			Total:     4,
			Likes:     1,
			Comments:  1,
			Following: 1,
			Followers: 1,
			Skipped:   1,
		},
		Warnings: []string{"settings/unknown_shape.json: unsupported Instagram JSON skipped"},
	}

	updated, cmd := m.Update(importFinishedMsg{result: result, source: "demo instagram export"})
	if cmd != nil {
		t.Fatalf("expected result message not to return a command")
	}

	next := requireModel(t, updated)
	view := next.View().Content
	for _, want := range []string{
		"Import Complete",
		"Total parsed items: 4",
		"Likes: 1",
		"Comments: 1",
		"Following: 1",
		"Followers: 1",
		"Skipped or unknown files: 1",
		"Warnings: 1",
		"View parsed items",
		"View warnings",
		"Back home",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected result view to contain %q, got:\n%s", want, view)
		}
	}
}

func TestImportResultActionsOpenItemsAndWarnings(t *testing.T) {
	m := importedModel(t, fakeImportResult())

	next := updateModel(t, m, keyPress("enter"))
	if next.current != screenItemsBrowser {
		t.Fatalf("expected enter to open items browser, got %v", next.current)
	}
	itemView := next.View().Content
	for _, want := range []string{
		"Parsed Items",
		"like | demo_artist | https://www.instagram.com/p/demo_like/ | 2024-03-09T16:00:00Z",
		"ID: item-like",
		"Safe text hash: sha256:abcdef",
	} {
		if !strings.Contains(itemView, want) {
			t.Fatalf("expected items browser to contain %q, got:\n%s", want, itemView)
		}
	}
	if strings.Contains(itemView, "raw private comment") {
		t.Fatalf("expected items browser not to render raw text preview, got:\n%s", itemView)
	}

	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenImportResult {
		t.Fatalf("expected esc to return to import summary, got %v", next.current)
	}

	next = updateModel(t, next, keyPress("down"))
	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenWarnings {
		t.Fatalf("expected enter to open warnings, got %v", next.current)
	}
	warningView := next.View().Content
	if !strings.Contains(warningView, "Import Warnings") || !strings.Contains(warningView, "unsupported Instagram JSON skipped") {
		t.Fatalf("expected warnings view, got:\n%s", warningView)
	}

	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenImportResult {
		t.Fatalf("expected esc to return to import summary, got %v", next.current)
	}
}

func TestItemsBrowserNavigationUsesArrowAndJK(t *testing.T) {
	result := fakeImportResult()
	result.Items = append(result.Items, domain.ActivityItem{
		ID:        "item-follow",
		Platform:  domain.PlatformInstagram,
		Type:      domain.ItemTypeFollow,
		Actor:     "demo_following",
		TargetID:  "demo_following",
		Metadata:  map[string]string{"relationship": "following"},
		Source:    domain.SourceMetadata{FileName: "following.json"},
		TargetURL: "https://www.instagram.com/demo_following/",
	})

	m := importedModel(t, result)
	next := updateModel(t, m, keyPress("enter"))

	next = updateModel(t, next, keyPress("down"))
	if next.itemCursor != 1 {
		t.Fatalf("expected down to move item cursor, got %d", next.itemCursor)
	}

	next = updateModel(t, next, keyPress("j"))
	if next.itemCursor != 2 {
		t.Fatalf("expected j to move item cursor, got %d", next.itemCursor)
	}

	next = updateModel(t, next, keyPress("j"))
	if next.itemCursor != 2 {
		t.Fatalf("expected item cursor to stay at bottom, got %d", next.itemCursor)
	}

	next = updateModel(t, next, keyPress("up"))
	if next.itemCursor != 1 {
		t.Fatalf("expected up to move item cursor, got %d", next.itemCursor)
	}

	next = updateModel(t, next, keyPress("k"))
	if next.itemCursor != 0 {
		t.Fatalf("expected k to move item cursor, got %d", next.itemCursor)
	}
}

func TestItemsBrowserShowsVisibleAndTotalCount(t *testing.T) {
	m := importedModel(t, fakeImportResult())

	next := updateModel(t, m, keyPress("enter"))
	view := next.View().Content

	if !strings.Contains(view, "Visible: 2 / Total: 2 | Selected: 0") {
		t.Fatalf("expected visible and total count, got:\n%s", view)
	}
}

func TestItemsBrowserSelectionRowsAndToggleWithSpace(t *testing.T) {
	m := importedModel(t, fakeImportResult())
	next := updateModel(t, m, keyPress("enter"))

	view := next.View().Content
	if !strings.Contains(view, "[ ] like | demo_artist") {
		t.Fatalf("expected unselected item row, got:\n%s", view)
	}

	next = updateModel(t, next, keyPress(" "))
	view = next.View().Content
	if next.selection.Len() != 1 {
		t.Fatalf("expected one selected item, got %d", next.selection.Len())
	}
	for _, want := range []string{"[x] like | demo_artist", "Visible: 2 / Total: 2 | Selected: 1"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected view to contain %q, got:\n%s", want, view)
		}
	}

	next = updateModel(t, next, keyPress(" "))
	if next.selection.Len() != 0 {
		t.Fatalf("expected toggle to deselect item, got %d", next.selection.Len())
	}
}

func TestItemsBrowserSelectsAndDeselectsVisibleItems(t *testing.T) {
	m := importedModel(t, fakeImportResultWithRelationships())
	next := updateModel(t, m, keyPress("enter"))
	next = applyTypeFilter(t, next, filterRowLike)

	next = updateModel(t, next, keyPress("a"))
	if next.selection.Len() != 1 || !next.selection.Contains("item-like") {
		t.Fatalf("expected filtered like to be selected")
	}
	if next.selection.Contains("item-comment") {
		t.Fatalf("expected hidden comment not to be selected")
	}

	next = updateModel(t, next, keyPress("n"))
	if next.selection.Len() != 0 {
		t.Fatalf("expected visible like to be deselected, got %d", next.selection.Len())
	}
}

func TestItemsBrowserSelectionShortcutsAcceptUppercase(t *testing.T) {
	m := importedModel(t, fakeImportResult())
	next := updateModel(t, m, keyPress("enter"))

	next = updateModel(t, next, keyPress("A"))
	if next.selection.Len() != 2 {
		t.Fatalf("expected uppercase A to select visible items, got %d", next.selection.Len())
	}

	next = updateModel(t, next, keyPress("N"))
	if next.selection.Len() != 0 {
		t.Fatalf("expected uppercase N to deselect visible items, got %d", next.selection.Len())
	}

	next = updateModel(t, next, keyPress("S"))
	if next.current != screenSelectionSummary {
		t.Fatalf("expected uppercase S to open selection summary, got %v", next.current)
	}
}

func TestSelectionPersistsWhenFiltersChangeAndClear(t *testing.T) {
	m := importedModel(t, fakeImportResultWithRelationships())
	next := updateModel(t, m, keyPress("enter"))
	next = updateModel(t, next, keyPress(" "))

	next = applyTypeFilter(t, next, filterRowComment)
	if !next.selection.Contains("item-like") {
		t.Fatalf("expected selection to persist after filter changed")
	}
	if !strings.Contains(next.View().Content, "Visible: 1 / Total: 4 | Selected: 1") {
		t.Fatalf("expected selected count to persist, got:\n%s", next.View().Content)
	}

	next = clearFilters(t, next)
	if !next.selection.Contains("item-like") {
		t.Fatalf("expected selection to persist after filters clear")
	}
	if !strings.Contains(next.View().Content, "Visible: 4 / Total: 4 | Selected: 1") {
		t.Fatalf("expected selected count after clear filters, got:\n%s", next.View().Content)
	}
}

func TestNewImportResetsSelection(t *testing.T) {
	m := importedModel(t, fakeImportResult())
	next := updateModel(t, m, keyPress("enter"))
	next = updateModel(t, next, keyPress(" "))

	if next.selection.Len() != 1 {
		t.Fatalf("expected test setup to select item")
	}

	updated, _ := next.Update(importFinishedMsg{result: fakeImportResultWithRelationships(), source: "new demo"})
	next = requireModel(t, updated)

	if next.selection.Len() != 0 {
		t.Fatalf("expected new import to reset selection, got %d", next.selection.Len())
	}
}

func TestSelectionSummaryShowsCountsAndClearSelection(t *testing.T) {
	m := importedModel(t, fakeImportResultWithRelationships())
	next := updateModel(t, m, keyPress("enter"))
	next = updateModel(t, next, keyPress("a"))
	next = updateModel(t, next, keyPress("s"))

	view := next.View().Content
	for _, want := range []string{
		"Selection Summary",
		"Total selected: 4",
		"Selected likes: 1",
		"Selected comments: 1",
		"Selected following: 1",
		"Selected followers: 1",
		"Generate dry-run plan",
		"View selected items",
		"Clear selection",
		"Back",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected selection summary to contain %q, got:\n%s", want, view)
		}
	}

	next = updateModel(t, next, keyPress("down"))
	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenSelectedItems {
		t.Fatalf("expected selected items screen, got %v", next.current)
	}
	if !strings.Contains(next.View().Content, "Selected Items") || !strings.Contains(next.View().Content, "[x] like | demo_artist") {
		t.Fatalf("expected selected items list, got:\n%s", next.View().Content)
	}

	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenSelectionSummary {
		t.Fatalf("expected esc to return to selection summary, got %v", next.current)
	}
	next = updateModel(t, next, keyPress("down"))
	next = updateModel(t, next, keyPress("enter"))
	if next.selection.Len() != 0 {
		t.Fatalf("expected clear selection to remove all selected IDs, got %d", next.selection.Len())
	}
	if !strings.Contains(next.View().Content, "Total selected: 0") {
		t.Fatalf("expected summary to show cleared selection, got:\n%s", next.View().Content)
	}
}

func TestSelectionSummaryWithoutSelectionShowsPlanMessage(t *testing.T) {
	m := importedModel(t, fakeImportResult())
	next := updateModel(t, m, keyPress("enter"))
	next = updateModel(t, next, keyPress("s"))

	next = updateModel(t, next, keyPress("enter"))

	if next.current != screenSelectionSummary {
		t.Fatalf("expected to stay on selection summary, got %v", next.current)
	}
	if !strings.Contains(next.View().Content, "Select at least one item before generating a plan.") {
		t.Fatalf("expected friendly no-selection message, got:\n%s", next.View().Content)
	}
}

func TestPlanPreviewShowsCountsAndUnsupportedFollowers(t *testing.T) {
	next := planPreviewModel(t)

	if next.current != screenPlanPreview {
		t.Fatalf("expected plan preview, got %v", next.current)
	}
	view := next.View().Content
	for _, want := range []string{
		"Dry-Run Plan Preview",
		"Plan mode: dry-run",
		"Source platform: instagram",
		"Selected items: 4",
		"Supported actions: 3",
		"Unsupported/skipped selected items: 1",
		"Action counts: unlike 1, delete_comment 1, unfollow 1",
		"unlike | item-like | https://www.instagram.com/p/demo_like/",
		"delete_comment | item-comment | demo_bakery",
		"unfollow | item-following | https://www.instagram.com/demo_following/",
		"skipped | item-follower",
		"unsupported follower",
		"Export JSON",
		"Back",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected plan preview to contain %q, got:\n%s", want, view)
		}
	}
}

func TestPlanExportPathDefaultsAndWritesReadableJSON(t *testing.T) {
	next := planPreviewModel(t)
	next = updateModel(t, next, keyPress("enter"))

	if next.current != screenPlanExportPath {
		t.Fatalf("expected export path screen, got %v", next.current)
	}
	if next.planPathInput.Value() != defaultPlanExportPath {
		t.Fatalf("expected default export path %q, got %q", defaultPlanExportPath, next.planPathInput.Value())
	}

	outputPath := filepath.Join(t.TempDir(), "plan.json")
	next.planPathInput.SetValue(outputPath)
	updated, cmd := next.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatalf("expected export command")
	}
	next = requireModel(t, updated)
	next = updateModel(t, next, cmd())

	view := next.View().Content
	if next.planExportStatus != "Saved plan to "+outputPath || !strings.Contains(view, "Saved plan to") {
		t.Fatalf("expected export success message, status=%q view:\n%s", next.planExportStatus, view)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("expected exported plan JSON: %v", err)
	}
	jsonText := string(data)
	for _, want := range []string{
		"\n  \"format_version\"",
		"\"mode\": \"dry-run\"",
		"\"type\": \"unlike\"",
		"\"type\": \"delete_comment\"",
		"\"type\": \"unfollow\"",
	} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("expected exported JSON to contain %q, got:\n%s", want, jsonText)
		}
	}
	if strings.Contains(jsonText, "raw private comment") {
		t.Fatalf("expected raw private text not to be exported, got:\n%s", jsonText)
	}
}

func TestFiltersScreenOpensFromItemsBrowser(t *testing.T) {
	m := importedModel(t, fakeImportResult())
	next := updateModel(t, m, keyPress("enter"))

	next = updateModel(t, next, keyPress("f"))
	view := next.View().Content

	if next.current != screenFilters {
		t.Fatalf("expected filters screen, got %v", next.current)
	}
	for _, want := range []string{"Filters", "[ ] Like", "Actor contains", "Apply filters", "Clear all filters"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected filters view to contain %q, got:\n%s", want, view)
		}
	}
}

func TestApplyingTypeFilterUpdatesItemsBrowser(t *testing.T) {
	m := importedModel(t, fakeImportResultWithRelationships())
	next := updateModel(t, m, keyPress("enter"))
	next = updateModel(t, next, keyPress("f"))

	next = updateModel(t, next, keyPress("enter"))
	for range filterRowApply {
		next = updateModel(t, next, keyPress("down"))
	}
	next = updateModel(t, next, keyPress("enter"))

	view := next.View().Content
	if next.current != screenItemsBrowser {
		t.Fatalf("expected items browser, got %v", next.current)
	}
	for _, want := range []string{
		"Visible: 1 / Total: 4",
		"Filters active",
		"like | demo_artist",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected filtered items view to contain %q, got:\n%s", want, view)
		}
	}
	if strings.Contains(view, "comment | demo_bakery") {
		t.Fatalf("expected comment to be filtered out, got:\n%s", view)
	}
}

func TestInvalidFilterDateShowsFriendlyErrorAndDoesNotApply(t *testing.T) {
	m := importedModel(t, fakeImportResultWithRelationships())
	next := updateModel(t, m, keyPress("enter"))
	next = updateModel(t, next, keyPress("f"))

	for range filterRowOlder {
		next = updateModel(t, next, keyPress("down"))
	}
	next = updateModel(t, next, keyPress("enter"))
	for _, keyName := range []string{"n", "o", "t", "-", "a", "-", "d", "a", "t", "e"} {
		next = updateModel(t, next, keyPress(keyName))
	}
	next = updateModel(t, next, keyPress("enter"))
	for range filterRowApply - filterRowOlder {
		next = updateModel(t, next, keyPress("down"))
	}
	next = updateModel(t, next, keyPress("enter"))

	view := next.View().Content
	if next.current != screenFilters {
		t.Fatalf("expected filters screen after invalid date, got %v", next.current)
	}
	if next.itemFilter.Active() {
		t.Fatalf("expected invalid date not to apply filter")
	}
	if !strings.Contains(view, "Older than date must use YYYY-MM-DD.") {
		t.Fatalf("expected friendly date error, got:\n%s", view)
	}

	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenItemsBrowser {
		t.Fatalf("expected esc to return to items browser, got %v", next.current)
	}
}

func TestWarningsViewShowsEmptyState(t *testing.T) {
	result := fakeImportResult()
	result.Warnings = nil
	m := importedModel(t, result)
	m.resultCursor = resultViewWarnings

	next := updateModel(t, m, keyPress("enter"))
	view := next.View().Content
	if !strings.Contains(view, "No warnings.") {
		t.Fatalf("expected empty warnings state, got:\n%s", view)
	}
}

func TestImportResultViewShowsFailure(t *testing.T) {
	m := NewModel()

	updated, _ := m.Update(importFinishedMsg{
		err:    errors.New("open instagram export zip: not found"),
		source: "missing.zip",
	})

	next := requireModel(t, updated)
	view := next.View().Content
	if !strings.Contains(view, "Import Failed") || !strings.Contains(view, "not found") {
		t.Fatalf("expected failure view, got:\n%s", view)
	}
}

func fakeImportResult() instagram.ImportResult {
	occurred := time.Date(2024, 3, 9, 16, 0, 0, 0, time.UTC)
	return instagram.ImportResult{
		Items: []domain.ActivityItem{
			{
				ID:         "item-like",
				Platform:   domain.PlatformInstagram,
				Type:       domain.ItemTypeLike,
				Actor:      "demo_artist",
				TargetURL:  "https://www.instagram.com/p/demo_like/",
				TargetID:   "demo_artist",
				OccurredAt: &occurred,
				Source:     domain.SourceMetadata{FileName: "liked_posts.json"},
				Metadata:   map[string]string{"instagram_kind": "liked_post", "username": "demo_artist"},
				Text:       &domain.SafeTextReference{Hash: "sha256:abcdef", Preview: "raw private comment"},
			},
			{
				ID:       "item-comment",
				Platform: domain.PlatformInstagram,
				Type:     domain.ItemTypeComment,
				Actor:    "demo_bakery",
				TargetID: "demo_bakery",
				Source:   domain.SourceMetadata{FileName: "post_comments_1.json"},
				Metadata: map[string]string{"instagram_kind": "comment", "media_owner": "demo_bakery"},
				Text:     &domain.SafeTextReference{Hash: "sha256:123456"},
			},
		},
		Summary: instagram.ImportSummary{
			Total:    2,
			Likes:    1,
			Comments: 1,
			Skipped:  1,
		},
		Warnings: []string{"settings/unknown_shape.json: unsupported Instagram JSON skipped"},
	}
}

func fakeImportResultWithRelationships() instagram.ImportResult {
	result := fakeImportResult()
	followingOccurred := time.Date(2024, 3, 10, 16, 0, 0, 0, time.UTC)
	followerOccurred := time.Date(2024, 3, 11, 16, 0, 0, 0, time.UTC)
	result.Items = append(result.Items,
		domain.ActivityItem{
			ID:         "item-following",
			Platform:   domain.PlatformInstagram,
			Type:       domain.ItemTypeFollow,
			Actor:      "demo_following",
			TargetID:   "demo_following",
			TargetURL:  "https://www.instagram.com/demo_following/",
			OccurredAt: &followingOccurred,
			Metadata:   map[string]string{"relationship": "following"},
			Source:     domain.SourceMetadata{FileName: "following.json"},
		},
		domain.ActivityItem{
			ID:         "item-follower",
			Platform:   domain.PlatformInstagram,
			Type:       domain.ItemTypeFollow,
			Actor:      "demo_follower",
			TargetID:   "demo_follower",
			TargetURL:  "https://www.instagram.com/demo_follower/",
			OccurredAt: &followerOccurred,
			Metadata:   map[string]string{"relationship": "follower"},
			Source:     domain.SourceMetadata{FileName: "followers_1.json"},
		},
	)
	result.Summary.Total = len(result.Items)
	result.Summary.Following = 1
	result.Summary.Followers = 1
	return result
}

func importedModel(t *testing.T, result instagram.ImportResult) Model {
	t.Helper()

	updated, cmd := NewModel().Update(importFinishedMsg{result: result, source: "demo instagram export"})
	if cmd != nil {
		t.Fatalf("expected import result message not to return command")
	}
	return requireModel(t, updated)
}

func planPreviewModel(t *testing.T) Model {
	t.Helper()

	next := importedModel(t, fakeImportResultWithRelationships())
	next = updateModel(t, next, keyPress("enter"))
	next = updateModel(t, next, keyPress("a"))
	next = updateModel(t, next, keyPress("s"))
	return updateModel(t, next, keyPress("enter"))
}

func fakeCleanupPlan() domain.CleanupPlan {
	createdAt := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	actions := []domain.CleanupAction{
		{
			ID:                   "action-1",
			Platform:             domain.PlatformInstagram,
			Type:                 domain.ActionUnlike,
			TargetURL:            "https://instagram.example/p/1",
			TargetID:             "target-1",
			SourceActivityItemID: "item-1",
			Status:               domain.ActionStatusPending,
			CreatedAt:            createdAt,
		},
		{
			ID:                   "action-2",
			Platform:             domain.PlatformInstagram,
			Type:                 domain.ActionDeleteComment,
			TargetID:             "comment-1",
			SourceActivityItemID: "item-2",
			Status:               domain.ActionStatusDone,
			CreatedAt:            createdAt.Add(time.Minute),
		},
		{
			ID:                   "action-3",
			Platform:             domain.PlatformInstagram,
			Type:                 domain.ActionUnfollow,
			TargetURL:            "https://instagram.example/demo",
			SourceActivityItemID: "item-3",
			Status:               domain.ActionStatusSkipped,
			CreatedAt:            createdAt.Add(2 * time.Minute),
		},
	}
	return domain.NewCleanupPlan("plan-loaded", domain.PlatformInstagram, "instagram-export", createdAt, actions)
}

func writeTUIPlan(t *testing.T, plan domain.CleanupPlan) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "vanish-plan.json")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	defer file.Close()

	if err := domain.WritePlanJSON(file, plan); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	return path
}

func applyTypeFilter(t *testing.T, model Model, row int) Model {
	t.Helper()

	next := model
	if next.current != screenFilters {
		next = updateModel(t, next, keyPress("f"))
	}
	for next.filterCursor < row {
		next = updateModel(t, next, keyPress("down"))
	}
	for next.filterCursor > row {
		next = updateModel(t, next, keyPress("up"))
	}
	next = updateModel(t, next, keyPress("enter"))
	for next.filterCursor < filterRowApply {
		next = updateModel(t, next, keyPress("down"))
	}
	return updateModel(t, next, keyPress("enter"))
}

func clearFilters(t *testing.T, model Model) Model {
	t.Helper()

	next := model
	if next.current != screenFilters {
		next = updateModel(t, next, keyPress("f"))
	}
	for next.filterCursor < filterRowClear {
		next = updateModel(t, next, keyPress("down"))
	}
	next = updateModel(t, next, keyPress("enter"))
	if next.current == screenFilters {
		next = updateModel(t, next, keyPress("esc"))
	}
	return next
}

func updateModel(t *testing.T, model Model, msg tea.Msg) Model {
	t.Helper()

	updated, _ := model.Update(msg)
	return requireModel(t, updated)
}

func keyPress(text string) tea.KeyPressMsg {
	switch text {
	case "up":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyUp})
	case "down":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
	case "enter":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	case "esc":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc})
	case " ":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeySpace})
	case "ctrl+c":
		return tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})
	case "ctrl+q":
		return tea.KeyPressMsg(tea.Key{Code: 'q', Mod: tea.ModCtrl})
	}

	key := tea.Key{Text: text}
	if len(text) == 1 {
		key.Code = []rune(strings.ToLower(text))[0]
		key.ShiftedCode = []rune(text)[0]
	}

	return tea.KeyPressMsg(key)
}

func requireModel(t *testing.T, model tea.Model) Model {
	t.Helper()

	next, ok := model.(Model)
	if !ok {
		t.Fatalf("expected updated model to be tui.Model")
	}
	return next
}
