package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/leonardomarzeuski/symbion/internal/doctor"
	"github.com/leonardomarzeuski/symbion/internal/output"
	"github.com/spf13/cobra"
)

// doctorJSON is the machine-readable shape of a doctor report: the report
// fields plus a computed issue count and pass flag.
type doctorJSON struct {
	doctor.Report
	Issues int  `json:"issue_count"`
	OK     bool `json:"ok"`
}

func newDoctorCommand() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
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

			if asJSON {
				payload := doctorJSON{Report: report, Issues: report.IssueCount(), OK: !report.HasIssues()}
				data, err := json.MarshalIndent(payload, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				output.PrintDoctorReport(cmd.OutOrStdout(), report)
			}

			if report.HasIssues() {
				return &ExitError{Code: 1}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "output the report as JSON")
	return cmd
}
