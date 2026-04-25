package cli

import (
	"fmt"
	"os"

	"github.com/leonardomarzeuski/symbion/internal/vault"
	"github.com/spf13/cobra"
)

func newProfilesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "profiles",
		Short: "List saved .env profiles for the current project",
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

			profiles, err := store.List(project)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "Symbion Profiles")
			fmt.Fprintln(out, "----------------")
			fmt.Fprintf(out, "Project: %s\n", project)
			fmt.Fprintf(out, "Storage: %s\n\n", store.ProfileDir(project))

			if len(profiles) == 0 {
				fmt.Fprintln(out, "No profiles saved yet.")
				fmt.Fprintln(out, "Run: symbion save local")
				return nil
			}

			for _, profile := range profiles {
				if profile.Encrypted {
					fmt.Fprintf(out, "  - %s (encrypted)\n", profile.Name)
					continue
				}
				fmt.Fprintf(out, "  - %s\n", profile.Name)
			}

			return nil
		},
	}
}
