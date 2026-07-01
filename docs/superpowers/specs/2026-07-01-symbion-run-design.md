# Symbion `run` — Design Spec

- **Date:** 2026-07-01
- **Status:** Approved (design), pending implementation plan
- **Feature:** `symbion run` — inject a resolved environment into a subprocess without materializing secrets on disk
- **North star:** Personal power tool + portfolio showcase. First flagship in a sequence (`run` → shell auto-load → `export`) that shares one environment-resolution core.

## 1. Summary

`symbion run [profile] -- <command>` resolves a merged environment (profile-or-`.env` + schema
defaults + the current shell), decrypts any encrypted profile **in memory**, and executes the given
command with that environment. Nothing is written to disk. Signals are forwarded to the child and its
exit code is propagated exactly.

This closes Symbion's single biggest capability gap: today you must `symbion use <profile>` to
materialize a plaintext `.env` on disk before running your app. `run` lets you launch with a profile's
values — including encrypted ones — while keeping plaintext out of the filesystem.

## 2. Goals / Non-goals

### Goals
- Run any command with the environment produced by a saved profile or `.env`.
- Decrypt encrypted profiles in memory only; never write plaintext to disk.
- Clear, predictable precedence between the profile, the ambient shell, and schema defaults.
- Faithful process semantics: stdio passthrough, signal forwarding, exact exit-code propagation.
- Showcase polish: `--dry-run` (masked preview), `--strict` (refuse on missing required vars),
  `--isolated` (hermetic environment).
- A reusable, pure `internal/resolve` engine that later features (auto-load, `export`) build on.
- No new dependencies; stay lean, local-first, no-network.

### Non-goals (v1 — explicitly out, YAGNI)
- Windows-specific signal handling. v1 targets macOS/Linux.
- `--export` / eval mode (`export KEY=…` for the shell). Deferred to the auto-load/export feature.
- `.env.local` / multi-file layering. Separate roadmap item.
- Source-code scanning, value validation, secret-leak reporting. The *next* flagships; `run` only
  borrows the `looksSensitive` helper.
- Any write to disk or to the vault. `run` is fully read-only.

## 3. Command surface

```
symbion run [profile] [flags] -- <command> [args…]
```

Examples:
```bash
symbion run staging -- npm run dev       # inject the 'staging' profile, then exec
symbion run -- go test ./...             # no profile → inject .env (the current local env)
symbion run prod --strict -- ./server    # refuse to launch if a required var is missing
symbion run staging --dry-run            # print resolved env (secrets masked), don't run
symbion run ci --isolated -- ./build.sh  # only resolved vars + safe minimum, no ambient leak
```

### Positional argument
- `profile` (optional): a saved profile name, or the literals `.env` / `env` / `current` (matching
  `diff`). **When omitted, `run` uses `.env`.**

### Flags
| Flag | Behavior |
|---|---|
| `-n, --dry-run` | Resolve and print the environment (secret values masked), do **not** execute. |
| `--strict` | Doctor-style precheck: if any schema-`required` var is missing from the resolved set, print the missing keys and exit `1` without launching. |
| `--isolated` | Do not inherit the ambient shell. Start from a safe-minimum allowlist + resolved vars. |
| `--no-override` | Existing shell vars win over the profile (default: profile wins). |
| `--show-values` | With `--dry-run`, reveal masked values (prints a warning line first). |

Encrypted profiles read the passphrase from the existing `SYMBION_PASSPHRASE` env var.

### Exit codes (consistent with the existing `ExitError` discipline)
- Real run: the child's **exact** exit code; `128 + signal` if the child was terminated by a signal.
- `1` — `--strict` precheck failed (missing required vars).
- `2` — usage / resolution error (no command after `--`, profile not found, bad/missing passphrase,
  unreadable `.env`).
- `127` — command not found; `126` — command found but not executable.

The command's exit code is returned as `&cli.ExitError{Code}` so `main.go` exits silently with that
code (no `symbion:` prefix on a child's non-zero exit).

## 4. Precedence & resolution semantics

Default resolution starts from the shell environment, layers the profile on top (profile wins), and
fills any still-missing documented keys from schema defaults. Every other shell var passes through.

Worked example — shell `FOO=shell`; profile `staging` has `FOO=staging`, `BAR=staging`; schema
documents `BAZ` with default `baz`:

| mode | result |
|---|---|
| **default** | `FOO=staging`, `BAR=staging`, `BAZ=baz`, + all other shell vars (`PATH`, …) |
| `--no-override` | `FOO=shell` (shell wins), `BAR=staging`, `BAZ=baz`, + other shell vars |
| `--isolated` | `FOO=staging`, `BAR=staging`, `BAZ=baz`, + only safe-minimum OS vars |

Layering is expressed as an ordered list of layers where **later wins**, then flattened:
- default → `[defaults, shell, source]`
- `--no-override` → `[defaults, source, shell]`
- `--isolated` → `[safeMinimum(shell), defaults, source]`

`defaults` only contributes keys that are documented in the schema **with a non-empty `default`**, and
because it is the lowest layer it only ever fills genuinely missing keys.

`safeMinimum` pulls a fixed allowlist from the real shell so the child can execute in isolated mode:
`PATH, HOME, LANG, LC_*` (all `LC_`-prefixed), `TERM, TMPDIR, USER, SHELL, PWD`.

## 5. Architecture

### 5.1 `internal/resolve` — pure engine, zero I/O

```go
package resolve

type Source string // "shell" | "profile" | ".env" | "default" | "safe-min"

type Var struct {
    Key    string
    Value  string
    Secret bool
    Source Source
}

type Options struct {
    InheritShell bool              // false when --isolated
    Override     bool              // true = profile wins over shell (default)
    SourceName   Source            // "profile" or ".env" — used for attribution
    Defaults     map[string]string // schema documented non-empty defaults
    SecretKeys   map[string]bool   // schema keys with secret: true
}

// Resolve merges the layers per Options and returns Vars sorted by Key.
func Resolve(shell []string, source map[string]string, opts Options) []Var
```

- `shell` is `os.Environ()` (`KEY=VALUE` slice); the engine parses it internally so it stays pure and
  testable.
- Each resolved `Var` records the winning layer as `Source`, and
  `Secret = SecretKeys[Key] || looksSensitive(Key)`.
- Output is sorted by `Key` for determinism.

`looksSensitive(key)` — case-insensitive substring match against a built-in list:
`SECRET, TOKEN, PASSWORD, PASSWD, PASS, API_KEY, APIKEY, PRIVATE, CREDENTIAL, ACCESS_KEY, SESSION,
AUTH`. This masks even undocumented secrets in `--dry-run`, and is the same helper the later
secret-detection feature will reuse.

A helper `ToEnviron(vars []Var) []string` converts back to a `KEY=VALUE` slice for `exec.Cmd.Env`.

### 5.2 `internal/cli/run.go` — command wiring

Flow:
1. Split args at `--` via cobra `ArgsLenAtDash()`. Before the dash: optional profile. After: the
   command + args. If there is no command **and** `--dry-run` is not set → exit `2`
   (`"nothing to run; pass a command after -- or use --dry-run"`).
2. Resolve the working directory and load the schema **if present** (for defaults, secret keys, and the
   project name). If the schema is absent: proceed with an empty schema. A *named* profile then errors
   (it needs the project id to locate the vault → `"run symbion init or symbion scan first"`); the
   `.env` default still works with no schema.
3. Load the source environment:
   - named profile → `store.ReadProfile(project, name, optionalPassphrase())` (**in-memory decrypt**)
     → `parser.ParseEnv`.
   - `.env` / `env` / `current` / omitted → read `.env`; missing file → exit `2`.
4. Build `resolve.Options` from the flags + schema and call
   `vars := resolve.Resolve(os.Environ(), source, opts)`.
5. `--strict`: compute schema-`required` keys absent from `vars`; if any, print them (doctor-style
   list) and return `&ExitError{Code: 1}`.
6. `--dry-run`: print each resolved var as `KEY  <value|••••>  (source)`; secret vars are masked unless
   `--show-values` is set (which first prints a warning). Return `nil` (exit 0). Do not exec.
7. Execute:
   - `cmd := exec.Command(argv[0], argv[1:]...)`
   - `cmd.Env = resolve.ToEnviron(vars)`
   - `cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr`
   - Start; forward signals; `Wait`; map the exit code (see 5.3).

Register `newRunCommand()` in `internal/cli/root.go`.

### 5.3 Signals & exit fidelity
- `signal.Notify(ch, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)` before `Start`.
- A goroutine relays each received signal to `cmd.Process.Signal(sig)`; it stops (and `signal.Stop`)
  once `Wait` returns.
- On `Wait`:
  - `nil` → exit 0.
  - `*exec.ExitError` → read `ProcessState`: `ExitCode()`, or `128 + int(sig)` when `Signaled()`.
  - `exec.ErrNotFound` / `ENOENT` → 127; permission denied → 126.
- The resulting code is wrapped in `&ExitError{Code}` so `main.go` exits silently with it.

### 5.4 Secret-safety guarantees (showcase talking points)
- Encrypted profiles decrypt **only in memory**; plaintext never touches disk (contrast with `use`,
  which writes `.env`).
- The environment is passed via `cmd.Env`, never through argv, so values are not visible in another
  process's `ps` argument list.
- `--dry-run` masks schema-secret **and** name-heuristic keys; resolved values are never logged.
- `run` performs no writes to the project or the vault.

## 6. Error handling

| Situation | Behavior |
|---|---|
| No command after `--` (and not `--dry-run`) | exit 2, message suggesting `--dry-run` |
| `.env` requested/defaulted but missing | exit 2, `".env not found"` |
| Named profile but no schema/project | exit 2, `"run symbion init or symbion scan first"` |
| Profile not found | reuse `store.ResolveProfile` error (exit 2) |
| Encrypted profile, no `SYMBION_PASSPHRASE` | reuse `explainVaultError` / `ErrPassphraseRequired` |
| `--strict` required var missing | print missing keys, exit 1 |
| Command not found / not executable | exit 127 / 126 with a clear message |
| Child exits non-zero | propagate exact code, no `symbion:` prefix |

## 7. Testing strategy

**`internal/resolve/resolve_test.go`** (pure → exhaustive table tests):
- precedence in all three modes (default profile-wins, `--no-override` shell-wins, `--isolated`)
- defaults fill only genuinely-missing keys
- secret marking via schema and via `looksSensitive`; non-secret stays `false`
- source attribution correctness
- deterministic, key-sorted output
- edges: empty shell, empty source, key present in every layer, `LC_*` wildcard inclusion in safe-min

**`internal/cli/run_test.go`** (portable `os/exec` `TestHelperProcess` pattern — a test that, under
`GO_WANT_HELPER_PROCESS=1`, becomes the child: echoes selected env vars / exits with a requested code;
no external shell needed):
- injected env reaches the child (profile var visible)
- child exit code propagates exactly (child `exit 3` → symbion exits 3)
- `--dry-run` masks secrets and does **not** exec (exit 0)
- `--strict` fails (exit 1) on a missing required var; passes when present
- `--isolated` strips an ambient var but keeps `PATH`
- arg splitting at `--` (profile vs command); "no command" error
- encrypted profile: with `SYMBION_PASSPHRASE` injects; without → clear error

All existing tests stay green. The signal-forwarding path is exercised under `go test -race`.

## 8. File changes

| File | Change |
|---|---|
| `internal/resolve/resolve.go` | **new** — pure engine (`Var`, `Options`, `Resolve`, `ToEnviron`, `looksSensitive`, `safeMinimum`) |
| `internal/resolve/resolve_test.go` | **new** — table tests |
| `internal/cli/run.go` | **new** — `newRunCommand()` + exec/signal/exit handling |
| `internal/cli/run_test.go` | **new** — command tests incl. `TestHelperProcess` |
| `internal/cli/root.go` | **edit** — register `newRunCommand()` |
| `README.md` | **edit** — `run` under Commands, a "secrets never touch disk" highlight, Core Workflow mention |

## 9. How this composes (the sequence)

The `internal/resolve` engine is deliberately I/O-free so later flagships reuse it:
- **Shell auto-load** (`symbion hook`) resolves the active profile and emits it into the shell — same
  engine, different sink.
- **`export`** emits `KEY=VALUE` for CI or `eval` — same engine, an export sink.
- **Secret-leak detection** reuses `looksSensitive` as its name-based signal alongside entropy/pattern
  checks.

Building `run` first means the hardest shared piece (correct precedence + secret marking) is designed,
tested, and paid for once.

## 10. Toolchain note

The Go toolchain is not currently installed on the development machine, so `go build` / `go test`
cannot run locally yet. Installing it (e.g. `brew install go`) is a prerequisite for the implementation
phase and will be confirmed before any install is performed.
