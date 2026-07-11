package instagram

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

// ImportResult is the safe, platform-independent result of reading one local
// Instagram export ZIP. It contains normalized activity items plus warnings for
// files or records Vanish chose not to trust yet.
type ImportResult struct {
	Items    []domain.ActivityItem
	Summary  ImportSummary
	Warnings ImportWarningSummary
}

// ImportSummary gives the TUI a compact count view without having to know the
// parser's internal details.
type ImportSummary struct {
	Total     int
	Likes     int
	Comments  int
	Following int
	Followers int
	Skipped   int
}

type activityKind string

const (
	kindUnknown      activityKind = ""
	kindLike         activityKind = "liked_post"
	kindComment      activityKind = "comment"
	kindFollowing    activityKind = "following"
	kindFollower     activityKind = "follower"
	kindLikedComment activityKind = "liked_comment"
)

// ImportZIP parses supported Instagram activity from a local export ZIP. It
// never talks to Instagram or any other network service.
func ImportZIP(zipPath string) (ImportResult, error) {
	zipPath = strings.Trim(strings.TrimSpace(zipPath), `"'`)
	if zipPath == "" {
		return ImportResult{}, fmt.Errorf("instagram export zip path is required")
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ImportResult{}, fmt.Errorf("open instagram export zip: file not found")
		}
		return ImportResult{}, fmt.Errorf("open instagram export zip: unreadable or invalid ZIP")
	}
	defer reader.Close()

	importedAt := time.Now().UTC()
	var result ImportResult
	var warnings warningCollector
	skippedFiles := 0

	for _, file := range reader.File {
		if file.FileInfo().IsDir() || !strings.EqualFold(path.Ext(file.Name), ".json") {
			continue
		}

		if isChunkedJSONFile(path.Base(file.Name), "liked_posts") {
			items, handled, parseErr := parseLikedPostsArrayFile(file, &importedAt, &warnings)
			if parseErr != nil {
				warnings.add(file.Name, "liked-post", "malformed JSON", WarningUnitFile, 1, nil)
				skippedFiles++
				continue
			}
			if handled {
				if len(items) == 0 {
					skippedFiles++
				}
				result.Items = appendOwnedItems(result.Items, items)
				continue
			}
		}

		raw, err := readJSONFile(file)
		if err != nil {
			warnings.add(file.Name, warningCategoryForFileName(file.Name), "malformed JSON", WarningUnitFile, 1, nil)
			skippedFiles++
			continue
		}

		items, handled := parseJSONActivityFile(file.Name, raw, &importedAt, &warnings)
		if !handled || len(items) == 0 {
			skippedFiles++
		}
		result.Items = appendOwnedItems(result.Items, items)
	}

	result.Warnings = warnings.finish()
	result.Summary = summarize(result.Items, skippedFiles)
	return result, nil
}

func appendOwnedItems(existing, added []domain.ActivityItem) []domain.ActivityItem {
	if len(existing) == 0 {
		return added
	}
	return append(existing, added...)
}

type likedPostArrayRecord struct {
	Title          string                     `json:"title"`
	Username       string                     `json:"username"`
	Href           string                     `json:"href"`
	URL            string                     `json:"url"`
	Timestamp      any                        `json:"timestamp"`
	StringListData []likedPostArrayStringData `json:"string_list_data"`
	LabelValues    []likedPostArrayLabelValue `json:"label_values"`
}

type likedPostArrayStringData struct {
	Value     string `json:"value"`
	Text      string `json:"text"`
	Href      string `json:"href"`
	URL       string `json:"url"`
	Timestamp any    `json:"timestamp"`
}

type likedPostArrayLabelValue struct {
	Href string `json:"href"`
}

func parseLikedPostsArrayFile(file *zip.File, importedAt *time.Time, warnings *warningCollector) ([]domain.ActivityItem, bool, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, true, err
	}
	defer rc.Close()

	decoder := json.NewDecoder(bufio.NewReaderSize(rc, 64*1024))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return nil, true, err
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != '[' {
		return nil, false, nil
	}

	items := make([]domain.ActivityItem, 0)
	recordCount := 0
	for decoder.More() {
		recordCount++
		var record likedPostArrayRecord
		if err := decoder.Decode(&record); err != nil {
			var typeErr *json.UnmarshalTypeError
			if errors.As(err, &typeErr) {
				warnings.add(file.Name, "liked-post", "unsupported record shape", WarningUnitRecord, 1, func() string {
					return "non-object or incompatible liked-post record"
				})
				continue
			}
			return nil, true, err
		}

		item, ok := activityItemFromLikedPostArrayRecord(file.Name, record, importedAt)
		if !ok {
			warnings.add(file.Name, "liked-post", "unsupported target shape", WarningUnitRecord, 1, func() string {
				return "object{label_values:array,timestamp:number}"
			})
			continue
		}
		items = appendValidItem(items, file.Name, item, "liked-post", warnings)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, true, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, true, fmt.Errorf("unexpected trailing JSON value")
		}
		return nil, true, err
	}
	if recordCount == 0 {
		warnings.add(file.Name, "liked-post", "no supported records", WarningUnitFile, 1, nil)
	}
	return items, true, nil
}

func activityItemFromLikedPostArrayRecord(fileName string, record likedPostArrayRecord, importedAt *time.Time) (domain.ActivityItem, bool) {
	if record.LabelValues != nil {
		targetURL, ok := uniqueTrustedInstagramMediaURL(record.LabelValues)
		if !ok {
			return domain.ActivityItem{}, false
		}
		return newActivityItem(
			kindLike,
			fileName,
			targetURL,
			"",
			"",
			parseTimeValue(record.Timestamp),
			importedAt,
			nil,
			nil,
		), true
	}

	listData := firstLikedPostArrayStringData(record.StringListData)
	username := firstNonEmpty(listData.Value, record.Title, record.Username)
	targetURL := firstNonEmpty(listData.Href, record.Href, record.URL)
	targetID := firstNonEmpty(username, targetURL)
	if targetURL == "" && targetID == "" {
		return domain.ActivityItem{}, false
	}
	metadata := map[string]string{"instagram_kind": string(kindLike)}
	addIfNotEmpty(metadata, "username", username)
	return newActivityItem(
		kindLike,
		fileName,
		targetURL,
		targetID,
		username,
		firstTime(listData.Timestamp, parseTimeValue(record.Timestamp)),
		importedAt,
		metadata,
		nil,
	), true
}

func uniqueTrustedInstagramMediaURL(values []likedPostArrayLabelValue) (string, bool) {
	candidate := ""
	for _, value := range values {
		trusted, ok := trustedInstagramMediaURL(value.Href)
		if !ok {
			continue
		}
		if candidate == "" {
			candidate = trusted
			continue
		}
		if candidate != trusted {
			return "", false
		}
	}
	return candidate, candidate != ""
}

func trustedInstagramMediaURL(value string) (string, bool) {
	target, err := parseTrustedInstagramTarget(value)
	if err != nil || target.Kind == TargetProfile {
		return "", false
	}
	return target.URL, true
}

func firstLikedPostArrayStringData(entries []likedPostArrayStringData) extractedStringData {
	for _, entry := range entries {
		data := extractedStringData{
			Value:     firstNonEmpty(entry.Value, entry.Text),
			Href:      firstNonEmpty(entry.Href, entry.URL),
			Timestamp: parseTimeValue(entry.Timestamp),
		}
		if data.Value != "" || data.Href != "" || data.Timestamp != nil {
			return data
		}
	}
	return extractedStringData{}
}

func readJSONFile(file *zip.File) (any, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	var raw any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func parseJSONActivityFile(fileName string, raw any, importedAt *time.Time, warnings *warningCollector) ([]domain.ActivityItem, bool) {
	kind := detectKind(fileName, raw)
	switch kind {
	case kindLike:
		return parseLikes(fileName, raw, importedAt, warnings), true
	case kindComment:
		return parseComments(fileName, raw, importedAt, warnings), true
	case kindFollowing:
		return parseRelationships(fileName, raw, importedAt, kindFollowing, warnings), true
	case kindFollower:
		return parseRelationships(fileName, raw, importedAt, kindFollower, warnings), true
	case kindLikedComment:
		records := rootRecords(raw, "likes_comment_likes")
		if len(records) == 0 {
			warnings.add(fileName, "liked-comment", "unsupported activity category", WarningUnitFile, 1, nil)
		} else {
			warnings.add(fileName, "liked-comment", "unsupported activity category", WarningUnitRecord, len(records), nil)
		}
		return nil, true
	default:
		warnings.add(fileName, "instagram-json", "unsupported activity file", WarningUnitFile, 1, nil)
		return nil, false
	}
}

func detectKind(fileName string, raw any) activityKind {
	base := strings.ToLower(path.Base(fileName))

	switch {
	case hasTopLevelKey(raw, "relationships_following"):
		return kindFollowing
	case hasTopLevelKey(raw, "relationships_followers"):
		return kindFollower
	case hasTopLevelKey(raw, "likes_comment_likes"):
		return kindLikedComment
	case hasTopLevelKey(raw, "likes_media_likes"), hasTopLevelKey(raw, "media_likes"):
		return kindLike
	case hasTopLevelKey(raw, "comments_media_comments"), hasTopLevelKey(raw, "media_comments"):
		return kindComment
	case isChunkedJSONFile(base, "following"):
		return kindFollowing
	case isChunkedJSONFile(base, "followers"):
		return kindFollower
	case isChunkedJSONFile(base, "liked_posts"):
		return kindLike
	case isChunkedJSONFile(base, "post_comments"), isChunkedJSONFile(base, "comments"):
		return kindComment
	default:
		return kindUnknown
	}
}

func isChunkedJSONFile(base, stem string) bool {
	base = strings.ToLower(strings.TrimSpace(base))
	stem = strings.ToLower(strings.TrimSpace(stem))
	if base == stem+".json" {
		return true
	}
	prefix := stem + "_"
	if !strings.HasPrefix(base, prefix) || !strings.HasSuffix(base, ".json") {
		return false
	}
	chunk := strings.TrimSuffix(strings.TrimPrefix(base, prefix), ".json")
	if chunk == "" {
		return false
	}
	for _, char := range chunk {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func warningCategoryForFileName(fileName string) string {
	base := strings.ToLower(path.Base(fileName))
	switch {
	case isChunkedJSONFile(base, "liked_posts"):
		return "liked-post"
	case base == "liked_comments.json":
		return "liked-comment"
	case isChunkedJSONFile(base, "post_comments"), isChunkedJSONFile(base, "comments"):
		return "comment"
	case isChunkedJSONFile(base, "following"):
		return "following"
	case isChunkedJSONFile(base, "followers"):
		return "follower"
	default:
		return "instagram-json"
	}
}

func parseLikes(fileName string, raw any, importedAt *time.Time, warnings *warningCollector) []domain.ActivityItem {
	records := rootRecords(raw, "likes_media_likes", "media_likes", "liked_posts")
	if len(records) == 0 {
		warnings.add(fileName, "liked-post", "no supported records", WarningUnitFile, 1, nil)
		return nil
	}

	items := make([]domain.ActivityItem, 0, len(records))
	for _, record := range records {
		rec, ok := record.(map[string]any)
		if !ok {
			warnings.add(fileName, "liked-post", "unsupported record shape", WarningUnitRecord, 1, func() string {
				return structuralShape(record)
			})
			continue
		}

		listData := firstStringListData(rec)
		username := firstNonEmpty(listData.Value, stringField(rec, "title"), stringField(rec, "username"))
		targetURL := firstNonEmpty(listData.Href, stringField(rec, "href"), stringField(rec, "url"))
		targetID := firstNonEmpty(username, targetURL)
		occurredAt := firstTime(listData.Timestamp, parseTimeValue(rec["timestamp"]))

		if targetURL == "" && targetID == "" {
			warnings.add(fileName, "liked-post", "unsupported target shape", WarningUnitRecord, 1, func() string {
				return structuralShape(record)
			})
			continue
		}

		metadata := map[string]string{"instagram_kind": string(kindLike)}
		addIfNotEmpty(metadata, "username", username)

		item := newActivityItem(kindLike, fileName, targetURL, targetID, username, occurredAt, importedAt, metadata, nil)
		items = appendValidItem(items, fileName, item, "liked-post", warnings)
	}

	return items
}

func parseComments(fileName string, raw any, importedAt *time.Time, warnings *warningCollector) []domain.ActivityItem {
	records := rootRecords(raw, "comments_media_comments", "media_comments", "comments")
	if len(records) == 0 {
		warnings.add(fileName, "comment", "no supported records", WarningUnitFile, 1, nil)
		return nil
	}

	items := make([]domain.ActivityItem, 0, len(records))
	for _, record := range records {
		rec, ok := record.(map[string]any)
		if !ok {
			warnings.add(fileName, "comment", "unsupported record shape", WarningUnitRecord, 1, func() string {
				return structuralShape(record)
			})
			continue
		}

		listData := firstStringListData(rec)
		commentData := firstStringMapData(rec, "comment")
		ownerData := firstStringMapData(rec, "media owner", "owner", "username")

		commentText := firstNonEmpty(
			stringField(rec, "comment"),
			stringField(rec, "text"),
			commentData.Value,
		)
		textHash := ""
		var safeText *domain.SafeTextReference
		if commentText != "" {
			textHash = hashString(commentText)
			safeText = &domain.SafeTextReference{Hash: textHash, Preview: commentPreview(commentText)}
		}

		mediaOwner := firstNonEmpty(
			stringField(rec, "media_owner"),
			ownerData.Value,
			stringField(rec, "title"),
			listData.Value,
		)
		targetURL := firstNonEmpty(listData.Href, commentData.Href, stringField(rec, "href"), stringField(rec, "url"))
		targetID := firstNonEmpty(mediaOwner, shortHash(textHash), targetURL)
		occurredAt := firstTime(
			parseTimeValue(rec["timestamp"]),
			commentData.Timestamp,
			listData.Timestamp,
		)

		if targetURL == "" && targetID == "" {
			warnings.add(fileName, "comment", "unsupported target shape", WarningUnitRecord, 1, func() string {
				return structuralShape(record)
			})
			continue
		}

		metadata := map[string]string{"instagram_kind": string(kindComment)}
		addIfNotEmpty(metadata, "media_owner", mediaOwner)

		item := newActivityItem(kindComment, fileName, targetURL, targetID, mediaOwner, occurredAt, importedAt, metadata, safeText)
		items = appendValidItem(items, fileName, item, "comment", warnings)
	}

	return items
}

func commentPreview(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= 80 {
		return value
	}
	return strings.TrimSpace(string(runes[:79])) + "…"
}

func parseRelationships(fileName string, raw any, importedAt *time.Time, kind activityKind, warnings *warningCollector) []domain.ActivityItem {
	keys := []string{"relationships_following"}
	relationship := "following"
	if kind == kindFollower {
		keys = []string{"relationships_followers"}
		relationship = "follower"
	}

	records := rootRecords(raw, keys...)
	if len(records) == 0 {
		warnings.add(fileName, relationship, "no supported records", WarningUnitFile, 1, nil)
		return nil
	}

	items := make([]domain.ActivityItem, 0, len(records))
	for _, record := range records {
		rec, ok := record.(map[string]any)
		if !ok {
			warnings.add(fileName, relationship, "unsupported record shape", WarningUnitRecord, 1, func() string {
				return structuralShape(record)
			})
			continue
		}

		listData := firstStringListData(rec)
		username := firstNonEmpty(listData.Value, stringField(rec, "title"), stringField(rec, "username"))
		targetURL := firstNonEmpty(listData.Href, stringField(rec, "href"), stringField(rec, "url"))
		targetID := firstNonEmpty(username, targetURL)
		occurredAt := firstTime(listData.Timestamp, parseTimeValue(rec["timestamp"]))

		if targetURL == "" && targetID == "" {
			warnings.add(fileName, relationship, "unsupported target shape", WarningUnitRecord, 1, func() string {
				return structuralShape(record)
			})
			continue
		}

		metadata := map[string]string{
			"instagram_kind": string(kind),
			"relationship":   relationship,
		}
		addIfNotEmpty(metadata, "username", username)

		item := newActivityItem(kind, fileName, targetURL, targetID, username, occurredAt, importedAt, metadata, nil)
		items = appendValidItem(items, fileName, item, relationship, warnings)
	}

	return items
}

func appendValidItem(items []domain.ActivityItem, fileName string, item domain.ActivityItem, category string, warnings *warningCollector) []domain.ActivityItem {
	if err := item.Validate(); err != nil {
		warnings.add(fileName, category, "normalized item failed validation", WarningUnitRecord, 1, nil)
		return items
	}
	return append(items, item)
}

func newActivityItem(kind activityKind, fileName, targetURL, targetID, actor string, occurredAt, importedAt *time.Time, metadata map[string]string, safeText *domain.SafeTextReference) domain.ActivityItem {
	itemType := domain.ItemTypeFollow
	switch kind {
	case kindLike:
		itemType = domain.ItemTypeLike
	case kindComment:
		itemType = domain.ItemTypeComment
	}

	item := domain.ActivityItem{
		ID:         itemID(kind, fileName, targetURL, targetID, occurredAt, safeText),
		Platform:   domain.PlatformInstagram,
		Type:       itemType,
		TargetURL:  targetURL,
		TargetID:   targetID,
		Actor:      actor,
		OccurredAt: occurredAt,
		Source: domain.SourceMetadata{
			Name:       "instagram-export",
			ImportedAt: importedAt,
			FileName:   fileName,
		},
		Metadata: metadata,
		Text:     safeText,
	}

	return item
}

func itemID(kind activityKind, fileName, targetURL, targetID string, occurredAt *time.Time, safeText *domain.SafeTextReference) string {
	parts := []string{string(kind), fileName, targetURL, targetID}
	if occurredAt != nil {
		parts = append(parts, occurredAt.Format(time.RFC3339))
	}
	if safeText != nil {
		parts = append(parts, safeText.Hash)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "instagram:" + string(kind) + ":" + hex.EncodeToString(sum[:])[:16]
}

func summarize(items []domain.ActivityItem, skipped int) ImportSummary {
	summary := ImportSummary{
		Total:   len(items),
		Skipped: skipped,
	}

	for _, item := range items {
		switch item.Type {
		case domain.ItemTypeLike:
			summary.Likes++
		case domain.ItemTypeComment:
			summary.Comments++
		case domain.ItemTypeFollow:
			switch item.Metadata["relationship"] {
			case "follower":
				summary.Followers++
			default:
				summary.Following++
			}
		}
	}

	return summary
}

func rootRecords(raw any, keys ...string) []any {
	if records, ok := raw.([]any); ok {
		return records
	}

	obj, ok := raw.(map[string]any)
	if !ok {
		return nil
	}

	for _, key := range keys {
		if value, ok := lookupInsensitive(obj, key); ok {
			return valueRecords(value)
		}
	}

	if _, ok := lookupInsensitive(obj, "string_list_data"); ok {
		return []any{obj}
	}
	if _, ok := lookupInsensitive(obj, "string_map_data"); ok {
		return []any{obj}
	}

	return nil
}

func valueRecords(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case map[string]any:
		return []any{typed}
	default:
		return nil
	}
}

func hasTopLevelKey(raw any, key string) bool {
	obj, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	_, ok = lookupInsensitive(obj, key)
	return ok
}

type extractedStringData struct {
	Value     string
	Href      string
	Timestamp *time.Time
}

func firstStringListData(record map[string]any) extractedStringData {
	value, ok := lookupInsensitive(record, "string_list_data")
	if !ok {
		return extractedStringData{}
	}

	entries, ok := value.([]any)
	if !ok || len(entries) == 0 {
		return extractedStringData{}
	}

	for _, entry := range entries {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		data := extractedStringData{
			Value:     firstNonEmpty(stringField(entryMap, "value"), stringField(entryMap, "text")),
			Href:      firstNonEmpty(stringField(entryMap, "href"), stringField(entryMap, "url")),
			Timestamp: parseTimeValue(entryMap["timestamp"]),
		}
		if data.Value != "" || data.Href != "" || data.Timestamp != nil {
			return data
		}
	}

	return extractedStringData{}
}

func firstStringMapData(record map[string]any, names ...string) extractedStringData {
	value, ok := lookupInsensitive(record, "string_map_data")
	if !ok {
		return extractedStringData{}
	}

	entries, ok := value.(map[string]any)
	if !ok {
		return extractedStringData{}
	}

	for _, name := range names {
		for key, entry := range entries {
			if normalizeKey(key) != normalizeKey(name) {
				continue
			}
			entryMap, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			return extractedStringData{
				Value:     firstNonEmpty(stringField(entryMap, "value"), stringField(entryMap, "text")),
				Href:      firstNonEmpty(stringField(entryMap, "href"), stringField(entryMap, "url")),
				Timestamp: parseTimeValue(entryMap["timestamp"]),
			}
		}
	}

	return extractedStringData{}
}

func lookupInsensitive(obj map[string]any, key string) (any, bool) {
	if value, ok := obj[key]; ok {
		return value, true
	}

	want := normalizeKey(key)
	for candidate, value := range obj {
		if normalizeKey(candidate) == want {
			return value, true
		}
	}
	return nil, false
}

func stringField(obj map[string]any, key string) string {
	value, ok := lookupInsensitive(obj, key)
	if !ok {
		return ""
	}
	return stringValue(value)
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return strings.TrimSpace(typed.String())
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func firstTime(values ...*time.Time) *time.Time {
	for _, value := range values {
		if value != nil && !value.IsZero() {
			return value
		}
	}
	return nil
}

func parseTimeValue(value any) *time.Time {
	switch typed := value.(type) {
	case json.Number:
		if timestamp, err := typed.Int64(); err == nil {
			return unixTime(timestamp)
		}
	case float64:
		return unixTime(int64(typed))
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		if timestamp, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return unixTime(timestamp)
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z0700", "2006-01-02 15:04:05"} {
			if parsed, err := time.Parse(layout, trimmed); err == nil {
				utc := parsed.UTC()
				return &utc
			}
		}
	}
	return nil
}

func unixTime(timestamp int64) *time.Time {
	if timestamp <= 0 {
		return nil
	}
	if timestamp > 9999999999 {
		timestamp = timestamp / 1000
	}
	parsed := time.Unix(timestamp, 0).UTC()
	return &parsed
}

func addIfNotEmpty(metadata map[string]string, key, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		metadata[key] = value
	}
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func shortHash(hash string) string {
	hash = strings.TrimPrefix(hash, "sha256:")
	if len(hash) > 16 {
		return "comment:" + hash[:16]
	}
	if hash != "" {
		return "comment:" + hash
	}
	return ""
}

func normalizeKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, "-", "_")
	key = strings.ReplaceAll(key, " ", "_")
	return key
}
