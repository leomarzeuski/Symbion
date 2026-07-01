# Symbion `export` — Design Spec

- **Date:** 2026-07-01
- **Status:** Approved (design), pending implementation plan
- **Feature:** `symbion export` — emit a resolved environment as POSIX `export` statements for `eval`
- **North star:** Personal power tool + portfolio showcase. Foundation slice of "shell auto-load"; the
  next cycle (`symbion hook`) reuses this command's emit logic and the `resolve` engine.

## 1. Summary

`symbion export [profile]` resolves the **managed** environment for a project (the profile-or-`.env`
values plus schema defaults) and prints POSIX `export KEY='VALUE'` lines to stdout. The intended use is
`eval "$(symbion export staging)"`, which loads a profile's values into the current shell. Encrypted
profiles are decrypted in memory. It is the foundation the later `symbion hook` (direnv-style
auto-load) builds on.

Unlike `run`, which injects the *full* environment into a subprocess, `export` targets a shell that
already has `PATH`, `HOME`, etc., so it emits **only the managed variables** — never the inherited
shell or an OS safe-minimum.

## 2. Goals / Non-goals

### Goals
- Print `export KEY='VALUE'` for the resolved managed environment, sorted by key, correctly quoted.
- Reuse the `run` source-loading path (profile/`.env`, in-memory decrypt) and the schema helpers.
- Add a small, pure, reusable `resolve.Managed(...)` (managed-only resolution) without changing
  `Resolve`'s behavior.
- `--strict` for CI: emit nothing and exit `1` if a required variable is missing.
- No new dependencies.

### Non-goals (v1 — explicitly out, YAGNI)
- fish (`set -gx`), JSON, and dotenv output formats. POSIX shell only.
- A TTY "you are printing secret values" warning.
- `unset`/unload emission and any shell hook — that is the next slice (`symbion hook`).
- Inheriting or emitting ambient shell variables.

## 3. Command surface

```
symbion export [profile] [--strict]
```

Examples:
```bash
eval "$(symbion export staging)"   # load a profile into the current shell
symbion export                     # no profile → uses .env
symbion export --strict prod       # exit 1 (emit nothing) if a required var is missing
```

- `profile` (optional, at most one): a saved profile name, or the literals `.env` / `env` / `current`.
  **Omitted → `.env`.** Same semantics as `run`.
- Encrypted profiles read `SYMBION_PASSPHRASE`.
- `--strict`: if any schema-`required`, non-deprecated key is absent from the resolved set, print the
  missing keys to stderr and exit `1` **without emitting any `export` lines** (so a failed
  `eval "$(...)"` sets nothing).

### Exit codes
- `0` — success (export lines emitted).
- `1` — `--strict` precheck failed.
- `2` — resolution error (`.env` not found, profile not found, missing/invalid passphrase, no schema
  for a named profile).

Errors are plain errors (exit `2`, `symbion:` prefix via `main.go`); `--strict` returns
`&cli.ExitError{Code: 1}`.

## 4. Resolution — managed-only

Add a pure managed-only resolver to `internal/resolve`, and refactor the existing merge/mark/sort out
of `Resolve` so both share it (DRY, and `Resolve`'s behavior is unchanged).

```go
// flatten merges ordered layers (later wins), marks secrets, and returns Vars
// sorted by Key. Extracted from Resolve; used by both Resolve and Managed.
func flatten(layers []layer, secretKeys map[string]bool) []Var

// Managed resolves only the managed variables: [defaults, source] with source
// winning. No shell layer, no safe-minimum. sourceName ("" defaults to
// SourceEnvFile) attributes the source layer.
func Managed(source, defaults map[string]string, secretKeys map[string]bool, sourceName Source) []Var
```

`Resolve` becomes `return flatten(buildLayers(shell, source, opts), opts.SecretKeys)`.

`Managed` builds `[]layer{{SourceDefault, defaults}, {sourceName, source}}` and calls `flatten`. Each
`Var.Secret = secretKeys[key] || looksSensitive(key)` (unchanged marking; `export` emits values
regardless of `Secret`, but the field is preserved for reuse by `hook`).

## 5. Output & quoting

For each resolved `Var` (already sorted by key), print one line:

```
export KEY='VALUE'
```

Values are single-quote quoted with POSIX-safe escaping so spaces, quotes, `$`, backticks, and newlines
survive `eval`:

```go
func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}
```

(`'\''` closes the quote, emits an escaped literal `'`, and reopens — the standard POSIX idiom.)

Keys are emitted verbatim (they originate from `.env`/profile parsing and are valid environment
names). Values are emitted as-is (no masking): loading real values, including secrets, into the shell
is the purpose of the command.

## 6. Error handling

| Situation | Behavior |
|---|---|
| `.env` requested/defaulted but missing | exit 2, `".env not found"` |
| Named profile but no schema/project | exit 2, `"run symbion init or symbion scan first"` |
| Profile not found | reuse `store.ResolveProfile` error (exit 2) |
| Encrypted profile, no `SYMBION_PASSPHRASE` | reuse `explainVaultError` |
| `--strict` required var missing | print missing keys to stderr, emit nothing, exit 1 |
| More than one positional arg | exit 2, argument error (`cobra.MaximumNArgs(1)`) |

Source loading, schema loading, and the missing-required computation reuse `loadRunSource`,
`optionalSchema`, `schemaDefaults`, `schemaSecretKeys`, and `missingRequired` from `internal/cli/run.go`.

## 7. Testing strategy

**`internal/resolve/resolve_test.go`** — add `TestManaged`:
- `[defaults, source]` with source winning; a default fills a key absent from source.
- output sorted by key; **no shell variables** appear (only managed keys).
- secret marking via schema and via `looksSensitive`.

Existing `Resolve` tests must stay green after the `flatten` refactor (behavior unchanged).

**`internal/cli/export_test.go`**:
- `TestShellQuote` — table: plain, spaces, embedded single quote (`a'b` → `'a'\''b'`), `$VAR`, empty.
- `TestExportEmitsManagedOnly` — project with `.env` (e.g. `FOO=bar`, `PORT=3000`) and a schema
  default; assert output contains `export FOO='bar'`, is sorted, and contains **no** `export PATH=`
  (proves the shell is not emitted).
- `TestExportEscapesQuotes` — a `.env` value containing a single quote emits the escaped form.
- `TestExportStrictFailsOnMissingRequired` — schema-required key absent ⇒ exit 1, nothing on stdout,
  missing key on stderr.
- `TestExportRoundTripsViaEval` — capture the command output, run
  `sh -c '<output>; printenv KEY'`, and assert the printed value equals the original (proves the
  emitted script is valid, eval-able shell). Uses `sh` (POSIX; present on macOS/Linux).

All tests use the existing `runCommand`/`writeFile` helpers and `t.Setenv("HOME", ...)` for vault
isolation.

## 8. File changes

| File | Change |
|---|---|
| `internal/resolve/resolve.go` | **edit** — extract `flatten`, add `Managed`; `Resolve` delegates to `flatten` |
| `internal/resolve/resolve_test.go` | **edit** — add `TestManaged` |
| `internal/cli/export.go` | **new** — `newExportCommand`, `shellQuote`, emit loop |
| `internal/cli/export_test.go` | **new** — tests above |
| `internal/cli/root.go` | **edit** — register `newExportCommand()` |
| `README.md` | **edit** — `export` under Commands + a Core Workflow / Features mention |

## 9. How this composes

`resolve.Managed` and the `shellQuote`/emit logic are the reusable core of the next slice:

- **`symbion hook <shell>`** prints shell integration; on `cd` it calls an internal command that emits
  the same `export` lines **plus** `unset` for variables leaving scope, gated by a per-directory
  trust/allow step. `export` is that emitter without the hook, trust, or unload — so building it first
  isolates and tests the quoting and managed-only resolution once.
