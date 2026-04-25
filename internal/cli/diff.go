package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/leonardomarzeuski/symbion/internal/envdiff"
	"github.com/leonardomarzeuski/symbion/internal/parser"
	"github.com/leonardomarzeuski/symbion/internal/vault"
	"github.com/spf13/cobra"
)

type diffSource struct {
	Label     string
	Data      []byte
	Encrypted bool
}

func newDiffCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "diff <left> <right>",
		Short: "Compare two env sources without printing values",
		Args:  cobra.ExactArgs(2),
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

			passphrase := optionalPassphrase()
			left, err := loadDiffSource(store, project, cwd, args[0], passphrase)
			if err != nil {
				return explainVaultError(err)
			}
			right, err := loadDiffSource(store, project, cwd, args[1], passphrase)
			if err != nil {
				return explainVaultError(err)
			}

			leftValues, err := parser.ParseEnv(left.Data)
			if err != nil {
				return fmt.Errorf("parse %s: %w", left.Label, err)
			}
			rightValues, err := parser.ParseEnv(right.Data)
			if err != nil {
				return fmt.Errorf("parse %s: %w", right.Label, err)
			}

			result := envdiff.Compare(leftValues, rightValues)
			printDiffReport(cmd, project, left, right, result)
			if result.HasDifferences() {
				return &ExitError{Code: 1}
			}
			return nil
		},
	}
}

func loadDiffSource(store vault.Store, project string, cwd string, name string, passphrase []byte) (diffSource, error) {
	switch name {
	case ".env", "env", "current":
		data, err := os.ReadFile(filepath.Join(cwd, ".env"))
		if err != nil {
			if os.IsNotExist(err) {
				return diffSource{}, fmt.Errorf(".env not found")
			}
			return diffSource{}, err
		}
		return diffSource{Label: ".env", Data: data}, nil
	default:
		data, profile, err := store.ReadProfile(project, name, passphrase)
		if err != nil {
			return diffSource{}, err
		}

		label := profile.Name
		if profile.Encrypted {
			label += " (encrypted)"
		}
		return diffSource{Label: label, Data: data, Encrypted: profile.Encrypted}, nil
	}
}

func printDiffReport(cmd *cobra.Command, project string, left diffSource, right diffSource, result envdiff.Result) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Symbion Diff")
	fmt.Fprintln(out, "------------")
	fmt.Fprintf(out, "Project: %s\n", project)
	fmt.Fprintf(out, "Left: %s\n", left.Label)
	fmt.Fprintf(out, "Right: %s\n\n", right.Label)

	printDiffList(out, "Changed values", result.Changed)
	printDiffList(out, "Only in "+left.Label, result.OnlyLeft)
	printDiffList(out, "Only in "+right.Label, result.OnlyRight)
	fmt.Fprintf(out, "\nSame values: %d\n", result.SameCount)

	if result.HasDifferences() {
		fmt.Fprintf(out, "Summary: %d difference(s)\n", result.DifferenceCount())
		return
	}

	fmt.Fprintln(out, "Summary: no differences")
}

func printDiffList(out interface{ Write([]byte) (int, error) }, label string, items []string) {
	if len(items) == 0 {
		fmt.Fprintf(out, "[OK] %s: none\n", label)
		return
	}

	if len(items) == 1 {
		fmt.Fprintf(out, "[!] %s: %s\n", label, items[0])
		return
	}

	fmt.Fprintf(out, "[!] %s:\n", label)
	for _, item := range items {
		fmt.Fprintf(out, "  - %s\n", item)
	}
}
