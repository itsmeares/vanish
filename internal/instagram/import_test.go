package instagram

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itsmeares/vanish/internal/domain"
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

	if result.Summary.Total != 4 {
		t.Fatalf("expected 4 parsed items, got %#v", result.Summary)
	}
	if result.Summary.Likes != 1 || result.Summary.Comments != 1 || result.Summary.Following != 1 || result.Summary.Followers != 1 {
		t.Fatalf("expected one item in each supported category, got %#v", result.Summary)
	}
	if result.Summary.Skipped != 1 {
		t.Fatalf("expected unknown file to be skipped, got %#v", result.Summary)
	}
	if len(result.Warnings) == 0 || !strings.Contains(result.Warnings[0], "unsupported Instagram JSON skipped") {
		t.Fatalf("expected unsupported JSON warning, got %#v", result.Warnings)
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
