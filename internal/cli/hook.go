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

var hookSnippets = map[string]string{
	"zsh": `_symbion_hook() {
  eval "$(command symbion hook-env zsh)"
}
typeset -ag precmd_functions
if (( ! ${precmd_functions[(Ie)_symbion_hook]} )); then
  precmd_functions=(_symbion_hook $precmd_functions)
fi
`,
	"bash": `_symbion_hook() {
  eval "$(command symbion hook-env bash)"
}
case "$PROMPT_COMMAND" in
  *_symbion_hook*) ;;
  *) PROMPT_COMMAND="_symbion_hook${PROMPT_COMMAND:+;$PROMPT_COMMAND}" ;;
esac
`,
	"fish": `function _symbion_hook --on-event fish_prompt
    symbion hook-env fish | source
end
`,
}

func newHookCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "hook <shell>",
		Short: "Print shell integration for auto-loading trusted .env files",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			snippet, ok := hookSnippets[args[0]]
			if !ok {
				return fmt.Errorf("unsupported shell %q; supported: zsh, bash, fish", args[0])
			}
			fmt.Fprint(cmd.OutOrStdout(), snippet)
			return nil
		},
	}
}

func newHookEnvCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "hook-env [shell]",
		Short:  "Emit export/unset for the current directory (used by the shell hook)",
		Hidden: true,
		Args:   cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			shell := ""
			if len(args) == 1 {
				shell = args[0]
			}
			emitHookEnv(cmd.OutOrStdout(), cmd.ErrOrStderr(), shell)
			return nil
		},
	}
}

// shellSyntax formats set/unset statements for a target shell.
type shellSyntax struct {
	export func(key, value string) string
	unset  func(key string) string
}

func syntaxFor(shell string) shellSyntax {
	if shell == "fish" {
		return shellSyntax{
			export: func(k, v string) string { return fmt.Sprintf("set -gx %s %s", k, fishQuote(v)) },
			unset:  func(k string) string { return "set -e " + k },
		}
	}
	return shellSyntax{
		export: func(k, v string) string { return fmt.Sprintf("export %s=%s", k, shellQuote(v)) },
		unset:  func(k string) string { return "unset " + k },
	}
}

// fishQuote single-quotes a value using fish's escaping rules (only backslash
// and single quote are special inside single quotes).
func fishQuote(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `'`, `\'`)
	return "'" + v + "'"
}

// emitHookEnv writes set/unset statements for the current directory in the
// target shell's syntax. It never returns an error: the hook must not break
// the prompt.
func emitHookEnv(stdout, stderr io.Writer, shell string) {
	syntax := syntaxFor(shell)
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
			fmt.Fprintln(stdout, syntax.unset(k))
		}
	}

	keys := make([]string, 0, len(target))
	for k := range target {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintln(stdout, syntax.export(k, target[k]))
	}

	if len(keys) > 0 {
		fmt.Fprintln(stdout, syntax.export("SYMBION_LOADED", strings.Join(keys, " ")))
	} else if len(loaded) > 0 {
		fmt.Fprintln(stdout, syntax.unset("SYMBION_LOADED"))
	}
}
