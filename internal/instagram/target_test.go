package instagram

import (
	"testing"

	"github.com/itsmeares/vanish/internal/domain"
)

func TestValidateCleanupTarget(t *testing.T) {
	tests := []struct {
		name       string
		actionType domain.ActionType
		value      string
		targetID   string
		wantKind   TargetKind
		wantURL    string
	}{
		{"profile", domain.ActionUnfollow, "https://instagram.com/demo.user/?igsh=tracking", "", TargetProfile, "https://www.instagram.com/demo.user/"},
		{"profile fallback", domain.ActionUnfollow, "", "@demo_user", TargetProfile, "https://www.instagram.com/demo_user/"},
		{"post", domain.ActionUnlike, "https://www.instagram.com/p/ABC_123/?utm_source=test#frag", "", TargetPost, "https://www.instagram.com/p/ABC_123/"},
		{"reel", domain.ActionUnlike, "https://instagram.com/reel/ABC-123/", "", TargetReel, "https://www.instagram.com/reel/ABC-123/"},
		{"tv comment", domain.ActionDeleteComment, "https://www.instagram.com/tv/ABC123/", "", TargetTV, "https://www.instagram.com/tv/ABC123/"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target, err := ValidateCleanupTarget(test.actionType, test.value, test.targetID)
			if err != nil {
				t.Fatalf("ValidateCleanupTarget: %v", err)
			}
			if target.Kind != test.wantKind || target.URL != test.wantURL {
				t.Fatalf("target = %#v, want kind=%q url=%q", target, test.wantKind, test.wantURL)
			}
		})
	}
}

func TestValidateCleanupTargetRejectsUnsafeURLs(t *testing.T) {
	values := []string{
		"http://www.instagram.com/p/ABC/",
		"https://evil.example/p/ABC/",
		"https://instagram.com.evil.example/p/ABC/",
		"https://user:pass@www.instagram.com/p/ABC/",
		"https://www.instagram.com:443/p/ABC/",
		"https://www.instagram.com/p/ABC/extra",
		"https://www.instagram.com/%70/ABC/",
		"https://www.instagram.com/p/",
		"//www.instagram.com/p/ABC/",
		"not a URL",
	}
	for _, value := range values {
		if _, err := ValidateCleanupTarget(domain.ActionUnlike, value, ""); err == nil {
			t.Fatalf("expected unsafe URL rejection: %q", value)
		}
	}
	if _, err := ValidateCleanupTarget(domain.ActionDeletePost, "https://www.instagram.com/p/ABC/", ""); err == nil {
		t.Fatal("expected unsupported action rejection")
	}
	if _, err := ValidateCleanupTarget(domain.ActionUnfollow, "https://www.instagram.com/p/ABC/", ""); err == nil {
		t.Fatal("expected media URL rejection for unfollow")
	}
	if _, err := ValidateCleanupTarget(domain.ActionUnfollow, "https://www.instagram.com/p/", ""); err == nil {
		t.Fatal("expected reserved profile path rejection")
	}
}
