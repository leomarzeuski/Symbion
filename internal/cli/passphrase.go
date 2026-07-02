package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/leonardomarzeuski/symbion/internal/keychain"
	"github.com/leonardomarzeuski/symbion/internal/vault"
	"github.com/spf13/cobra"
)

const passphraseEnv = "SYMBION_PASSPHRASE"

// Keychain access is indirected so tests can stub it.
var (
	keychainGet   = keychain.Get
	keychainSet   = keychain.Set
	keychainClear = keychain.Delete
)

func optionalPassphrase() []byte {
	if value := os.Getenv(passphraseEnv); value != "" {
		return []byte(value)
	}
	if pass, ok, err := keychainGet(); err == nil && ok && pass != "" {
		return []byte(pass)
	}
	return nil
}

func requiredPassphrase() ([]byte, error) {
	passphrase := optionalPassphrase()
	if len(passphrase) == 0 {
		return nil, fmt.Errorf("no passphrase found; set %s or run 'symbion passphrase set'", passphraseEnv)
	}
	return passphrase, nil
}

func explainVaultError(err error) error {
	if errors.Is(err, vault.ErrPassphraseRequired) {
		return fmt.Errorf("%w; set %s or run 'symbion passphrase set'", err, passphraseEnv)
	}
	return err
}

func newPassphraseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "passphrase",
		Short: "Manage the encryption passphrase stored in the OS keychain",
	}
	cmd.AddCommand(newPassphraseSetCommand())
	cmd.AddCommand(newPassphraseClearCommand())
	return cmd
}

func newPassphraseSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set",
		Short: "Store the encryption passphrase in the OS keychain",
		Long:  "Store the encryption passphrase in the OS keychain. Reads SYMBION_PASSPHRASE if set, otherwise from stdin.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			pass := os.Getenv(passphraseEnv)
			if pass == "" {
				fmt.Fprint(cmd.ErrOrStderr(), "Enter passphrase: ")
				line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				pass = strings.TrimRight(line, "\r\n")
			}
			if pass == "" {
				return fmt.Errorf("passphrase is empty; set %s or pipe it via stdin", passphraseEnv)
			}
			if err := keychainSet(pass); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Passphrase stored in the keychain.")
			return nil
		},
	}
}

func newPassphraseClearCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Remove the encryption passphrase from the OS keychain",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := keychainClear(); err != nil {
				fmt.Fprintln(cmd.OutOrStdout(), "No passphrase was stored.")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Passphrase removed from the keychain.")
			return nil
		},
	}
}
