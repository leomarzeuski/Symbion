package cli

import (
	"fmt"
	"os"

	"github.com/leonardomarzeuski/symbion/internal/vault"
	"github.com/spf13/cobra"
)

func newBackupsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "backups",
		Short: "List automatic .env backups for the current project",
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

			backups, err := store.ListBackups(project)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "Symbion Backups")
			fmt.Fprintln(out, "---------------")
			fmt.Fprintf(out, "Project: %s\n", project)
			fmt.Fprintf(out, "Storage: %s\n\n", store.BackupDir(project))

			if len(backups) == 0 {
				fmt.Fprintln(out, "No backups found yet.")
				return nil
			}

			for _, backup := range backups {
				fmt.Fprintf(out, "  - %s\n", backup.Name)
			}

			return nil
		},
	}
}
