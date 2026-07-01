package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/leonardomarzeuski/symbion/internal/codescan"
	"github.com/leonardomarzeuski/symbion/internal/parser"
	"github.com/leonardomarzeuski/symbion/internal/schema"
	"github.com/spf13/cobra"
)

func newScanCommand() *cobra.Command {
	var scanCode bool

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan .env.example (and optionally source code) and update .symbion.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			schemaPath := filepath.Join(cwd, schema.DefaultFilename)
			currentSchema, created, err := loadOrCreateSchema(schemaPath, filepath.Base(cwd))
			if err != nil {
				return err
			}

			envExamplePath := filepath.Join(cwd, ".env.example")
			envExample, found, err := parser.LoadEnvFile(envExamplePath)
			if err != nil {
				return err
			}

			added := currentSchema.AddKeys(parser.SortedKeys(envExample))

			codeAdded := 0
			if scanCode {
				keys, err := codescan.Scan(cwd)
				if err != nil {
					return err
				}
				fromCode := currentSchema.AddKeys(keys)
				codeAdded = len(fromCode)
				added = append(added, fromCode...)
				sort.Strings(added)
			}

			if err := schema.Save(schemaPath, currentSchema); err != nil {
				return err
			}

			printScanSummary(cmd, currentSchema.Project, created, found, len(envExample), added, scanCode, codeAdded)
			return nil
		},
	}

	cmd.Flags().BoolVar(&scanCode, "code", false, "also scan source code for env var references (process.env, os.Getenv, ...)")
	return cmd
}

func loadOrCreateSchema(path string, project string) (*schema.Schema, bool, error) {
	currentSchema, err := schema.Load(path)
	if err == nil {
		return currentSchema, false, nil
	}
	if !os.IsNotExist(err) {
		return nil, false, err
	}

	return schema.New(project), true, nil
}

func printScanSummary(cmd *cobra.Command, project string, created bool, envExampleFound bool, envExampleCount int, added []string, scanCode bool, codeAdded int) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Symbion Scan")
	fmt.Fprintln(out, "------------")
	fmt.Fprintf(out, "Project: %s\n\n", project)

	if created {
		fmt.Fprintf(out, "[OK] Created %s\n", schema.DefaultFilename)
	} else {
		fmt.Fprintf(out, "[OK] Updated %s\n", schema.DefaultFilename)
	}

	if envExampleFound {
		fmt.Fprintf(out, "[OK] Loaded .env.example (%d keys)\n", envExampleCount)
	} else {
		fmt.Fprintln(out, "[!] .env.example not found; schema was kept empty")
	}

	if scanCode {
		fmt.Fprintf(out, "[OK] Scanned source code (%d new key(s))\n", codeAdded)
	}

	if len(added) == 0 {
		fmt.Fprintln(out, "[OK] No new variables added")
		return
	}

	fmt.Fprintln(out, "\nAdded variables:")
	for _, key := range added {
		fmt.Fprintf(out, "  - %s\n", key)
	}
	fmt.Fprintf(out, "\nSummary: %d new variable(s) added\n", len(added))
}
