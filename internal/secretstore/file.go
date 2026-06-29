package secretstore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const fileSecretDirName = "secrets"

type FileStore struct {
	dir       string
	confirmed bool
}

func NewFileStore(appDir string, confirmed bool) (*FileStore, error) {
	appDir = strings.TrimSpace(appDir)
	if appDir == "" {
		return nil, errors.New("app dir is required")
	}
	dir := filepath.Join(filepath.Clean(appDir), fileSecretDirName)
	return &FileStore{dir: dir, confirmed: confirmed}, nil
}

func (store *FileStore) Mode() Mode {
	return ModeFile
}

func (store *FileStore) Dir() string {
	if store == nil {
		return ""
	}
	return store.dir
}

func (store *FileStore) Available() error {
	if store == nil {
		return ErrUnavailable
	}
	if !store.confirmed {
		return ErrFallbackConfirmationRequired
	}
	return nil
}

func (store *FileStore) Save(key Key, secret Secret) error {
	if err := key.validate(); err != nil {
		return err
	}
	if secret.Empty() {
		return errors.New("secret value is required")
	}
	if err := store.ensureDir(); err != nil {
		return err
	}
	path, err := store.pathForKey(key)
	if err != nil {
		return err
	}

	temp, err := os.OpenFile(path+".tmp", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("%w: file fallback save failed", ErrUnavailable)
	}
	tempName := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempName)
		}
	}()

	if _, err := temp.WriteString(secret.Reveal()); err != nil {
		_ = temp.Close()
		return fmt.Errorf("%w: file fallback save failed", ErrUnavailable)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("%w: file fallback save failed", ErrUnavailable)
	}
	if err := os.Chmod(tempName, 0o600); err != nil {
		return fmt.Errorf("%w: file fallback permission update failed", ErrUnavailable)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("%w: file fallback save failed", ErrUnavailable)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("%w: file fallback permission update failed", ErrUnavailable)
	}
	cleanup = false
	return nil
}

func (store *FileStore) Load(key Key) (Secret, error) {
	if err := key.validate(); err != nil {
		return Secret{}, err
	}
	if err := store.Available(); err != nil {
		return Secret{}, err
	}
	path, err := store.pathForKey(key)
	if err != nil {
		return Secret{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Secret{}, ErrNotFound
	}
	if err != nil {
		return Secret{}, fmt.Errorf("%w: file fallback load failed", ErrUnavailable)
	}
	return NewSecret(string(data))
}

func (store *FileStore) Delete(key Key) error {
	if err := key.validate(); err != nil {
		return err
	}
	if err := store.Available(); err != nil {
		return err
	}
	path, err := store.pathForKey(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("%w: file fallback delete failed", ErrUnavailable)
	}
	return nil
}

func (store *FileStore) Exists(key Key) (bool, error) {
	if err := key.validate(); err != nil {
		return false, err
	}
	if err := store.Available(); err != nil {
		return false, err
	}
	path, err := store.pathForKey(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("%w: file fallback exists check failed", ErrUnavailable)
}

func (store *FileStore) ensureDir() error {
	if err := store.Available(); err != nil {
		return err
	}
	if err := os.MkdirAll(store.dir, 0o700); err != nil {
		return fmt.Errorf("%w: file fallback dir create failed", ErrUnavailable)
	}
	if err := os.Chmod(store.dir, 0o700); err != nil {
		return fmt.Errorf("%w: file fallback dir permission update failed", ErrUnavailable)
	}
	return nil
}

func (store *FileStore) pathForKey(key Key) (string, error) {
	if store == nil || strings.TrimSpace(store.dir) == "" {
		return "", ErrUnavailable
	}
	sum := sha256.Sum256([]byte(key.stableID()))
	name := hex.EncodeToString(sum[:]) + ".secret"
	path := filepath.Join(store.dir, name)
	cleanDir := filepath.Clean(store.dir)
	cleanPath := filepath.Clean(path)
	if filepath.Dir(cleanPath) != cleanDir {
		return "", errors.New("secret path escaped app dir")
	}
	return cleanPath, nil
}
