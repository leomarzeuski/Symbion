package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileCommandsEndToEnd(t *testing.T) {
	projectRoot := newCLIProject(t)
	t.Setenv("HOME", t.TempDir())

	out, errOut, err := runCommand(t, projectRoot, "save", "local")
	if err != nil {
		t.Fatalf("save error = %v, stderr = %s", err, errOut)
	}
	if !strings.Contains(out, `Saved .env as profile "local"`) {
		t.Fatalf("save output = %q", out)
	}

	writeFile(t, filepath.Join(projectRoot, ".env"), "DATABASE_URL=postgres://changed\nAPI_KEY=changed\n")
	out, errOut, err = runCommand(t, projectRoot, "use", "local", "--dry-run")
	if err != nil {
		t.Fatalf("use --dry-run error = %v, stderr = %s", err, errOut)
	}
	if !strings.Contains(out, "Dry run: no files changed.") {
		t.Fatalf("dry-run output = %q", out)
	}
	assertFile(t, filepath.Join(projectRoot, ".env"), "DATABASE_URL=postgres://changed\nAPI_KEY=changed\n")

	out, errOut, err = runCommand(t, projectRoot, "use", "local")
	if err != nil {
		t.Fatalf("use error = %v, stderr = %s", err, errOut)
	}
	if !strings.Contains(out, "Backup:") {
		t.Fatalf("use output = %q", out)
	}
	assertFile(t, filepath.Join(projectRoot, ".env"), "DATABASE_URL=postgres://local\nAPI_KEY=secret\n")

	out, errOut, err = runCommand(t, projectRoot, "backups")
	if err != nil {
		t.Fatalf("backups error = %v, stderr = %s", err, errOut)
	}
	if !strings.Contains(out, "before-use-local.env") {
		t.Fatalf("backups output = %q", out)
	}
}

func TestEncryptedProfileCommandsEndToEnd(t *testing.T) {
	projectRoot := newCLIProject(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv(passphraseEnv, "test-passphrase")

	out, errOut, err := runCommand(t, projectRoot, "save", "local", "--encrypt")
	if err != nil {
		t.Fatalf("save --encrypt error = %v, stderr = %s", err, errOut)
	}
	if !strings.Contains(out, "Encryption: enabled") {
		t.Fatalf("save --encrypt output = %q", out)
	}

	out, errOut, err = runCommand(t, projectRoot, "profiles")
	if err != nil {
		t.Fatalf("profiles error = %v, stderr = %s", err, errOut)
	}
	if !strings.Contains(out, "local (encrypted)") {
		t.Fatalf("profiles output = %q", out)
	}

	writeFile(t, filepath.Join(projectRoot, ".env"), "DATABASE_URL=postgres://changed\nAPI_KEY=changed\n")
	out, errOut, err = runCommand(t, projectRoot, "use", "local")
	if err != nil {
		t.Fatalf("use encrypted error = %v, stderr = %s", err, errOut)
	}
	if !strings.Contains(out, "Encryption: enabled") {
		t.Fatalf("use encrypted output = %q", out)
	}
	assertFile(t, filepath.Join(projectRoot, ".env"), "DATABASE_URL=postgres://local\nAPI_KEY=secret\n")
}

func runCommand(t *testing.T, dir string, args ...string) (string, string, error) {
	t.Helper()

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%q) error = %v", dir, err)
	}
	defer os.Chdir(previous)

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs(args)
	err = cmd.Execute()

	return out.String(), errOut.String(), err
}

func newCLIProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".symbion.yaml"), "project: billing-api\nenvs: []\n")
	writeFile(t, filepath.Join(dir, ".env"), "DATABASE_URL=postgres://local\nAPI_KEY=secret\n")
	writeFile(t, filepath.Join(dir, ".env.example"), "DATABASE_URL=\nAPI_KEY=\n")

	return dir
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func assertFile(t *testing.T, path string, want string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, data, want)
	}
}
