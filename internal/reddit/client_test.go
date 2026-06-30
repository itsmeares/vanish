package reddit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/itsmeares/vanish/internal/secretstore"
)

func TestClientMeUsesAuthenticatedOfficialAPIShape(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotUserAgent = r.Header.Get("User-Agent")
		writeJSON(t, w, map[string]any{
			"id":          "abc123",
			"name":        "test_user",
			"created_utc": float64(1767225600),
		})
	}))
	defer server.Close()

	client, err := NewClient(secretstore.MustSecret("access-secret"), ClientOptions{
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	user, err := client.Me(context.Background())
	if err != nil {
		t.Fatalf("Me returned error: %v", err)
	}
	if gotPath != "/api/v1/me" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer access-secret" {
		t.Fatalf("Authorization header = %q", gotAuth)
	}
	if gotUserAgent == "" {
		t.Fatal("User-Agent header was not set")
	}
	if user.ID != "abc123" || user.Name != "test_user" {
		t.Fatalf("user = %#v", user)
	}
	wantCreated := time.Unix(1767225600, 0).UTC()
	if !user.CreatedAt.Equal(wantCreated) {
		t.Fatalf("CreatedAt = %s, want %s", user.CreatedAt, wantCreated)
	}
}

func TestClientGetJSONRejectsNonPathEndpoint(t *testing.T) {
	client, err := NewClient(secretstore.MustSecret("access-secret"), ClientOptions{})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	err = client.GetJSON(context.Background(), "https://example.com/api/v1/me", nil, &map[string]any{})
	if err == nil {
		t.Fatal("GetJSON returned nil error for absolute URL")
	}
}

func TestListingParamsValuesClampLimit(t *testing.T) {
	values := (ListingParams{
		Limit:  250,
		After:  " t3_after ",
		Before: " t3_before ",
		Count:  25,
	}).Values()

	want := url.Values{
		"limit":  []string{"100"},
		"after":  []string{"t3_after"},
		"before": []string{"t3_before"},
		"count":  []string{"25"},
	}
	for key, wantValues := range want {
		if got := values[key]; len(got) != 1 || got[0] != wantValues[0] {
			t.Fatalf("%s = %#v, want %#v", key, got, wantValues)
		}
	}

	defaults := (ListingParams{}).Values()
	if defaults.Get("limit") != "25" {
		t.Fatalf("default limit = %q", defaults.Get("limit"))
	}
}
