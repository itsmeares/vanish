package reddit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/itsmeares/vanish/internal/secretstore"
	"github.com/itsmeares/vanish/internal/workspace"
)

const (
	ClientIDEnv        = "VANISH_REDDIT_CLIENT_ID"
	DefaultRedirectURI = "http://127.0.0.1:53682/reddit/oauth/callback"
	DefaultUserAgent   = "vanish:v0.5 (local planner)"

	ScopeIdentity = "identity"
	ScopeHistory  = "history"

	defaultAuthorizeEndpoint = "https://www.reddit.com/api/v1/authorize"
	defaultTokenEndpoint     = "https://www.reddit.com/api/v1/access_token"
	defaultRevokeEndpoint    = "https://www.reddit.com/api/v1/revoke_token"
)

var ReadOnlyScopes = []string{ScopeIdentity, ScopeHistory}

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type OAuthConfig struct {
	ClientID          string
	RedirectURI       string
	UserAgent         string
	AuthorizeEndpoint string
	TokenEndpoint     string
	RevokeEndpoint    string
	HTTPClient        HTTPDoer
	Vault             secretstore.Vault
	Now               func() time.Time
}

type OAuth struct {
	clientID          string
	redirectURI       string
	userAgent         string
	authorizeEndpoint *url.URL
	tokenEndpoint     string
	revokeEndpoint    string
	httpClient        HTTPDoer
	vault             secretstore.Vault
	now               func() time.Time
}

type TokenSet struct {
	Access    secretstore.Secret
	Refresh   secretstore.Secret
	ExpiresAt time.Time
	Scopes    []string
	TokenType string
}

func ClientIDFromEnv() (string, error) {
	clientID := strings.TrimSpace(os.Getenv(ClientIDEnv))
	if clientID == "" {
		return "", fmt.Errorf("%s is required", ClientIDEnv)
	}
	return clientID, nil
}

func NewOAuth(config OAuthConfig) (*OAuth, error) {
	clientID := strings.TrimSpace(config.ClientID)
	if clientID == "" {
		return nil, fmt.Errorf("%s is required", ClientIDEnv)
	}

	redirectURI := strings.TrimSpace(config.RedirectURI)
	if redirectURI == "" {
		redirectURI = DefaultRedirectURI
	}
	if _, err := parseEndpoint(redirectURI); err != nil {
		return nil, fmt.Errorf("invalid reddit redirect URI: %w", err)
	}

	authorizeEndpoint, err := parseEndpoint(defaultString(config.AuthorizeEndpoint, defaultAuthorizeEndpoint))
	if err != nil {
		return nil, fmt.Errorf("invalid reddit authorize endpoint: %w", err)
	}
	tokenEndpoint, err := parseEndpoint(defaultString(config.TokenEndpoint, defaultTokenEndpoint))
	if err != nil {
		return nil, fmt.Errorf("invalid reddit token endpoint: %w", err)
	}
	revokeEndpoint, err := parseEndpoint(defaultString(config.RevokeEndpoint, defaultRevokeEndpoint))
	if err != nil {
		return nil, fmt.Errorf("invalid reddit revoke endpoint: %w", err)
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	now := config.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	return &OAuth{
		clientID:          clientID,
		redirectURI:       redirectURI,
		userAgent:         defaultString(config.UserAgent, DefaultUserAgent),
		authorizeEndpoint: authorizeEndpoint,
		tokenEndpoint:     tokenEndpoint.String(),
		revokeEndpoint:    revokeEndpoint.String(),
		httpClient:        httpClient,
		vault:             config.Vault,
		now:               now,
	}, nil
}

func (oauth *OAuth) AuthURL(state string) (string, error) {
	if oauth == nil {
		return "", errors.New("reddit oauth is required")
	}
	state = strings.TrimSpace(state)
	if state == "" {
		return "", errors.New("reddit oauth state is required")
	}

	endpoint := *oauth.authorizeEndpoint
	query := endpoint.Query()
	query.Set("client_id", oauth.clientID)
	query.Set("response_type", "code")
	query.Set("state", state)
	query.Set("redirect_uri", oauth.redirectURI)
	query.Set("duration", "permanent")
	query.Set("scope", strings.Join(ReadOnlyScopes, " "))
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func (oauth *OAuth) ExchangeCode(ctx context.Context, code string) (TokenSet, error) {
	if oauth == nil {
		return TokenSet{}, errors.New("reddit oauth is required")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return TokenSet{}, errors.New("reddit oauth code is required")
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", oauth.redirectURI)
	return oauth.postTokenForm(ctx, form, secretstore.Secret{})
}

func (oauth *OAuth) Refresh(ctx context.Context, refresh secretstore.Secret) (TokenSet, error) {
	if oauth == nil {
		return TokenSet{}, errors.New("reddit oauth is required")
	}
	if refresh.Empty() {
		return TokenSet{}, errors.New("reddit refresh secret is required")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refresh.Reveal())
	return oauth.postTokenForm(ctx, form, refresh)
}

func (oauth *OAuth) Revoke(ctx context.Context, token secretstore.Secret) error {
	if oauth == nil {
		return errors.New("reddit oauth is required")
	}
	if token.Empty() {
		return errors.New("reddit revoke secret is required")
	}

	form := url.Values{}
	form.Set("token", token.Reveal())
	form.Set("token_type_hint", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauth.revokeEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	oauth.setTokenRequestHeaders(req)

	res, err := oauth.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("reddit oauth revoke failed: status %d", res.StatusCode)
	}
	return nil
}

func (oauth *OAuth) SaveRefreshToken(username string, tokens TokenSet) (secretstore.OperationResult, error) {
	if tokens.Refresh.Empty() {
		return secretstore.OperationResult{}, errors.New("reddit refresh secret is required")
	}
	return oauth.saveSecret(username, tokens.Refresh)
}

func (oauth *OAuth) LoadRefreshToken(username string) (secretstore.Secret, secretstore.OperationResult, error) {
	if oauth == nil {
		return secretstore.Secret{}, secretstore.OperationResult{}, errors.New("reddit oauth is required")
	}
	key, err := redditRefreshKey(username)
	if err != nil {
		return secretstore.Secret{}, secretstore.OperationResult{}, err
	}
	return oauth.vault.Load(key)
}

func (oauth *OAuth) RefreshTokenStatus(username string) (secretstore.Status, error) {
	if oauth == nil {
		return secretstore.Status{}, errors.New("reddit oauth is required")
	}
	key, err := redditRefreshKey(username)
	if err != nil {
		return secretstore.Status{}, err
	}
	return oauth.vault.Status(key), nil
}

func (oauth *OAuth) ForgetLocal(username string) error {
	if oauth == nil {
		return errors.New("reddit oauth is required")
	}
	key, err := redditRefreshKey(username)
	if err != nil {
		return err
	}
	return oauth.vault.Delete(key)
}

func (oauth *OAuth) DisconnectAndRevoke(ctx context.Context, username string) error {
	refresh, _, err := oauth.LoadRefreshToken(username)
	if err != nil {
		if errors.Is(err, secretstore.ErrNotFound) {
			_ = oauth.ForgetLocal(username)
		}
		return err
	}
	if err := oauth.Revoke(ctx, refresh); err != nil {
		return err
	}
	return oauth.ForgetLocal(username)
}

func WorkspaceMetadata(username string, tokens TokenSet, result secretstore.OperationResult, connectedAt time.Time) *workspace.RedditConfig {
	username = strings.TrimSpace(username)
	scopes := cloneScopes(tokens.Scopes)
	if len(scopes) == 0 {
		scopes = cloneScopes(ReadOnlyScopes)
	}
	return &workspace.RedditConfig{
		Username:         username,
		OAuthConnectedAt: timePtr(connectedAt),
		Scopes:           scopes,
		TokenStorageMode: string(result.Mode),
		CredentialStore:  string(result.Mode),
		ExpiresAt:        timePtr(tokens.ExpiresAt),
	}
}

func (oauth *OAuth) saveSecret(username string, secret secretstore.Secret) (secretstore.OperationResult, error) {
	if oauth == nil {
		return secretstore.OperationResult{}, errors.New("reddit oauth is required")
	}
	key, err := redditRefreshKey(username)
	if err != nil {
		return secretstore.OperationResult{}, err
	}
	return oauth.vault.Save(key, secret)
}

func (oauth *OAuth) postTokenForm(ctx context.Context, form url.Values, existingRefresh secretstore.Secret) (TokenSet, error) {
	if oauth == nil {
		return TokenSet{}, errors.New("reddit oauth is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauth.tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return TokenSet{}, err
	}
	oauth.setTokenRequestHeaders(req)

	res, err := oauth.httpClient.Do(req)
	if err != nil {
		return TokenSet{}, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return TokenSet{}, fmt.Errorf("reddit oauth token request failed: status %d", res.StatusCode)
	}

	var payload map[string]any
	decoder := json.NewDecoder(io.LimitReader(res.Body, 1<<20))
	if err := decoder.Decode(&payload); err != nil {
		return TokenSet{}, err
	}
	return parseTokenSet(payload, oauth.now(), existingRefresh)
}

func (oauth *OAuth) setTokenRequestHeaders(req *http.Request) {
	req.SetBasicAuth(oauth.clientID, "")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", oauth.userAgent)
}

func parseTokenSet(payload map[string]any, now time.Time, existingRefresh secretstore.Secret) (TokenSet, error) {
	accessValue := stringField(payload, "access_token")
	if accessValue == "" {
		return TokenSet{}, errors.New("reddit oauth response missing access value")
	}
	access, err := secretstore.NewSecret(accessValue)
	if err != nil {
		return TokenSet{}, err
	}

	refresh := existingRefresh
	if refreshValue := stringField(payload, "refresh_token"); refreshValue != "" {
		refresh, err = secretstore.NewSecret(refreshValue)
		if err != nil {
			return TokenSet{}, err
		}
	}
	if refresh.Empty() {
		return TokenSet{}, errors.New("reddit oauth response missing refresh value")
	}

	scopes, err := normalizeReadScopes(splitScopes(stringField(payload, "scope")))
	if err != nil {
		return TokenSet{}, err
	}
	tokenType := strings.ToLower(defaultString(stringField(payload, "token_type"), "bearer"))
	if tokenType != "bearer" {
		return TokenSet{}, fmt.Errorf("reddit oauth response token type %q is unsupported", tokenType)
	}

	var expiresAt time.Time
	if seconds := intField(payload, "expires_in"); seconds > 0 {
		expiresAt = now.UTC().Add(time.Duration(seconds) * time.Second)
	}

	return TokenSet{
		Access:    access,
		Refresh:   refresh,
		ExpiresAt: expiresAt,
		Scopes:    scopes,
		TokenType: tokenType,
	}, nil
}

func normalizeReadScopes(scopes []string) ([]string, error) {
	if len(scopes) == 0 {
		return cloneScopes(ReadOnlyScopes), nil
	}
	seen := map[string]bool{}
	for _, scope := range scopes {
		scope = strings.ToLower(strings.TrimSpace(scope))
		if scope == "" {
			continue
		}
		if scope != ScopeIdentity && scope != ScopeHistory {
			return nil, fmt.Errorf("reddit returned unsupported OAuth scope %q", scope)
		}
		seen[scope] = true
	}
	normalized := make([]string, 0, len(ReadOnlyScopes))
	for _, scope := range ReadOnlyScopes {
		if seen[scope] {
			normalized = append(normalized, scope)
		}
	}
	if len(normalized) != len(ReadOnlyScopes) {
		return nil, errors.New("reddit OAuth response missing required scope")
	}
	return normalized, nil
}

func splitScopes(value string) []string {
	return strings.Fields(strings.ReplaceAll(value, ",", " "))
}

func cloneScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	clone := make([]string, len(scopes))
	copy(clone, scopes)
	return clone
}

func redditRefreshKey(username string) (secretstore.Key, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return secretstore.Key{}, errors.New("reddit username is required")
	}
	return secretstore.RedditRefreshKey(username), nil
}

func parseEndpoint(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, errors.New("endpoint must use http or https")
	}
	if parsed.Host == "" {
		return nil, errors.New("endpoint host is required")
	}
	return parsed, nil
}

func stringField(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func intField(payload map[string]any, key string) int {
	value, ok := payload[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case json.Number:
		n, _ := strconv.Atoi(typed.String())
		return n
	default:
		return 0
	}
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func timePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	value = value.UTC()
	return &value
}
