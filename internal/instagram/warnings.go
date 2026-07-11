package instagram

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	maxImportWarningGroups           = 128
	maxImportWarningExamplesPerGroup = 2
	maxImportWarningExampleLength    = 160
	maxImportWarningSourceLength     = 240
	maxImportWarningLabelLength      = 80
)

type WarningUnit string

const (
	WarningUnitRecord  WarningUnit = "record"
	WarningUnitFile    WarningUnit = "file"
	WarningUnitWarning WarningUnit = "warning"
)

type ImportWarningSummary struct {
	Total  int
	Groups []ImportWarningGroup
}

type ImportWarningGroup struct {
	SourceFile string
	Category   string
	Reason     string
	Unit       WarningUnit
	Count      int
	Examples   []string
}

type warningGroupKey struct {
	SourceFile string
	Category   string
	Reason     string
	Unit       WarningUnit
}

type warningCollector struct {
	summary ImportWarningSummary
	indexes map[warningGroupKey]int
	samples map[int]int
	sources map[string]string
}

func (collector *warningCollector) add(sourceFile, category, reason string, unit WarningUnit, count int, example func() string) {
	if count <= 0 {
		return
	}
	collector.summary.Total += count
	if collector.indexes == nil {
		collector.indexes = make(map[warningGroupKey]int)
	}

	key := warningGroupKey{
		SourceFile: collector.source(sourceFile),
		Category:   truncateWarningText(strings.TrimSpace(category), maxImportWarningLabelLength),
		Reason:     truncateWarningText(strings.TrimSpace(reason), maxImportWarningLabelLength),
		Unit:       unit,
	}
	index, ok := collector.indexes[key]
	if !ok {
		if len(collector.summary.Groups) >= maxImportWarningGroups-1 {
			key = warningGroupKey{
				Category: "other",
				Reason:   "additional warning groups omitted",
				Unit:     WarningUnitWarning,
			}
			index, ok = collector.indexes[key]
		}
		if !ok {
			index = len(collector.summary.Groups)
			collector.indexes[key] = index
			collector.summary.Groups = append(collector.summary.Groups, ImportWarningGroup{
				SourceFile: key.SourceFile,
				Category:   key.Category,
				Reason:     key.Reason,
				Unit:       key.Unit,
			})
		}
	}

	group := &collector.summary.Groups[index]
	group.Count += count
	if example == nil || len(group.Examples) >= maxImportWarningExamplesPerGroup || group.Reason == "additional warning groups omitted" {
		return
	}
	if collector.samples == nil {
		collector.samples = make(map[int]int)
	}
	if collector.samples[index] >= maxImportWarningExamplesPerGroup {
		return
	}
	collector.samples[index]++
	value := boundedWarningExample(example())
	if value == "" {
		return
	}
	for _, existing := range group.Examples {
		if existing == value {
			return
		}
	}
	group.Examples = append(group.Examples, value)
}

func (collector *warningCollector) source(source string) string {
	if collector.sources != nil {
		if cached, ok := collector.sources[source]; ok {
			return cached
		}
	}
	bounded := boundedWarningSource(source)
	if collector.sources == nil {
		collector.sources = make(map[string]string)
	}
	if len(collector.sources) < maxImportWarningGroups {
		collector.sources[source] = bounded
	}
	return bounded
}

func (collector *warningCollector) finish() ImportWarningSummary {
	sort.SliceStable(collector.summary.Groups, func(i, j int) bool {
		left := collector.summary.Groups[i]
		right := collector.summary.Groups[j]
		if left.Reason == "additional warning groups omitted" {
			return false
		}
		if right.Reason == "additional warning groups omitted" {
			return true
		}
		leftKey := left.SourceFile + "\x00" + left.Category + "\x00" + left.Reason + "\x00" + string(left.Unit)
		rightKey := right.SourceFile + "\x00" + right.Category + "\x00" + right.Reason + "\x00" + string(right.Unit)
		return leftKey < rightKey
	})
	return collector.summary
}

func boundedWarningSource(source string) string {
	source = strings.TrimSpace(strings.ReplaceAll(source, "\\", "/"))
	if source == "" {
		return ""
	}
	base := path.Base(path.Clean("/" + source))
	if warningCategoryForFileName(base) == "instagram-json" {
		return "other Instagram JSON"
	}
	return truncateWarningText(base, maxImportWarningSourceLength)
}

func boundedWarningExample(example string) string {
	example = strings.Join(strings.Fields(example), " ")
	return truncateWarningText(example, maxImportWarningExampleLength)
}

func truncateWarningText(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}

func structuralShape(value any) string {
	object, ok := value.(map[string]any)
	if !ok {
		return jsonShapeType(value)
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 8 {
		keys = keys[:8]
	}
	fields := make([]string, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, fmt.Sprintf("%s:%s", safeStructuralKey(key), jsonShapeType(object[key])))
	}
	return "object{" + strings.Join(fields, ",") + "}"
}

func safeStructuralKey(key string) string {
	normalized := normalizeKey(key)
	switch normalized {
	case "comments", "comments_media_comments", "dict", "fbid", "href", "label", "label_values", "likes_comment_likes", "likes_media_likes", "media", "media_comments", "media_likes", "media_owner", "relationships_followers", "relationships_following", "string_list_data", "string_map_data", "text", "timestamp", "title", "url", "username", "value":
		return normalized
	default:
		return "field"
	}
}

func jsonShapeType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case bool:
		return "bool"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "number"
	}
}
