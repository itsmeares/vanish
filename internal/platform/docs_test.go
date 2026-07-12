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
		"| Instagram Export | Supported | Unsupported | Supported | Supported | Supported | No-op only | Unsupported |",
		"| Reddit | Unsupported | Prototype, access pending | Supported | Prototype | Unsupported | No-op only | Unsupported |",
		"execution never changes platform content",
		"Assisted cleanup",
		"Automatic cleanup",
		"approval has not been granted",
		"provider-specific routing but performs no platform changes",
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
		"Vanish is an open-source, local-first app for finding, reviewing, and cleaning",
		"Vanish currently runs as an interactive terminal application.",
		"Instagram export data and assisted manual cleanup",
		"Offer a read-only Reddit official API planner prototype.",
		"Vanish never performs those Instagram changes itself.",
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
