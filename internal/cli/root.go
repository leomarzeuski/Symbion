package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

const version = "0.1.0"

type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit status %d", e.Code)
}

func Execute() error {
	return NewRootCommand(os.Stdout, os.Stderr).Execute()
}

func NewRootCommand(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "symbion",
		Short:         "Local environment intelligence for development projects",
		Long:          "Symbion keeps .env, .env.example, .symbion.yaml and Docker Compose references in sync.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.AddCommand(newInitCommand())
	cmd.AddCommand(newScanCommand())
	cmd.AddCommand(newDoctorCommand())
	cmd.AddCommand(newSaveCommand())
	cmd.AddCommand(newUseCommand())
	cmd.AddCommand(newProfilesCommand())
	cmd.AddCommand(newBackupsCommand())
	cmd.AddCommand(newUndoCommand())
	cmd.AddCommand(newDiffCommand())
	cmd.AddCommand(newRunCommand())
	cmd.AddCommand(newExportCommand())
	cmd.AddCommand(newAllowCommand())
	cmd.AddCommand(newDenyCommand())
	cmd.AddCommand(newHookCommand())
	cmd.AddCommand(newHookEnvCommand())

	return cmd
}
