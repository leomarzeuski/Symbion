package cli

import (
	"os"

	"github.com/leonardomarzeuski/symbion/internal/doctor"
	"github.com/leonardomarzeuski/symbion/internal/output"
	"github.com/spf13/cobra"
)

func newDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Validate local environment files",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			report, err := doctor.InspectProject(cwd)
			if err != nil {
				return err
			}

			output.PrintDoctorReport(cmd.OutOrStdout(), report)
			if report.HasIssues() {
				return &ExitError{Code: 1}
			}

			return nil
		},
	}
}
