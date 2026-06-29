package secretstore

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestSecretRedactsStringGoStringAndJSON(t *testing.T) {
	secret := MustSecret("super-sensitive-value")

	values := []string{
		fmt.Sprint(secret),
		fmt.Sprintf("%#v", secret),
	}
	encoded, err := json.Marshal(secret)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	values = append(values, string(encoded))

	for _, value := range values {
		if strings.Contains(value, secret.Reveal()) {
			t.Fatalf("secret leaked in rendered value %q", value)
		}
		if !strings.Contains(value, "redacted") {
			t.Fatalf("expected redacted marker in %q", value)
		}
	}
}

func TestRedditRefreshKeyUsesStableNonSecretAccount(t *testing.T) {
	key := RedditRefreshKey(" Test_User ")
	if key.Service != "vanish/reddit" || key.Account != "refresh:test_user" {
		t.Fatalf("unexpected reddit key: %#v", key)
	}
	if err := key.validate(); err != nil {
		t.Fatalf("key validation returned error: %v", err)
	}
}
