package instagram

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

const commentPreviewLimit = 80

func SanitizeCommentPreview(value string) string {
	value = ansi.Strip(value)
	value = strings.Map(func(r rune) rune {
		if !unicode.IsControl(r) {
			return r
		}
		switch r {
		case '\t', '\n', '\r':
			return ' '
		default:
			return -1
		}
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= commentPreviewLimit {
		return value
	}
	return strings.TrimSpace(string(runes[:commentPreviewLimit-1])) + "…"
}
