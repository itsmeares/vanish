package secretstore

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFileStoreRequiresExplicitConfirmation(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), false)
	if err != nil {
		t.Fatalf("NewFileStore returned error: %v", err)
	}

	err = store.Save(RedditRefreshKey("user"), MustSecret("secret"))
	if !errors.Is(err, ErrFallbackConfirmationRequired) {
		t.Fatalf("Save error = %v, want ErrFallbackConfirmationRequired", err)
	}
}

func TestFileStoreRoundTripDeleteAndPermissions(t *testing.T) {
	appDir := t.TempDir()
	store, err := NewFileStore(appDir, true)
	if err != nil {
		t.Fatalf("NewFileStore returned error: %v", err)
	}

	key := RedditRefreshKey("user")
	want := MustSecret("refresh-value")
	if err := store.Save(key, want); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	path := onlySecretFile(t, store.Dir())
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(appDir)) {
		t.Fatalf("secret path %q escaped app dir %q", path, appDir)
	}
	if runtime.GOOS != "windows" {
		assertPerm(t, store.Dir(), 0o700)
		assertPerm(t, path, 0o600)
	}

	exists, err := store.Exists(key)
	if err != nil {
		t.Fatalf("Exists returned error: %v", err)
	}
	if !exists {
		t.Fatal("Exists returned false after Save")
	}

	got, err := store.Load(key)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got.Reveal() != want.Reveal() {
		t.Fatalf("Load = %q, want secret value", got.Reveal())
	}

	if err := store.Delete(key); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	exists, err = store.Exists(key)
	if err != nil {
		t.Fatalf("Exists after delete returned error: %v", err)
	}
	if exists {
		t.Fatal("Exists returned true after Delete")
	}
}

func TestFileStoreMissingLoadReturnsNotFound(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), true)
	if err != nil {
		t.Fatalf("NewFileStore returned error: %v", err)
	}

	_, err = store.Load(RedditRefreshKey("missing"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load error = %v, want ErrNotFound", err)
	}
}

func onlySecretFile(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("secret file count = %d, want 1", len(entries))
	}
	return filepath.Join(dir, entries[0].Name())
}

func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) returned error: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", path, got, want)
	}
}
