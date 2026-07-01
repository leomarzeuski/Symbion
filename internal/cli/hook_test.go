package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestHookZshSnippet(t *testing.T) {
	out, _, err := runCommand(t, t.TempDir(), "hook", "zsh")
	if err != nil {
		t.Fatalf("hook zsh error = %v", err)
	}
	if !strings.Contains(out, "precmd_functions") || !strings.Contains(out, "symbion hook-env") {
		t.Fatalf("snippet missing pieces:\n%s", out)
	}
}

func TestHookUnsupportedShell(t *testing.T) {
	if _, _, err := runCommand(t, t.TempDir(), "hook", "bash"); err == nil {
		t.Fatal("hook bash should error")
	}
}

func TestHookEnvTrustedEmitsExports(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFile(t, filepath.Join(dir, ".symbion.yaml"), "project: billing-api\nenvs: []\n")
	writeFile(t, filepath.Join(dir, ".env"), "FOO=bar\n")
	if _, _, err := runCommand(t, dir, "allow"); err != nil {
		t.Fatalf("allow error = %v", err)
	}
	t.Setenv("SYMBION_LOADED", "OLD")
	out, _, err := runCommand(t, dir, "hook-env")
	if err != nil {
		t.Fatalf("hook-env error = %v", err)
	}
	for _, want := range []string{"unset OLD", "export FOO='bar'", "export SYMBION_LOADED='FOO'"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestHookEnvUntrustedUnloads(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFile(t, filepath.Join(dir, ".env"), "FOO=bar\n")
	t.Setenv("SYMBION_LOADED", "FOO")
	out, errOut, err := runCommand(t, dir, "hook-env")
	if err != nil {
		t.Fatalf("hook-env error = %v", err)
	}
	if !strings.Contains(out, "unset FOO") || !strings.Contains(out, "unset SYMBION_LOADED") {
		t.Fatalf("expected unloads:\n%s", out)
	}
	if strings.Contains(out, "export FOO") {
		t.Fatalf("untrusted must not export:\n%s", out)
	}
	if !strings.Contains(errOut, "blocked") {
		t.Fatalf("expected blocked note: %q", errOut)
	}
}

func TestHookEnvNoEnvUnloads(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SYMBION_LOADED", "FOO")
	out, _, err := runCommand(t, dir, "hook-env")
	if err != nil {
		t.Fatalf("hook-env error = %v", err)
	}
	if !strings.Contains(out, "unset FOO") || !strings.Contains(out, "unset SYMBION_LOADED") {
		t.Fatalf("expected unloads:\n%s", out)
	}
}

func TestAllowDenyChangesTrust(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFile(t, filepath.Join(dir, ".env"), "FOO=bar\n")
	if _, _, err := runCommand(t, dir, "allow"); err != nil {
		t.Fatalf("allow: %v", err)
	}
	if _, _, err := runCommand(t, dir, "deny"); err != nil {
		t.Fatalf("deny: %v", err)
	}
	out, _, err := runCommand(t, dir, "hook-env")
	if err != nil {
		t.Fatalf("hook-env: %v", err)
	}
	if strings.Contains(out, "export FOO") {
		t.Fatalf("after deny must not export:\n%s", out)
	}
}
