// Package trust records which project directories are allowed to auto-load
// their .env into the shell. Trust is per-directory and pinned to the .env's
// content hash, so a changed .env re-blocks until re-allowed.
package trust

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

const DefaultDirname = ".symbion"

type Store struct{ Root string }

func NewDefaultStore() (Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Store{}, err
	}
	return Store{Root: filepath.Join(home, DefaultDirname)}, nil
}

func (s Store) Allow(dir string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	envHash, err := envFileHash(absDir)
	if err != nil {
		return err
	}
	if envHash == "" {
		return fmt.Errorf("no .env in %s to allow", absDir)
	}
	path := s.trustPath(absDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(envHash+"\n"+absDir+"\n"), 0o600)
}

func (s Store) Deny(dir string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if err := os.Remove(s.trustPath(absDir)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s Store) IsTrusted(dir string) (bool, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(s.trustPath(absDir))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	current, err := envFileHash(absDir)
	if err != nil || current == "" {
		return false, err
	}
	return storedHash(data) == current, nil
}

func (s Store) trustPath(absDir string) string {
	sum := sha256.Sum256([]byte(absDir))
	return filepath.Join(s.Root, "trust", hex.EncodeToString(sum[:]))
}

// envFileHash returns the hex sha256 of dir/.env, or "" if it does not exist.
func envFileHash(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func storedHash(data []byte) string {
	for i, b := range data {
		if b == '\n' {
			return string(data[:i])
		}
	}
	return string(data)
}
