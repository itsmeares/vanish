package reddit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/itsmeares/vanish/internal/secretstore"
)

const (
	DefaultAPIBaseURL  = "https://oauth.reddit.com"
	DefaultListingSize = 25
	MaxListingSize     = 100
)

type ClientOptions struct {
	BaseURL    string
	UserAgent  string
	HTTPClient HTTPDoer
}

type Client struct {
	baseURL    *url.URL
	userAgent  string
	httpClient HTTPDoer
	access     secretstore.Secret
}

type User struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

type ListingParams struct {
	Limit  int
	After  string
	Before string
	Count  int
}

func NewClient(access secretstore.Secret, options ClientOptions) (*Client, error) {
	if access.Empty() {
		return nil, errors.New("reddit access secret is required")
	}
	baseURL, err := parseEndpoint(defaultString(options.BaseURL, DefaultAPIBaseURL))
	if err != nil {
		return nil, fmt.Errorf("invalid reddit API base URL: %w", err)
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		baseURL:    baseURL,
		userAgent:  defaultString(options.UserAgent, DefaultUserAgent),
		httpClient: httpClient,
		access:     access,
	}, nil
}

func (client *Client) Me(ctx context.Context) (User, error) {
	var payload map[string]any
	if err := client.GetJSON(ctx, "/api/v1/me", nil, &payload); err != nil {
		return User{}, err
	}
	return userFromPayload(payload)
}

func (client *Client) GetJSON(ctx context.Context, path string, query url.Values, target any) error {
	if client == nil {
		return errors.New("reddit client is required")
	}
	if target == nil {
		return errors.New("reddit API response target is required")
	}
	endpoint, err := client.endpoint(path, query)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+client.access.Reveal())
	req.Header.Set("User-Agent", client.userAgent)

	res, err := client.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return fmt.Errorf("reddit API request failed: GET %s status %d", path, res.StatusCode)
	}

	decoder := json.NewDecoder(io.LimitReader(res.Body, 10<<20))
	return decoder.Decode(target)
}

func (params ListingParams) Values() url.Values {
	query := url.Values{}
	query.Set("limit", strconv.Itoa(clampListingSize(params.Limit)))
	if after := strings.TrimSpace(params.After); after != "" {
		query.Set("after", after)
	}
	if before := strings.TrimSpace(params.Before); before != "" {
		query.Set("before", before)
	}
	if params.Count > 0 {
		query.Set("count", strconv.Itoa(params.Count))
	}
	return query
}

func (client *Client) endpoint(path string, query url.Values) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || !strings.HasPrefix(path, "/") || strings.Contains(path, "://") {
		return "", errors.New("reddit API path must be an absolute path")
	}
	endpoint := *client.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func userFromPayload(payload map[string]any) (User, error) {
	name := stringField(payload, "name")
	if name == "" {
		return User{}, errors.New("reddit identity response missing username")
	}
	user := User{
		ID:   stringField(payload, "id"),
		Name: name,
	}
	if created := floatField(payload, "created_utc"); created > 0 {
		seconds := int64(created)
		nanos := int64((created - float64(seconds)) * 1e9)
		user.CreatedAt = time.Unix(seconds, nanos).UTC()
	}
	return user, nil
}

func floatField(payload map[string]any, key string) float64 {
	value, ok := payload[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case json.Number:
		n, _ := strconv.ParseFloat(typed.String(), 64)
		return n
	default:
		return 0
	}
}

func clampListingSize(limit int) int {
	if limit <= 0 {
		return DefaultListingSize
	}
	if limit > MaxListingSize {
		return MaxListingSize
	}
	return limit
}
