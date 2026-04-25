package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/leonardomarzeuski/symbion/internal/schema"
	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create a new .symbion.yaml schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			path := filepath.Join(cwd, schema.DefaultFilename)
			if _, err := os.Stat(path); err == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Symbion schema already exists: %s\n", schema.DefaultFilename)
				fmt.Fprintln(cmd.OutOrStdout(), "Nothing was overwritten.")
				return nil
			} else if !os.IsNotExist(err) {
				return err
			}

			project := filepath.Base(cwd)
			if err := schema.Save(path, schema.New(project)); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Created %s for project %q.\n", schema.DefaultFilename, project)
			return nil
		},
	}
}
