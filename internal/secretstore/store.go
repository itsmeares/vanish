package secretstore

import (
	"errors"
	"fmt"
	"strings"
)

type Mode string

const (
	ModeKeyring Mode = "keyring"
	ModeFile    Mode = "file"
	ModeMemory  Mode = "memory"
)

var (
	ErrNotFound                     = errors.New("secret not found")
	ErrUnavailable                  = errors.New("secret store unavailable")
	ErrFallbackConfirmationRequired = errors.New("file fallback requires explicit confirmation")
)

type Key struct {
	Service string
	Account string
}

func (key Key) validate() error {
	if strings.TrimSpace(key.Service) == "" {
		return errors.New("secret service is required")
	}
	if strings.TrimSpace(key.Account) == "" {
		return errors.New("secret account is required")
	}
	return nil
}

func (key Key) stableID() string {
	return strings.TrimSpace(key.Service) + "\x00" + strings.TrimSpace(key.Account)
}

type Secret struct {
	value string
}

func NewSecret(value string) (Secret, error) {
	if strings.TrimSpace(value) == "" {
		return Secret{}, errors.New("secret value is required")
	}
	return Secret{value: value}, nil
}

func MustSecret(value string) Secret {
	secret, err := NewSecret(value)
	if err != nil {
		panic(err)
	}
	return secret
}

func (secret Secret) Reveal() string {
	return secret.value
}

func (secret Secret) Empty() bool {
	return secret.value == ""
}

func (secret Secret) String() string {
	if secret.Empty() {
		return ""
	}
	return "[redacted]"
}

func (secret Secret) GoString() string {
	return secret.String()
}

func (secret Secret) MarshalJSON() ([]byte, error) {
	if secret.Empty() {
		return []byte(`""`), nil
	}
	return []byte(`"[redacted]"`), nil
}

type Store interface {
	Mode() Mode
	Available() error
	Save(Key, Secret) error
	Load(Key) (Secret, error)
	Delete(Key) error
	Exists(Key) (bool, error)
}

type OperationResult struct {
	Mode         Mode
	UsedFallback bool
	Migrated     bool
}

type Vault struct {
	Primary           Store
	Fallback          Store
	AllowFileFallback bool
}

func (vault Vault) Save(key Key, secret Secret) (OperationResult, error) {
	if secret.Empty() {
		return OperationResult{}, errors.New("secret value is required")
	}

	primaryErr := storeAvailable(vault.Primary)
	if primaryErr == nil {
		if err := vault.Primary.Save(key, secret); err != nil {
			return OperationResult{}, err
		}
		_ = deleteIfExists(vault.Fallback, key)
		return OperationResult{Mode: vault.Primary.Mode()}, nil
	}

	if !vault.AllowFileFallback {
		return OperationResult{}, unavailableError(primaryErr)
	}
	if vault.Fallback == nil {
		return OperationResult{}, unavailableError(primaryErr)
	}
	if err := vault.Fallback.Save(key, secret); err != nil {
		return OperationResult{}, err
	}
	return OperationResult{Mode: vault.Fallback.Mode(), UsedFallback: true}, nil
}

func (vault Vault) Load(key Key) (Secret, OperationResult, error) {
	primaryErr := storeAvailable(vault.Primary)
	if primaryErr == nil {
		primaryExists, err := vault.Primary.Exists(key)
		if err != nil {
			return Secret{}, OperationResult{}, err
		}
		if primaryExists {
			secret, err := vault.Primary.Load(key)
			if err != nil {
				return Secret{}, OperationResult{}, err
			}
			_ = deleteIfExists(vault.Fallback, key)
			return secret, OperationResult{Mode: vault.Primary.Mode()}, nil
		}

		fallbackExists, err := storeExists(vault.Fallback, key)
		if err != nil {
			return Secret{}, OperationResult{}, err
		}
		if fallbackExists {
			secret, err := vault.Fallback.Load(key)
			if err != nil {
				return Secret{}, OperationResult{}, err
			}
			if err := vault.Primary.Save(key, secret); err != nil {
				return Secret{}, OperationResult{}, err
			}
			if err := vault.Fallback.Delete(key); err != nil {
				return Secret{}, OperationResult{}, err
			}
			return secret, OperationResult{Mode: vault.Primary.Mode(), Migrated: true}, nil
		}

		return Secret{}, OperationResult{Mode: vault.Primary.Mode()}, ErrNotFound
	}

	if !vault.AllowFileFallback {
		return Secret{}, OperationResult{}, unavailableError(primaryErr)
	}
	if vault.Fallback == nil {
		return Secret{}, OperationResult{}, unavailableError(primaryErr)
	}
	secret, err := vault.Fallback.Load(key)
	if err != nil {
		return Secret{}, OperationResult{}, err
	}
	return secret, OperationResult{Mode: vault.Fallback.Mode(), UsedFallback: true}, nil
}

func (vault Vault) Delete(key Key) error {
	primaryErr := storeAvailable(vault.Primary)
	if primaryErr == nil {
		if err := vault.Primary.Delete(key); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		return deleteIfExists(vault.Fallback, key)
	}

	if !vault.AllowFileFallback {
		return unavailableError(primaryErr)
	}
	return deleteIfExists(vault.Fallback, key)
}

type Status struct {
	PrimaryMode       Mode
	PrimaryAvailable  bool
	PrimaryExists     bool
	FallbackMode      Mode
	FallbackAvailable bool
	FallbackExists    bool
	ActiveMode        Mode
	MigrationNeeded   bool
}

func (vault Vault) Status(key Key) Status {
	status := Status{}
	if vault.Primary != nil {
		status.PrimaryMode = vault.Primary.Mode()
		status.PrimaryAvailable = vault.Primary.Available() == nil
		if status.PrimaryAvailable {
			status.PrimaryExists, _ = vault.Primary.Exists(key)
		}
	}
	if vault.Fallback != nil {
		status.FallbackMode = vault.Fallback.Mode()
		status.FallbackAvailable = vault.Fallback.Available() == nil
		if status.FallbackAvailable {
			status.FallbackExists, _ = vault.Fallback.Exists(key)
		}
	}
	if status.PrimaryAvailable {
		status.ActiveMode = status.PrimaryMode
		status.MigrationNeeded = !status.PrimaryExists && status.FallbackExists
	} else if vault.AllowFileFallback && status.FallbackAvailable {
		status.ActiveMode = status.FallbackMode
	}
	return status
}

func storeAvailable(store Store) error {
	if store == nil {
		return ErrUnavailable
	}
	if err := store.Available(); err != nil {
		return err
	}
	return nil
}

func storeExists(store Store, key Key) (bool, error) {
	if store == nil {
		return false, nil
	}
	if err := store.Available(); err != nil {
		return false, nil
	}
	return store.Exists(key)
}

func deleteIfExists(store Store, key Key) error {
	if store == nil {
		return nil
	}
	if err := store.Available(); err != nil {
		return nil
	}
	if err := store.Delete(key); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}

func unavailableError(err error) error {
	if err == nil {
		err = ErrUnavailable
	}
	return fmt.Errorf("%w: %v", ErrUnavailable, err)
}
