package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/itsmeares/vanish/internal/domain"
	"github.com/itsmeares/vanish/internal/instagram"
	"github.com/itsmeares/vanish/internal/manualcleanup"
)

func TestLargeItemsBrowserFormatsOnlyVisibleRows(t *testing.T) {
	m := benchmarkItemsModel(benchmarkActivityItemCount)
	viewport := m.parsedItemsViewport()
	formatted := 0
	rows := m.parsedItemRows(
		m.visibleItemCount(),
		m.itemCursor,
		viewport.Offset,
		viewport.VisibleRows,
		80,
		func(domain.ActivityItem) string {
			formatted++
			return "synthetic row"
		},
	)

	if formatted != viewport.End-viewport.Offset {
		t.Fatalf("expected only visible items to be formatted: formatted=%d viewport=%#v", formatted, viewport)
	}
	if formatted > viewport.VisibleRows || len(rows) > viewport.VisibleRows+1 {
		t.Fatalf("visible row work exceeded viewport: formatted=%d rows=%d viewport=%#v", formatted, len(rows), viewport)
	}
}

func TestManualCleanupLargeSessionRendersCurrentActionOnly(t *testing.T) {
	const count = 150_000
	actions := make([]manualcleanup.Action, count)
	outcomes := make([]manualcleanup.Outcome, count)
	for i := range actions {
		actions[i] = manualcleanup.Action{
			ActionID:   fmt.Sprintf("action-%06d", i),
			Type:       domain.ActionUnfollow,
			TargetURL:  fmt.Sprintf("https://www.instagram.com/demo_%06d/", i),
			TargetKind: instagram.TargetProfile,
			TargetID:   fmt.Sprintf("demo_%06d", i),
		}
		outcomes[i] = manualcleanup.OutcomePending
	}
	m := NewModel()
	m.current = screenManualCleanupAction
	m.width = 100
	m.height = 30
	m.manualSession = manualcleanup.Session{
		Manifest:        manualcleanup.Manifest{ID: "large", PlanID: "large-plan", Mode: manualcleanup.ModeInstagramManual, Actions: actions},
		CurrentPosition: count - 1,
		State:           manualcleanup.StateActive,
		Outcomes:        outcomes,
	}
	view := m.View().Content
	if !strings.Contains(view, "150000 of 150000") || !strings.Contains(view, "demo_149999") {
		t.Fatalf("current action missing from large session view:\n%s", view)
	}
	if strings.Contains(view, "demo_000000") {
		t.Fatal("large session rendered non-current action")
	}
}

func TestFilteredItemCacheInvalidatesOnlyForFilterOrSourceChanges(t *testing.T) {
	m := importedModel(t, fakeImportResultWithRelationships())
	m.draftFilter = domain.ActivityItemFilter{
		IncludeTypes: map[domain.ActivityFilterType]bool{domain.ActivityFilterFollowing: true},
	}
	m.applyDraftFilter()
	if m.visibleItemCount() != 1 {
		t.Fatalf("expected one filtered item, got %d", m.visibleItemCount())
	}
	filteredGeneration := m.itemIndex.generation

	_ = m.View()
	next := updateModel(t, m, keyPress("down"))
	_ = next.View()
	if next.itemIndex.generation != filteredGeneration {
		t.Fatalf("cursor/view work rebuilt filter cache: before=%d after=%d", filteredGeneration, next.itemIndex.generation)
	}

	next.draftFilter = domain.ActivityItemFilter{
		IncludeTypes: map[domain.ActivityFilterType]bool{domain.ActivityFilterComment: true},
	}
	next.applyDraftFilter()
	if next.itemIndex.generation <= filteredGeneration || next.visibleItemCount() != 1 {
		t.Fatalf("filter change did not rebuild cache correctly: generation=%d visible=%d", next.itemIndex.generation, next.visibleItemCount())
	}
	filterGeneration := next.itemIndex.generation

	updated, _ := next.Update(importFinishedMsg{
		result: fakeImportResult(),
		source: "replacement synthetic export",
	})
	replaced := updated.(Model)
	if replaced.itemIndex.generation <= filterGeneration {
		t.Fatalf("source replacement did not invalidate cache: before=%d after=%d", filterGeneration, replaced.itemIndex.generation)
	}
	if replaced.itemFilter.Active() || replaced.visibleItemCount() != len(replaced.importResult.Items) {
		t.Fatalf("replacement source should reset to complete unfiltered data: filter=%#v visible=%d total=%d", replaced.itemFilter, replaced.visibleItemCount(), len(replaced.importResult.Items))
	}
}

func TestSelectionCountsStayCachedAndCorrect(t *testing.T) {
	m := importedModel(t, fakeImportResultWithRelationships())
	m.setVisibleItemsSelected(true)
	if m.selectionCounts.Total != 4 || m.selectionCounts.Likes != 1 || m.selectionCounts.Comments != 1 || m.selectionCounts.Following != 1 || m.selectionCounts.Followers != 1 {
		t.Fatalf("unexpected cached selection counts: %#v", m.selectionCounts)
	}
	before := m.selectionCounts
	_ = m.View()
	if m.selectionCounts != before {
		t.Fatalf("view changed cached selection counts: before=%#v after=%#v", before, m.selectionCounts)
	}
	m.clearSelection()
	if m.selectionCounts.Total != 0 || m.selection.Len() != 0 {
		t.Fatalf("clear did not reset cached selection: counts=%#v selected=%d", m.selectionCounts, m.selection.Len())
	}
}

func TestWarningsViewRendersGroupedViewportOnly(t *testing.T) {
	m := NewModel()
	m.width = 100
	m.height = 30
	m.current = screenWarnings
	m.importSource = "synthetic export"
	m.importResult.WarningCount = 150_000
	m.importResult.Warnings = []activityWarningGroup{{
		SourceFile: "your_instagram_activity/likes/liked_posts.json",
		Category:   "liked-post",
		Reason:     "unsupported target shape",
		Unit:       "record",
		Count:      150_000,
		Examples:   []string{"object{label_values:array,timestamp:number}"},
	}}

	view := m.View().Content
	if !strings.Contains(view, "150,000 liked-post records skipped: unsupported target shape") {
		t.Fatalf("expected grouped warning summary, got:\n%s", view)
	}
	if !strings.Contains(view, "Structure: object{label_values:array,timestamp:number}") {
		t.Fatalf("expected bounded structural detail, got:\n%s", view)
	}

	m.importResult.Warnings = make([]activityWarningGroup, 128)
	for index := range m.importResult.Warnings {
		m.importResult.Warnings[index] = activityWarningGroup{
			Category: "synthetic",
			Reason:   "unsupported target shape",
			Unit:     "record",
			Count:    1,
		}
	}
	_, rows := m.warningRowsForViewport()
	if len(rows) > m.warningListHeight() {
		t.Fatalf("warning renderer exceeded viewport: rows=%d height=%d", len(rows), m.warningListHeight())
	}
}
