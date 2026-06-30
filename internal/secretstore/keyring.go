package secretstore

import (
	"errors"
	"fmt"

	keyring "github.com/zalando/go-keyring"
)

const availabilityAccount = "__vanish_availability_check__"

type keyringBackend interface {
	Set(service, account, value string) error
	Get(service, account string) (string, error)
	Delete(service, account string) error
}

type defaultKeyringBackend struct{}

func (defaultKeyringBackend) Set(service, account, value string) error {
	return keyring.Set(service, account, value)
}

func (defaultKeyringBackend) Get(service, account string) (string, error) {
	return keyring.Get(service, account)
}

func (defaultKeyringBackend) Delete(service, account string) error {
	return keyring.Delete(service, account)
}

type KeyringStore struct {
	backend keyringBackend
}

func NewKeyringStore() *KeyringStore {
	return NewKeyringStoreWithBackend(defaultKeyringBackend{})
}

func NewKeyringStoreWithBackend(backend keyringBackend) *KeyringStore {
	if backend == nil {
		backend = defaultKeyringBackend{}
	}
	return &KeyringStore{backend: backend}
}

func (store *KeyringStore) Mode() Mode {
	return ModeKeyring
}

func (store *KeyringStore) Available() error {
	if store == nil || store.backend == nil {
		return ErrUnavailable
	}
	_, err := store.backend.Get(defaultService, availabilityAccount)
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return fmt.Errorf("%w: keyring unavailable", ErrUnavailable)
}

func (store *KeyringStore) Save(key Key, secret Secret) error {
	if err := key.validate(); err != nil {
		return err
	}
	if secret.Empty() {
		return errors.New("secret value is required")
	}
	if err := store.Available(); err != nil {
		return err
	}
	if err := store.backend.Set(key.Service, key.Account, secret.Reveal()); err != nil {
		return fmt.Errorf("%w: keyring save failed", ErrUnavailable)
	}
	return nil
}

func (store *KeyringStore) Load(key Key) (Secret, error) {
	if err := key.validate(); err != nil {
		return Secret{}, err
	}
	if err := store.Available(); err != nil {
		return Secret{}, err
	}
	value, err := store.backend.Get(key.Service, key.Account)
	if errors.Is(err, keyring.ErrNotFound) {
		return Secret{}, ErrNotFound
	}
	if err != nil {
		return Secret{}, fmt.Errorf("%w: keyring load failed", ErrUnavailable)
	}
	return NewSecret(value)
}

func (store *KeyringStore) Delete(key Key) error {
	if err := key.validate(); err != nil {
		return err
	}
	if err := store.Available(); err != nil {
		return err
	}
	if err := store.backend.Delete(key.Service, key.Account); errors.Is(err, keyring.ErrNotFound) {
		return nil
	} else if err != nil {
		return fmt.Errorf("%w: keyring delete failed", ErrUnavailable)
	}
	return nil
}

func (store *KeyringStore) Exists(key Key) (bool, error) {
	if err := key.validate(); err != nil {
		return false, err
	}
	if err := store.Available(); err != nil {
		return false, err
	}
	_, err := store.backend.Get(key.Service, key.Account)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, keyring.ErrNotFound) {
		return false, nil
	}
	return false, fmt.Errorf("%w: keyring exists check failed", ErrUnavailable)
}
