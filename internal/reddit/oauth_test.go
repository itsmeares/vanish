package reddit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/itsmeares/vanish/internal/secretstore"
)

func TestClientIDFromEnv(t *testing.T) {
	t.Setenv(ClientIDEnv, "  reddit-client  ")

	got, err := ClientIDFromEnv()
	if err != nil {
		t.Fatalf("ClientIDFromEnv returned error: %v", err)
	}
	if got != "reddit-client" {
		t.Fatalf("ClientIDFromEnv = %q", got)
	}
}

func TestAuthURLUsesInstalledAppReadOnlyScopes(t *testing.T) {
	oauth := newTestOAuth(t, nil)

	authURL, err := oauth.AuthURL("state-123")
	if err != nil {
		t.Fatalf("AuthURL returned error: %v", err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse AuthURL: %v", err)
	}
	query := parsed.Query()
	assertQuery(t, query, "client_id", "test-client")
	assertQuery(t, query, "response_type", "code")
	assertQuery(t, query, "state", "state-123")
	assertQuery(t, query, "redirect_uri", DefaultRedirectURI)
	assertQuery(t, query, "duration", "permanent")
	assertQuery(t, query, "scope", "identity history")
	for _, forbidden := range []string{"edit", "save", "vote", "submit", "privatemessages", "modconfig"} {
		if strings.Contains(query.Get("scope"), forbidden) {
			t.Fatalf("AuthURL requested forbidden scope %q in %q", forbidden, query.Get("scope"))
		}
	}
}

func TestExchangeCodeSendsInstalledAppTokenRequest(t *testing.T) {
	var gotPath string
	var gotForm url.Values
	var gotAuth string
	var gotUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUserAgent = r.Header.Get("User-Agent")
		gotAuth = r.Header.Get("Authorization")
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		gotForm = r.PostForm
		writeJSON(t, w, map[string]any{
			"access_token":  "access-value",
			"refresh_token": "refresh-value",
			"expires_in":    3600,
			"scope":         "identity history",
			"token_type":    "bearer",
		})
	}))
	defer server.Close()

	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	oauth := newTestOAuth(t, func(config *OAuthConfig) {
		config.TokenEndpoint = server.URL + "/api/v1/access_token"
		config.Now = func() time.Time { return now }
	})

	tokens, err := oauth.ExchangeCode(context.Background(), "code-value")
	if err != nil {
		t.Fatalf("ExchangeCode returned error: %v", err)
	}
	if gotPath != "/api/v1/access_token" {
		t.Fatalf("token path = %q", gotPath)
	}
	assertQuery(t, gotForm, "grant_type", "authorization_code")
	assertQuery(t, gotForm, "code", "code-value")
	assertQuery(t, gotForm, "redirect_uri", DefaultRedirectURI)
	if gotUserAgent == "" {
		t.Fatal("User-Agent header was not set")
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("test-client:"))
	if gotAuth != wantAuth {
		t.Fatalf("Authorization header = %q, want installed-app basic auth", gotAuth)
	}
	if tokens.Access.Reveal() != "access-value" || tokens.Refresh.Reveal() != "refresh-value" {
		t.Fatalf("unexpected token values: access=%q refresh=%q", tokens.Access.Reveal(), tokens.Refresh.Reveal())
	}
	if !tokens.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("ExpiresAt = %s", tokens.ExpiresAt)
	}
	assertScopes(t, tokens.Scopes, []string{"identity", "history"})
}

func TestRefreshKeepsExistingRefreshWhenResponseOmitsNewValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		assertQuery(t, r.PostForm, "grant_type", "refresh_token")
		assertQuery(t, r.PostForm, "refresh_token", "old-refresh")
		writeJSON(t, w, map[string]any{
			"access_token": "new-access",
			"expires_in":   1800,
			"scope":        "identity history",
			"token_type":   "bearer",
		})
	}))
	defer server.Close()

	oauth := newTestOAuth(t, func(config *OAuthConfig) {
		config.TokenEndpoint = server.URL + "/api/v1/access_token"
	})

	tokens, err := oauth.Refresh(context.Background(), secretstore.MustSecret("old-refresh"))
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	if tokens.Access.Reveal() != "new-access" || tokens.Refresh.Reveal() != "old-refresh" {
		t.Fatalf("unexpected refreshed secrets: access=%q refresh=%q", tokens.Access.Reveal(), tokens.Refresh.Reveal())
	}
}

func TestRefreshRejectsUnexpectedScope(t *testing.T) {
	payload := map[string]any{
		"access_token": "access",
		"expires_in":   10,
		"scope":        "identity submit",
		"token_type":   "bearer",
	}

	_, err := parseTokenSet(payload, time.Now(), secretstore.MustSecret("refresh"))
	if err == nil {
		t.Fatal("parseTokenSet returned nil error for mutation scope")
	}
	if strings.Contains(err.Error(), "access") || strings.Contains(err.Error(), "refresh") {
		t.Fatalf("error leaked secret material: %v", err)
	}
}

func TestRefreshTokenStorageUsesSecretVault(t *testing.T) {
	store := secretstore.NewMemoryStore()
	store.SetMode(secretstore.ModeKeyring)
	oauth := newTestOAuth(t, func(config *OAuthConfig) {
		config.Vault = secretstore.Vault{Primary: store}
	})

	result, err := oauth.SaveRefreshToken("UserName", TokenSet{Refresh: secretstore.MustSecret("refresh-secret")})
	if err != nil {
		t.Fatalf("SaveRefreshToken returned error: %v", err)
	}
	if result.Mode != secretstore.ModeKeyring || result.UsedFallback {
		t.Fatalf("unexpected save result: %#v", result)
	}

	loaded, loadResult, err := oauth.LoadRefreshToken("username")
	if err != nil {
		t.Fatalf("LoadRefreshToken returned error: %v", err)
	}
	if loaded.Reveal() != "refresh-secret" || loadResult.Mode != secretstore.ModeKeyring {
		t.Fatalf("unexpected loaded secret/result: %q %#v", loaded.Reveal(), loadResult)
	}
	status, err := oauth.RefreshTokenStatus("username")
	if err != nil {
		t.Fatalf("RefreshTokenStatus returned error: %v", err)
	}
	if !status.PrimaryExists || status.ActiveMode != secretstore.ModeKeyring {
		t.Fatalf("unexpected status: %#v", status)
	}
	if err := oauth.ForgetLocal("username"); err != nil {
		t.Fatalf("ForgetLocal returned error: %v", err)
	}
	_, _, err = oauth.LoadRefreshToken("username")
	if !errors.Is(err, secretstore.ErrNotFound) {
		t.Fatalf("LoadRefreshToken error = %v, want ErrNotFound", err)
	}
}

func TestDisconnectAndRevokeIsExplicit(t *testing.T) {
	var revokeCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		revokeCalls++
		if r.URL.Path != "/api/v1/revoke_token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		assertQuery(t, r.PostForm, "token", "refresh-secret")
		assertQuery(t, r.PostForm, "token_type_hint", "refresh_token")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := secretstore.NewMemoryStore()
	store.SetMode(secretstore.ModeKeyring)
	oauth := newTestOAuth(t, func(config *OAuthConfig) {
		config.RevokeEndpoint = server.URL + "/api/v1/revoke_token"
		config.Vault = secretstore.Vault{Primary: store}
	})
	if _, err := oauth.SaveRefreshToken("user", TokenSet{Refresh: secretstore.MustSecret("refresh-secret")}); err != nil {
		t.Fatalf("SaveRefreshToken returned error: %v", err)
	}
	if revokeCalls != 0 {
		t.Fatal("revoke should not happen during save")
	}

	if err := oauth.DisconnectAndRevoke(context.Background(), "user"); err != nil {
		t.Fatalf("DisconnectAndRevoke returned error: %v", err)
	}
	if revokeCalls != 1 {
		t.Fatalf("revoke calls = %d, want 1", revokeCalls)
	}
	exists, err := store.Exists(secretstore.RedditRefreshKey("user"))
	if err != nil {
		t.Fatalf("Exists returned error: %v", err)
	}
	if exists {
		t.Fatal("refresh secret should be deleted after revoke")
	}
}

func TestWorkspaceMetadataContainsOnlyNonSecrets(t *testing.T) {
	connectedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	expiresAt := connectedAt.Add(time.Hour)
	metadata := WorkspaceMetadata(" user ", TokenSet{
		Refresh:   secretstore.MustSecret("refresh-secret"),
		Access:    secretstore.MustSecret("access-secret"),
		ExpiresAt: expiresAt,
		Scopes:    []string{"identity", "history"},
	}, secretstore.OperationResult{Mode: secretstore.ModeKeyring}, connectedAt)

	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("Marshal metadata: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "refresh-secret") || strings.Contains(text, "access-secret") {
		t.Fatalf("metadata leaked secret value: %s", text)
	}
	for _, allowed := range []string{"username", "oauth_connected_at", "scopes", "token_storage_mode", "credential_store", "expires_at"} {
		if !strings.Contains(text, allowed) {
			t.Fatalf("metadata missing allowed field %q: %s", allowed, text)
		}
	}
}

func newTestOAuth(t *testing.T, update func(*OAuthConfig)) *OAuth {
	t.Helper()
	config := OAuthConfig{
		ClientID: "test-client",
		Vault: secretstore.Vault{
			Primary: secretstore.NewMemoryStore(),
		},
	}
	if update != nil {
		update(&config)
	}
	oauth, err := NewOAuth(config)
	if err != nil {
		t.Fatalf("NewOAuth returned error: %v", err)
	}
	return oauth
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("Encode response: %v", err)
	}
}

func assertQuery(t *testing.T, values url.Values, key, want string) {
	t.Helper()
	if got := values.Get(key); got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
}

func assertScopes(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("scope count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scope[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
