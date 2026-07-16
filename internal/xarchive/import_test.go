package xarchive

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestDemoImportPersistsCurrentPostsAndMedia(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	result, err := store.ImportDemo()
	if err != nil {
		t.Fatalf("import demo: %v", err)
	}
	if result.Summary.Counts.Total != 6 || result.Summary.Counts.Posts != 3 || result.Summary.Counts.Replies != 1 || result.Summary.Counts.QuotePosts != 1 || result.Summary.Counts.Reposts != 1 {
		t.Fatalf("unexpected counts: %#v", result.Summary.Counts)
	}
	if result.Summary.Counts.Media != 2 || result.Warnings.Total != 2 {
		t.Fatalf("unexpected media/warnings: counts=%#v warnings=%#v", result.Summary.Counts, result.Warnings)
	}
	listed, err := store.List()
	if err != nil || len(listed) != 1 || listed[0].DatasetID != result.Summary.DatasetID {
		t.Fatalf("list datasets: %#v, %v", listed, err)
	}
	dataset, err := store.Open(result.Summary.DatasetID)
	if err != nil {
		t.Fatalf("reopen dataset: %v", err)
	}
	var sawMedia bool
	for index := 0; index < dataset.Len(); index++ {
		activity, err := dataset.ActivityAt(index)
		if err != nil {
			t.Fatalf("activity %d: %v", index, err)
		}
		for _, media := range activity.Media {
			sawMedia = true
			if _, err := dataset.ResolveMedia(media); err != nil {
				t.Fatalf("resolve media: %v", err)
			}
		}
	}
	if !sawMedia {
		t.Fatal("expected persisted media")
	}
}

func TestManifestAndIndexDoNotContainPostText(t *testing.T) {
	store := NewStore(t.TempDir())
	result, err := store.ImportDemo()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(store.Root(), result.Summary.DatasetID)
	for _, name := range []string{manifestName, indexName} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("A normal local post")) {
			t.Fatalf("post text leaked into %s", name)
		}
	}
	posts, err := os.ReadFile(filepath.Join(root, postsName))
	if err != nil || !bytes.Contains(posts, []byte("A normal local post")) {
		t.Fatalf("posts file did not retain normalized text: %v", err)
	}
}

func TestCurrentPostsOnlyIgnoresDeletedTweets(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "archive.zip")
	if err := WriteDemoZIP(archivePath); err != nil {
		t.Fatal(err)
	}
	if err := appendZipEntry(archivePath, "data/deleted-tweets.js", `window.YTD.deleted_tweets.part0 = [{"tweet":{"id_str":"9999","created_at":"Thu Jun 12 12:00:00 +0000 2025","full_text":"must not import"}}];`); err != nil {
		t.Fatal(err)
	}
	store := NewStore(t.TempDir())
	result, err := store.ImportZIP(archivePath, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.Counts.Total != 6 {
		t.Fatalf("imported deleted post: %#v", result.Summary.Counts)
	}
}

func TestImportRejectsUnsafeArchivePath(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "unsafe.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	if err := writeDemoEntry(archive, "../outside", []byte("no")); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = NewStore(t.TempDir()).ImportZIP(archivePath, false)
	if err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("expected unsafe path error, got %v", err)
	}
}

func TestActivityIntegrityFailureIsDetected(t *testing.T) {
	store := NewStore(t.TempDir())
	result, err := store.ImportDemo()
	if err != nil {
		t.Fatal(err)
	}
	postsPath := filepath.Join(store.Root(), result.Summary.DatasetID, postsName)
	file, err := os.OpenFile(postsPath, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte{'X'}, 0); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	_, err = result.Dataset.ActivityAt(0)
	if err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Fatalf("expected integrity error, got %v", err)
	}
}

func appendZipEntry(path, name, value string) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	entries := make([]struct {
		name string
		data []byte
	}, 0, len(reader.File)+1)
	for _, current := range reader.File {
		opened, openErr := current.Open()
		if openErr != nil {
			_ = reader.Close()
			return openErr
		}
		var buffer bytes.Buffer
		_, copyErr := buffer.ReadFrom(opened)
		_ = opened.Close()
		if copyErr != nil {
			_ = reader.Close()
			return copyErr
		}
		entries = append(entries, struct {
			name string
			data []byte
		}{current.Name, buffer.Bytes()})
	}
	_ = reader.Close()
	entries = append(entries, struct {
		name string
		data []byte
	}{name, []byte(value)})
	temp := path + ".new"
	file, err := os.Create(temp)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(file)
	for _, entry := range entries {
		if err := writeDemoEntry(writer, entry.name, entry.data); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func TestWarningSummaryContainsNoRawRecordValues(t *testing.T) {
	store := NewStore(t.TempDir())
	result, err := store.ImportDemo()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result.Warnings)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("Malformed synthetic record")) || bytes.Contains(data, []byte("1001")) {
		t.Fatalf("warning leaked raw values: %s", data)
	}
}

func TestResolveMediaRejectsTraversal(t *testing.T) {
	store := NewStore(t.TempDir())
	result, err := store.ImportDemo()
	if err != nil {
		t.Fatal(err)
	}
	_, err = result.Dataset.ResolveMedia(MediaRef{RelativePath: "../outside", Bytes: 1})
	if err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected sanitized traversal rejection, got %v", err)
	}
}

func TestResolveMediaDetectsSameSizeTampering(t *testing.T) {
	store := NewStore(t.TempDir())
	result, err := store.ImportDemo()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < result.Dataset.Len(); index++ {
		activity, err := result.Dataset.ActivityAt(index)
		if err != nil {
			t.Fatal(err)
		}
		if len(activity.Media) == 0 {
			continue
		}
		ref := activity.Media[0]
		path, err := result.Dataset.ResolveMedia(ref)
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		_ = file.Close()
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			t.Fatalf("read media fixture: %v", err)
		}
		data[len(data)-1] ^= 0xff
		file, err = os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(data); err != nil {
			t.Fatal(err)
		}
		_ = file.Close()
		if _, err := result.Dataset.ResolveMedia(ref); err == nil || !strings.Contains(err.Error(), "integrity") {
			t.Fatalf("expected media integrity rejection, got %v", err)
		}
		return
	}
	t.Fatal("demo dataset had no media")
}

func TestArchiveNamesCanonicalizeRepeatedSeparatorsAndRejectCollisions(t *testing.T) {
	if clean, ok := safeArchiveName("assets//ignored.js"); !ok || clean != "assets/ignored.js" {
		t.Fatalf("official repeated separator was not canonicalized: %q %v", clean, ok)
	}
	if _, ok := safeArchiveName("data/../account.js"); ok {
		t.Fatal("traversal path was accepted")
	}

	archivePath := filepath.Join(t.TempDir(), "collision.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for _, name := range []string{"data/account.js", "data//account.js"} {
		if err := writeDemoEntry(archive, name, []byte("[]")); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = NewStore(t.TempDir()).ImportZIP(archivePath, false)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected canonical collision rejection, got %v", err)
	}
}

func TestDatasetIdentityIsDeterministic(t *testing.T) {
	store := NewStore(t.TempDir())
	first, err := store.ImportDemo()
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.ImportDemo()
	if err != nil {
		t.Fatal(err)
	}
	if first.Summary.DatasetID != second.Summary.DatasetID {
		t.Fatalf("dataset identity changed: %q != %q", first.Summary.DatasetID, second.Summary.DatasetID)
	}
	listed, err := store.List()
	if err != nil || len(listed) != 1 {
		t.Fatalf("duplicate deterministic import created extra dataset: %#v %v", listed, err)
	}
}

func TestClassificationPrecedenceAndUTF16QuoteOffsets(t *testing.T) {
	account := AccountIdentity{ID: "42", Username: "owner"}
	text := "😀 reply https://t.co/q"
	start := len(utf16.Encode([]rune("😀 reply ")))
	end := len(utf16.Encode([]rune(text)))
	activity, reason := normalizeTweet(rawTweet{
		ID: "123", CreatedAt: "Thu Jun 12 12:00:00 +0000 2025", FullText: text,
		ReplyStatusID: "100", ReplyUsername: "friend",
		Entities: rawEntities{URLs: []rawURL{{ExpandedURL: "https://x.com/quoted/status/200", Indices: []archiveInt{archiveInt(start), archiveInt(end)}}}},
	}, account, "tweet")
	if reason != "" || activity.Type != ActivityReply || activity.ReplyTo == nil || activity.Quote == nil || activity.Quote.PostID != "200" {
		t.Fatalf("reply/quote normalization mismatch: %#v reason=%q", activity, reason)
	}

	repost, reason := normalizeTweet(rawTweet{
		ID: "124", CreatedAt: "Thu Jun 12 12:00:00 +0000 2025", FullText: "RT @friend: hello",
		ReplyStatusID: "101", Entities: rawEntities{Mentions: []rawMention{{Username: "friend", Indices: []archiveInt{3, 10}}}},
	}, account, "tweet")
	if reason != "" || repost.Type != ActivityRepost || repost.RepostOf == nil {
		t.Fatalf("repost precedence mismatch: %#v reason=%q", repost, reason)
	}
}

func TestStoreRejectsNonDirectoryArchiveRoot(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	if err := os.WriteFile(store.Root(), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ImportDemo(); err == nil {
		t.Fatal("unsafe X archive root was accepted")
	}
}

func TestRawTweetAcceptsStringNumericEntityMetadata(t *testing.T) {
	var envelope rawEnvelope
	err := json.Unmarshal([]byte(`{"tweet":{"id_str":"123","created_at":"Thu Jun 12 12:00:00 +0000 2025","full_text":"photo","entities":{"user_mentions":[{"screen_name":"friend","indices":["3","10"]}],"media":[{"id_str":"700","type":"photo","media_url_https":"https://example.invalid/image.jpg","sizes":{"large":{"w":"640","h":"480"}}}]}}}`), &envelope)
	if err != nil {
		t.Fatalf("string numeric metadata was rejected: %v", err)
	}
	if len(envelope.Tweet.Entities.Mentions) != 1 || envelope.Tweet.Entities.Mentions[0].Indices[0] != 3 || envelope.Tweet.Entities.Media[0].Sizes.Large.W != 640 {
		t.Fatalf("string numeric metadata was not normalized: %#v", envelope.Tweet.Entities)
	}
}

func BenchmarkDemoArchiveImport(b *testing.B) {
	archivePath := filepath.Join(b.TempDir(), "demo.zip")
	if err := WriteDemoZIP(archivePath); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		store := NewStore(filepath.Join(b.TempDir(), "workspace"))
		if err := os.MkdirAll(store.workspaceDir, 0o700); err != nil {
			b.Fatal(err)
		}
		if _, err := store.ImportZIP(archivePath, true); err != nil {
			b.Fatal(err)
		}
	}
}
