package secretstore

import (
	"errors"
	"testing"

	keyring "github.com/zalando/go-keyring"
)

type fakeKeyringBackend struct {
	values map[string]string
	err    error
}

func newFakeKeyringBackend() *fakeKeyringBackend {
	return &fakeKeyringBackend{values: make(map[string]string)}
}

func (backend *fakeKeyringBackend) Set(service, account, value string) error {
	if backend.err != nil {
		return backend.err
	}
	backend.values[service+"\x00"+account] = value
	return nil
}

func (backend *fakeKeyringBackend) Get(service, account string) (string, error) {
	if backend.err != nil {
		return "", backend.err
	}
	value, ok := backend.values[service+"\x00"+account]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}

func (backend *fakeKeyringBackend) Delete(service, account string) error {
	if backend.err != nil {
		return backend.err
	}
	delete(backend.values, service+"\x00"+account)
	return nil
}

func TestKeyringStoreMapsMissingAndUnavailable(t *testing.T) {
	backend := newFakeKeyringBackend()
	store := NewKeyringStoreWithBackend(backend)

	if err := store.Available(); err != nil {
		t.Fatalf("Available returned error for missing probe key: %v", err)
	}
	exists, err := store.Exists(RedditRefreshKey("user"))
	if err != nil {
		t.Fatalf("Exists returned error: %v", err)
	}
	if exists {
		t.Fatal("Exists returned true for missing key")
	}
	_, err = store.Load(RedditRefreshKey("user"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load error = %v, want ErrNotFound", err)
	}

	backend.err = errors.New("backend unavailable")
	if !errors.Is(store.Available(), ErrUnavailable) {
		t.Fatalf("Available should map backend error to ErrUnavailable")
	}
}

func TestKeyringStoreRoundTrip(t *testing.T) {
	store := NewKeyringStoreWithBackend(newFakeKeyringBackend())
	key := RedditRefreshKey("user")
	want := MustSecret("refresh-value")

	if err := store.Save(key, want); err != nil {
		t.Fatalf("Save returned error: %v", err)
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
	exists, err := store.Exists(key)
	if err != nil {
		t.Fatalf("Exists returned error: %v", err)
	}
	if exists {
		t.Fatal("Exists returned true after Delete")
	}
}
