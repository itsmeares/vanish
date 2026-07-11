package instagram

import (
	"strings"
	"testing"
)

func TestSanitizeCommentPreview(t *testing.T) {
	raw := "\x1b[31mHello\x1b[0m\nworld\x00\t café 👋\x07"
	if got, want := SanitizeCommentPreview(raw), "Hello world café 👋"; got != want {
		t.Fatalf("SanitizeCommentPreview() = %q, want %q", got, want)
	}

	long := strings.Repeat("界", 90)
	got := SanitizeCommentPreview(long)
	if len([]rune(got)) != commentPreviewLimit || !strings.HasSuffix(got, "…") {
		t.Fatalf("long preview was not limited to %d runes: %q", commentPreviewLimit, got)
	}
}

func TestNormalizeUsername(t *testing.T) {
	for _, value := range []string{"demo_user", "@demo.user", "Name123"} {
		if username, ok := NormalizeUsername(value); !ok || username != strings.TrimPrefix(value, "@") {
			t.Fatalf("valid username %q normalized to %q ok=%t", value, username, ok)
		}
	}
	for _, value := range []string{"", "bad user", "bad/user", "ümlaut", "\x1b[31mbad", "direct", strings.Repeat("a", 31)} {
		if username, ok := NormalizeUsername(value); ok || username != "" {
			t.Fatalf("invalid username %q normalized to %q ok=%t", value, username, ok)
		}
	}
}
