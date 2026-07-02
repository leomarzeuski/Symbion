package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/leonardomarzeuski/symbion/internal/vault"
	"github.com/spf13/cobra"
)

// sharedPath returns the committable, encrypted shared-profile path for a name.
// Files use a .enc extension (not .env*) so they are not caught by .gitignore.
func sharedPath(cwd, name string) string {
	return filepath.Join(cwd, ".symbion", "shared", name+".enc")
}

func newShareCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "share <name>",
		Short: "Encrypt the current .env into a committable shared profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := vault.ValidateProfileName(name); err != nil {
				return err
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			passphrase, err := requiredPassphrase()
			if err != nil {
				return err
			}

			data, err := os.ReadFile(filepath.Join(cwd, ".env"))
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf(".env not found; create it before sharing")
				}
				return err
			}
			encrypted, err := vault.Encrypt(data, passphrase)
			if err != nil {
				return err
			}

			path := sharedPath(cwd, name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, encrypted, 0o644); err != nil {
				return err
			}

			rel, relErr := filepath.Rel(cwd, path)
			if relErr != nil {
				rel = path
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Shared .env as %q.\nCommit %s to share it with your team.\n", name, rel)
			return nil
		},
	}
}

func newAdoptCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "adopt <name>",
		Short: "Decrypt a shared profile into .env (with a backup)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := vault.ValidateProfileName(name); err != nil {
				return err
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			encrypted, err := os.ReadFile(sharedPath(cwd, name))
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("shared profile %q not found; is it committed under .symbion/shared?", name)
				}
				return err
			}
			passphrase, err := requiredPassphrase()
			if err != nil {
				return err
			}
			data, err := vault.Decrypt(encrypted, passphrase)
			if err != nil {
				return explainVaultError(err)
			}

			project, err := currentProject(cwd)
			if err != nil {
				return err
			}
			store, err := vault.NewDefaultStore()
			if err != nil {
				return err
			}
			envPath := filepath.Join(cwd, ".env")
			backup, created, err := store.BackupCurrent(project, envPath, "before-adopt-"+name, time.Now())
			if err != nil {
				return err
			}
			if err := os.WriteFile(envPath, data, 0o600); err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Adopted shared profile %q into .env.\n", name)
			if created {
				fmt.Fprintf(out, "Backup: %s\n", backup.Path)
			}
			return nil
		},
	}
}
