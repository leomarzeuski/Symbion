package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/leonardomarzeuski/symbion/internal/resolve"
	"github.com/spf13/cobra"
)

func newExportCommand() *cobra.Command {
	var strict bool

	cmd := &cobra.Command{
		Use:   "export [profile]",
		Short: "Print resolved environment as shell export statements for eval",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			profile := ""
			if len(args) == 1 {
				profile = args[0]
			}

			loadedSchema, schemaFound := optionalSchema(cwd)
			source, sourceName, err := loadRunSource(cwd, profile, loadedSchema, schemaFound)
			if err != nil {
				return explainVaultError(err)
			}

			vars := resolve.Managed(source, schemaDefaults(loadedSchema), schemaSecretKeys(loadedSchema), sourceName)

			if strict {
				if missing := missingRequired(loadedSchema, vars); len(missing) > 0 {
					printMissingRequired(cmd, missing)
					return &ExitError{Code: 1}
				}
			}

			out := cmd.OutOrStdout()
			for _, v := range vars {
				fmt.Fprintf(out, "export %s=%s\n", v.Key, shellQuote(v.Value))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&strict, "strict", false, "exit 1 without emitting if a required variable is missing")
	return cmd
}

// shellQuote wraps a value in single quotes with POSIX-safe escaping so it
// survives eval: a literal single quote becomes '\'' (close, escaped quote, reopen).
func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}
