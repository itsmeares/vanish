package secretstore

import (
	"errors"
	"sync"
)

type MemoryStore struct {
	mu           sync.Mutex
	values       map[string]Secret
	availableErr error
	mode         Mode
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		values: make(map[string]Secret),
		mode:   ModeMemory,
	}
}

func NewUnavailableMemoryStore(err error) *MemoryStore {
	if err == nil {
		err = ErrUnavailable
	}
	store := NewMemoryStore()
	store.availableErr = err
	return store
}

func (store *MemoryStore) Mode() Mode {
	if store == nil || store.mode == "" {
		return ModeMemory
	}
	return store.mode
}

func (store *MemoryStore) Available() error {
	if store == nil {
		return ErrUnavailable
	}
	if store.availableErr != nil {
		return store.availableErr
	}
	return nil
}

func (store *MemoryStore) Save(key Key, secret Secret) error {
	if err := key.validate(); err != nil {
		return err
	}
	if secret.Empty() {
		return errors.New("secret value is required")
	}
	if err := store.Available(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.values[key.stableID()] = secret
	return nil
}

func (store *MemoryStore) Load(key Key) (Secret, error) {
	if err := key.validate(); err != nil {
		return Secret{}, err
	}
	if err := store.Available(); err != nil {
		return Secret{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	secret, ok := store.values[key.stableID()]
	if !ok {
		return Secret{}, ErrNotFound
	}
	return secret, nil
}

func (store *MemoryStore) Delete(key Key) error {
	if err := key.validate(); err != nil {
		return err
	}
	if err := store.Available(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.values, key.stableID())
	return nil
}

func (store *MemoryStore) Exists(key Key) (bool, error) {
	if err := key.validate(); err != nil {
		return false, err
	}
	if err := store.Available(); err != nil {
		return false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	_, ok := store.values[key.stableID()]
	return ok, nil
}

func (store *MemoryStore) SetMode(mode Mode) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.mode = mode
}
