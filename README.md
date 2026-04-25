# Symbion

Symbion is a Go CLI that keeps local environment variables organized, documented and safe to evolve.

It is not just a `.env` storage helper. It acts as an **Env Manager + Env Doctor** for development projects, comparing `.env`, `.env.example`, `.symbion.yaml` and Docker Compose references before small configuration mistakes become onboarding or deployment problems.

```text
  _____ __  ____  ______  ____  ____  _   __
 / ___// / / /  |/  / __ )/  _/ __ \/ | / /
 \__ \/ / / / /|_/ / __  |/ // / / /  |/ /
___/ / /_/ / /  / / /_/ // // /_/ / /|  /
/____/\__, /_/  /_/_____/___/\____/_/ |_/
     /____/
Local Environment Intelligence
```

## What It Solves

- Missing variables in local `.env`
- Drift between `.env`, `.env.example` and the source-of-truth schema
- Docker Compose references to variables that do not exist locally
- Obsolete variables that stay alive for months
- Weak onboarding for developers joining an existing project
- CI readiness through meaningful exit codes

## Install And Run

```bash
go mod tidy
go run ./cmd/symbion --help
go run ./cmd/symbion init
go run ./cmd/symbion scan
go run ./cmd/symbion doctor
go run ./cmd/symbion save local
go run ./cmd/symbion profiles
go run ./cmd/symbion use local
go run ./cmd/symbion diff .env local
go run ./cmd/symbion backups
go run ./cmd/symbion undo
```

Build a local binary:

```bash
go build -o bin/symbion ./cmd/symbion
./bin/symbion doctor
```

## Commands

### `symbion init`

Creates `.symbion.yaml` using the current folder name as the project name. If the schema already exists, Symbion reports it and does not overwrite it.

Generated schema:

```yaml
project: my-project
envs: []
```

### `symbion scan`

Reads `.env.example`, adds new keys to `.symbion.yaml`, preserves existing metadata and marks newly discovered variables as `required: true`.

### `symbion doctor`

Compares:

- `.env`
- `.env.example`
- `.symbion.yaml`
- `docker-compose.yml`
- `docker-compose.yaml`
- `compose.yml`
- `compose.yaml`

It detects missing required envs, missing example envs, Docker Compose references absent from `.env`, extra variables, deprecated variables and suggested replacements.

Exit codes:

- `0`: everything is valid
- `1`: inconsistencies were found
- `2+`: real execution error, such as invalid files or missing schema

### `symbion save <profile>`

Saves the current project's `.env` as a reusable local profile.

Profiles are stored outside the project, under:

```text
~/.symbion/projects/<project>/profiles/<profile>.env
```

Example:

```bash
symbion save local
symbion save staging
```

Symbion copies the raw `.env` file, preserving comments, ordering and formatting. It does not print secret values.

Encrypted profiles:

```bash
export SYMBION_PASSPHRASE="choose-a-strong-local-passphrase"
symbion save local --encrypt
```

Encrypted profiles are stored as `.env.enc` files and are decrypted only when `SYMBION_PASSPHRASE` is set.
New encrypted profiles use Argon2id with AES-GCM. Older PBKDF2 profiles remain readable.

### `symbion profiles`

Lists saved profiles for the current project:

```bash
symbion profiles
```

### `symbion use <profile>`

Restores a saved profile into the current project's `.env`:

```bash
symbion use local
```

Preview before restoring:

```bash
symbion use local --dry-run
```

This shows a safe diff and does not change files.

Without `--dry-run`, this overwrites the current `.env` with the saved profile. Before writing, Symbion creates an automatic backup under:

```text
~/.symbion/projects/<project>/backups/
```

Writes are atomic and guarded by a per-project lock so concurrent Symbion commands do not corrupt `.env` or profile files.

### `symbion backups`

Lists automatic backups for the current project:

```bash
symbion backups
```

### `symbion undo`

Restores the latest automatic backup into `.env`:

```bash
symbion undo
```

Before restoring, Symbion backs up the current `.env`, so undo can be undone.

### `symbion diff <left> <right>`

Compares two env sources without printing secret values:

```bash
symbion diff .env local
symbion diff local staging
```

Supported sources:

- `.env`, `env` or `current`: current project `.env`
- any other value: a saved profile name

The report only shows key names that changed, exist only on one side, or match.

## Schema Format

```yaml
project: billing-api
envs:
  - key: DATABASE_URL
    description: URL of the local database
    required: true
    secret: false
    default: ""
    deprecated: false
    replacement: ""
```

Field reference:

- `key`: environment variable name
- `description`: human-friendly explanation
- `required`: whether `.env` must contain the variable
- `secret`: marks variables that should never be committed with real values
- `default`: optional default value documentation
- `deprecated`: marks variables that should be removed
- `replacement`: suggested replacement for deprecated variables

## Environment Storage

Symbion separates the environment contract from real secret values:

- `.env`: real local values for the active project profile; do not commit it
- `.env.example`: safe template that can be committed
- `.symbion.yaml`: documented contract that can be committed
- `~/.symbion/projects/.../profiles`: local profile storage for reusable `.env` values
- `~/.symbion/projects/.../backups`: automatic backups created before restoring profiles

Recommended workflow:

```bash
symbion init
symbion scan
symbion doctor
symbion save local
symbion diff .env local
symbion use local
```

## Doctor Output Example

```text
$ symbion doctor

Symbion Doctor Report
---------------------
Project: billing-api

Schema Status
  [OK] .symbion.yaml loaded
  [OK] 8 tracked variables

Files
  [OK] .env loaded
  [OK] .env.example loaded
  [OK] compose files loaded: docker-compose.yml

Checks
  [OK] Missing in .env: none
  [OK] Missing in .env.example: none
  [!] Deprecated variables in .env: OLD_API_KEY -> use API_KEY
  [!] Missing for docker-compose: REDIS_URL
  [OK] Extra in .env: none
  [OK] Extra in .env.example: none

Summary
  2 issues found
  Review: .env, .env.example and .symbion.yaml
```

## Development

Run all tests:

```bash
go test ./...
```

Run with fixture data:

```bash
cd testdata/valid
go run ../../cmd/symbion doctor
```

## Roadmap

- `symbion sync`
- JSON output with `--output json`
- `.env.local` support
- source-code scan for `os.Getenv(...)`
- Node, Python and Go validation helpers
- CI mode
- macOS Keychain integration for profile passphrases

## License

MIT
