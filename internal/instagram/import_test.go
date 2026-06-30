package instagram

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itsmeares/vanish/internal/domain"
	"github.com/itsmeares/vanish/internal/platform"
)

func TestImportZIPParsesDemoExport(t *testing.T) {
	zipPath, err := CreateDemoExportZIP(t.TempDir())
	if err != nil {
		t.Fatalf("expected demo zip, got error: %v", err)
	}

	result, err := ImportZIP(zipPath)
	if err != nil {
		t.Fatalf("expected import to succeed, got error: %v", err)
	}

	if result.Summary.Total < 24 {
		t.Fatalf("expected at least 24 parsed demo items, got %#v", result.Summary)
	}
	if result.Summary.Likes < 6 || result.Summary.Comments < 6 || result.Summary.Following < 6 || result.Summary.Followers < 6 {
		t.Fatalf("expected at least six items in each supported category, got %#v", result.Summary)
	}
	if result.Summary.Skipped < 2 {
		t.Fatalf("expected skipped unknown files, got %#v", result.Summary)
	}
	if len(result.Warnings) == 0 || !strings.Contains(result.Warnings[0], "unsupported Instagram JSON skipped") {
		t.Fatalf("expected unsupported JSON warning, got %#v", result.Warnings)
	}

	var likes, comments, following, followers int
	for _, item := range result.Items {
		switch domain.ActivityFilterTypeForItem(item) {
		case domain.ActivityFilterLike:
			likes++
		case domain.ActivityFilterComment:
			comments++
		case domain.ActivityFilterFollowing:
			following++
		case domain.ActivityFilterFollower:
			followers++
		}
	}
	if likes < 6 || comments < 6 || following < 6 || followers < 6 {
		t.Fatalf("expected demo data useful for scroll/filter/select testing, got likes=%d comments=%d following=%d followers=%d", likes, comments, following, followers)
	}

	for _, item := range result.Items {
		if err := item.Validate(); err != nil {
			t.Fatalf("expected parsed item to validate: %#v: %v", item, err)
		}
		if item.Platform != domain.PlatformInstagram {
			t.Fatalf("expected instagram platform, got %q", item.Platform)
		}
		if item.Source.FileName == "" {
			t.Fatalf("expected source file name on %#v", item)
		}
	}
}

func TestDemoExportUsesReadableSampleData(t *testing.T) {
	zipPath, err := CreateDemoExportZIP(t.TempDir())
	if err != nil {
		t.Fatalf("expected demo zip, got error: %v", err)
	}

	result, err := ImportZIP(zipPath)
	if err != nil {
		t.Fatalf("expected import to succeed, got error: %v", err)
	}

	var rendered strings.Builder
	for _, item := range result.Items {
		fmt.Fprintf(&rendered, "%s %s %s\n", item.Actor, item.TargetURL, item.TargetID)
	}
	text := rendered.String()
	for _, want := range []string{"mira_studio", "https://www.instagram.com/p/C7mug01/", "old_band", "marina_reads"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected readable demo data to contain %q, got:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"vanish_demo", "fake_", "test_account", "demo_"} {
		if strings.Contains(strings.ToLower(text), unwanted) {
			t.Fatalf("expected demo data not to contain %q, got:\n%s", unwanted, text)
		}
	}
}

func TestDemoExportCanBuildMixedSupportPlan(t *testing.T) {
	zipPath, err := CreateDemoExportZIP(t.TempDir())
	if err != nil {
		t.Fatalf("expected demo zip, got error: %v", err)
	}

	result, err := ImportZIP(zipPath)
	if err != nil {
		t.Fatalf("expected import to succeed, got error: %v", err)
	}

	plan, err := BuildCleanupPlan(platform.BuildPlanRequest{
		Platform:   domain.PlatformInstagram,
		SourceName: "demo instagram export",
		Items:      result.Items,
	})
	if err != nil {
		t.Fatalf("expected demo items to build a plan, got error: %v", err)
	}
	if len(plan.Plan.Actions) == 0 {
		t.Fatalf("expected supported demo actions")
	}
	if len(plan.Skipped) == 0 {
		t.Fatalf("expected unsupported follower demo items to be skipped")
	}
	if plan.Counts.Unlike == 0 || plan.Counts.DeleteComment == 0 || plan.Counts.Unfollow == 0 {
		t.Fatalf("expected demo plan to cover unlike/delete_comment/unfollow, got %#v", plan.Counts)
	}
}

func TestImportZIPSkipsUnknownJSONWithoutFailing(t *testing.T) {
	zipPath := writeTestZip(t, map[string]string{
		"likes/liked_posts.json": `{
  "likes_media_likes": [
    {
      "title": "demo_artist",
      "string_list_data": [
        {"href": "https://www.instagram.com/p/one/", "value": "demo_artist", "timestamp": 1710000000}
      ]
    }
  ]
}`,
		"unknown/settings.json": `{"shape": "not supported"}`,
	})

	result, err := ImportZIP(zipPath)
	if err != nil {
		t.Fatalf("expected import to continue, got error: %v", err)
	}
	if result.Summary.Total != 1 || result.Summary.Likes != 1 {
		t.Fatalf("expected one like, got %#v", result.Summary)
	}
	if result.Summary.Skipped != 1 || len(result.Warnings) != 1 {
		t.Fatalf("expected one skipped unknown file and warning, got summary=%#v warnings=%#v", result.Summary, result.Warnings)
	}
}

func TestImportZIPRejectsInvalidZIP(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "not-a-zip.zip")
	if err := os.WriteFile(zipPath, []byte("not a zip"), 0o600); err != nil {
		t.Fatalf("write invalid zip: %v", err)
	}

	_, err := ImportZIP(zipPath)
	if err == nil {
		t.Fatalf("expected invalid zip error")
	}
}

func TestImportZIPWarnsAndContinuesOnMalformedJSON(t *testing.T) {
	zipPath := writeTestZip(t, map[string]string{
		"likes/liked_posts.json": `{
  "likes_media_likes": [
    {
      "title": "demo_artist",
      "string_list_data": [
        {"href": "https://www.instagram.com/p/one/", "value": "demo_artist", "timestamp": 1710000000}
      ]
    }
  ]
}`,
		"comments/post_comments_1.json": `{"comments_media_comments": [`,
	})

	result, err := ImportZIP(zipPath)
	if err != nil {
		t.Fatalf("expected malformed JSON to warn, got error: %v", err)
	}
	if result.Summary.Total != 1 || result.Summary.Likes != 1 {
		t.Fatalf("expected valid file to be parsed, got %#v", result.Summary)
	}
	if result.Summary.Skipped != 1 || len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "malformed JSON skipped") {
		t.Fatalf("expected malformed JSON warning, got summary=%#v warnings=%#v", result.Summary, result.Warnings)
	}
}

func TestImportZIPDoesNotPersistSecretLikeMetadata(t *testing.T) {
	zipPath := writeTestZip(t, map[string]string{
		"likes/liked_posts.json": `{
  "likes_media_likes": [
    {
      "title": "demo_artist",
      "access_token": "must-not-persist",
      "sessionid": "must-not-persist",
      "string_list_data": [
        {
          "href": "https://www.instagram.com/p/one/",
          "value": "demo_artist",
          "timestamp": 1710000000,
          "cookie": "must-not-persist"
        }
      ]
    }
  ]
}`,
	})

	result, err := ImportZIP(zipPath)
	if err != nil {
		t.Fatalf("expected import to succeed, got error: %v", err)
	}
	if result.Summary.Total != 1 {
		t.Fatalf("expected one parsed item, got %#v", result.Summary)
	}

	for _, item := range result.Items {
		if err := item.Validate(); err != nil {
			t.Fatalf("expected item metadata to validate: %v", err)
		}
		assertNoForbiddenKeys(t, item.Metadata)
		assertNoForbiddenKeys(t, item.Source.Metadata)
		dump := fmt.Sprintf("%#v", item)
		for _, forbiddenValue := range []string{"must-not-persist", "access_token", "sessionid", "cookie"} {
			if strings.Contains(dump, forbiddenValue) {
				t.Fatalf("parsed item persisted forbidden value %q: %#v", forbiddenValue, item)
			}
		}
	}
}

func TestImportZIPHashesCommentTextWithoutRawPreview(t *testing.T) {
	const rawComment = "This fake comment should never be stored raw."
	zipPath := writeTestZip(t, map[string]string{
		"comments/post_comments_1.json": `{
  "comments_media_comments": [
    {
      "media_owner": "demo_owner",
      "comment": "` + rawComment + `",
      "timestamp": 1710000000
    }
  ]
}`,
	})

	result, err := ImportZIP(zipPath)
	if err != nil {
		t.Fatalf("expected import to succeed, got error: %v", err)
	}
	if result.Summary.Total != 1 || result.Summary.Comments != 1 {
		t.Fatalf("expected one comment, got %#v", result.Summary)
	}

	item := result.Items[0]
	if item.Text == nil || !strings.HasPrefix(item.Text.Hash, "sha256:") {
		t.Fatalf("expected safe hash reference, got %#v", item.Text)
	}
	if item.Text.Preview != "" {
		t.Fatalf("expected no raw preview, got %q", item.Text.Preview)
	}
	if strings.Contains(fmt.Sprintf("%#v", item), rawComment) {
		t.Fatalf("parsed item persisted raw comment text: %#v", item)
	}
}

func writeTestZip(t *testing.T, files map[string]string) string {
	t.Helper()

	zipPath := filepath.Join(t.TempDir(), "instagram-export.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create test zip: %v", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	for name, body := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close test zip: %v", err)
	}

	return zipPath
}

func assertNoForbiddenKeys(t *testing.T, metadata map[string]string) {
	t.Helper()

	for key := range metadata {
		normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
		for _, part := range []string{"password", "cookie", "token", "session", "secret", "credential", "authorization"} {
			if strings.Contains(normalized, part) {
				t.Fatalf("metadata key %q should not be persisted", key)
			}
		}
	}
}
