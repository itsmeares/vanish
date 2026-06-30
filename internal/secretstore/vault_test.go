package secretstore

import (
	"errors"
	"testing"
)

func TestVaultUsesPrimaryWhenAvailableAndCleansFallback(t *testing.T) {
	primary := NewMemoryStore()
	primary.SetMode(ModeKeyring)
	fallback := NewMemoryStore()
	fallback.SetMode(ModeFile)
	key := RedditRefreshKey("user")

	if err := fallback.Save(key, MustSecret("old-fallback")); err != nil {
		t.Fatalf("fallback Save returned error: %v", err)
	}

	result, err := (Vault{Primary: primary, Fallback: fallback, AllowFileFallback: true}).Save(key, MustSecret("new-primary"))
	if err != nil {
		t.Fatalf("Vault Save returned error: %v", err)
	}
	if result.Mode != ModeKeyring || result.UsedFallback {
		t.Fatalf("unexpected result: %#v", result)
	}

	primarySecret, err := primary.Load(key)
	if err != nil {
		t.Fatalf("primary Load returned error: %v", err)
	}
	if primarySecret.Reveal() != "new-primary" {
		t.Fatalf("primary secret = %q", primarySecret.Reveal())
	}
	fallbackExists, err := fallback.Exists(key)
	if err != nil {
		t.Fatalf("fallback Exists returned error: %v", err)
	}
	if fallbackExists {
		t.Fatal("fallback should be deleted after primary save")
	}
}

func TestVaultDoesNotSilentlyFallbackWhenPrimaryUnavailable(t *testing.T) {
	primary := NewUnavailableMemoryStore(errors.New("keyring unavailable"))
	primary.SetMode(ModeKeyring)
	fallback := NewMemoryStore()
	fallback.SetMode(ModeFile)
	key := RedditRefreshKey("user")

	_, err := (Vault{Primary: primary, Fallback: fallback}).Save(key, MustSecret("secret"))
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Save error = %v, want ErrUnavailable", err)
	}

	exists, err := fallback.Exists(key)
	if err != nil {
		t.Fatalf("fallback Exists returned error: %v", err)
	}
	if exists {
		t.Fatal("fallback should remain empty without explicit allowance")
	}
}

func TestVaultUsesFallbackWhenExplicitlyAllowed(t *testing.T) {
	primary := NewUnavailableMemoryStore(errors.New("keyring unavailable"))
	primary.SetMode(ModeKeyring)
	fallback := NewMemoryStore()
	fallback.SetMode(ModeFile)
	key := RedditRefreshKey("user")

	result, err := (Vault{Primary: primary, Fallback: fallback, AllowFileFallback: true}).Save(key, MustSecret("secret"))
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if result.Mode != ModeFile || !result.UsedFallback {
		t.Fatalf("unexpected fallback result: %#v", result)
	}
	got, err := fallback.Load(key)
	if err != nil {
		t.Fatalf("fallback Load returned error: %v", err)
	}
	if got.Reveal() != "secret" {
		t.Fatalf("fallback secret = %q", got.Reveal())
	}
}

func TestVaultMigratesFallbackToPrimaryOnLoad(t *testing.T) {
	primary := NewMemoryStore()
	primary.SetMode(ModeKeyring)
	fallback := NewMemoryStore()
	fallback.SetMode(ModeFile)
	key := RedditRefreshKey("user")
	if err := fallback.Save(key, MustSecret("fallback-secret")); err != nil {
		t.Fatalf("fallback Save returned error: %v", err)
	}

	secret, result, err := (Vault{Primary: primary, Fallback: fallback, AllowFileFallback: true}).Load(key)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if secret.Reveal() != "fallback-secret" || !result.Migrated || result.Mode != ModeKeyring {
		t.Fatalf("unexpected migration result secret=%q result=%#v", secret.Reveal(), result)
	}

	primaryExists, err := primary.Exists(key)
	if err != nil {
		t.Fatalf("primary Exists returned error: %v", err)
	}
	fallbackExists, err := fallback.Exists(key)
	if err != nil {
		t.Fatalf("fallback Exists returned error: %v", err)
	}
	if !primaryExists || fallbackExists {
		t.Fatalf("migration state primary=%v fallback=%v", primaryExists, fallbackExists)
	}
}

func TestVaultStatusReportsMigrationNeed(t *testing.T) {
	primary := NewMemoryStore()
	primary.SetMode(ModeKeyring)
	fallback := NewMemoryStore()
	fallback.SetMode(ModeFile)
	key := RedditRefreshKey("user")
	if err := fallback.Save(key, MustSecret("fallback-secret")); err != nil {
		t.Fatalf("fallback Save returned error: %v", err)
	}

	status := (Vault{Primary: primary, Fallback: fallback, AllowFileFallback: true}).Status(key)
	if !status.PrimaryAvailable || !status.FallbackExists || !status.MigrationNeeded || status.ActiveMode != ModeKeyring {
		t.Fatalf("unexpected status: %#v", status)
	}
}
