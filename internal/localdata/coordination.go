package localdata

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gofrs/flock"
)

var ErrActive = errors.New("local data is active in another process")

type Lease struct {
	lock *flock.Flock
}

func TryUse(workspaceDir string) (*Lease, error) {
	return tryLock(workspaceDir, true)
}

func TryWipe(workspaceDir string) (*Lease, error) {
	return tryLock(workspaceDir, false)
}

func (lease *Lease) Close() error {
	if lease == nil || lease.lock == nil {
		return nil
	}
	err := lease.lock.Close()
	lease.lock = nil
	return err
}

func tryLock(workspaceDir string, shared bool) (*Lease, error) {
	path, err := coordinationLockPath(workspaceDir)
	if err != nil {
		return nil, err
	}
	if info, statErr := os.Lstat(path); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, ErrActive
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}
	lock := flock.New(path, flock.SetPermissions(0o600))
	var locked bool
	if shared {
		locked, err = lock.TryRLock()
	} else {
		locked, err = lock.TryLock()
	}
	if err != nil {
		_ = lock.Close()
		return nil, err
	}
	if !locked {
		_ = lock.Close()
		return nil, ErrActive
	}
	return &Lease{lock: lock}, nil
}

func coordinationLockPath(workspaceDir string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(workspaceDir))
	if clean == "" || clean == "." {
		return "", errors.New("workspace dir is unavailable")
	}
	absolute, err := filepath.Abs(clean)
	if err != nil {
		return "", err
	}
	parent := filepath.Dir(absolute)
	if info, err := os.Lstat(parent); err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		if err != nil {
			return "", err
		}
		return "", ErrActive
	}
	identity := filepath.Clean(absolute)
	if runtime.GOOS == "windows" {
		identity = strings.ToLower(identity)
	}
	sum := sha256.Sum256([]byte(identity))
	name := ".vanish-local-data-" + hex.EncodeToString(sum[:]) + ".lock"
	return filepath.Join(parent, name), nil
}
