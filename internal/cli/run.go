package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/leonardomarzeuski/symbion/internal/parser"
	"github.com/leonardomarzeuski/symbion/internal/resolve"
	"github.com/leonardomarzeuski/symbion/internal/schema"
	"github.com/leonardomarzeuski/symbion/internal/vault"
	"github.com/spf13/cobra"
)

const maskedValue = "********"

func newRunCommand() *cobra.Command {
	var (
		dryRun     bool
		strict     bool
		isolated   bool
		noOverride bool
		showValues bool
	)

	cmd := &cobra.Command{
		Use:   "run [profile] -- <command> [args...]",
		Short: "Run a command with a resolved environment (no secrets on disk)",
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, command, err := splitRunArgs(cmd.ArgsLenAtDash(), args)
			if err != nil {
				return err
			}
			if len(command) == 0 && !dryRun {
				return fmt.Errorf("nothing to run; pass a command after -- or use --dry-run")
			}

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			loadedSchema, schemaFound := optionalSchema(cwd)
			source, sourceName, err := loadRunSource(cwd, profile, loadedSchema, schemaFound)
			if err != nil {
				return explainVaultError(err)
			}

			opts := resolve.Options{
				InheritShell: !isolated,
				Override:     !noOverride,
				SourceName:   sourceName,
				Defaults:     schemaDefaults(loadedSchema),
				SecretKeys:   schemaSecretKeys(loadedSchema),
			}
			vars := resolve.Resolve(os.Environ(), source, opts)

			if strict {
				if missing := missingRequired(loadedSchema, vars); len(missing) > 0 {
					printMissingRequired(cmd, missing)
					return &ExitError{Code: 1}
				}
			}

			if dryRun {
				printRunDryRun(cmd, vars, showValues)
				return nil
			}

			return runProcess(command, resolve.ToEnviron(vars),
				cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}

	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "resolve and print the environment without running")
	cmd.Flags().BoolVar(&strict, "strict", false, "refuse to run if a required variable is missing")
	cmd.Flags().BoolVar(&isolated, "isolated", false, "do not inherit the ambient shell environment")
	cmd.Flags().BoolVar(&noOverride, "no-override", false, "let existing shell variables win over the profile")
	cmd.Flags().BoolVar(&showValues, "show-values", false, "with --dry-run, reveal masked secret values")
	return cmd
}

// splitRunArgs separates an optional profile from the command, using cobra's
// ArgsLenAtDash (the count of args before "--", or -1 when "--" is absent).
func splitRunArgs(dash int, args []string) (string, []string, error) {
	if dash < 0 {
		switch len(args) {
		case 0:
			return "", nil, nil
		case 1:
			return args[0], nil, nil
		default:
			return "", nil, fmt.Errorf("put the command after --, e.g. symbion run %s -- <command>", args[0])
		}
	}
	before, command := args[:dash], args[dash:]
	switch len(before) {
	case 0:
		return "", command, nil
	case 1:
		return before[0], command, nil
	default:
		return "", nil, fmt.Errorf("expected at most one profile before --, got %d arguments", len(before))
	}
}

// optionalSchema loads .symbion.yaml if present. When absent it returns an
// empty schema (so .env runs still work) and false.
func optionalSchema(cwd string) (*schema.Schema, bool) {
	s, err := schema.Load(filepath.Join(cwd, schema.DefaultFilename))
	if err != nil {
		return schema.New(filepath.Base(cwd)), false
	}
	return s, true
}

func loadRunSource(cwd, profile string, s *schema.Schema, schemaFound bool) (map[string]string, resolve.Source, error) {
	switch profile {
	case "", ".env", "env", "current":
		values, found, err := parser.LoadLocalEnv(cwd)
		if err != nil {
			return nil, "", err
		}
		if !found {
			return nil, "", fmt.Errorf(".env not found")
		}
		return values, resolve.SourceEnvFile, nil
	default:
		if !schemaFound {
			return nil, "", fmt.Errorf("%s not found; run symbion init or symbion scan first", schema.DefaultFilename)
		}
		store, err := vault.NewDefaultStore()
		if err != nil {
			return nil, "", err
		}
		data, _, err := store.ReadProfile(s.Project, profile, optionalPassphrase())
		if err != nil {
			return nil, "", err
		}
		values, err := parser.ParseEnv(data)
		if err != nil {
			return nil, "", fmt.Errorf("parse profile %q: %w", profile, err)
		}
		return values, resolve.SourceProfile, nil
	}
}

func schemaDefaults(s *schema.Schema) map[string]string {
	out := make(map[string]string)
	for _, spec := range s.Envs {
		if strings.TrimSpace(spec.Default) != "" {
			out[spec.Key] = spec.Default
		}
	}
	return out
}

func schemaSecretKeys(s *schema.Schema) map[string]bool {
	out := make(map[string]bool)
	for _, spec := range s.Envs {
		if spec.Secret {
			out[spec.Key] = true
		}
	}
	return out
}

func missingRequired(s *schema.Schema, vars []resolve.Var) []string {
	present := make(map[string]bool, len(vars))
	for _, v := range vars {
		present[v.Key] = true
	}
	var missing []string
	for _, spec := range s.Envs {
		if spec.Required && !spec.Deprecated && !present[spec.Key] {
			missing = append(missing, spec.Key)
		}
	}
	sort.Strings(missing)
	return missing
}

func printMissingRequired(cmd *cobra.Command, missing []string) {
	out := cmd.ErrOrStderr()
	fmt.Fprintln(out, "Refusing to run: required variables are missing:")
	for _, key := range missing {
		fmt.Fprintf(out, "  - %s\n", key)
	}
}

func printRunDryRun(cmd *cobra.Command, vars []resolve.Var, showValues bool) {
	out := cmd.OutOrStdout()
	if showValues {
		fmt.Fprintln(out, "Warning: --show-values reveals secret values in plaintext.")
	}
	fmt.Fprintf(out, "Resolved environment (%d variables):\n", len(vars))
	for _, v := range vars {
		value := v.Value
		if v.Secret && !showValues {
			value = maskedValue
		}
		fmt.Fprintf(out, "  %s=%s  (%s)\n", v.Key, value, v.Source)
	}
}
