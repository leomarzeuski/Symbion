# Symbion `hook` — Design Spec

- **Date:** 2026-07-01
- **Status:** Approved (design), pending implementation plan
- **Feature:** `symbion hook` — direnv-style shell auto-load of a trusted project's `.env`, with a
  content-hash trust model and unset-on-leave
- **North star:** Personal power tool + portfolio showcase. Builds directly on `resolve.Managed` +
  `shellQuote` (from `export`) and completes the "shell auto-load" sequence.

## 1. Summary

`symbion hook zsh` prints a shell integration snippet. Once added to `~/.zshrc`, entering a **trusted**
project directory auto-loads its `.env` (managed variables) into the interactive shell, and leaving (or
switching projects) unsets exactly the variables Symbion set. Trust is explicit and content-hashed:
a directory only auto-loads after `symbion allow`, and editing `.env` re-blocks it until re-allowed —
so a freshly cloned repository never injects environment into your shell.

This composes with `use` (which writes `.env`): `symbion use staging` then auto-loads on the next
prompt. It reuses the `resolve.Managed` + `shellQuote` emitter built for `export`.

## 2. Goals / Non-goals

### Goals
- `symbion hook zsh` prints a zsh `precmd` integration snippet.
- `symbion allow [dir]` / `symbion deny [dir]` manage per-directory trust by `.env` content hash.
- A hidden `symbion hook-env` emits `export`/`unset` for the current directory each prompt.
- Explicit-allow trust: auto-load only trusted dirs; a changed `.env` re-blocks until re-allowed.
- Unset-what-we-set unload: track loaded keys in a `SYMBION_LOADED` marker var; unset them on leave.
- `hook-env` never breaks the prompt (always exits 0; errors go to stderr).
- Only ever touches keys it tracks — never the user's other shell variables.
- No new dependencies; reuse `resolve.Managed`, `shellQuote`, `schemaDefaults`, `parser.LoadEnvFile`.

### Non-goals (v1 — explicitly out, YAGNI)
- bash / fish integration (zsh only; `symbion hook bash` errors clearly).
- Save/restore of a variable's *prior* value on unload (we `unset`, not restore).
- An `allow --list` / trust-status view.
- Loading a *named/encrypted profile* on `cd` (hook loads `.env`; use `symbion use` to stage a profile
  into `.env` first).

## 3. Command surface

```bash
symbion hook zsh          # prints integration; add: eval "$(symbion hook zsh)" to ~/.zshrc
symbion allow [dir]       # trust the current (or given) dir's .env for auto-load
symbion deny  [dir]       # revoke trust
symbion hook-env          # (hidden) emit export/unset for cwd; the snippet calls this each prompt
```

- `hook <shell>`: `zsh` supported; any other value → exit 2, `"unsupported shell %q; only zsh is supported"`.
- `allow`/`deny`: operate on cwd by default, or a single path argument (`cobra.MaximumNArgs(1)`).
  `allow` requires the target dir to contain a `.env`.
- `hook-env`: hidden (`Hidden: true`), no args; **always exits 0**.

### Exit codes
- `allow` / `deny`: `0` on success; `2` on error (e.g. `allow` with no `.env`).
- `hook <shell>`: `0` for zsh; `2` for unsupported shell.
- `hook-env`: always `0`.

## 4. Trust store (`internal/trust`)

Content-hash based, one small file per allowed directory under the Symbion home:

```
<root>/trust/<hex(sha256(absDir))>   →  content: "<sha256(.env bytes)>\n<absDir>\n"
```

`root` is `~/.symbion` (reuse `vault.NewDefaultStore().Root` semantics: `os.UserHomeDir()` +
`.symbion`). A `Store{Root string}` mirrors the vault's testable shape.

Types & API:
```go
type Store struct { Root string }

func NewDefaultStore() (Store, error)          // ~/.symbion
func (s Store) Allow(dir string) error         // requires dir/.env; writes trust file (0600, dir 0700)
func (s Store) Deny(dir string) error          // removes trust file; no-op if absent
func (s Store) IsTrusted(dir string) (bool, error) // true iff file exists AND stored hash == current .env hash
```

- `absDir` = `filepath.Abs(dir)`; the trust filename is `hex(sha256(absDir))`.
- `.env` hash = `sha256` of the file bytes, hex-encoded.
- `IsTrusted` returns `false` (no error) when the trust file is absent, the `.env` is absent, or the
  stored hash differs from the current `.env` hash.
- `Allow` returns an error if `dir/.env` does not exist (`"no .env in %s to allow"`).
- Stores **only hashes and the absolute path** — never environment values. Files written with the
  existing private-permission approach (dir `0700`, file `0600`).

## 5. The emitter — `symbion hook-env`

Called by the zsh snippet each prompt; stdout is `eval`'d. Algorithm:

1. `loaded` = fields of `os.Getenv("SYMBION_LOADED")` (space-separated keys set last prompt).
2. `cwd` = `os.Getwd()`.
3. Build **target** (`map[string]string`, plus a sorted key list):
   - `env, found = parser.LoadEnvFile(cwd/.env)`.
   - If `found` and `trust.IsTrusted(cwd)`: `vars = resolve.Managed(env, schemaDefaults(optionalSchema(cwd)), nil, resolve.SourceEnvFile)`; target = those keys/values.
   - Else target is empty. If `found` and not trusted, write one stderr line:
     `symbion: .env in <cwd> is blocked; run 'symbion allow'`.
4. Emit to stdout, in order:
   - for each key in `loaded` not in target (sorted): `unset KEY`
   - for each target var (sorted by key): `export KEY=<shellQuote(value)>`
   - if target non-empty: `export SYMBION_LOADED=<shellQuote(space-joined sorted target keys)>`
     else if `loaded` non-empty: `unset SYMBION_LOADED`
5. Return `nil` always. Any internal error → best-effort stderr note, no stdout, still exit 0.

Notes:
- The marker key `SYMBION_LOADED` is managed by Symbion; target keys never include it.
- Only keys in `loaded` are ever unset — the user's unrelated shell vars are untouched.
- Values are emitted as-is (loading real values, including secrets, into the shell is the intent).

## 6. The zsh snippet — `symbion hook zsh`

Prints exactly:
```zsh
_symbion_hook() {
  eval "$(command symbion hook-env)"
}
typeset -ag precmd_functions
if (( ! ${precmd_functions[(Ie)_symbion_hook]} )); then
  precmd_functions=(_symbion_hook $precmd_functions)
fi
```
- Command substitution captures stdout (the `export`/`unset` lines); the untrusted **stderr** note
  shows through to the terminal.
- The idempotence guard avoids double-registration if the snippet is sourced twice.

## 7. Error handling

| Situation | Behavior |
|---|---|
| `symbion allow` where no `.env` exists | exit 2, `"no .env in <dir> to allow"` |
| `symbion deny` on an untrusted dir | exit 0 (no-op) |
| `symbion hook <unsupported>` | exit 2, `"unsupported shell \"x\"; only zsh is supported"` |
| `hook-env` in a dir with no `.env` | unset previously-loaded keys + `SYMBION_LOADED`; exit 0 |
| `hook-env` with untrusted `.env` | stderr note; unset previously-loaded keys; exit 0 |
| any `hook-env` internal error | stderr note; no stdout; exit 0 |

## 8. Testing strategy

**`internal/trust/trust_test.go`** (temp `Root`):
- `Allow` a temp dir with `.env` → `IsTrusted` true.
- mutate `.env` bytes → `IsTrusted` false (stale hash).
- `Deny` → `IsTrusted` false.
- dir with no `.env` → `Allow` returns an error; `IsTrusted` false.

**`internal/cli`** (reuse `runCommand`, `writeFile`, `t.Setenv("HOME", ...)`):
- `TestAllowDenyRoundTrip` — `allow` marks trusted, `deny` clears it (assert via `hook-env` behavior or `trust.Store`).
- `TestHookEnvTrustedEmitsExports` — trusted dir + `t.Setenv("SYMBION_LOADED", "OLD")`; `hook-env`
  stdout contains `unset OLD`, `export FOO='bar'`, `export SYMBION_LOADED='FOO'`.
- `TestHookEnvUntrustedUnloads` — `.env` present, not allowed, `SYMBION_LOADED=FOO`; stdout has
  `unset FOO` and `unset SYMBION_LOADED`, no `export FOO`, and stderr contains `blocked`.
- `TestHookEnvNoEnvUnloads` — no `.env`, `SYMBION_LOADED=FOO` → `unset FOO`, `unset SYMBION_LOADED`.
- `TestHookZshSnippet` — `hook zsh` output contains `precmd_functions` and `symbion hook-env`.
- `TestHookUnsupportedShell` — `hook bash` → error (exit 2).

`hook-env` runs with cwd set by `runCommand`'s chdir; trust store uses the isolated `HOME`.

## 9. File changes

| File | Change |
|---|---|
| `internal/trust/trust.go` | **new** — `Store`, `NewDefaultStore`, `Allow`, `Deny`, `IsTrusted`, hashing/paths |
| `internal/trust/trust_test.go` | **new** — trust tests |
| `internal/cli/allow.go` | **new** — `newAllowCommand`, `newDenyCommand` |
| `internal/cli/hook.go` | **new** — `newHookCommand`, `newHookEnvCommand`, emit logic, zsh snippet |
| `internal/cli/hook_test.go` | **new** — allow/deny + hook-env + snippet tests |
| `internal/cli/root.go` | **edit** — register `allow`, `deny`, `hook`, `hook-env` |
| `README.md` | **edit** — `hook`/`allow`/`deny` docs + a quick-start "auto-load" note |

## 10. How it composes

This completes the shell-auto-load sequence: `resolve` (engine) → `run` (subprocess) → `export`
(emit for eval) → `hook` (trusted auto-load). The trust store (`internal/trust`) is a reusable
primitive a future `symbion status` or team-sync trust could build on. bash/fish support is an
additive follow-up (a new snippet + the same `hook-env`).
