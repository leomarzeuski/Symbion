package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/leonardomarzeuski/symbion/internal/parser"
	"github.com/leonardomarzeuski/symbion/internal/resolve"
	"github.com/leonardomarzeuski/symbion/internal/trust"
	"github.com/spf13/cobra"
)

const zshHookSnippet = `_symbion_hook() {
  eval "$(command symbion hook-env)"
}
typeset -ag precmd_functions
if (( ! ${precmd_functions[(Ie)_symbion_hook]} )); then
  precmd_functions=(_symbion_hook $precmd_functions)
fi
`

func newHookCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "hook <shell>",
		Short: "Print shell integration for auto-loading trusted .env files",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "zsh" {
				return fmt.Errorf("unsupported shell %q; only zsh is supported", args[0])
			}
			fmt.Fprint(cmd.OutOrStdout(), zshHookSnippet)
			return nil
		},
	}
}

func newHookEnvCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "hook-env",
		Short:  "Emit export/unset for the current directory (used by the shell hook)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			emitHookEnv(cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
}

// emitHookEnv writes export/unset statements for the current directory. It
// never returns an error: the shell hook must not break the prompt.
func emitHookEnv(stdout, stderr io.Writer) {
	loaded := strings.Fields(os.Getenv("SYMBION_LOADED"))

	target := map[string]string{}
	if cwd, err := os.Getwd(); err == nil {
		if env, found, lerr := parser.LoadLocalEnv(cwd); lerr == nil && found {
			if store, serr := trust.NewDefaultStore(); serr == nil {
				trusted, terr := store.IsTrusted(cwd)
				switch {
				case terr == nil && trusted:
					sch, _ := optionalSchema(cwd)
					for _, v := range resolve.Managed(env, schemaDefaults(sch), nil, resolve.SourceEnvFile) {
						target[v.Key] = v.Value
					}
				case terr == nil && !trusted:
					fmt.Fprintf(stderr, "symbion: .env in %s is blocked; run 'symbion allow'\n", cwd)
				}
			}
		}
	}

	sort.Strings(loaded)
	for _, k := range loaded {
		if _, ok := target[k]; !ok {
			fmt.Fprintf(stdout, "unset %s\n", k)
		}
	}

	keys := make([]string, 0, len(target))
	for k := range target {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(stdout, "export %s=%s\n", k, shellQuote(target[k]))
	}

	if len(keys) > 0 {
		fmt.Fprintf(stdout, "export SYMBION_LOADED=%s\n", shellQuote(strings.Join(keys, " ")))
	} else if len(loaded) > 0 {
		fmt.Fprintln(stdout, "unset SYMBION_LOADED")
	}
}
