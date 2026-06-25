package instagram

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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
	Warnings []string
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
	kindUnknown   activityKind = ""
	kindLike      activityKind = "liked_post"
	kindComment   activityKind = "comment"
	kindFollowing activityKind = "following"
	kindFollower  activityKind = "follower"
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
		return ImportResult{}, fmt.Errorf("open instagram export zip: %w", err)
	}
	defer reader.Close()

	importedAt := time.Now().UTC()
	var result ImportResult

	for _, file := range reader.File {
		if file.FileInfo().IsDir() || !strings.EqualFold(path.Ext(file.Name), ".json") {
			continue
		}

		raw, err := readJSONFile(file)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: malformed JSON skipped: %v", file.Name, err))
			result.Summary.Skipped++
			continue
		}

		items, warnings, handled := parseJSONActivityFile(file.Name, raw, importedAt)
		result.Warnings = append(result.Warnings, warnings...)
		if !handled || len(items) == 0 {
			result.Summary.Skipped++
		}
		result.Items = append(result.Items, items...)
	}

	result.Summary = summarize(result.Items, result.Summary.Skipped)
	return result, nil
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

func parseJSONActivityFile(fileName string, raw any, importedAt time.Time) ([]domain.ActivityItem, []string, bool) {
	kind := detectKind(fileName, raw)
	switch kind {
	case kindLike:
		return parseLikes(fileName, raw, importedAt)
	case kindComment:
		return parseComments(fileName, raw, importedAt)
	case kindFollowing:
		return parseRelationships(fileName, raw, importedAt, kindFollowing)
	case kindFollower:
		return parseRelationships(fileName, raw, importedAt, kindFollower)
	default:
		return nil, []string{fmt.Sprintf("%s: unsupported Instagram JSON skipped", fileName)}, false
	}
}

func detectKind(fileName string, raw any) activityKind {
	base := strings.ToLower(path.Base(fileName))

	switch {
	case hasTopLevelKey(raw, "relationships_following"):
		return kindFollowing
	case hasTopLevelKey(raw, "relationships_followers"):
		return kindFollower
	case hasTopLevelKey(raw, "likes_media_likes"), hasTopLevelKey(raw, "media_likes"):
		return kindLike
	case hasTopLevelKey(raw, "comments_media_comments"), hasTopLevelKey(raw, "media_comments"):
		return kindComment
	case strings.Contains(base, "following") && !strings.Contains(base, "follower"):
		return kindFollowing
	case strings.Contains(base, "follower"):
		return kindFollower
	case strings.Contains(base, "liked") || strings.Contains(base, "like"):
		return kindLike
	case strings.Contains(base, "comment"):
		return kindComment
	default:
		return kindUnknown
	}
}

func parseLikes(fileName string, raw any, importedAt time.Time) ([]domain.ActivityItem, []string, bool) {
	records := rootRecords(raw, "likes_media_likes", "media_likes", "liked_posts")
	if len(records) == 0 {
		return nil, []string{fmt.Sprintf("%s: liked posts file had no supported records", fileName)}, true
	}

	var items []domain.ActivityItem
	var warnings []string
	for i, record := range records {
		rec, ok := record.(map[string]any)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%s: liked post record %d skipped: unsupported shape", fileName, i+1))
			continue
		}

		listData := firstStringListData(rec)
		username := firstNonEmpty(listData.Value, stringField(rec, "title"), stringField(rec, "username"))
		targetURL := firstNonEmpty(listData.Href, stringField(rec, "href"), stringField(rec, "url"))
		targetID := firstNonEmpty(username, targetURL)
		occurredAt := firstTime(listData.Timestamp, parseTimeValue(rec["timestamp"]))

		if targetURL == "" && targetID == "" {
			warnings = append(warnings, fmt.Sprintf("%s: liked post record %d skipped: no safe target", fileName, i+1))
			continue
		}

		metadata := map[string]string{"instagram_kind": string(kindLike)}
		addIfNotEmpty(metadata, "username", username)

		item := newActivityItem(kindLike, fileName, targetURL, targetID, username, occurredAt, importedAt, metadata, nil)
		items, warnings = appendValidItem(items, warnings, fileName, i+1, item)
	}

	return items, warnings, true
}

func parseComments(fileName string, raw any, importedAt time.Time) ([]domain.ActivityItem, []string, bool) {
	records := rootRecords(raw, "comments_media_comments", "media_comments", "comments")
	if len(records) == 0 {
		return nil, []string{fmt.Sprintf("%s: comments file had no supported records", fileName)}, true
	}

	var items []domain.ActivityItem
	var warnings []string
	for i, record := range records {
		rec, ok := record.(map[string]any)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%s: comment record %d skipped: unsupported shape", fileName, i+1))
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
			safeText = &domain.SafeTextReference{Hash: textHash}
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
			warnings = append(warnings, fmt.Sprintf("%s: comment record %d skipped: no safe target", fileName, i+1))
			continue
		}

		metadata := map[string]string{"instagram_kind": string(kindComment)}
		addIfNotEmpty(metadata, "media_owner", mediaOwner)

		item := newActivityItem(kindComment, fileName, targetURL, targetID, mediaOwner, occurredAt, importedAt, metadata, safeText)
		items, warnings = appendValidItem(items, warnings, fileName, i+1, item)
	}

	return items, warnings, true
}

func parseRelationships(fileName string, raw any, importedAt time.Time, kind activityKind) ([]domain.ActivityItem, []string, bool) {
	keys := []string{"relationships_following"}
	relationship := "following"
	if kind == kindFollower {
		keys = []string{"relationships_followers"}
		relationship = "follower"
	}

	records := rootRecords(raw, keys...)
	if len(records) == 0 {
		return nil, []string{fmt.Sprintf("%s: %s file had no supported records", fileName, relationship)}, true
	}

	var items []domain.ActivityItem
	var warnings []string
	for i, record := range records {
		rec, ok := record.(map[string]any)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%s: %s record %d skipped: unsupported shape", fileName, relationship, i+1))
			continue
		}

		listData := firstStringListData(rec)
		username := firstNonEmpty(listData.Value, stringField(rec, "title"), stringField(rec, "username"))
		targetURL := firstNonEmpty(listData.Href, stringField(rec, "href"), stringField(rec, "url"))
		targetID := firstNonEmpty(username, targetURL)
		occurredAt := firstTime(listData.Timestamp, parseTimeValue(rec["timestamp"]))

		if targetURL == "" && targetID == "" {
			warnings = append(warnings, fmt.Sprintf("%s: %s record %d skipped: no safe target", fileName, relationship, i+1))
			continue
		}

		metadata := map[string]string{
			"instagram_kind": string(kind),
			"relationship":   relationship,
		}
		addIfNotEmpty(metadata, "username", username)

		item := newActivityItem(kind, fileName, targetURL, targetID, username, occurredAt, importedAt, metadata, nil)
		items, warnings = appendValidItem(items, warnings, fileName, i+1, item)
	}

	return items, warnings, true
}

func appendValidItem(items []domain.ActivityItem, warnings []string, fileName string, recordNumber int, item domain.ActivityItem) ([]domain.ActivityItem, []string) {
	if err := item.Validate(); err != nil {
		warnings = append(warnings, fmt.Sprintf("%s: record %d skipped: %v", fileName, recordNumber, err))
		return items, warnings
	}
	return append(items, item), warnings
}

func newActivityItem(kind activityKind, fileName, targetURL, targetID, actor string, occurredAt *time.Time, importedAt time.Time, metadata map[string]string, safeText *domain.SafeTextReference) domain.ActivityItem {
	itemType := domain.ItemTypeFollow
	switch kind {
	case kindLike:
		itemType = domain.ItemTypeLike
	case kindComment:
		itemType = domain.ItemTypeComment
	}

	sourceImportedAt := importedAt
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
			ImportedAt: &sourceImportedAt,
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
