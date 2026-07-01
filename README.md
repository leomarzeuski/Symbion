# Symbion

Local environment intelligence for development projects.

Symbion is a small Go CLI that keeps `.env`, `.env.example`, `.symbion.yaml` and Docker Compose references in sync. It also lets you save, compare, restore and back up local `.env` profiles per project.

It is built for the quiet problems that slow teams down: missing env vars, stale examples, undocumented secrets, broken onboarding and unsafe local profile switching.

```text
.env              real local values, never committed
.env.example      safe template for the team
.symbion.yaml     documented environment contract
~/.symbion/       local profiles and backups
```

## Features

- Detect missing, extra and deprecated environment variables.
- Compare `.env`, `.env.example`, `.symbion.yaml` and Docker Compose references.
- Save reusable `.env` profiles per project.
- Restore profiles with automatic backups.
- Preview changes with safe diffs that never print secret values.
- Optionally encrypt saved profiles with Argon2id + AES-GCM.
- Run any command with a profile's environment injected in memory, without writing secrets to disk (`symbion run`).
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

### `symbion doctor`

Validates the project environment contract.

It checks:

- required variables missing from `.env`
- variables missing from `.env.example`
- Docker Compose references missing from `.env`
- extra variables in `.env` or `.env.example`
- deprecated variables and replacements

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
