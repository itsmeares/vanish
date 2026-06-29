package platform_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlatformDocsStateCurrentSupportAndBoundaries(t *testing.T) {
	text := readRepoFile(t, "docs", "platforms.md")
	for _, want := range []string{
		"| Instagram Export | prototype |",
		"| Reddit | planned |",
		"Apply / execution",
		"no | no | no",
		"does not delete platform content or apply account changes",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected docs/platforms.md to contain %q, got:\n%s", want, text)
		}
	}
}

func TestReadmeLinksPlatformDocsAndAvoidsOverstatingSupport(t *testing.T) {
	text := readRepoFile(t, "README.md")
	for _, want := range []string{
		"[docs/platforms.md](docs/platforms.md)",
		"Instagram Export prototype",
		"Reddit import, account connection, or scanning",
		"only in the platform selector",
		"does not delete platform content or apply account changes",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected README.md to contain %q, got:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Reddit, X, YouTube, or other platform integrations.") {
		t.Fatalf("expected README.md not to use old unsupported-platform wording")
	}
}

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()

	path := filepath.Join(append([]string{"..", ".."}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
