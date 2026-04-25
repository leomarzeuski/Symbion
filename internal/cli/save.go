package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/leonardomarzeuski/symbion/internal/vault"
	"github.com/spf13/cobra"
)

func newSaveCommand() *cobra.Command {
	var encrypt bool

	cmd := &cobra.Command{
		Use:   "save <profile>",
		Short: "Save the current .env as a reusable profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			project, err := currentProject(cwd)
			if err != nil {
				return err
			}

			store, err := vault.NewDefaultStore()
			if err != nil {
				return err
			}

			profile := args[0]
			envPath := filepath.Join(cwd, ".env")
			var path string
			if encrypt {
				passphrase, err := requiredPassphrase()
				if err != nil {
					return err
				}
				path, err = store.SaveEncrypted(project, profile, envPath, passphrase)
				if err != nil {
					return explainVaultError(err)
				}
			} else {
				path, err = store.Save(project, profile, envPath)
				if err != nil {
					return err
				}
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Saved .env as profile %q for project %q.\n", profile, project)
			if encrypt {
				fmt.Fprintln(out, "Encryption: enabled")
			}
			fmt.Fprintf(out, "Stored at: %s\n", path)
			return nil
		},
	}

	cmd.Flags().BoolVar(&encrypt, "encrypt", false, "encrypt the saved profile using SYMBION_PASSPHRASE")
	return cmd
}
