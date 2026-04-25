package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/leonardomarzeuski/symbion/internal/vault"
)

const passphraseEnv = "SYMBION_PASSPHRASE"

func optionalPassphrase() []byte {
	value := os.Getenv(passphraseEnv)
	if value == "" {
		return nil
	}
	return []byte(value)
}

func requiredPassphrase() ([]byte, error) {
	passphrase := optionalPassphrase()
	if len(passphrase) == 0 {
		return nil, fmt.Errorf("set %s before using encrypted profiles", passphraseEnv)
	}
	return passphrase, nil
}

func explainVaultError(err error) error {
	if errors.Is(err, vault.ErrPassphraseRequired) {
		return fmt.Errorf("%w; set %s before using encrypted profiles", err, passphraseEnv)
	}
	return err
}
