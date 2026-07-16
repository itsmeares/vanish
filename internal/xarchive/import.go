package xarchive

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/itsmeares/vanish/internal/localdata"
)

const (
	maxArchiveBytes       = int64(32 << 30)
	maxEntries            = 250_000
	maxActivities         = 5_000_000
	maxPostEntryBytes     = uint64(2 << 30)
	maxAccountEntryBytes  = uint64(2 << 20)
	maxRawRecordBytes     = 8 << 20
	maxTextBytes          = 1 << 20
	maxNormalizedRecord   = 2 << 20
	maxEntitiesPerRecord  = 10_000
	maxMediaPerActivity   = 64
	maxMediaItemBytes     = uint64(1 << 30)
	maxRetainedMediaBytes = int64(20 << 30)
	maxCompressionRatio   = uint64(200)
)

var supportedPostFiles = []struct {
	name    string
	wrapper string
	kind    string
	media   string
}{
	{"data/tweets.js", "window.YTD.tweets.part0", "tweet", "data/tweets_media/"},
	{"data/community-tweet.js", "window.YTD.community_tweet.part0", "community_tweet", "data/community_tweet_media/"},
}

type archiveFiles struct {
	account *zip.File
	posts   map[string]*zip.File
	media   map[string]*zip.File
}

type rawEnvelope struct {
	Tweet rawTweet `json:"tweet"`
}

type rawTweet struct {
	CreatedAt        string      `json:"created_at"`
	FullText         string      `json:"full_text"`
	ID               string      `json:"id_str"`
	ReplyStatusID    string      `json:"in_reply_to_status_id_str"`
	ReplyUsername    string      `json:"in_reply_to_screen_name"`
	SourceStatusID   string      `json:"source_status_id_str"`
	Entities         rawEntities `json:"entities"`
	ExtendedEntities rawEntities `json:"extended_entities"`
}

type rawEntities struct {
	URLs     []rawURL     `json:"urls"`
	Mentions []rawMention `json:"user_mentions"`
	Media    []rawMedia   `json:"media"`
}

type rawURL struct {
	ExpandedURL string       `json:"expanded_url"`
	Indices     []archiveInt `json:"indices"`
}

type rawMention struct {
	Username string       `json:"screen_name"`
	Indices  []archiveInt `json:"indices"`
}

type rawMedia struct {
	ID        string       `json:"id_str"`
	Type      string       `json:"type"`
	MediaURL  string       `json:"media_url_https"`
	Sizes     rawSizes     `json:"sizes"`
	VideoInfo rawVideoInfo `json:"video_info"`
}

type rawSizes struct {
	Large rawDimensions `json:"large"`
	Small rawDimensions `json:"small"`
}

type rawDimensions struct {
	W archiveInt `json:"w"`
	H archiveInt `json:"h"`
}
type rawVideoInfo struct {
	Duration archiveInt        `json:"duration_millis"`
	Variants []rawVideoVariant `json:"variants"`
}
type rawVideoVariant struct {
	Bitrate     archiveInt `json:"bitrate"`
	ContentType string     `json:"content_type"`
	URL         string     `json:"url"`
}

type archiveInt int64

func (value *archiveInt) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if text == "null" || text == `""` {
		*value = 0
		return nil
	}
	if strings.HasPrefix(text, `"`) {
		var decoded string
		if err := json.Unmarshal(data, &decoded); err != nil {
			return err
		}
		text = strings.TrimSpace(decoded)
	}
	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return err
	}
	*value = archiveInt(parsed)
	return nil
}

type datasetWriter struct {
	stage      string
	posts      *os.File
	entries    []IndexEntry
	recordSums []string
	seen       map[string]struct{}
	mediaBytes int64
	counts     ActivityCounts
	warnings   warningCollector
}

func (s *Store) ImportZIP(zipPath string, demo bool) (result ImportResult, err error) {
	if s == nil || s.workspaceDir == "" || s.workspaceDir == "." {
		return result, errors.New("local workspace is unavailable")
	}
	lease, err := localdata.TryUse(s.workspaceDir)
	if err != nil {
		return result, err
	}
	defer lease.Close()
	info, err := os.Lstat(filepath.Clean(strings.TrimSpace(zipPath)))
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return result, errors.New("X archive ZIP is unavailable")
	}
	if info.Size() <= 0 || info.Size() > maxArchiveBytes {
		return result, errors.New("X archive ZIP size is outside supported limits")
	}
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return result, errors.New("X archive ZIP could not be opened")
	}
	defer reader.Close()
	files, err := preflightArchive(reader.File)
	if err != nil {
		return result, err
	}
	account, err := parseAccount(files.account)
	if err != nil {
		return result, err
	}
	if err := validateDirectory(s.workspaceDir); err != nil {
		return result, errors.New("local workspace path is unsafe")
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return result, err
	}
	if err := validateDirectory(s.root); err != nil {
		return result, err
	}
	importLock, locked, err := s.tryImportLock()
	if err != nil {
		return result, err
	}
	if !locked {
		return result, localdata.ErrActive
	}
	defer importLock.Close()
	if err := s.cleanupInterruptedImports(); err != nil {
		return result, err
	}
	stage, err := os.MkdirTemp(s.root, ".import-")
	if err != nil {
		return result, err
	}
	keepStage := false
	defer func() {
		if !keepStage {
			_ = os.RemoveAll(stage)
		}
	}()
	posts, err := os.OpenFile(filepath.Join(stage, postsName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return result, err
	}
	w := &datasetWriter{stage: stage, posts: posts, seen: make(map[string]struct{})}
	for _, spec := range supportedPostFiles {
		file := files.posts[spec.name]
		if file == nil {
			continue
		}
		if err := streamPostFile(file, spec, files.media, account, w); err != nil {
			_ = posts.Close()
			return result, err
		}
	}
	if err := posts.Sync(); err != nil {
		_ = posts.Close()
		return result, err
	}
	if err := posts.Close(); err != nil {
		return result, err
	}
	if w.counts.Total == 0 {
		return result, errors.New("X archive contains no supported current posts")
	}
	sort.Slice(w.entries, func(i, j int) bool {
		if w.entries[i].OccurredAt.Equal(w.entries[j].OccurredAt) {
			return w.entries[i].ID < w.entries[j].ID
		}
		return w.entries[i].OccurredAt.After(w.entries[j].OccurredAt)
	})
	indexBytes, indexDigest, err := writeIndex(filepath.Join(stage, indexName), w.entries)
	if err != nil {
		return result, err
	}
	postInfo, err := os.Stat(filepath.Join(stage, postsName))
	if err != nil {
		return result, err
	}
	sort.Strings(w.recordSums)
	datasetHash := sha256.New()
	_, _ = io.WriteString(datasetHash, account.ID+"\n")
	for _, digest := range w.recordSums {
		_, _ = io.WriteString(datasetHash, digest+"\n")
	}
	datasetID := hex.EncodeToString(datasetHash.Sum(nil))
	warnings := w.warnings.finish()
	manifest := DatasetManifest{
		FormatVersion: DatasetFormatVersion, DatasetID: datasetID, Account: account, ImportedAt: time.Now().UTC(), Demo: demo,
		Counts: w.counts, WarningCount: warnings.Total, PostBytes: postInfo.Size(), IndexBytes: indexBytes,
		IndexSHA256: indexDigest, MediaBytes: w.mediaBytes,
	}
	if err := writeJSONFile(filepath.Join(stage, manifestName), manifest); err != nil {
		return result, err
	}
	final := filepath.Join(s.root, datasetID)
	if err := os.Rename(stage, final); err != nil {
		if statErr := validateDirectory(final); statErr != nil {
			return result, err
		}
		_ = os.RemoveAll(stage)
	} else {
		keepStage = true
	}
	dataset, err := s.Open(datasetID)
	if err != nil {
		return result, err
	}
	return ImportResult{Dataset: dataset, Summary: dataset.Summary(), Warnings: warnings}, nil
}

func preflightArchive(entries []*zip.File) (archiveFiles, error) {
	if len(entries) == 0 || len(entries) > maxEntries {
		return archiveFiles{}, errors.New("X archive entry count is outside supported limits")
	}
	result := archiveFiles{posts: make(map[string]*zip.File), media: make(map[string]*zip.File)}
	seen := make(map[string]struct{}, len(entries))
	logicalSeen := make(map[string]struct{})
	supportedRoot := ""
	supportedRootSet := false
	for _, file := range entries {
		name, ok := safeArchiveName(file.Name)
		if !ok {
			return archiveFiles{}, errors.New("X archive contains an unsafe path")
		}
		fold := strings.ToLower(name)
		if _, exists := seen[fold]; exists {
			return archiveFiles{}, errors.New("X archive contains duplicate paths")
		}
		seen[fold] = struct{}{}
		if file.Mode()&os.ModeSymlink != 0 {
			return archiveFiles{}, errors.New("X archive contains a symbolic link")
		}
		logical := logicalArchiveName(name)
		limit := uint64(0)
		switch logical {
		case "data/account.js":
			result.account = file
			limit = maxAccountEntryBytes
		case "data/tweets.js", "data/community-tweet.js":
			result.posts[logical] = file
			limit = maxPostEntryBytes
		default:
			if strings.HasPrefix(logical, "data/tweets_media/") || strings.HasPrefix(logical, "data/community_tweet_media/") {
				result.media[logical] = file
				limit = maxMediaItemBytes
			}
		}
		if limit > 0 {
			logicalFold := strings.ToLower(logical)
			if _, exists := logicalSeen[logicalFold]; exists {
				return archiveFiles{}, errors.New("X archive contains colliding supported paths")
			}
			logicalSeen[logicalFold] = struct{}{}
			root := archiveRoot(name)
			if supportedRootSet && root != supportedRoot {
				return archiveFiles{}, errors.New("X archive contains mixed supported roots")
			}
			supportedRoot = root
			supportedRootSet = true
			if file.UncompressedSize64 > limit {
				return archiveFiles{}, errors.New("X archive supported entry exceeds its size limit")
			}
			compressed := file.CompressedSize64
			minimumCompressed := (file.UncompressedSize64 + maxCompressionRatio - 1) / maxCompressionRatio
			if compressed < minimumCompressed {
				return archiveFiles{}, errors.New("X archive supported entry exceeds its compression limit")
			}
		}
	}
	if result.account == nil || result.posts["data/tweets.js"] == nil {
		return archiveFiles{}, errors.New("X archive is missing required account or current-post data")
	}
	return result, nil
}

func safeArchiveName(name string) (string, bool) {
	if name == "" || strings.Contains(name, "\\") || strings.ContainsRune(name, '\x00') || strings.HasPrefix(name, "/") {
		return "", false
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "." || segment == ".." {
			return "", false
		}
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	first := strings.SplitN(clean, "/", 2)[0]
	if strings.Contains(first, ":") {
		return "", false
	}
	return clean, true
}

func logicalArchiveName(name string) string {
	if strings.HasPrefix(name, "data/") || name == "data" {
		return name
	}
	if slash := strings.IndexByte(name, '/'); slash >= 0 {
		return name[slash+1:]
	}
	return name
}

func archiveRoot(name string) string {
	if strings.HasPrefix(name, "data/") || name == "data" {
		return ""
	}
	root, _, _ := strings.Cut(name, "/")
	return root
}

func parseAccount(file *zip.File) (AccountIdentity, error) {
	data, err := readZipBounded(file, maxAccountEntryBytes)
	if err != nil {
		return AccountIdentity{}, errors.New("X archive account data could not be read")
	}
	payload, err := jsPayload(data, "window.YTD.account.part0")
	if err != nil {
		return AccountIdentity{}, errors.New("X archive account data has an unsupported format")
	}
	var rows []struct {
		Account struct {
			ID          string `json:"accountId"`
			Username    string `json:"username"`
			DisplayName string `json:"accountDisplayName"`
		} `json:"account"`
	}
	if json.Unmarshal(payload, &rows) != nil || len(rows) == 0 {
		return AccountIdentity{}, errors.New("X archive account data is invalid")
	}
	account := AccountIdentity{ID: strings.TrimSpace(rows[0].Account.ID), Username: strings.TrimPrefix(strings.TrimSpace(rows[0].Account.Username), "@"), DisplayName: strings.TrimSpace(rows[0].Account.DisplayName)}
	if !validPostID(account.ID) || account.Username == "" || len(account.Username) > 256 || len(account.DisplayName) > 512 {
		return AccountIdentity{}, errors.New("X archive account identity is incomplete")
	}
	return account, nil
}

func streamPostFile(file *zip.File, spec struct{ name, wrapper, kind, media string }, mediaFiles map[string]*zip.File, account AccountIdentity, w *datasetWriter) error {
	reader, err := file.Open()
	if err != nil {
		return errors.New("X archive post data could not be read")
	}
	defer reader.Close()
	buffered := bufio.NewReaderSize(reader, 64*1024)
	prefix, err := readBoundedUntil(buffered, '=', 256)
	if err != nil || strings.TrimSpace(strings.TrimSuffix(string(prefix), "=")) != spec.wrapper {
		return errors.New("X archive post data has an unsupported wrapper")
	}
	first, err := readNonSpaceByte(buffered)
	if err != nil || first != '[' {
		return errors.New("X archive post data is invalid")
	}
	firstRecord := true
	for {
		raw, done, err := readBoundedArrayObject(buffered, firstRecord, maxRawRecordBytes)
		if err != nil {
			if errors.Is(err, errRawRecordTooLarge) {
				return errors.New("X archive post record exceeds its size limit")
			}
			return errors.New("X archive post data is invalid")
		}
		if done {
			break
		}
		if w.counts.Total >= maxActivities {
			return errors.New("X archive activity limit exceeded")
		}
		firstRecord = false
		var envelope rawEnvelope
		if json.Unmarshal(raw, &envelope) != nil {
			w.warnings.add(spec.kind, "record", "invalid record skipped", WarningRecord, 1)
			continue
		}
		if len(envelope.Tweet.Entities.URLs)+len(envelope.Tweet.Entities.Mentions)+len(envelope.Tweet.Entities.Media)+len(envelope.Tweet.ExtendedEntities.Media) > maxEntitiesPerRecord {
			w.warnings.add(spec.kind, "record", "entity limit exceeded", WarningRecord, 1)
			continue
		}
		activity, reason := normalizeTweet(envelope.Tweet, account, spec.kind)
		if reason != "" {
			w.warnings.add(spec.kind, "record", reason, WarningRecord, 1)
			continue
		}
		if _, duplicate := w.seen[activity.ID]; duplicate {
			w.warnings.add(spec.kind, "record", "duplicate post skipped", WarningRecord, 1)
			continue
		}
		w.seen[activity.ID] = struct{}{}
		activity.Media = w.extractMedia(envelope.Tweet, spec.media, mediaFiles)
		if err := w.append(activity); err != nil {
			return err
		}
	}
	tail, err := io.ReadAll(io.LimitReader(buffered, 17))
	if err != nil || len(tail) > 16 || (strings.TrimSpace(string(tail)) != "" && strings.TrimSpace(string(tail)) != ";") {
		return errors.New("X archive post data has unexpected trailing content")
	}
	return nil
}

var errRawRecordTooLarge = errors.New("raw X archive record is too large")

func readBoundedArrayObject(reader *bufio.Reader, first bool, limit int) ([]byte, bool, error) {
	marker, err := readNonSpaceByte(reader)
	if err != nil {
		return nil, false, err
	}
	if !first {
		if marker == ']' {
			return nil, true, nil
		}
		if marker != ',' {
			return nil, false, errors.New("missing array separator")
		}
		marker, err = readNonSpaceByte(reader)
		if err != nil {
			return nil, false, err
		}
	} else if marker == ']' {
		return nil, true, nil
	}
	if marker != '{' {
		return nil, false, errors.New("array element is not an object")
	}

	capacity := limit
	if capacity > 64*1024 {
		capacity = 64 * 1024
	}
	raw := make([]byte, 1, capacity)
	raw[0] = marker
	stack := []byte{'}'}
	inString := false
	escaped := false
	for len(stack) > 0 {
		value, err := reader.ReadByte()
		if err != nil {
			return nil, false, err
		}
		if len(raw) >= limit {
			return nil, false, errRawRecordTooLarge
		}
		raw = append(raw, value)
		if inString {
			switch {
			case escaped:
				escaped = false
			case value == '\\':
				escaped = true
			case value == '"':
				inString = false
			}
			continue
		}
		switch value {
		case '"':
			inString = true
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if stack[len(stack)-1] != value {
				return nil, false, errors.New("mismatched JSON delimiter")
			}
			stack = stack[:len(stack)-1]
		}
	}
	return raw, false, nil
}

func readNonSpaceByte(reader *bufio.Reader) (byte, error) {
	for {
		value, err := reader.ReadByte()
		if err != nil {
			return 0, err
		}
		switch value {
		case ' ', '\t', '\r', '\n':
			continue
		default:
			return value, nil
		}
	}
}

func readBoundedUntil(reader *bufio.Reader, delimiter byte, limit int) ([]byte, error) {
	value := make([]byte, 0, limit)
	for len(value) < limit {
		current, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		value = append(value, current)
		if current == delimiter {
			return value, nil
		}
	}
	return nil, errors.New("bounded delimiter was not found")
}

func normalizeTweet(tweet rawTweet, account AccountIdentity, sourceKind string) (Activity, string) {
	id := strings.TrimSpace(tweet.ID)
	text := tweet.FullText
	if !validPostID(id) || len(text) > maxTextBytes {
		return Activity{}, "missing or oversized required field"
	}
	when, err := time.Parse(time.RubyDate, strings.TrimSpace(tweet.CreatedAt))
	if err != nil {
		return Activity{}, "invalid timestamp"
	}
	sum := sha256.Sum256([]byte(account.ID + "\x00" + id))
	activity := Activity{ID: hex.EncodeToString(sum[:]), SourcePostID: id, SourceKind: sourceKind, Type: ActivityPost, OccurredAt: when.UTC(), Text: text}
	quote := terminalQuote(text, tweet.Entities.URLs)
	if quote != nil {
		activity.Quote = quote
	}
	if username, ok := structuralRepost(text, tweet.Entities.Mentions); ok {
		activity.Type = ActivityRepost
		activity.RepostOf = &RelatedPost{Username: username}
	} else if validPostID(strings.TrimSpace(tweet.ReplyStatusID)) {
		activity.Type = ActivityReply
		activity.ReplyTo = &RelatedPost{PostID: strings.TrimSpace(tweet.ReplyStatusID), Username: boundedUsername(tweet.ReplyUsername)}
	} else if quote != nil {
		activity.Type = ActivityQuotePost
	}
	return activity, ""
}

func structuralRepost(text string, mentions []rawMention) (string, bool) {
	if !strings.HasPrefix(text, "RT @") {
		return "", false
	}
	colon := strings.Index(text, ": ")
	if colon < 5 {
		return "", false
	}
	username := text[4:colon]
	if len(username) > 256 {
		return "", false
	}
	for _, mention := range mentions {
		if strings.EqualFold(mention.Username, username) && len(mention.Indices) == 2 && mention.Indices[0] == archiveInt(3) {
			return mention.Username, true
		}
	}
	return "", false
}

func terminalQuote(text string, urls []rawURL) *RelatedPost {
	units := utf16.Encode([]rune(text))
	for _, entity := range urls {
		if len(entity.ExpandedURL) > 4096 {
			continue
		}
		if len(entity.Indices) != 2 || int64(entity.Indices[1]) != int64(len(units)) || entity.Indices[0] < 0 || entity.Indices[0] >= entity.Indices[1] {
			continue
		}
		parsed, err := url.Parse(strings.TrimSpace(entity.ExpandedURL))
		if err != nil {
			continue
		}
		host := strings.ToLower(parsed.Hostname())
		if host != "x.com" && host != "www.x.com" && host != "mobile.x.com" && host != "twitter.com" && host != "www.twitter.com" && host != "mobile.twitter.com" {
			continue
		}
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) == 3 && parts[1] == "status" && len(parts[0]) <= 256 && validPostID(parts[2]) {
			return &RelatedPost{PostID: parts[2], Username: parts[0]}
		}
		if len(parts) == 4 && parts[0] == "i" && parts[1] == "web" && parts[2] == "status" && validPostID(parts[3]) {
			return &RelatedPost{PostID: parts[3]}
		}
	}
	return nil
}

func (w *datasetWriter) append(activity Activity) error {
	record, err := json.Marshal(activity)
	if err != nil {
		return err
	}
	if len(record) > maxNormalizedRecord {
		return errors.New("X archive normalized record exceeds its size limit")
	}
	offset, err := w.posts.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	if _, err := w.posts.Write(append(record, '\n')); err != nil {
		return err
	}
	sum := sha256.Sum256(record)
	digest := hex.EncodeToString(sum[:])
	w.recordSums = append(w.recordSums, digest)
	relevant := ""
	for _, relation := range []*RelatedPost{activity.RepostOf, activity.ReplyTo, activity.Quote} {
		if relation != nil && relation.Username != "" {
			relevant = relation.Username
			break
		}
	}
	w.entries = append(w.entries, IndexEntry{ID: activity.ID, SourcePostID: activity.SourcePostID, Type: activity.Type, OccurredAt: activity.OccurredAt, RelevantAccount: relevant, Media: len(activity.Media), Offset: offset, Length: int64(len(record)), RecordSHA256: digest})
	w.counts.Total++
	w.counts.Media += len(activity.Media)
	switch activity.Type {
	case ActivityPost:
		w.counts.Posts++
	case ActivityReply:
		w.counts.Replies++
	case ActivityQuotePost:
		w.counts.QuotePosts++
	case ActivityRepost:
		w.counts.Reposts++
	}
	return nil
}

func (w *datasetWriter) extractMedia(tweet rawTweet, mediaDir string, files map[string]*zip.File) []MediaRef {
	entities := tweet.ExtendedEntities.Media
	if len(entities) == 0 {
		entities = tweet.Entities.Media
	}
	if len(entities) > maxMediaPerActivity {
		w.warnings.add("media", "record", "additional media references omitted", WarningMedia, len(entities)-maxMediaPerActivity)
		entities = entities[:maxMediaPerActivity]
	}
	refs := make([]MediaRef, 0, len(entities))
	for _, media := range entities {
		file, kind, mime, bitrate := selectMediaFile(tweet.ID, tweet.SourceStatusID, mediaDir, media, files)
		if file == nil {
			w.warnings.add("media", "file", "referenced media unavailable", WarningMedia, 1)
			continue
		}
		ref, err := w.extractMediaFile(file, kind, mime)
		if err != nil {
			w.warnings.add("media", "file", "referenced media rejected", WarningMedia, 1)
			continue
		}
		dims := media.Sizes.Large
		if dims.W == 0 {
			dims = media.Sizes.Small
		}
		ref.Width, ref.Height, ref.DurationMillis, ref.Bitrate = int(dims.W), int(dims.H), int64(media.VideoInfo.Duration), bitrate
		refs = append(refs, ref)
	}
	return refs
}

func selectMediaFile(postID, sourceStatusID, mediaDir string, media rawMedia, files map[string]*zip.File) (*zip.File, MediaKind, string, int64) {
	kind := MediaPhoto
	if media.Type == "video" {
		kind = MediaVideo
	}
	if media.Type == "animated_gif" {
		kind = MediaAnimation
	}
	type candidate struct {
		url, mime string
		bitrate   int64
	}
	candidates := []candidate{}
	if kind == MediaPhoto {
		candidates = append(candidates, candidate{media.MediaURL, "image/jpeg", 0})
	} else {
		for _, variant := range media.VideoInfo.Variants {
			if variant.ContentType == "video/mp4" {
				candidates = append(candidates, candidate{variant.URL, variant.ContentType, int64(variant.Bitrate)})
			}
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].bitrate > candidates[j].bitrate })
	}
	for _, candidate := range candidates {
		if len(candidate.url) > 8192 {
			continue
		}
		parsed, err := url.Parse(candidate.url)
		if err != nil {
			continue
		}
		base := path.Base(parsed.Path)
		if base == "." || base == "/" || base == "" {
			continue
		}
		for _, prefix := range []string{postID, sourceStatusID, media.ID} {
			if !validPostID(strings.TrimSpace(prefix)) {
				continue
			}
			if file := files[mediaDir+prefix+"-"+base]; file != nil {
				return file, kind, candidate.mime, candidate.bitrate
			}
		}
	}
	return nil, kind, "", 0
}

func (w *datasetWriter) extractMediaFile(file *zip.File, kind MediaKind, declaredMIME string) (MediaRef, error) {
	if file.UncompressedSize64 > maxMediaItemBytes {
		return MediaRef{}, errors.New("media too large")
	}
	if w.mediaBytes+int64(file.UncompressedSize64) > maxRetainedMediaBytes {
		return MediaRef{}, errors.New("media total too large")
	}
	reader, err := file.Open()
	if err != nil {
		return MediaRef{}, err
	}
	defer reader.Close()
	tmp, err := os.CreateTemp(w.stage, ".media-")
	if err != nil {
		return MediaRef{}, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(reader, int64(maxMediaItemBytes)+1))
	syncErr := tmp.Sync()
	closeErr := tmp.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || written != int64(file.UncompressedSize64) || written > int64(maxMediaItemBytes) {
		return MediaRef{}, errors.New("media read failed")
	}
	tempReader, err := os.Open(tmpPath)
	if err != nil {
		return MediaRef{}, err
	}
	head := make([]byte, 16)
	headBytes, readErr := io.ReadFull(tempReader, head)
	_ = tempReader.Close()
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return MediaRef{}, readErr
	}
	head = head[:headBytes]
	mime, ext, ok := mediaFormat(head, kind, declaredMIME)
	if !ok {
		return MediaRef{}, errors.New("unsupported media format")
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	rel := path.Join("media", digest[:2], digest+ext)
	dest := filepath.Join(w.stage, filepath.FromSlash(rel))
	if _, err := os.Stat(dest); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return MediaRef{}, err
		}
		if err := os.Rename(tmpPath, dest); err != nil {
			return MediaRef{}, err
		}
		w.mediaBytes += written
	}
	return MediaRef{Kind: kind, MIME: mime, RelativePath: rel, SHA256: digest, Bytes: written}, nil
}

func mediaFormat(head []byte, kind MediaKind, declared string) (string, string, bool) {
	if kind == MediaPhoto && len(head) >= 3 && bytes.Equal(head[:3], []byte{0xff, 0xd8, 0xff}) {
		return "image/jpeg", ".jpg", true
	}
	if kind == MediaPhoto && len(head) >= 8 && bytes.Equal(head[:8], []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}) {
		return "image/png", ".png", true
	}
	if (kind == MediaVideo || kind == MediaAnimation) && len(head) >= 8 && string(head[4:8]) == "ftyp" && declared == "video/mp4" {
		return "video/mp4", ".mp4", true
	}
	return "", "", false
}

func writeIndex(path string, entries []IndexEntry) (int64, string, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, "", err
	}
	hash := sha256.New()
	writer := io.MultiWriter(file, hash)
	var total int64
	for _, entry := range entries {
		line, err := json.Marshal(entry)
		if err != nil {
			_ = file.Close()
			return 0, "", err
		}
		line = append(line, '\n')
		n, err := writer.Write(line)
		total += int64(n)
		if err != nil {
			_ = file.Close()
			return 0, "", err
		}
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return 0, "", err
	}
	if err := file.Close(); err != nil {
		return 0, "", err
	}
	return total, hex.EncodeToString(hash.Sum(nil)), nil
}

func readZipBounded(file *zip.File, limit uint64) ([]byte, error) {
	if file.UncompressedSize64 > limit {
		return nil, errors.New("entry too large")
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	if err != nil || uint64(len(data)) > limit || uint64(len(data)) != file.UncompressedSize64 {
		return nil, errors.New("entry read failed")
	}
	return data, nil
}

func jsPayload(data []byte, wrapper string) ([]byte, error) {
	equals := bytes.IndexByte(data, '=')
	if equals < 0 || strings.TrimSpace(string(data[:equals])) != wrapper {
		return nil, errors.New("wrapper mismatch")
	}
	payload := bytes.TrimSpace(data[equals+1:])
	payload = bytes.TrimSuffix(payload, []byte(";"))
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 {
		return nil, errors.New("missing payload")
	}
	return payload, nil
}

func validPostID(value string) bool { _, err := strconv.ParseUint(value, 10, 64); return err == nil }

func boundedUsername(value string) string {
	value = strings.TrimPrefix(strings.TrimSpace(value), "@")
	if len(value) > 256 {
		return ""
	}
	return value
}
