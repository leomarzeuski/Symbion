package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/leonardomarzeuski/symbion/internal/envdiff"
	"github.com/leonardomarzeuski/symbion/internal/parser"
	"github.com/leonardomarzeuski/symbion/internal/vault"
	"github.com/spf13/cobra"
)

func newUseCommand() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "use <profile>",
		Short: "Restore a saved profile into .env",
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
			if dryRun {
				if err := printUseDryRun(cmd, store, project, cwd, profile, optionalPassphrase()); err != nil {
					return explainVaultError(err)
				}
				return nil
			}

			result, err := store.UseProfile(project, profile, envPath, optionalPassphrase(), time.Now())
			if err != nil {
				return explainVaultError(err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Loaded profile %q into .env for project %q.\n", profile, project)
			if result.Source.Encrypted {
				fmt.Fprintln(out, "Encryption: enabled")
			}
			fmt.Fprintf(out, "Source: %s\n", result.Source.Path)
			if result.BackupCreated {
				fmt.Fprintf(out, "Backup: %s\n", result.Backup.Path)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the restore without changing .env")
	return cmd
}

func printUseDryRun(cmd *cobra.Command, store vault.Store, project string, cwd string, profile string, passphrase []byte) error {
	left, err := loadDiffSource(store, project, cwd, ".env", passphrase)
	if err != nil {
		return err
	}
	right, err := loadDiffSource(store, project, cwd, profile, passphrase)
	if err != nil {
		return err
	}

	leftValues, err := parser.ParseEnv(left.Data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", left.Label, err)
	}
	rightValues, err := parser.ParseEnv(right.Data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", right.Label, err)
	}

	printDiffReport(cmd, project, left, right, envdiff.Compare(leftValues, rightValues))
	fmt.Fprintln(cmd.OutOrStdout(), "\nDry run: no files changed.")
	return nil
}
