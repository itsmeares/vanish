package instagram

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

const benchmarkInstagramRecordCount = 150_000

var benchmarkImportSink int

func BenchmarkImportCurrentLikedPosts150K(b *testing.B) {
	zipPath := writeLikedPostsBenchmarkZIP(b, benchmarkInstagramRecordCount, true)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		result, err := ImportZIP(zipPath)
		if err != nil {
			b.Fatal("synthetic Instagram benchmark import failed")
		}
		benchmarkImportSink = len(result.Items) + result.Summary.Skipped
	}
}

func BenchmarkImportRejectedLikedPosts150K(b *testing.B) {
	zipPath := writeLikedPostsBenchmarkZIP(b, benchmarkInstagramRecordCount, false)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		result, err := ImportZIP(zipPath)
		if err != nil {
			b.Fatal("synthetic rejected Instagram benchmark import failed")
		}
		benchmarkImportSink = len(result.Items) + result.Warnings.Total
	}
}

func BenchmarkImportZIPFromEnv(b *testing.B) {
	zipPath := os.Getenv("VANISH_BENCHMARK_ZIP")
	if zipPath == "" {
		b.Skip("VANISH_BENCHMARK_ZIP is not set")
	}
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		result, err := ImportZIP(zipPath)
		if err != nil {
			b.Fatal("local Instagram benchmark import failed")
		}
		benchmarkImportSink = len(result.Items) + result.Summary.Skipped
	}
}

func writeLikedPostsBenchmarkZIP(tb testing.TB, recordCount int, includeTarget bool) string {
	tb.Helper()

	zipPath := filepath.Join(tb.TempDir(), "synthetic-instagram-export.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		tb.Fatal("create synthetic benchmark ZIP")
	}

	writer := zip.NewWriter(file)
	entry, err := writer.Create("your_instagram_activity/likes/liked_posts.json")
	if err != nil {
		_ = file.Close()
		tb.Fatal("create synthetic benchmark entry")
	}
	buffered := bufio.NewWriterSize(entry, 256*1024)
	if _, err := io.WriteString(buffered, "["); err != nil {
		tb.Fatal("start synthetic benchmark JSON")
	}
	for i := range recordCount {
		if i > 0 {
			if _, err := io.WriteString(buffered, ","); err != nil {
				tb.Fatal("separate synthetic benchmark record")
			}
		}
		body := `{"fbid":"%d","timestamp":1710000000,"media":[],"label_values":[{"label":"URI","value":"synthetic_actor"}]}`
		args := []any{10_000_000 + i}
		if includeTarget {
			body = `{"fbid":"%d","timestamp":1710000000,"media":[],"label_values":[{"label":"URI","href":"https://www.instagram.com/p/SYNTHETIC%06d/","value":"synthetic_actor"}]}`
			args = append(args, i)
		}
		if _, err := fmt.Fprintf(buffered, body, args...); err != nil {
			tb.Fatal("write synthetic benchmark record")
		}
	}
	if _, err := io.WriteString(buffered, "]"); err != nil {
		tb.Fatal("finish synthetic benchmark JSON")
	}
	if err := buffered.Flush(); err != nil {
		tb.Fatal("flush synthetic benchmark JSON")
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		tb.Fatal("close synthetic benchmark ZIP")
	}
	if err := file.Close(); err != nil {
		tb.Fatal("close synthetic benchmark file")
	}
	return zipPath
}
