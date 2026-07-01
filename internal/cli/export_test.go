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
