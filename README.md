# Symbion

Local environment intelligence for development projects.

Symbion is a small Go CLI that keeps `.env`, `.env.example`, `.symbion.yaml` and Docker Compose references in sync. It also lets you save, compare, restore and back up local `.env` profiles per project.

It is built for the quiet problems that slow teams down: missing env vars, stale examples, undocumented secrets, broken onboarding and unsafe local profile switching.

```text
.env              real local values, never committed
.env.local        local-only overrides layered on top of .env
.env.example      safe template for the team
.symbion.yaml     documented environment contract
~/.symbion/       local profiles and backups
```

## Features

- Detect missing, extra and deprecated environment variables.
- Validate `.env` values against declared types, enums and patterns.
- Discover env vars referenced in your source code (`symbion scan --code`).
- Emit `doctor` results as JSON for CI and tooling (`symbion doctor --json`).
- Layer `.env.local` overrides on top of `.env` (local values win) across doctor, run, export and hook.
- Store the encryption passphrase in the macOS Keychain (`symbion passphrase set`).
- Compare `.env`, `.env.example`, `.symbion.yaml` and Docker Compose references.
- Save reusable `.env` profiles per project.
- Restore profiles with automatic backups.
- Preview changes with safe diffs that never print secret values.
- Optionally encrypt saved profiles with Argon2id + AES-GCM.
- Run any command with a profile's environment injected in memory, without writing secrets to disk (`symbion run`).
- Print a profile's environment as shell `export` statements for `eval` (`symbion export`).
- Auto-load a trusted project's `.env` into your shell on `cd`, direnv-style, with explicit trust (`symbion hook`).
- Use meaningful exit codes for scripts and CI.
- Write files atomically with a per-project lock.

## Install

From source:

```bash
go build -o bin/symbion ./cmd/symbion
```

Then run:

```bash
./bin/symbion --help
```

If the repository is published under the module path, you can install with:

```bash
go install github.com/leonardomarzeuski/symbion/cmd/symbion@latest
```

## Quick Start

Inside any project:

```bash
symbion init
symbion scan
symbion doctor
```

Save your current local env:

```bash
symbion save local
```

Compare the current `.env` with a saved profile:

```bash
symbion diff .env local
```

Preview a restore:

```bash
symbion use local --dry-run
```

Restore a profile:

```bash
symbion use local
```

Symbion creates a backup before replacing `.env`.

## Core Workflow

1. Keep real values in `.env`.
2. Keep safe examples in `.env.example`.
3. Keep documentation and rules in `.symbion.yaml`.
4. Run `symbion doctor` when envs change.
5. Use `symbion save`, `symbion diff` and `symbion use` to manage local profiles.
6. Use `symbion run <profile> -- <command>` to launch your app with a profile's values, without writing secrets to disk.

## Commands

### `symbion init`

Creates `.symbion.yaml` in the current project:

```yaml
project: my-project
envs: []
```

### `symbion scan`

Reads `.env.example` and adds discovered keys to `.symbion.yaml`.

New keys are marked as required by default.

With `--code`, it also scans your source for env var references (`process.env.X`, `os.Getenv("X")`,
`os.environ['X']`, `import.meta.env.X`, `ENV['X']`) and adds any undocumented keys:

```bash
symbion scan --code
```

### `symbion doctor`

Validates the project environment contract.

It checks:

- required variables missing from `.env`
- variables missing from `.env.example`
- Docker Compose references missing from `.env`
- extra variables in `.env` or `.env.example`
- deprecated variables and replacements
- values that violate their declared type, enum, or pattern

For machine-readable output (CI, tooling), add `--json`:

```bash
symbion doctor --json
```

Exit codes:

- `0`: all checks passed
- `1`: environment issues were found
- `2+`: execution error, such as invalid files or missing schema

### `symbion save <profile>`

Saves the current `.env` as a local profile:

```bash
symbion save local
symbion save staging
```

Profiles are stored outside the project:

```text
~/.symbion/projects/<project>/profiles/
```

Symbion copies the raw `.env` file, preserving comments, order and formatting.

### `symbion save <profile> --encrypt`

Saves an encrypted profile:

```bash
export SYMBION_PASSPHRASE="choose-a-strong-passphrase"
symbion save local --encrypt
```

Encrypted profiles are stored as `.env.enc` files and require `SYMBION_PASSPHRASE` to read.

### `symbion passphrase`

Stores the encryption passphrase in your OS keychain (macOS) so you don't need `SYMBION_PASSPHRASE`
in your environment:

```bash
symbion passphrase set     # reads SYMBION_PASSPHRASE, or prompts on stdin
symbion passphrase clear   # removes it
```

When resolving a passphrase, Symbion uses `SYMBION_PASSPHRASE` if set, otherwise the keychain.

### `symbion profiles`

Lists saved profiles for the current project:

```bash
symbion profiles
```

### `symbion diff <left> <right>`

Compares two env sources without printing values:

```bash
symbion diff .env local
symbion diff local staging
```

Supported sources:

- `.env`, `env` or `current`
- any saved profile name

### `symbion use <profile>`

Restores a profile into `.env`:

```bash
symbion use local
```

Preview first:

```bash
symbion use local --dry-run
```

Before writing, Symbion creates a backup under:

```text
~/.symbion/projects/<project>/backups/
```

### `symbion hook`

Auto-loads a trusted project's `.env` into your shell when you `cd` in (direnv-style), and unsets it
when you leave. Add the integration to your shell config:

```bash
eval "$(symbion hook zsh)"    # ~/.zshrc
eval "$(symbion hook bash)"   # ~/.bashrc
symbion hook fish | source    # ~/.config/fish/config.fish
```

A directory only auto-loads after you trust it, and any edit to `.env` re-blocks it until you allow it
again:

```bash
symbion allow    # trust this directory's .env for auto-load
symbion deny     # revoke it
```

Only managed variables are loaded, and only the variables Symbion set are unset on leave — your other
shell variables are untouched. Supported shells: zsh, bash, fish.

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

### `symbion run`

Runs a command with a resolved environment injected, without writing secrets to disk:

```bash
symbion run staging -- npm run dev
symbion run -- go test ./...          # no profile, uses .env
```

Resolution precedence (default): start from your shell, the profile overrides matching keys, and
schema defaults fill anything still missing. Encrypted profiles are decrypted in memory only
(set `SYMBION_PASSPHRASE`).

Flags:

- `--dry-run`: print the resolved environment (secret values masked) without running.
- `--strict`: refuse to launch if a schema-required variable is missing.
- `--isolated`: do not inherit the ambient shell; use only resolved vars plus OS essentials.
- `--no-override`: let existing shell variables win over the profile.
- `--show-values`: with `--dry-run`, reveal masked values.

The command's exit code is propagated exactly. Secret values are never printed or written to disk.

### `symbion backups`

Lists automatic backups:

```bash
symbion backups
```

### `symbion undo`

Restores the latest automatic backup:

```bash
symbion undo
```

## Schema

`.symbion.yaml` is the project contract:

```yaml
project: billing-api
envs:
  - key: DATABASE_URL
    description: Local database connection string
    required: true
    secret: false
    default: ""
    deprecated: false
    replacement: ""

  - key: OLD_API_KEY
    description: Previous API key name
    required: false
    secret: true
    default: ""
    deprecated: true
    replacement: API_KEY
```

Fields:

- `key`: environment variable name
- `description`: human-friendly explanation
- `required`: whether `.env` must contain the key
- `secret`: whether the value is sensitive
- `default`: documented default value
- `deprecated`: whether the key should be removed
- `replacement`: suggested replacement for deprecated keys
- `type`: optional value type to validate (`int`, `bool`, `port`, `url`, `duration`, `string`)
- `enum`: optional list of allowed values
- `pattern`: optional regular expression the value must match

## Security

Symbion is local-first.

- It does not print secret values in `doctor` or `diff`.
- It stores profiles under `~/.symbion`, outside the project.
- It writes saved profiles, backups and restored `.env` files with private permissions.
- It creates backups before restoring profiles.
- It writes files atomically and uses a per-project lock.
- Encrypted profiles use Argon2id + AES-GCM.

Do not commit `.env` or unencrypted profile files.

## Development

Run checks:

```bash
go test ./...
go vet ./...
go test -race ./...
```

Build:

```bash
go build -o bin/symbion ./cmd/symbion
```

Run against fixture data:

```bash
cd testdata/valid
go run ../../cmd/symbion doctor
```

## Roadmap

- JSON output
- `.env.local` support
- source-code scanning for `process.env.*`, `os.Getenv(...)` and similar patterns
- stronger schema validation with types, enums and patterns
- macOS Keychain support for passphrases
- encrypted team sync
- GitHub Actions and deployment-provider integrations

## License

MIT
