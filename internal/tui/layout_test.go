package tui

import "testing"

func TestLayoutSpecClampsSmallTerminals(t *testing.T) {
	spec := layoutSpec(1, 1)

	for name, value := range map[string]int{
		"width":        spec.width,
		"height":       spec.height,
		"contentWidth": spec.contentWidth,
		"bodyHeight":   spec.bodyHeight,
		"listHeight":   spec.listHeight,
		"sidebarWidth": spec.sidebarWidth,
		"mainWidth":    spec.mainWidth,
		"detailWidth":  spec.detailWidth,
	} {
		if value <= 0 {
			t.Fatalf("expected %s to be positive, got %d", name, value)
		}
	}
	if !spec.narrow {
		t.Fatalf("expected tiny terminal to use narrow layout")
	}
}

func TestTruncateMiddleKeepsBothEnds(t *testing.T) {
	got := truncateMiddle("C:/very/long/path/to/vanish-plan.json", 20)
	if got != "C:/very/l...lan.json" {
		t.Fatalf("unexpected middle truncation: %q", got)
	}
}

func TestCompactCount(t *testing.T) {
	for _, tc := range []struct {
		count int
		want  string
	}{
		{count: 42, want: "42"},
		{count: 1200, want: "1.2k"},
		{count: 10500, want: "10k"},
	} {
		if got := compactCount(tc.count); got != tc.want {
			t.Fatalf("compactCount(%d) = %q, want %q", tc.count, got, tc.want)
		}
	}
}
