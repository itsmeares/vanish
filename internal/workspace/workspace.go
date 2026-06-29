package workspace

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	appDirEnv             = "VANISH_APP_DIR"
	appDirName            = "vanish"
	configFileName        = "config.json"
	recentImportsFileName = "recent-imports.json"
	recentPlansFileName   = "recent-plans.json"
	auditFileName         = "audit.jsonl"
	ConfigVersion         = 1
	MaxRecentImports      = 20
	MaxRecentPlans        = 20
	DefaultPlanExportPath = "vanish-plan.json"
)

// Workspace owns Vanish's local metadata directory. It must not own imported
// exports or generated cleanup plan files.
type Workspace struct {
	dir string
}

type Config struct {
	Version               int             `json:"version"`
	Telemetry             TelemetryConfig `json:"telemetry"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
	DefaultPlanExportPath string          `json:"default_plan_export_path"`
	LastOpenedPlanPath    string          `json:"last_opened_plan_path,omitempty"`
	Reddit                *RedditConfig   `json:"reddit,omitempty"`
}

type TelemetryConfig struct {
	Enabled bool `json:"enabled"`
}

type RedditConfig struct {
	Username         string     `json:"username,omitempty"`
	OAuthConnectedAt *time.Time `json:"oauth_connected_at,omitempty"`
	Scopes           []string   `json:"scopes,omitempty"`
	TokenStorageMode string     `json:"token_storage_mode,omitempty"`
	CredentialStore  string     `json:"credential_store,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
}

type RecentImport struct {
	SourceLabel    string    `json:"source_label,omitempty"`
	SourcePath     string    `json:"source_path,omitempty"`
	Platform       string    `json:"platform,omitempty"`
	ImportedAt     time.Time `json:"imported_at"`
	Demo           bool      `json:"demo"`
	ItemCount      int       `json:"item_count"`
	LikeCount      int       `json:"like_count"`
	CommentCount   int       `json:"comment_count"`
	PostCount      int       `json:"post_count"`
	FollowingCount int       `json:"following_count"`
	FollowerCount  int       `json:"follower_count"`
	WarningCount   int       `json:"warning_count"`
	SkippedCount   int       `json:"skipped_count"`
}

type RecentPlan struct {
	ID            string         `json:"id"`
	Path          string         `json:"path,omitempty"`
	Mode          string         `json:"mode,omitempty"`
	SourceName    string         `json:"source_name,omitempty"`
	PlanCreatedAt time.Time      `json:"plan_created_at"`
	LastUsedAt    time.Time      `json:"last_used_at"`
	LastOperation string         `json:"last_operation,omitempty"`
	ActionCounts  map[string]int `json:"action_counts,omitempty"`
	StatusCounts  map[string]int `json:"status_counts,omitempty"`
}

type recentPlanFileEntry struct {
	ID            string         `json:"id"`
	Path          string         `json:"path,omitempty"`
	Mode          string         `json:"mode,omitempty"`
	SourceName    string         `json:"source_name,omitempty"`
	CreatedAt     time.Time      `json:"created_at,omitempty"`
	PlanCreatedAt time.Time      `json:"plan_created_at"`
	LastUsedAt    time.Time      `json:"last_used_at"`
	LastOperation string         `json:"last_operation,omitempty"`
	ActionCounts  map[string]int `json:"action_counts,omitempty"`
	StatusCounts  map[string]int `json:"status_counts,omitempty"`
}

type AuditEvent struct {
	Type      string         `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Fields    map[string]any `json:"fields,omitempty"`
}

type AuditReadResult struct {
	Events         []AuditEvent
	MalformedLines int
}

// DefaultDir returns the directory used for local metadata. VANISH_APP_DIR is
// honored first so tests and portable installs can isolate all writes.
func DefaultDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv(appDirEnv)); override != "" {
		return filepath.Clean(override), nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, appDirName), nil
}

func OpenDefault() (*Workspace, error) {
	dir, err := DefaultDir()
	if err != nil {
		return nil, err
	}
	return Open(dir)
}

func Open(dir string) (*Workspace, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("workspace dir is required")
	}
	w := &Workspace{dir: filepath.Clean(dir)}
	if err := w.init(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Workspace) Dir() string {
	return w.dir
}

func (w *Workspace) LoadConfig() (Config, error) {
	var config Config
	if err := readJSON(w.path(configFileName), &config); err != nil {
		return Config{}, err
	}
	if config.Version != ConfigVersion {
		return Config{}, fmt.Errorf("unsupported config version %d", config.Version)
	}
	return config, nil
}

func (w *Workspace) SaveConfig(config Config) error {
	if config.Version == 0 {
		config.Version = ConfigVersion
	}
	if config.Version != ConfigVersion {
		return fmt.Errorf("unsupported config version %d", config.Version)
	}
	if existing, err := w.LoadConfig(); err == nil && !existing.CreatedAt.IsZero() {
		config.CreatedAt = existing.CreatedAt
	}
	if config.CreatedAt.IsZero() {
		config.CreatedAt = time.Now().UTC()
	}
	config.DefaultPlanExportPath = strings.TrimSpace(config.DefaultPlanExportPath)
	if config.DefaultPlanExportPath == "" {
		config.DefaultPlanExportPath = DefaultPlanExportPath
	}
	config.LastOpenedPlanPath = strings.TrimSpace(config.LastOpenedPlanPath)
	config.Reddit = sanitizeRedditConfig(config.Reddit)
	config.UpdatedAt = time.Now().UTC()
	return writeJSON(w.path(configFileName), config)
}

func (w *Workspace) UpdateConfig(update func(*Config)) error {
	if update == nil {
		return errors.New("config update is required")
	}
	config, err := w.LoadConfig()
	if err != nil {
		return err
	}
	update(&config)
	return w.SaveConfig(config)
}

func (w *Workspace) RecentImports() ([]RecentImport, error) {
	var imports []RecentImport
	if err := readJSON(w.path(recentImportsFileName), &imports); err != nil {
		return nil, err
	}
	return imports, nil
}

func (w *Workspace) UpsertRecentImport(entry RecentImport) error {
	if entry.ImportedAt.IsZero() {
		entry.ImportedAt = time.Now().UTC()
	}
	imports, err := w.RecentImports()
	if err != nil {
		return err
	}
	key := recentImportKey(entry)
	filtered := make([]RecentImport, 0, len(imports)+1)
	filtered = append(filtered, entry)
	for _, existing := range imports {
		if recentImportKey(existing) == key {
			continue
		}
		filtered = append(filtered, existing)
		if len(filtered) == MaxRecentImports {
			break
		}
	}
	return writeJSON(w.path(recentImportsFileName), filtered)
}

func (w *Workspace) RecentPlans() ([]RecentPlan, error) {
	return readRecentPlans(w.path(recentPlansFileName))
}

func (w *Workspace) UpsertRecentPlan(entry RecentPlan) error {
	if entry.LastUsedAt.IsZero() {
		entry.LastUsedAt = time.Now().UTC()
	}
	plans, err := w.RecentPlans()
	if err != nil {
		return err
	}
	key := recentPlanKey(entry)
	filtered := make([]RecentPlan, 0, len(plans)+1)
	filtered = append(filtered, entry)
	for _, existing := range plans {
		if recentPlanKey(existing) == key {
			continue
		}
		filtered = append(filtered, existing)
		if len(filtered) == MaxRecentPlans {
			break
		}
	}
	return writeJSON(w.path(recentPlansFileName), filtered)
}

func (w *Workspace) AppendAudit(event AuditEvent) error {
	if strings.TrimSpace(event.Type) == "" {
		return errors.New("audit event type is required")
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if err := validateAuditFields(event.Fields); err != nil {
		return err
	}
	file, err := os.OpenFile(w.path(auditFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func (w *Workspace) ReadAudit() (AuditReadResult, error) {
	file, err := os.Open(w.path(auditFileName))
	if err != nil {
		return AuditReadResult{}, err
	}
	defer file.Close()

	var result AuditReadResult
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event AuditEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			result.MalformedLines++
			continue
		}
		if event.Type == "" || event.Timestamp.IsZero() {
			result.MalformedLines++
			continue
		}
		if err := validateAuditFields(event.Fields); err != nil {
			result.MalformedLines++
			continue
		}
		result.Events = append(result.Events, event)
	}
	if err := scanner.Err(); err != nil {
		return AuditReadResult{}, err
	}
	return result, nil
}

func (w *Workspace) Wipe() error {
	dir := strings.TrimSpace(w.dir)
	if !isSafeWipeDir(dir) {
		return errors.New("workspace dir is not safe to wipe")
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return w.init()
}

func (w *Workspace) init() error {
	if err := os.MkdirAll(w.dir, 0o700); err != nil {
		return err
	}
	if err := ensureJSONFile(w.path(configFileName), defaultConfig()); err != nil {
		return err
	}
	if err := ensureJSONFile(w.path(recentImportsFileName), []RecentImport{}); err != nil {
		return err
	}
	if err := ensureJSONFile(w.path(recentPlansFileName), []RecentPlan{}); err != nil {
		return err
	}
	return ensureFile(w.path(auditFileName))
}

func (w *Workspace) path(name string) string {
	return filepath.Join(w.dir, name)
}

func defaultConfig() Config {
	now := time.Now().UTC()
	return Config{
		Version:               ConfigVersion,
		Telemetry:             TelemetryConfig{Enabled: false},
		CreatedAt:             now,
		UpdatedAt:             now,
		DefaultPlanExportPath: DefaultPlanExportPath,
	}
}

func sanitizeRedditConfig(config *RedditConfig) *RedditConfig {
	if config == nil {
		return nil
	}
	config.Username = strings.TrimSpace(config.Username)
	config.TokenStorageMode = strings.TrimSpace(config.TokenStorageMode)
	config.CredentialStore = strings.TrimSpace(config.CredentialStore)
	if len(config.Scopes) > 0 {
		scopes := make([]string, 0, len(config.Scopes))
		for _, scope := range config.Scopes {
			scope = strings.TrimSpace(scope)
			if scope != "" {
				scopes = append(scopes, scope)
			}
		}
		config.Scopes = scopes
	}
	if config.empty() {
		return nil
	}
	return config
}

func (config RedditConfig) empty() bool {
	return config.Username == "" &&
		config.OAuthConnectedAt == nil &&
		len(config.Scopes) == 0 &&
		config.TokenStorageMode == "" &&
		config.CredentialStore == "" &&
		config.ExpiresAt == nil
}

func ensureJSONFile(path string, value any) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeJSON(path, value)
}

func ensureFile(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	return file.Close()
}

func readJSON(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("json file contains multiple values")
	}
	return nil
}

func readRecentPlans(path string) ([]RecentPlan, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var entries []recentPlanFileEntry
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entries); err != nil {
		return nil, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return nil, errors.New("json file contains multiple values")
	}

	plans := make([]RecentPlan, 0, len(entries))
	for _, entry := range entries {
		planCreatedAt := entry.PlanCreatedAt
		if planCreatedAt.IsZero() {
			planCreatedAt = entry.CreatedAt
		}
		lastUsedAt := entry.LastUsedAt
		if lastUsedAt.IsZero() {
			lastUsedAt = entry.CreatedAt
		}
		plans = append(plans, RecentPlan{
			ID:            entry.ID,
			Path:          entry.Path,
			Mode:          entry.Mode,
			SourceName:    entry.SourceName,
			PlanCreatedAt: planCreatedAt,
			LastUsedAt:    lastUsedAt,
			LastOperation: entry.LastOperation,
			ActionCounts:  entry.ActionCounts,
			StatusCounts:  entry.StatusCounts,
		})
	}
	return plans, nil
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempName)
		}
	}()

	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func recentImportKey(entry RecentImport) string {
	if key := strings.TrimSpace(entry.SourcePath); key != "" {
		return "path:" + filepath.Clean(key)
	}
	return "label:" + strings.TrimSpace(entry.SourceLabel)
}

func recentPlanKey(entry RecentPlan) string {
	if key := strings.TrimSpace(entry.ID); key != "" {
		return "id:" + key
	}
	if key := strings.TrimSpace(entry.Path); key != "" {
		return "path:" + filepath.Clean(key)
	}
	return "source:" + strings.TrimSpace(entry.SourceName)
}

func isSafeWipeDir(dir string) bool {
	clean := filepath.Clean(dir)
	if clean == "" || clean == "." {
		return false
	}
	volume := filepath.VolumeName(clean)
	root := volume + string(filepath.Separator)
	return clean != root
}

func validateAuditFields(fields map[string]any) error {
	for key, value := range fields {
		if looksSecretLike(key) {
			return fmt.Errorf("audit field %q looks secret-like and must not be persisted", key)
		}
		if !isSafeAuditScalar(value) {
			return fmt.Errorf("audit field %q is not a safe scalar", key)
		}
	}
	return nil
}

func isSafeAuditScalar(value any) bool {
	switch value.(type) {
	case nil, string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number:
		return true
	default:
		return false
	}
}

func looksSecretLike(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	for _, part := range forbiddenAuditKeyParts {
		if strings.Contains(normalized, part) {
			return true
		}
	}
	return false
}

var forbiddenAuditKeyParts = []string{
	"authorization",
	"cookie",
	"credential",
	"oauth",
	"password",
	"passwd",
	"secret",
	"session",
	"token",
}
