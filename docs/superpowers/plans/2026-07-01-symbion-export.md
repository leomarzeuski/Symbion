# symbion export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `symbion export [profile]` that prints the resolved managed environment as POSIX `export KEY='VALUE'` statements for `eval "$(symbion export ...)"`.

**Architecture:** Add a pure managed-only resolver `resolve.Managed` (refactoring the existing merge/mark/sort out of `Resolve` into a shared `flatten`). Add an `export` cobra command that reuses `run`'s source-loading and schema helpers, quotes values POSIX-safely, and emits sorted `export` lines.

**Tech Stack:** Go stdlib (`strings`, `os/exec` in tests), `github.com/spf13/cobra`, existing `internal/{resolve,schema,parser,vault}` and `internal/cli` helpers. No new dependencies.

## Global Constraints

- Go version floor: `go 1.23.0` (from `go.mod`). Do not raise it. (Installed toolchain is 1.26.4.)
- No new module dependencies. Standard library + already-vendored modules only.
- Module path: `github.com/leonardomarzeuski/symbion`.
- POSIX shell output only (`export KEY='VALUE'`). No fish/JSON/dotenv, no TTY warning, no `unset`.
- `export` emits **only managed variables** (profile/`.env` + schema defaults) — never the inherited
  shell or a safe-minimum.
- Reuse existing `internal/cli` helpers: `optionalSchema`, `loadRunSource`, `schemaDefaults`,
  `schemaSecretKeys`, `missingRequired`, `printMissingRequired`, `explainVaultError`, `ExitError`, and
  the test helpers `runCommand`/`writeFile` and `findVar` (resolve tests).
- Exit-code discipline: `--strict` failure returns `&cli.ExitError{Code: 1}`; other failures are plain
  errors (exit 2).
- Commit after every task. Run `go test ./...` before each commit.

## Prerequisite

- [ ] **P1: Confirm baseline green**

Run: `go test ./...`
Expected: all packages `ok` (pre-change baseline; the branch already contains the `run` feature).

---

### Task 1: `resolve.Managed` (managed-only resolver)

**Files:**
- Modify: `internal/resolve/resolve.go` (refactor `Resolve`, add `flatten` and `Managed`)
- Test: `internal/resolve/resolve_test.go` (add `TestManaged`)

**Interfaces:**
- Consumes: `Var`, `Source` consts, `layer`, `looksSensitive` (existing).
- Produces: `func Managed(source, defaults map[string]string, secretKeys map[string]bool, sourceName Source) []Var` — flattens `[defaults, source]` (source wins), no shell/safe-minimum, sorted by Key, secret-marked. Also `func flatten(layers []layer, secretKeys map[string]bool) []Var` (private, shared with `Resolve`).

- [ ] **Step 1: Write the failing test**

Append to `internal/resolve/resolve_test.go`:
```go
func TestManaged(t *testing.T) {
	source := map[string]string{"FOO": "src", "MY_TOKEN": "t"}
	defaults := map[string]string{"FOO": "def", "BAZ": "dv"}

	vars := Managed(source, defaults, map[string]bool{}, SourceProfile)

	// source wins over default
	foo, _ := findVar(vars, "FOO")
	if foo.Value != "src" || foo.Source != SourceProfile {
		t.Errorf("FOO = %#v, want src/profile", foo)
	}
	// default fills a key missing from source
	baz, _ := findVar(vars, "BAZ")
	if baz.Value != "dv" || baz.Source != SourceDefault {
		t.Errorf("BAZ = %#v, want dv/default", baz)
	}
	// no shell variables leak in
	if _, ok := findVar(vars, "PATH"); ok {
		t.Error("Managed must not include shell variables")
	}
	// secret marking via heuristic still works
	tok, _ := findVar(vars, "MY_TOKEN")
	if !tok.Secret {
		t.Error("MY_TOKEN should be marked secret")
	}
	// sorted by key (BAZ first)
	if vars[0].Key != "BAZ" {
		t.Errorf("not sorted: first = %q", vars[0].Key)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/resolve/ -run TestManaged -v`
Expected: build failure / FAIL — `undefined: Managed`.

- [ ] **Step 3: Refactor `Resolve` and add `flatten` + `Managed`**

In `internal/resolve/resolve.go`, replace the entire existing `Resolve` function:
```go
func Resolve(shell []string, source map[string]string, opts Options) []Var {
	layers := buildLayers(shell, source, opts)

	type entry struct {
		value  string
		source Source
	}
	merged := make(map[string]entry)
	for _, l := range layers {
		for k, v := range l.values {
			merged[k] = entry{value: v, source: l.name}
		}
	}

	vars := make([]Var, 0, len(merged))
	for k, e := range merged {
		vars = append(vars, Var{
			Key:    k,
			Value:  e.value,
			Source: e.source,
			Secret: opts.SecretKeys[k] || looksSensitive(k),
		})
	}
	sort.Slice(vars, func(i, j int) bool { return vars[i].Key < vars[j].Key })
	return vars
}
```
with:
```go
func Resolve(shell []string, source map[string]string, opts Options) []Var {
	return flatten(buildLayers(shell, source, opts), opts.SecretKeys)
}

// Managed resolves only the managed variables: [defaults, source] with source
// winning. It includes no shell layer and no safe-minimum, for callers that
// load into an existing environment (e.g. `symbion export`). sourceName ("" =>
// SourceEnvFile) attributes the source layer.
func Managed(source, defaults map[string]string, secretKeys map[string]bool, sourceName Source) []Var {
	if sourceName == "" {
		sourceName = SourceEnvFile
	}
	return flatten([]layer{
		{name: SourceDefault, values: defaults},
		{name: sourceName, values: source},
	}, secretKeys)
}

// flatten merges ordered layers (later wins), marks secrets, and returns Vars
// sorted by Key.
func flatten(layers []layer, secretKeys map[string]bool) []Var {
	type entry struct {
		value  string
		source Source
	}
	merged := make(map[string]entry)
	for _, l := range layers {
		for k, v := range l.values {
			merged[k] = entry{value: v, source: l.name}
		}
	}

	vars := make([]Var, 0, len(merged))
	for k, e := range merged {
		vars = append(vars, Var{
			Key:    k,
			Value:  e.value,
			Source: e.source,
			Secret: secretKeys[k] || looksSensitive(k),
		})
	}
	sort.Slice(vars, func(i, j int) bool { return vars[i].Key < vars[j].Key })
	return vars
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/resolve/ -v`
Expected: PASS — `TestManaged` plus all existing resolve tests (behavior of `Resolve` is unchanged).

- [ ] **Step 5: Commit**

```bash
git add internal/resolve/resolve.go internal/resolve/resolve_test.go
git commit -m "feat(resolve): add managed-only resolver (Managed) via shared flatten"
```

---

### Task 2: `symbion export` command

**Files:**
- Create: `internal/cli/export.go`
- Modify: `internal/cli/root.go` (register the command)
- Test: `internal/cli/export_test.go`

**Interfaces:**
- Consumes: `resolve.Managed`, `resolve.Source` (Task 1); `optionalSchema`, `loadRunSource`, `schemaDefaults`, `schemaSecretKeys`, `missingRequired`, `printMissingRequired`, `explainVaultError`, `ExitError` (existing in `internal/cli`); test helpers `runCommand`, `writeFile` (cli_test.go).
- Produces: `func newExportCommand() *cobra.Command`; `func shellQuote(v string) string`.

- [ ] **Step 1: Write the failing test for `shellQuote`**

Create `internal/cli/export_test.go`:
```go
package cli

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"bar":   "'bar'",
		"a b":   "'a b'",
		"$HOME": "'$HOME'",
		"":      "''",
		"a'b":   `'a'\''b'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestShellQuote -v`
Expected: build failure / FAIL — `undefined: shellQuote`.

- [ ] **Step 3: Implement `export.go`**

Create `internal/cli/export.go`:
```go
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
```

- [ ] **Step 4: Run the `shellQuote` test to verify it passes**

Run: `go test ./internal/cli/ -run TestShellQuote -v`
Expected: PASS.

- [ ] **Step 5: Register the command in `root.go`**

In `internal/cli/root.go`, find:
```go
	cmd.AddCommand(newRunCommand())

	return cmd
```
Replace with:
```go
	cmd.AddCommand(newRunCommand())
	cmd.AddCommand(newExportCommand())

	return cmd
```

- [ ] **Step 6: Write the failing behavior tests**

Append to `internal/cli/export_test.go`:
```go
func TestExportEmitsManagedOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFile(t, filepath.Join(dir, ".symbion.yaml"), "project: billing-api\nenvs: []\n")
	writeFile(t, filepath.Join(dir, ".env"), "FOO=bar\nPORT=3000\n")

	out, errOut, err := runCommand(t, dir, "export")
	if err != nil {
		t.Fatalf("export error = %v, stderr = %s", err, errOut)
	}
	if !strings.Contains(out, "export FOO='bar'") {
		t.Fatalf("missing FOO export:\n%s", out)
	}
	if !strings.Contains(out, "export PORT='3000'") {
		t.Fatalf("missing PORT export:\n%s", out)
	}
	if strings.Contains(out, "export PATH=") {
		t.Fatalf("must not emit shell PATH:\n%s", out)
	}
	if strings.Index(out, "FOO") > strings.Index(out, "PORT") {
		t.Fatalf("not sorted (FOO should precede PORT):\n%s", out)
	}
}

func TestExportEscapesQuotes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFile(t, filepath.Join(dir, ".symbion.yaml"), "project: billing-api\nenvs: []\n")
	writeFile(t, filepath.Join(dir, ".env"), "MSG=\"it's ok\"\n")

	out, _, err := runCommand(t, dir, "export")
	if err != nil {
		t.Fatalf("export error = %v", err)
	}
	if !strings.Contains(out, `export MSG='it'\''s ok'`) {
		t.Fatalf("bad escaping:\n%s", out)
	}
}

func TestExportStrictFailsOnMissingRequired(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFile(t, filepath.Join(dir, ".symbion.yaml"),
		"project: billing-api\nenvs:\n  - key: SYMBION_NEEDS_THIS\n    required: true\n")
	writeFile(t, filepath.Join(dir, ".env"), "FOO=bar\n")

	out, errOut, err := runCommand(t, dir, "export", "--strict")
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("expected exit 1, got %v", err)
	}
	if strings.Contains(out, "export FOO") {
		t.Fatalf("strict must emit nothing on stdout, got:\n%s", out)
	}
	if !strings.Contains(errOut, "SYMBION_NEEDS_THIS") {
		t.Fatalf("missing var not reported: %q", errOut)
	}
}

func TestExportRoundTripsViaEval(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFile(t, filepath.Join(dir, ".symbion.yaml"), "project: billing-api\nenvs: []\n")
	writeFile(t, filepath.Join(dir, ".env"), "GREETING=hello world\n")

	out, _, err := runCommand(t, dir, "export")
	if err != nil {
		t.Fatalf("export error = %v", err)
	}

	script := out + "\nprintenv GREETING"
	got, err := exec.Command("sh", "-c", script).Output()
	if err != nil {
		t.Fatalf("sh eval error = %v", err)
	}
	if strings.TrimSpace(string(got)) != "hello world" {
		t.Fatalf("round-trip value = %q, want 'hello world'", got)
	}
}
```

- [ ] **Step 7: Run the full cli suite to verify everything passes**

Run: `go test ./internal/cli/ -v`
Expected: PASS — existing tests plus `TestShellQuote`, `TestExportEmitsManagedOnly`,
`TestExportEscapesQuotes`, `TestExportStrictFailsOnMissingRequired`, `TestExportRoundTripsViaEval`.

- [ ] **Step 8: Manual smoke test**

```bash
go build -o bin/symbion ./cmd/symbion
cd testdata/valid
../../bin/symbion export
eval "$(../../bin/symbion export)"; printenv REDIS_URL
cd ../..
```
Expected: the first prints `export DATABASE_URL='...'`, `export API_KEY='dev-api-key'`,
`export REDIS_URL='redis://localhost:6379'` (sorted, no `PATH`); the second prints
`redis://localhost:6379` — proving the output is valid, eval-able shell.

- [ ] **Step 9: Commit**

```bash
git add internal/cli/export.go internal/cli/export_test.go internal/cli/root.go
git commit -m "feat(cli): add symbion export command"
```

---

### Task 3: Document `symbion export`

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: nothing. Produces: nothing.

- [ ] **Step 1: Add a Features bullet**

In `README.md`, find:
```
- Run any command with a profile's environment injected in memory, without writing secrets to disk (`symbion run`).
```
Add immediately after it:
```
- Print a profile's environment as shell `export` statements for `eval` (`symbion export`).
```

- [ ] **Step 2: Add the command reference section**

In `README.md`, find the line `### \`symbion run\`` and, immediately **before** it, insert:
```markdown
### `symbion export`

Prints the resolved managed environment (profile/`.env` values plus schema defaults) as POSIX
`export` statements, for loading into your current shell:

```bash
eval "$(symbion export staging)"   # load a profile into the current shell
symbion export                     # no profile, uses .env
symbion export --strict prod       # exit 1 (emit nothing) if a required var is missing
```

Only managed variables are emitted (never your inherited shell). Values are single-quote escaped so
they survive `eval`. Encrypted profiles are decrypted in memory (set `SYMBION_PASSPHRASE`).

```

- [ ] **Step 3: Verify the docs render**

Run: `grep -n "symbion export" README.md`
Expected: matches in the Features list and the new command section.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: document symbion export"
```

---

### Final verification (after all tasks)

- [ ] **Run the full check suite (mirrors CI)**

Run:
```bash
go vet ./...
go test ./...
go test -race ./...
```
Expected: `go vet` prints nothing; both test runs report every package `ok` with no failures.

## Self-Review

**Spec coverage** (against `docs/superpowers/specs/2026-07-01-symbion-export-design.md`):
- §3 command surface (`[profile]`, `--strict`, exit codes) → Task 2.
- §4 managed-only resolution (`flatten` refactor + `Managed`) → Task 1.
- §5 output & POSIX quoting (`shellQuote`) → Task 2.
- §6 error handling (reused `loadRunSource`/`explainVaultError`, `MaximumNArgs(1)`, strict) → Task 2.
- §7 testing (`TestManaged`, `TestShellQuote`, managed-only, escaping, strict, eval round-trip) → Tasks 1-2 + Final verification.
- §8 file changes → Tasks 1-3 match exactly.

**Placeholder scan:** no `TBD`/`TODO`/"add error handling"/"similar to"/"write tests for the above" — every code/test step is complete.

**Type consistency:** `Managed(source, defaults map[string]string, secretKeys map[string]bool, sourceName Source) []Var`, `flatten(layers []layer, secretKeys map[string]bool) []Var`, `shellQuote(v string) string`, `newExportCommand() *cobra.Command` — names and signatures match across the tasks that define and consume them, and the reused helpers (`loadRunSource`, `optionalSchema`, `schemaDefaults`, `schemaSecretKeys`, `missingRequired`, `printMissingRequired`) match their definitions in `internal/cli/run.go`.
