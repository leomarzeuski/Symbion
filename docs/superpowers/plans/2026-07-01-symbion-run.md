# symbion run Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `symbion run [profile] -- <command>` that injects a resolved environment (profile/`.env` + schema defaults + shell) into a subprocess, decrypting encrypted profiles in memory only, with signal forwarding and exact exit-code propagation.

**Architecture:** A new pure `internal/resolve` package merges layered environment sources with explicit precedence and marks secrets. A new `internal/cli/exec.go` provides a testable subprocess runner (`runProcess`). A new `internal/cli/run.go` wires argument parsing, source loading, resolution, `--dry-run`/`--strict`/`--isolated`/`--no-override`, and calls `runProcess`. The command is registered in `root.go`.

**Tech Stack:** Go (stdlib `os/exec`, `os/signal`, `syscall`), `github.com/spf13/cobra`, existing `internal/{schema,parser,vault}` packages. No new dependencies.

## Global Constraints

- Go version floor: `go 1.23.0` (from `go.mod`). Do not raise it.
- No new module dependencies. Standard library + already-vendored modules only.
- Module path: `github.com/leonardomarzeuski/symbion`.
- Local-first, no network calls. `run` performs zero writes to disk or the vault (fully read-only).
- Secret-safety: never write decrypted plaintext to disk; never print secret values. `--dry-run` masks secret values with the literal `********`.
- Follow existing cobra command style (`newXCommand() *cobra.Command`, `RunE`, register in `root.go`).
- Exit-code discipline: return `&cli.ExitError{Code: N}` so `main.go` exits with `N` silently. Plain errors exit `2` with a `symbion:` prefix.
- Reuse existing test helpers in `internal/cli/cli_test.go`: `runCommand`, `newCLIProject`, `writeFile`, `assertFile`.
- Commit after every task. Run `go test ./...` before each commit.

## Prerequisites (do this once, before Task 1)

The Go toolchain is **not currently installed** on the dev machine. Every task below runs `go test`
/ `go build`, so install Go first.

- [ ] **P1: Verify or install Go**

Run: `go version`
Expected: `go version go1.23.x ...` (or newer).

If it prints `command not found`, install it (confirm with the user before installing):
```bash
brew install go
```
Then re-run `go version` and confirm it reports `go1.23` or newer.

- [ ] **P2: Confirm the baseline is green**

Run: `go test ./...`
Expected: all packages `ok` (no failures) — this is the pre-change baseline.

---

### Task 1: `internal/resolve` types & helpers

**Files:**
- Create: `internal/resolve/resolve.go`
- Test: `internal/resolve/resolve_test.go`

**Interfaces:**
- Consumes: nothing (leaf package, stdlib only).
- Produces:
  - `type Source string` with consts `SourceShell="shell"`, `SourceProfile="profile"`, `SourceEnvFile=".env"`, `SourceDefault="default"`, `SourceSafeMin="safe-min"`.
  - `type Var struct { Key string; Value string; Secret bool; Source Source }`
  - `type Options struct { InheritShell bool; Override bool; SourceName Source; Defaults map[string]string; SecretKeys map[string]bool }`
  - `func ToEnviron(vars []Var) []string`
  - unexported helpers `parseEnviron(environ []string) map[string]string`, `safeMinimum(shell map[string]string) map[string]string`, `looksSensitive(key string) bool` (consumed by Task 2 in the same package).

- [ ] **Step 1: Write failing tests for the helpers**

Create `internal/resolve/resolve_test.go`:
```go
package resolve

import (
	"reflect"
	"testing"
)

func TestLooksSensitive(t *testing.T) {
	cases := map[string]bool{
		"API_KEY":      true,
		"DB_PASSWORD":  true,
		"secret_token": true, // case-insensitive
		"GITHUB_TOKEN": true,
		"AUTH_HEADER":  true,
		"SESSION_ID":   true,
		"PORT":         false,
		"DATABASE_URL": false,
		"HOSTNAME":     false,
	}
	for key, want := range cases {
		if got := looksSensitive(key); got != want {
			t.Errorf("looksSensitive(%q) = %v, want %v", key, got, want)
		}
	}
}

func TestParseEnviron(t *testing.T) {
	got := parseEnviron([]string{"FOO=bar", "EQ=a=b", "NOEQUAL", "EMPTY="})
	want := map[string]string{"FOO": "bar", "EQ": "a=b", "EMPTY": ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseEnviron = %#v, want %#v", got, want)
	}
}

func TestSafeMinimum(t *testing.T) {
	shell := map[string]string{
		"PATH": "/bin", "HOME": "/home/u", "LANG": "en", "LC_ALL": "C",
		"APP_SECRET": "nope", "RANDOM_VAR": "nope",
	}
	got := safeMinimum(shell)
	want := map[string]string{"PATH": "/bin", "HOME": "/home/u", "LANG": "en", "LC_ALL": "C"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("safeMinimum = %#v, want %#v", got, want)
	}
}

func TestToEnviron(t *testing.T) {
	got := ToEnviron([]Var{{Key: "A", Value: "1"}, {Key: "B", Value: "x=y"}})
	want := []string{"A=1", "B=x=y"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ToEnviron = %#v, want %#v", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/resolve/ -run 'TestLooksSensitive|TestParseEnviron|TestSafeMinimum|TestToEnviron' -v`
Expected: build failure / FAIL — `undefined: looksSensitive` (and the other symbols).

- [ ] **Step 3: Implement the types and helpers**

Create `internal/resolve/resolve.go`:
```go
// Package resolve merges layered environment sources (schema defaults, the
// ambient shell, and a profile or .env) into a single ordered set of
// variables, marking which ones are secret. It performs no I/O.
package resolve

import "strings"

type Source string

const (
	SourceShell   Source = "shell"
	SourceProfile Source = "profile"
	SourceEnvFile Source = ".env"
	SourceDefault Source = "default"
	SourceSafeMin Source = "safe-min"
)

// Var is one fully-resolved environment variable.
type Var struct {
	Key    string
	Value  string
	Secret bool
	Source Source
}

// Options controls how Resolve merges its layers.
type Options struct {
	InheritShell bool              // false when --isolated
	Override     bool              // true = source (profile/.env) wins over shell
	SourceName   Source            // SourceProfile or SourceEnvFile, for attribution
	Defaults     map[string]string // schema documented non-empty defaults
	SecretKeys   map[string]bool   // schema keys with secret: true
}

// ToEnviron converts resolved Vars into a KEY=VALUE slice for exec.Cmd.Env.
func ToEnviron(vars []Var) []string {
	out := make([]string, 0, len(vars))
	for _, v := range vars {
		out = append(out, v.Key+"="+v.Value)
	}
	return out
}

var safeMinKeys = []string{"PATH", "HOME", "LANG", "TERM", "TMPDIR", "USER", "SHELL", "PWD"}

// safeMinimum returns the OS essentials a child needs to run, pulled from the
// real shell: a fixed allowlist plus every LC_* locale variable.
func safeMinimum(shell map[string]string) map[string]string {
	out := make(map[string]string)
	for _, k := range safeMinKeys {
		if v, ok := shell[k]; ok {
			out[k] = v
		}
	}
	for k, v := range shell {
		if strings.HasPrefix(k, "LC_") {
			out[k] = v
		}
	}
	return out
}

var sensitiveMarkers = []string{
	"SECRET", "TOKEN", "PASSWORD", "PASSWD", "PASS", "API_KEY", "APIKEY",
	"PRIVATE", "CREDENTIAL", "ACCESS_KEY", "SESSION", "AUTH",
}

// looksSensitive reports whether a variable name suggests a secret value.
func looksSensitive(key string) bool {
	upper := strings.ToUpper(key)
	for _, marker := range sensitiveMarkers {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

// parseEnviron turns a KEY=VALUE slice (os.Environ form) into a map, splitting
// on the first '=' and skipping entries without one.
func parseEnviron(environ []string) map[string]string {
	m := make(map[string]string, len(environ))
	for _, kv := range environ {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			m[kv[:i]] = kv[i+1:]
		}
	}
	return m
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/resolve/ -run 'TestLooksSensitive|TestParseEnviron|TestSafeMinimum|TestToEnviron' -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/resolve/resolve.go internal/resolve/resolve_test.go
git commit -m "feat(resolve): add env resolution types and helpers"
```

---

### Task 2: `internal/resolve` precedence engine

**Files:**
- Modify: `internal/resolve/resolve.go` (add `Resolve` + `buildLayers`)
- Test: `internal/resolve/resolve_test.go` (add precedence tests)

**Interfaces:**
- Consumes: `Var`, `Options`, `Source` consts, `parseEnviron`, `safeMinimum`, `looksSensitive` (Task 1).
- Produces: `func Resolve(shell []string, source map[string]string, opts Options) []Var` — returns Vars sorted by Key; later layers win; `Var.Secret = opts.SecretKeys[key] || looksSensitive(key)`; each `Var.Source` is the winning layer.

- [ ] **Step 1: Write failing precedence tests**

Append to `internal/resolve/resolve_test.go`:
```go
func findVar(vars []Var, key string) (Var, bool) {
	for _, v := range vars {
		if v.Key == key {
			return v, true
		}
	}
	return Var{}, false
}

func TestResolveDefaultProfileWins(t *testing.T) {
	shell := []string{"FOO=shell", "PATH=/bin", "OTHER=keep"}
	source := map[string]string{"FOO": "profile", "BAR": "profile"}
	opts := Options{
		InheritShell: true, Override: true, SourceName: SourceProfile,
		Defaults:   map[string]string{"BAZ": "def"},
		SecretKeys: map[string]bool{},
	}

	vars := Resolve(shell, source, opts)

	// sorted by key
	if vars[0].Key != "BAR" || vars[len(vars)-1].Key != "PATH" {
		t.Fatalf("not sorted by key: %#v", vars)
	}
	foo, _ := findVar(vars, "FOO")
	if foo.Value != "profile" || foo.Source != SourceProfile {
		t.Errorf("FOO = %#v, want profile/profile", foo)
	}
	baz, _ := findVar(vars, "BAZ")
	if baz.Value != "def" || baz.Source != SourceDefault {
		t.Errorf("BAZ = %#v, want def/default", baz)
	}
	other, ok := findVar(vars, "OTHER")
	if !ok || other.Source != SourceShell {
		t.Errorf("OTHER = %#v, want kept from shell", other)
	}
}

func TestResolveNoOverrideShellWins(t *testing.T) {
	shell := []string{"FOO=shell"}
	source := map[string]string{"FOO": "profile", "BAR": "profile"}
	opts := Options{InheritShell: true, Override: false, SourceName: SourceProfile}

	vars := Resolve(shell, source, opts)

	foo, _ := findVar(vars, "FOO")
	if foo.Value != "shell" || foo.Source != SourceShell {
		t.Errorf("FOO = %#v, want shell wins", foo)
	}
	bar, _ := findVar(vars, "BAR")
	if bar.Value != "profile" {
		t.Errorf("BAR = %#v, want profile", bar)
	}
}

func TestResolveIsolatedDropsAmbient(t *testing.T) {
	shell := []string{"PATH=/bin", "OTHER=drop"}
	source := map[string]string{"FOO": "profile"}
	opts := Options{InheritShell: false, Override: true, SourceName: SourceProfile}

	vars := Resolve(shell, source, opts)

	if _, ok := findVar(vars, "OTHER"); ok {
		t.Error("OTHER should be dropped in isolated mode")
	}
	path, ok := findVar(vars, "PATH")
	if !ok || path.Source != SourceSafeMin {
		t.Errorf("PATH = %#v, want kept via safe-min", path)
	}
	foo, _ := findVar(vars, "FOO")
	if foo.Value != "profile" {
		t.Errorf("FOO = %#v, want profile", foo)
	}
}

func TestResolveMarksSecrets(t *testing.T) {
	source := map[string]string{"MY_TOKEN": "t", "PLAIN": "p", "DOCUMENTED": "d"}
	opts := Options{
		InheritShell: false, Override: true, SourceName: SourceEnvFile,
		SecretKeys: map[string]bool{"DOCUMENTED": true},
	}

	vars := Resolve(nil, source, opts)

	tok, _ := findVar(vars, "MY_TOKEN")
	if !tok.Secret {
		t.Error("MY_TOKEN should be secret via heuristic")
	}
	doc, _ := findVar(vars, "DOCUMENTED")
	if !doc.Secret {
		t.Error("DOCUMENTED should be secret via schema")
	}
	plain, _ := findVar(vars, "PLAIN")
	if plain.Secret {
		t.Error("PLAIN should not be secret")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/resolve/ -run TestResolve -v`
Expected: build failure / FAIL — `undefined: Resolve`.

- [ ] **Step 3: Implement `Resolve` and `buildLayers`**

Append to `internal/resolve/resolve.go`:
```go
import "sort"

// layer is one contributor to the merged environment. Later layers win.
type layer struct {
	name   Source
	values map[string]string
}

// Resolve merges the layers implied by opts and returns the result sorted by
// Key. Precedence, lowest to highest:
//   default:       [defaults, shell, source]      (source/profile wins)
//   --no-override: [defaults, source, shell]       (shell wins)
//   --isolated:    [safeMinimum(shell), defaults, source]
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

func buildLayers(shell []string, source map[string]string, opts Options) []layer {
	shellMap := parseEnviron(shell)

	sourceName := opts.SourceName
	if sourceName == "" {
		sourceName = SourceEnvFile
	}
	defaults := layer{name: SourceDefault, values: opts.Defaults}
	src := layer{name: sourceName, values: source}

	if !opts.InheritShell {
		return []layer{
			{name: SourceSafeMin, values: safeMinimum(shellMap)},
			defaults,
			src,
		}
	}

	shellLayer := layer{name: SourceShell, values: shellMap}
	if opts.Override {
		return []layer{defaults, shellLayer, src}
	}
	return []layer{defaults, src, shellLayer}
}
```

Note: the new `import "sort"` must be merged into the existing import block — change
`import "strings"` at the top of the file to:
```go
import (
	"sort"
	"strings"
)
```
and delete the standalone `import "sort"` line shown above before saving.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/resolve/ -v`
Expected: PASS (all Task 1 + Task 2 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/resolve/resolve.go internal/resolve/resolve_test.go
git commit -m "feat(resolve): add Resolve precedence engine"
```

---

### Task 3: `runProcess` subprocess runner

**Files:**
- Create: `internal/cli/exec.go`
- Test: `internal/cli/exec_test.go`

**Interfaces:**
- Consumes: `ExitError` (defined in `internal/cli/root.go`).
- Produces:
  - `func runProcess(command []string, env []string, stdin io.Reader, stdout, stderr io.Writer) error` — execs `command[0]` with `command[1:]`, `env` as the full environment, forwards signals, returns `nil` on exit 0 or `&ExitError{Code}` otherwise (`127` not found, `126` not startable, child exit code, or `128+signal`).
  - `func exitCodeFromError(err error) int`
  - `func TestHelperProcess(t *testing.T)` and `func helperArgs() []string` (test-only child harness reused by Task 4).

- [ ] **Step 1: Write failing tests**

Create `internal/cli/exec_test.go`:
```go
package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestHelperProcess is not a real test: when GO_WANT_HELPER_PROCESS=1 the test
// binary re-executes itself and behaves as the child process under test.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := helperArgs()
	if len(args) == 0 {
		os.Exit(0)
	}
	switch args[0] {
	case "printenv":
		if len(args) > 1 {
			fmt.Fprint(os.Stdout, os.Getenv(args[1]))
		}
		os.Exit(0)
	case "exitcode":
		code := 0
		if len(args) > 1 {
			code, _ = strconv.Atoi(args[1])
		}
		os.Exit(code)
	default:
		os.Exit(0)
	}
}

func helperArgs() []string {
	for i, a := range os.Args {
		if a == "--" {
			return os.Args[i+1:]
		}
	}
	return nil
}

func helperCommand(args ...string) []string {
	base := []string{os.Args[0], "-test.run=^TestHelperProcess$", "--"}
	return append(base, args...)
}

func TestRunProcessInjectsEnv(t *testing.T) {
	var out bytes.Buffer
	env := append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "SYMBION_TEST_VAR=hello")
	err := runProcess(helperCommand("printenv", "SYMBION_TEST_VAR"), env, nil, &out, io.Discard)
	if err != nil {
		t.Fatalf("runProcess error = %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "hello" {
		t.Fatalf("child saw SYMBION_TEST_VAR=%q, want hello", got)
	}
}

func TestRunProcessPropagatesExitCode(t *testing.T) {
	env := append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	err := runProcess(helperCommand("exitcode", "3"), env, nil, io.Discard, io.Discard)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *ExitError, got %v", err)
	}
	if exitErr.Code != 3 {
		t.Fatalf("exit code = %d, want 3", exitErr.Code)
	}
}

func TestRunProcessSuccessReturnsNil(t *testing.T) {
	env := append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	if err := runProcess(helperCommand("exitcode", "0"), env, nil, io.Discard, io.Discard); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestRunProcessCommandNotFound(t *testing.T) {
	err := runProcess([]string{"symbion-nonexistent-xyz"}, os.Environ(), nil, io.Discard, io.Discard)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 127 {
		t.Fatalf("expected exit 127, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/ -run TestRunProcess -v`
Expected: build failure / FAIL — `undefined: runProcess`.

- [ ] **Step 3: Implement `runProcess`**

Create `internal/cli/exec.go`:
```go
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// runProcess executes command[0] with command[1:] using env as the child's
// entire environment. Signals are forwarded to the child; the child's exit
// code is propagated exactly. It never writes to disk.
func runProcess(command []string, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	bin, err := exec.LookPath(command[0])
	if err != nil {
		fmt.Fprintf(stderr, "symbion: %s: command not found\n", command[0])
		return &ExitError{Code: 127}
	}

	proc := exec.Command(bin, command[1:]...)
	proc.Env = env
	proc.Stdin = stdin
	proc.Stdout = stdout
	proc.Stderr = stderr

	if err := proc.Start(); err != nil {
		fmt.Fprintf(stderr, "symbion: %s: %v\n", command[0], err)
		return &ExitError{Code: 126}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer signal.Stop(sigCh)
	go func() {
		for sig := range sigCh {
			_ = proc.Process.Signal(sig)
		}
	}()

	if err := proc.Wait(); err != nil {
		return &ExitError{Code: exitCodeFromError(err)}
	}
	return nil
}

// exitCodeFromError extracts a process exit code from an *exec.ExitError,
// mapping signal termination to the conventional 128+signal.
func exitCodeFromError(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			if status.Signaled() {
				return 128 + int(status.Signal())
			}
			return status.ExitStatus()
		}
		return exitErr.ExitCode()
	}
	return 1
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/ -run 'TestRunProcess|TestHelperProcess' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/exec.go internal/cli/exec_test.go
git commit -m "feat(cli): add runProcess subprocess runner with exit-code fidelity"
```

---

### Task 4: `symbion run` command

**Files:**
- Create: `internal/cli/run.go`
- Modify: `internal/cli/root.go` (register the command)
- Test: `internal/cli/run_test.go`

**Interfaces:**
- Consumes: `resolve.{Resolve,Options,ToEnviron,Source,SourceProfile,SourceEnvFile}` (Tasks 1-2); `runProcess`, `TestHelperProcess`, `helperCommand` (Task 3); `ExitError` (root.go); `explainVaultError`, `optionalPassphrase` (passphrase.go); `schema.{Load,New,DefaultFilename,Schema}`; `parser.ParseEnv`; `vault.NewDefaultStore`; test helpers `runCommand`, `writeFile` (cli_test.go).
- Produces: `func newRunCommand() *cobra.Command`; `func splitRunArgs(dash int, args []string) (profile string, command []string, err error)`.

- [ ] **Step 1: Write a failing test for `splitRunArgs`**

Create `internal/cli/run_test.go`:
```go
package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitRunArgs(t *testing.T) {
	tests := []struct {
		name        string
		dash        int
		args        []string
		wantProfile string
		wantCommand []string
		wantErr     bool
	}{
		{"nothing", -1, nil, "", nil, false},
		{"profile only", -1, []string{"staging"}, "staging", nil, false},
		{"missing dash before command", -1, []string{"npm", "start"}, "", nil, true},
		{"command only", 0, []string{"npm", "start"}, "", []string{"npm", "start"}, false},
		{"profile and command", 1, []string{"staging", "npm", "run"}, "staging", []string{"npm", "run"}, false},
		{"too many before dash", 2, []string{"a", "b", "cmd"}, "", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, command, err := splitRunArgs(tt.dash, tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if profile != tt.wantProfile {
				t.Errorf("profile = %q, want %q", profile, tt.wantProfile)
			}
			if strings.Join(command, ",") != strings.Join(tt.wantCommand, ",") {
				t.Errorf("command = %v, want %v", command, tt.wantCommand)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestSplitRunArgs -v`
Expected: build failure / FAIL — `undefined: splitRunArgs`.

- [ ] **Step 3: Implement `run.go`**

Create `internal/cli/run.go`:
```go
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
		data, err := os.ReadFile(filepath.Join(cwd, ".env"))
		if err != nil {
			if os.IsNotExist(err) {
				return nil, "", fmt.Errorf(".env not found")
			}
			return nil, "", err
		}
		values, err := parser.ParseEnv(data)
		if err != nil {
			return nil, "", fmt.Errorf("parse .env: %w", err)
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
```

- [ ] **Step 4: Run the split test to verify it passes**

Run: `go test ./internal/cli/ -run TestSplitRunArgs -v`
Expected: PASS.

- [ ] **Step 5: Register the command in `root.go`**

Modify `internal/cli/root.go`. Find:
```go
	cmd.AddCommand(newDiffCommand())

	return cmd
```
Replace with:
```go
	cmd.AddCommand(newDiffCommand())
	cmd.AddCommand(newRunCommand())

	return cmd
```

- [ ] **Step 6: Write failing behavior tests (dry-run, strict, no-command, end-to-end injection)**

Append to `internal/cli/run_test.go`:
```go
func TestRunDryRunMasksSecrets(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFile(t, filepath.Join(dir, ".symbion.yaml"),
		"project: billing-api\nenvs:\n  - key: API_KEY\n    secret: true\n")
	writeFile(t, filepath.Join(dir, ".env"), "API_KEY=super-secret\nPORT=3000\n")

	out, errOut, err := runCommand(t, dir, "run", "--dry-run")
	if err != nil {
		t.Fatalf("run --dry-run error = %v, stderr = %s", err, errOut)
	}
	if strings.Contains(out, "super-secret") {
		t.Fatalf("dry-run leaked secret value:\n%s", out)
	}
	if !strings.Contains(out, "API_KEY=********") {
		t.Fatalf("expected masked API_KEY, got:\n%s", out)
	}
	if !strings.Contains(out, "PORT=3000") {
		t.Fatalf("expected visible PORT, got:\n%s", out)
	}
}

func TestRunStrictFailsOnMissingRequired(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFile(t, filepath.Join(dir, ".symbion.yaml"),
		"project: billing-api\nenvs:\n  - key: SYMBION_NEEDS_THIS\n    required: true\n")
	writeFile(t, filepath.Join(dir, ".env"), "PORT=3000\n")

	_, errOut, err := runCommand(t, dir, "run", "--strict", "--dry-run")
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("expected exit 1, got %v", err)
	}
	if !strings.Contains(errOut, "SYMBION_NEEDS_THIS") {
		t.Fatalf("expected missing var in stderr, got %q", errOut)
	}
}

func TestRunNoCommandErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFile(t, filepath.Join(dir, ".symbion.yaml"), "project: billing-api\nenvs: []\n")
	writeFile(t, filepath.Join(dir, ".env"), "PORT=3000\n")

	_, _, err := runCommand(t, dir, "run", "local")
	if err == nil {
		t.Fatalf("expected error when no command and no --dry-run")
	}
}

func TestRunInjectsProfileEndToEnd(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFile(t, filepath.Join(dir, ".symbion.yaml"), "project: billing-api\nenvs: []\n")
	// GO_WANT_HELPER_PROCESS travels through the profile so the re-exec'd test
	// binary acts as the child helper.
	writeFile(t, filepath.Join(dir, ".env"), "INJECTED_VAR=from-profile\nGO_WANT_HELPER_PROCESS=1\n")

	if _, errOut, err := runCommand(t, dir, "save", "local"); err != nil {
		t.Fatalf("save error = %v, stderr = %s", err, errOut)
	}
	// Overwrite .env to prove the value comes from the profile, not .env.
	writeFile(t, filepath.Join(dir, ".env"), "INJECTED_VAR=from-dotenv\nGO_WANT_HELPER_PROCESS=1\n")

	args := append([]string{"run", "local", "--"}, helperCommand("printenv", "INJECTED_VAR")...)
	out, errOut, err := runCommand(t, dir, args...)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, errOut)
	}
	if !strings.Contains(out, "from-profile") {
		t.Fatalf("expected child to see profile value, got:\n%s", out)
	}
}
```

- [ ] **Step 7: Run the full cli suite to verify everything passes**

Run: `go test ./internal/cli/ -v`
Expected: PASS (existing tests + `TestSplitRunArgs`, `TestRunDryRunMasksSecrets`, `TestRunStrictFailsOnMissingRequired`, `TestRunNoCommandErrors`, `TestRunInjectsProfileEndToEnd`).

- [ ] **Step 8: Manual smoke test**

```bash
go build -o bin/symbion ./cmd/symbion
cd testdata/valid
../../bin/symbion run --dry-run
../../bin/symbion run -- printenv DATABASE_URL
cd ../..
```
Expected: the first prints the resolved environment (any secret-looking keys masked with `********`);
the second prints the `DATABASE_URL` value from `testdata/valid/.env`.

- [ ] **Step 9: Commit**

```bash
git add internal/cli/run.go internal/cli/run_test.go internal/cli/root.go
git commit -m "feat(cli): add symbion run command"
```

---

### Task 5: Document `symbion run`

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: nothing (docs only).
- Produces: nothing.

- [ ] **Step 1: Add a Features bullet**

In `README.md`, find:
```
- Optionally encrypt saved profiles with Argon2id + AES-GCM.
```
Add immediately after it:
```
- Run any command with a profile's environment injected in memory, without writing secrets to disk (`symbion run`).
```

- [ ] **Step 2: Add the command reference section**

In `README.md`, find the `### symbion use` section and, immediately **before** the line
`### symbion backups`, insert:
```markdown
### `symbion run`

Runs a command with a resolved environment injected, without writing secrets to disk:

```bash
symbion run staging -- npm run dev
symbion run -- go test ./...          # no profile → uses .env
```

Resolution precedence (default): start from your shell, the profile overrides matching keys, and
schema defaults fill anything still missing. Encrypted profiles are decrypted in memory only
(set `SYMBION_PASSPHRASE`).

Flags:

- `--dry-run` — print the resolved environment (secret values masked) without running.
- `--strict` — refuse to launch if a schema-required variable is missing.
- `--isolated` — do not inherit the ambient shell; use only resolved vars + OS essentials.
- `--no-override` — let existing shell variables win over the profile.
- `--show-values` — with `--dry-run`, reveal masked values.

The command's exit code is propagated exactly. Secret values are never printed or written to disk.

```

- [ ] **Step 3: Mention it in Core Workflow**

In `README.md`, find:
```
5. Use `symbion save`, `symbion diff` and `symbion use` to manage local profiles.
```
Replace with:
```
5. Use `symbion save`, `symbion diff` and `symbion use` to manage local profiles.
6. Use `symbion run <profile> -- <command>` to launch your app with a profile's values, without writing secrets to disk.
```

- [ ] **Step 4: Verify the docs render**

Run: `grep -n "symbion run" README.md`
Expected: matches in the Features list, the new command section, and Core Workflow.

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: document symbion run"
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
Expected: `go vet` prints nothing; both test runs report every package `ok` with no failures. The
`-race` run exercises the signal-forwarding goroutine in `runProcess`.

## Self-Review

**Spec coverage** (checked against `docs/superpowers/specs/2026-07-01-symbion-run-design.md`):
- §3 command surface & flags → Task 4 (`newRunCommand`, all five flags, exit codes via `ExitError`/`runProcess`).
- §4 precedence & `safeMinimum` → Tasks 1-2 (`buildLayers`, `safeMinimum`) + tests.
- §5.1 resolve engine → Tasks 1-2.
- §5.2 run wiring (arg split, optional schema, source loading, in-memory decrypt) → Task 4.
- §5.3 signals & exit fidelity → Task 3 (`runProcess`, `exitCodeFromError`).
- §5.4 secret safety (in-memory decrypt, env via `cmd.Env`, dry-run masking) → Tasks 3-4.
- §6 error handling table → Task 4 (`loadRunSource`, no-command check, `explainVaultError`) + Task 3 (127/126).
- §7 testing (resolve tables, `TestHelperProcess` pattern, `-race`) → Tasks 1-4 + Final verification.
- §8 file changes → Tasks 1-5 file lists match exactly.

**Placeholder scan:** no `TBD`/`TODO`/"add error handling"/"similar to"/"write tests for the above" — every code and test step contains complete content.

**Type consistency:** `resolve.Options{InheritShell,Override,SourceName,Defaults,SecretKeys}`, `resolve.Var{Key,Value,Secret,Source}`, `Resolve(shell []string, source map[string]string, opts Options) []Var`, `ToEnviron([]Var) []string`, `runProcess(command, env []string, stdin io.Reader, stdout, stderr io.Writer) error`, `splitRunArgs(dash int, args []string) (string, []string, error)` — names and signatures are identical across the tasks that define and consume them. `SourceProfile`/`SourceEnvFile` used consistently in `loadRunSource` and tests.
