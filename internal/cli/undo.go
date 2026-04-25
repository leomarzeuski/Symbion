package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/leonardomarzeuski/symbion/internal/vault"
	"github.com/spf13/cobra"
)

func newUndoCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "undo",
		Short: "Restore the latest automatic .env backup",
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

			restored, currentBackup, currentBackupCreated, err := store.Undo(project, filepath.Join(cwd, ".env"), time.Now())
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Restored latest backup into .env for project %q.\n", project)
			fmt.Fprintf(out, "Restored: %s\n", restored.Path)
			if currentBackupCreated {
				fmt.Fprintf(out, "Previous .env backed up at: %s\n", currentBackup.Path)
			}
			return nil
		},
	}
}
