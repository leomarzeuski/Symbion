# symbion hook Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Add direnv-style shell auto-load: `symbion hook zsh` integration, an explicit-allow content-hash trust store, and a hidden `hook-env` emitter that exports a trusted project's `.env` (managed vars) each prompt and unsets them on leave.

**Architecture:** New pure-ish `internal/trust` store (per-directory allow files keyed by `.env` content hash). New `internal/cli` commands `allow`/`deny` (trust), `hook` (zsh snippet), and hidden `hook-env` (emitter reusing `resolve.Managed` + `shellQuote`, tracking loaded keys via a `SYMBION_LOADED` marker var).

**Tech Stack:** Go stdlib (`crypto/sha256`, `os`, `sort`, `strings`), `cobra`, existing `internal/{resolve,parser,schema}`. No new dependencies.

## Global Constraints

- Go floor `go 1.23.0`; no new deps; module `github.com/leonardomarzeuski/symbion`.
- zsh only (v1). Trust is explicit-allow, content-hash pinned. Unload = unset-what-we-set.
- `hook-env` always exits 0 and only touches keys tracked in `SYMBION_LOADED`.
- Reuse `resolve.Managed`, `shellQuote`, `optionalSchema`, `schemaDefaults`, `parser.LoadEnvFile`, and the test helpers `runCommand`/`writeFile`.
- Commit after each task; `go test ./...` before each commit.

## Prerequisite
- [ ] `go test ./...` green (baseline; branch has run + export).

---

### Task 1: `internal/trust` store

**Files:** create `internal/trust/trust.go`, `internal/trust/trust_test.go`.

**Produces:** `type Store struct{ Root string }`; `NewDefaultStore() (Store, error)`; `(Store) Allow(dir string) error`; `(Store) Deny(dir string) error`; `(Store) IsTrusted(dir string) (bool, error)`.

- [ ] **Step 1: failing tests** — `internal/trust/trust_test.go`:
```go
package trust

import (
	"os"
	"path/filepath"
	"testing"
)

func writeEnv(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
}

func TestAllowIsTrustedDenyLifecycle(t *testing.T) {
	store := Store{Root: t.TempDir()}
	project := t.TempDir()
	writeEnv(t, project, "FOO=bar\n")

	if err := store.Allow(project); err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if ok, err := store.IsTrusted(project); err != nil || !ok {
		t.Fatalf("IsTrusted after Allow = %v, %v; want true", ok, err)
	}
	writeEnv(t, project, "FOO=changed\n") // editing .env re-blocks
	if ok, _ := store.IsTrusted(project); ok {
		t.Fatal("IsTrusted after edit = true; want false")
	}
	if err := store.Allow(project); err != nil {
		t.Fatalf("re-Allow: %v", err)
	}
	if err := store.Deny(project); err != nil {
		t.Fatalf("Deny: %v", err)
	}
	if ok, _ := store.IsTrusted(project); ok {
		t.Fatal("IsTrusted after Deny = true; want false")
	}
}

func TestAllowWithoutEnvErrors(t *testing.T) {
	store := Store{Root: t.TempDir()}
	if err := store.Allow(t.TempDir()); err == nil {
		t.Fatal("Allow without .env should error")
	}
}

func TestIsTrustedUnknownDir(t *testing.T) {
	store := Store{Root: t.TempDir()}
	if ok, err := store.IsTrusted(t.TempDir()); err != nil || ok {
		t.Fatalf("IsTrusted unknown = %v, %v; want false, nil", ok, err)
	}
}
```
- [ ] **Step 2:** `go test ./internal/trust/` → FAIL (undefined `Store`).
- [ ] **Step 3: implement** `internal/trust/trust.go`:
```go
// Package trust records which project directories are allowed to auto-load
// their .env into the shell. Trust is per-directory and pinned to the .env's
// content hash, so a changed .env re-blocks until re-allowed.
package trust

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

const DefaultDirname = ".symbion"

type Store struct{ Root string }

func NewDefaultStore() (Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Store{}, err
	}
	return Store{Root: filepath.Join(home, DefaultDirname)}, nil
}

func (s Store) Allow(dir string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	envHash, err := envFileHash(absDir)
	if err != nil {
		return err
	}
	if envHash == "" {
		return fmt.Errorf("no .env in %s to allow", absDir)
	}
	path := s.trustPath(absDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(envHash+"\n"+absDir+"\n"), 0o600)
}

func (s Store) Deny(dir string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if err := os.Remove(s.trustPath(absDir)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s Store) IsTrusted(dir string) (bool, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(s.trustPath(absDir))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	current, err := envFileHash(absDir)
	if err != nil || current == "" {
		return false, err
	}
	return storedHash(data) == current, nil
}

func (s Store) trustPath(absDir string) string {
	sum := sha256.Sum256([]byte(absDir))
	return filepath.Join(s.Root, "trust", hex.EncodeToString(sum[:]))
}

// envFileHash returns hex sha256 of dir/.env, or "" if it does not exist.
func envFileHash(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func storedHash(data []byte) string {
	for i, b := range data {
		if b == '\n' {
			return string(data[:i])
		}
	}
	return string(data)
}
```
- [ ] **Step 4:** `go test ./internal/trust/ -v` → PASS.
- [ ] **Step 5:** commit `feat(trust): add per-directory .env trust store`.

---

### Task 2: `allow` / `deny` commands

**Files:** create `internal/cli/allow.go`. (Registered in Task 4.)

**Produces:** `newAllowCommand()`, `newDenyCommand()`, `trustDirArg([]string) (string, error)`.

- [ ] **Step 1–2:** (tested together with hook in Task 4's `hook_test.go` via `allow` + `hook-env`.) Implement now:
`internal/cli/allow.go`:
```go
package cli

import (
	"fmt"
	"os"

	"github.com/leonardomarzeuski/symbion/internal/trust"
	"github.com/spf13/cobra"
)

func newAllowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "allow [dir]",
		Short: "Trust the current directory's .env for shell auto-load",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := trustDirArg(args)
			if err != nil {
				return err
			}
			store, err := trust.NewDefaultStore()
			if err != nil {
				return err
			}
			if err := store.Allow(dir); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Allowed %s for shell auto-load.\n", dir)
			return nil
		},
	}
}

func newDenyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "deny [dir]",
		Short: "Revoke shell auto-load trust for a directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := trustDirArg(args)
			if err != nil {
				return err
			}
			store, err := trust.NewDefaultStore()
			if err != nil {
				return err
			}
			if err := store.Deny(dir); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Revoked auto-load trust for %s.\n", dir)
			return nil
		},
	}
}

func trustDirArg(args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	return os.Getwd()
}
```
- Commit folded into Task 4 (allow/deny are exercised by the hook tests + registration).

---

### Task 3: `hook` + hidden `hook-env`

**Files:** create `internal/cli/hook.go`.

**Produces:** `newHookCommand()`, `newHookEnvCommand()`, `emitHookEnv(stdout, stderr io.Writer)`, `zshHookSnippet`.

- [ ] **Step 1:** implement `internal/cli/hook.go`:
```go
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
		if env, found, lerr := parser.LoadEnvFile(filepath.Join(cwd, ".env")); lerr == nil && found {
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
```
- Commit folded into Task 4 (needs registration + tests to be exercisable).

---

### Task 4: register + tests

**Files:** modify `internal/cli/root.go`; create `internal/cli/hook_test.go`.

- [ ] **Step 1: register** in `internal/cli/root.go`, after `cmd.AddCommand(newExportCommand())`:
```go
	cmd.AddCommand(newAllowCommand())
	cmd.AddCommand(newDenyCommand())
	cmd.AddCommand(newHookCommand())
	cmd.AddCommand(newHookEnvCommand())
```
- [ ] **Step 2: tests** — `internal/cli/hook_test.go`:
```go
package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestHookZshSnippet(t *testing.T) {
	out, _, err := runCommand(t, t.TempDir(), "hook", "zsh")
	if err != nil {
		t.Fatalf("hook zsh error = %v", err)
	}
	if !strings.Contains(out, "precmd_functions") || !strings.Contains(out, "symbion hook-env") {
		t.Fatalf("snippet missing pieces:\n%s", out)
	}
}

func TestHookUnsupportedShell(t *testing.T) {
	if _, _, err := runCommand(t, t.TempDir(), "hook", "bash"); err == nil {
		t.Fatal("hook bash should error")
	}
}

func TestHookEnvTrustedEmitsExports(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFile(t, filepath.Join(dir, ".symbion.yaml"), "project: billing-api\nenvs: []\n")
	writeFile(t, filepath.Join(dir, ".env"), "FOO=bar\n")
	if _, _, err := runCommand(t, dir, "allow"); err != nil {
		t.Fatalf("allow error = %v", err)
	}
	t.Setenv("SYMBION_LOADED", "OLD")
	out, _, err := runCommand(t, dir, "hook-env")
	if err != nil {
		t.Fatalf("hook-env error = %v", err)
	}
	for _, want := range []string{"unset OLD", "export FOO='bar'", "export SYMBION_LOADED='FOO'"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestHookEnvUntrustedUnloads(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFile(t, filepath.Join(dir, ".env"), "FOO=bar\n")
	t.Setenv("SYMBION_LOADED", "FOO")
	out, errOut, err := runCommand(t, dir, "hook-env")
	if err != nil {
		t.Fatalf("hook-env error = %v", err)
	}
	if !strings.Contains(out, "unset FOO") || !strings.Contains(out, "unset SYMBION_LOADED") {
		t.Fatalf("expected unloads:\n%s", out)
	}
	if strings.Contains(out, "export FOO") {
		t.Fatalf("untrusted must not export:\n%s", out)
	}
	if !strings.Contains(errOut, "blocked") {
		t.Fatalf("expected blocked note: %q", errOut)
	}
}

func TestHookEnvNoEnvUnloads(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SYMBION_LOADED", "FOO")
	out, _, err := runCommand(t, dir, "hook-env")
	if err != nil {
		t.Fatalf("hook-env error = %v", err)
	}
	if !strings.Contains(out, "unset FOO") || !strings.Contains(out, "unset SYMBION_LOADED") {
		t.Fatalf("expected unloads:\n%s", out)
	}
}

func TestAllowDenyChangesTrust(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFile(t, filepath.Join(dir, ".env"), "FOO=bar\n")
	if _, _, err := runCommand(t, dir, "allow"); err != nil {
		t.Fatalf("allow: %v", err)
	}
	if _, _, err := runCommand(t, dir, "deny"); err != nil {
		t.Fatalf("deny: %v", err)
	}
	out, _, err := runCommand(t, dir, "hook-env")
	if err != nil {
		t.Fatalf("hook-env: %v", err)
	}
	if strings.Contains(out, "export FOO") {
		t.Fatalf("after deny must not export:\n%s", out)
	}
}
```
- [ ] **Step 3:** `go test ./internal/cli/ -v` → PASS (all hook/allow/deny tests + existing).
- [ ] **Step 4: smoke** — `go build -o bin/symbion ./cmd/symbion`; in `testdata/valid`: `symbion hook zsh` prints the snippet; `symbion hook-env` prints the blocked note (untrusted); `symbion allow` then `symbion hook-env` prints `export DATABASE_URL=...` + `export SYMBION_LOADED=...`.
- [ ] **Step 5: commit** `feat(cli): add symbion hook, hook-env, allow, deny` (allow.go, hook.go, hook_test.go, root.go).

---

### Task 5: README docs
- [ ] Add Features bullet: `- Auto-load a trusted project's .env into your shell on cd, direnv-style (\`symbion hook\`).`
- [ ] Add a `### \`symbion hook\`` section (before `### \`symbion export\``) documenting `eval "$(symbion hook zsh)"`, `symbion allow`/`deny`, and the trust model.
- [ ] Commit `docs: document symbion hook`.

---

### Final verification
- [ ] `go vet ./...` clean; `go test ./...` ok; `go test -race ./...` ok.

## Self-Review
- Spec §4 trust → Task 1; §5 emitter → Task 3; §6 snippet → Task 3; §3 surface/§7 errors → Tasks 2-4; §8 testing → Tasks 1,4 + final; §9 files → Tasks 1-5. No placeholders. Signatures (`Store`/`Allow`/`Deny`/`IsTrusted`, `emitHookEnv`, `resolve.Managed`, `shellQuote`, `schemaDefaults`, `optionalSchema`, `parser.LoadEnvFile`) match their definitions.
