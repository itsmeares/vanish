package tui

import (
	"fmt"
	"os"
	"testing"

	"github.com/itsmeares/vanish/internal/domain"
	"github.com/itsmeares/vanish/internal/instagram"
)

const benchmarkActivityItemCount = 150_000

var benchmarkViewSink string

func BenchmarkItemsBrowserView150K(b *testing.B) {
	m := benchmarkItemsModel(benchmarkActivityItemCount)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		benchmarkViewSink = m.View().Content
	}
}

func BenchmarkItemsBrowserCursorUpdateView150K(b *testing.B) {
	m := benchmarkItemsModel(benchmarkActivityItemCount)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		next, _ := m.Update(keyPress("down"))
		m = next.(Model)
		benchmarkViewSink = m.View().Content
	}
}

func BenchmarkApplyFilter150K(b *testing.B) {
	base := benchmarkItemsModel(benchmarkActivityItemCount)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		m := base
		m.draftFilter = domain.ActivityItemFilter{
			IncludeTypes: map[domain.ActivityFilterType]bool{domain.ActivityFilterLike: true},
		}
		m.applyDraftFilter()
		benchmarkViewSink = m.View().Content
	}
}

func BenchmarkLocalExportViews(b *testing.B) {
	zipPath := os.Getenv("VANISH_BENCHMARK_ZIP")
	if zipPath == "" {
		b.Skip("VANISH_BENCHMARK_ZIP is not set")
	}
	result, err := instagram.ImportZIP(zipPath)
	if err != nil {
		b.Fatal("local Instagram benchmark import failed")
	}

	base := NewModel()
	base.width = 120
	base.height = 40
	base.importResult = activityResultFromInstagram(result)

	b.Run("ItemsBrowserView", func(b *testing.B) {
		m := base
		m.current = screenItemsBrowser
		b.ReportAllocs()
		for range b.N {
			benchmarkViewSink = m.View().Content
		}
	})
	b.Run("WarningsView", func(b *testing.B) {
		m := base
		m.current = screenWarnings
		b.ReportAllocs()
		for range b.N {
			benchmarkViewSink = m.View().Content
		}
	})
}

func benchmarkItemsModel(count int) Model {
	m := NewModel()
	m.width = 120
	m.height = 40
	m.current = screenItemsBrowser
	m.importSource = "synthetic large export"
	m.importPlatform = domain.PlatformInstagram
	m.importResult.Items = make([]domain.ActivityItem, count)
	for i := range count {
		m.importResult.Items[i] = domain.ActivityItem{
			ID:        fmt.Sprintf("synthetic-like-%06d", i),
			Platform:  domain.PlatformInstagram,
			Type:      domain.ItemTypeLike,
			TargetURL: fmt.Sprintf("https://www.instagram.com/p/SYNTHETIC%06d/", i),
			Actor:     "synthetic_actor",
		}
	}
	m.importResult.Summary.Total = count
	m.importResult.Summary.Likes = count
	return m
}
