package xarchive

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestPreflightRejectsSupportedPathCollisionsAfterRootNormalization(t *testing.T) {
	tests := []struct {
		name    string
		entries []string
	}{
		{"account", []string{"root-a/data/account.js", "root-b/data/account.js", "root-a/data/tweets.js"}},
		{"posts", []string{"root-a/data/account.js", "root-a/data/tweets.js", "root-b/data/tweets.js"}},
		{"media", []string{"root-a/data/account.js", "root-a/data/tweets.js", "root-a/data/tweets_media/1-photo.jpg", "root-b/data/tweets_media/1-photo.jpg"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			archivePath := writeStoredZIP(t, test.entries, func(string) []byte { return []byte("{}") })
			archive, err := zip.OpenReader(archivePath)
			if err != nil {
				t.Fatal(err)
			}
			defer archive.Close()
			_, err = preflightArchive(archive.File)
			if err == nil || err.Error() != "X archive contains colliding supported paths" {
				t.Fatalf("expected normalized %s collision rejection, got %v", test.name, err)
			}
		})
	}
}

func TestImportRejectsMixedSupportedArchiveRootsBeforeRetention(t *testing.T) {
	tests := []struct {
		name    string
		entries []string
	}{
		{"account and posts", []string{"root-a/data/account.js", "root-b/data/tweets.js"}},
		{"posts and media", []string{"root-a/data/account.js", "root-a/data/tweets.js", "root-b/data/tweets_media/1-photo.jpg"}},
		{"three roots", []string{"root-a/data/account.js", "root-b/data/tweets.js", "root-c/data/tweets_media/1-photo.jpg"}},
		{"rooted and rootless", []string{"data/account.js", "root-a/data/tweets.js", "root-a/data/tweets_media/1-photo.jpg"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			archivePath := writeStoredZIP(t, test.entries, func(string) []byte { return []byte("invalid by design") })
			store := NewStore(t.TempDir())
			_, err := store.ImportZIP(archivePath, false)
			if err == nil || err.Error() != "X archive contains mixed supported roots" {
				t.Fatalf("expected mixed-root rejection, got %v", err)
			}
			if _, err := os.Lstat(store.Root()); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("mixed-root import retained storage: %v", err)
			}
		})
	}
}

func TestImportAcceptsRootlessAndSingleSharedRootLayouts(t *testing.T) {
	rootless := filepath.Join(t.TempDir(), "rootless.zip")
	if err := WriteDemoZIP(rootless); err != nil {
		t.Fatal(err)
	}
	sharedRoot := rewriteSyntheticZIPPaths(t, rootless, func(name string) string { return "archive-root/" + name })
	for _, test := range []struct {
		name string
		path string
	}{{"official rootless", rootless}, {"single shared root", sharedRoot}} {
		t.Run(test.name, func(t *testing.T) {
			result, err := NewStore(t.TempDir()).ImportZIP(test.path, false)
			if err != nil {
				t.Fatalf("import %s layout: %v", test.name, err)
			}
			if result.Summary.Counts.Total != 6 || result.Summary.Counts.Media != 2 {
				t.Fatalf("unexpected %s import counts: %#v", test.name, result.Summary.Counts)
			}
		})
	}
}

func TestImportRejectsOversizedRawArrayElementBeforeDecode(t *testing.T) {
	account := `window.YTD.account.part0 = [{"account":{"accountId":"42","username":"synthetic","accountDisplayName":"Synthetic"}}];`
	oversized := `window.YTD.tweets.part0 = [{"tweet":{"id_str":"1","created_at":"Thu Jun 12 12:00:00 +0000 2025","full_text":"small"},"ignored":"` + strings.Repeat("x", maxRawRecordBytes) + `"}];`
	archivePath := writeStoredZIP(t, []string{"data/account.js", "data/tweets.js"}, func(name string) []byte {
		if name == "data/account.js" {
			return []byte(account)
		}
		return []byte(oversized)
	})
	store := NewStore(t.TempDir())
	_, err := store.ImportZIP(archivePath, false)
	if err == nil || err.Error() != "X archive post record exceeds its size limit" {
		t.Fatalf("expected bounded-record rejection, got %v", err)
	}
}

func TestInterruptedImportStagesAreCleanedBeforeListing(t *testing.T) {
	store := NewStore(t.TempDir())
	stage := filepath.Join(store.Root(), ".import-interrupted")
	if err := os.MkdirAll(filepath.Join(stage, "media"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{postsName: "hidden post text", "media/private.bin": "hidden media"} {
		if err := os.WriteFile(filepath.Join(stage, filepath.FromSlash(name)), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.List(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(stage); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("interrupted stage retained: %v", err)
	}
}

func TestInterruptedImportCleanupWaitsForImportLock(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := os.MkdirAll(store.Root(), 0o700); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(store.Root(), ".import-active")
	if err := os.Mkdir(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	guard, locked, err := store.tryImportLock()
	if err != nil || !locked {
		t.Fatalf("acquire import lock: locked=%v err=%v", locked, err)
	}
	if _, err := store.List(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(stage); err != nil {
		t.Fatalf("active stage was removed: %v", err)
	}
	if err := guard.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(stage); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("released stage retained: %v", err)
	}
}

func TestInterruptedImportCleanupRejectsSymlinkWithoutFollowingIt(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := os.MkdirAll(store.Root(), 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	privatePath := filepath.Join(outside, "outside.txt")
	if err := os.WriteFile(privatePath, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(store.Root(), ".import-link")
	if err := os.Symlink(outside, stage); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := store.List(); err == nil || !strings.Contains(err.Error(), "staging path is unsafe") {
		t.Fatalf("expected staging symlink rejection, got %v", err)
	}
	if data, err := os.ReadFile(privatePath); err != nil || string(data) != "outside" {
		t.Fatalf("cleanup followed staging symlink: %q %v", data, err)
	}
}

func TestNormalizeTweetPreservesStoredControlBytesForSafeRendering(t *testing.T) {
	text := "visible\x1b[31mred\x1b[0m\x00tail"
	activity, reason := normalizeTweet(rawTweet{
		ID: "123", CreatedAt: time.Date(2025, 6, 12, 12, 0, 0, 0, time.UTC).Format(time.RubyDate), FullText: text,
	}, AccountIdentity{ID: "42", Username: "owner"}, "tweet")
	if reason != "" || activity.Text != text {
		t.Fatalf("stored post text changed: reason=%q text=%q", reason, activity.Text)
	}
}

func writeStoredZIP(t *testing.T, names []string, content func(string) []byte) string {
	t.Helper()
	archivePath := filepath.Join(t.TempDir(), "synthetic.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for _, name := range names {
		entry, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(content(name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return archivePath
}

func rewriteSyntheticZIPPaths(t *testing.T, source string, rewrite func(string) string) string {
	t.Helper()
	input, err := zip.OpenReader(source)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	destination := filepath.Join(t.TempDir(), "rewritten-synthetic.zip")
	file, err := os.Create(destination)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for _, entry := range input.File {
		opened, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(opened)
		closeErr := opened.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("read synthetic entry: %v %v", err, closeErr)
		}
		output, err := archive.CreateHeader(&zip.FileHeader{Name: rewrite(entry.Name), Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := output.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return destination
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
