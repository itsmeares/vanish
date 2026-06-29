package reddit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
	"github.com/itsmeares/vanish/internal/secretstore"
)

func TestScanActivityFetchesOwnCommentsAndPosts(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		switch r.URL.Path {
		case "/user/test_user/comments":
			writeJSON(t, w, map[string]any{
				"data": map[string]any{
					"after": nil,
					"children": []any{
						map[string]any{
							"kind": "t1",
							"data": map[string]any{
								"id":              "c1",
								"name":            "t1_c1",
								"author":          "test_user",
								"author_fullname": "t2_user",
								"subreddit":       "vanishdev",
								"subreddit_id":    "t5_sub",
								"created_utc":     float64(1767225600),
								"permalink":       "/r/vanishdev/comments/p1/post/c1/",
								"body":            "comment body kept as short preview only",
								"link_id":         "t3_p1",
								"parent_id":       "t3_p1",
							},
						},
					},
				},
			})
		case "/user/test_user/submitted":
			writeJSON(t, w, map[string]any{
				"data": map[string]any{
					"after": nil,
					"children": []any{
						map[string]any{
							"kind": "t3",
							"data": map[string]any{
								"id":          "p1",
								"name":        "t3_p1",
								"author":      "test_user",
								"subreddit":   "vanishdev",
								"created_utc": float64(1767312000),
								"permalink":   "/r/vanishdev/comments/p1/post/",
								"title":       "Post title",
								"selftext":    "post self text",
								"is_self":     true,
							},
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := newScanTestClient(t, server.URL)
	scannedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	result, err := client.ScanActivity(context.Background(), "test_user", ScanOptions{
		Now: func() time.Time { return scannedAt },
	})
	if err != nil {
		t.Fatalf("ScanActivity returned error: %v", err)
	}

	if result.Summary.Total != 2 || result.Summary.Comments != 1 || result.Summary.Posts != 1 || result.Summary.Skipped != 0 {
		t.Fatalf("unexpected summary: %#v", result.Summary)
	}
	if len(result.Items) != 2 {
		t.Fatalf("items = %#v", result.Items)
	}
	comment := result.Items[0]
	if comment.ID != "reddit:comment:t1_c1" || comment.Platform != domain.PlatformReddit || comment.Type != domain.ItemTypeComment {
		t.Fatalf("unexpected comment item: %#v", comment)
	}
	if comment.TargetURL != "https://www.reddit.com/r/vanishdev/comments/p1/post/c1/" || comment.TargetID != "t1_c1" {
		t.Fatalf("unexpected comment target: %#v", comment)
	}
	if comment.Text == nil || comment.Text.Hash == "" || !strings.Contains(comment.Text.Preview, "comment body") {
		t.Fatalf("expected safe comment text ref, got %#v", comment.Text)
	}
	if comment.Source.Name != redditSourceName || comment.Source.ImportedAt == nil || !comment.Source.ImportedAt.Equal(scannedAt) {
		t.Fatalf("unexpected source: %#v", comment.Source)
	}
	post := result.Items[1]
	if post.ID != "reddit:post:t3_p1" || post.Type != domain.ItemTypePost || post.Metadata["is_self"] != "true" {
		t.Fatalf("unexpected post item: %#v", post)
	}
	if len(paths) != 2 || !strings.Contains(paths[0], "limit=100") || !strings.Contains(paths[1], "limit=100") {
		t.Fatalf("unexpected requested paths: %#v", paths)
	}
}

func TestScanActivityPaginatesUntilAfterIsEmpty(t *testing.T) {
	var commentCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/test_user/comments" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		commentCalls++
		if commentCalls == 1 {
			writeJSON(t, w, listingResponse("t1", "next", "c1"))
			return
		}
		if got := r.URL.Query().Get("after"); got != "next" {
			t.Fatalf("after = %q, want next", got)
		}
		writeJSON(t, w, listingResponse("t1", "", "c2"))
	}))
	defer server.Close()

	client := newScanTestClient(t, server.URL)
	result, err := client.ScanActivity(context.Background(), "test_user", ScanOptions{
		IncludeComments: true,
		Limit:           50,
		MaxPages:        5,
	})
	if err != nil {
		t.Fatalf("ScanActivity returned error: %v", err)
	}
	if commentCalls != 2 || len(result.Items) != 2 {
		t.Fatalf("calls/items = %d/%d, result=%#v", commentCalls, len(result.Items), result)
	}
}

func TestScanActivitySkipsUnexpectedKinds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"data": map[string]any{
				"children": []any{
					map[string]any{"kind": "t3", "data": map[string]any{"name": "t3_p1", "permalink": "/r/test/comments/p1/post/"}},
				},
			},
		})
	}))
	defer server.Close()

	client := newScanTestClient(t, server.URL)
	result, err := client.ScanActivity(context.Background(), "test_user", ScanOptions{IncludeComments: true})
	if err == nil {
		t.Fatal("ScanActivity returned nil error for only unsupported items")
	}
	if result.Summary.Skipped != 1 || len(result.Warnings) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestScanActivityRespectsMaxItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, listingResponse("t1", "", "c1", "c2"))
	}))
	defer server.Close()

	client := newScanTestClient(t, server.URL)
	result, err := client.ScanActivity(context.Background(), "test_user", ScanOptions{
		IncludeComments: true,
		MaxItems:        1,
	})
	if err != nil {
		t.Fatalf("ScanActivity returned error: %v", err)
	}
	if len(result.Items) != 1 || result.Summary.Total != 1 {
		t.Fatalf("MaxItems not respected: %#v", result)
	}
}

func newScanTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	client, err := NewClient(secretstore.MustSecret("access-secret"), ClientOptions{BaseURL: baseURL})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	return client
}

func listingResponse(kind, after string, ids ...string) map[string]any {
	children := make([]any, 0, len(ids))
	for _, id := range ids {
		children = append(children, map[string]any{
			"kind": kind,
			"data": map[string]any{
				"id":          id,
				"name":        kind + "_" + id,
				"author":      "test_user",
				"subreddit":   "vanishdev",
				"created_utc": float64(1767225600),
				"permalink":   "/r/vanishdev/comments/p1/post/" + id + "/",
				"body":        "comment " + id,
			},
		})
	}
	return map[string]any{"data": map[string]any{"after": after, "children": children}}
}
