package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShareAdoptRoundTrip(t *testing.T) {
	dir := newCLIProject(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv(passphraseEnv, "team-secret")

	if _, errOut, err := runCommand(t, dir, "share", "team"); err != nil {
		t.Fatalf("share error = %v, stderr = %s", err, errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, ".symbion", "shared", "team.enc")); err != nil {
		t.Fatalf("shared file missing: %v", err)
	}

	// change .env, then adopt should restore it and back up the changed one
	writeFile(t, filepath.Join(dir, ".env"), "DATABASE_URL=changed\n")
	out, errOut, err := runCommand(t, dir, "adopt", "team")
	if err != nil {
		t.Fatalf("adopt error = %v, stderr = %s", err, errOut)
	}
	if !strings.Contains(out, "Backup:") {
		t.Fatalf("expected backup note:\n%s", out)
	}
	assertFile(t, filepath.Join(dir, ".env"), "DATABASE_URL=postgres://local\nAPI_KEY=secret\n")
}

func TestShareRequiresPassphrase(t *testing.T) {
	dir := newCLIProject(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv(passphraseEnv, "")
	if _, _, err := runCommand(t, dir, "share", "team"); err == nil {
		t.Fatal("share without passphrase should error")
	}
}

func TestAdoptMissing(t *testing.T) {
	dir := newCLIProject(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv(passphraseEnv, "team-secret")
	if _, _, err := runCommand(t, dir, "adopt", "nope"); err == nil {
		t.Fatal("adopt of a missing profile should error")
	}
}

func TestAdoptWrongPassphrase(t *testing.T) {
	dir := newCLIProject(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv(passphraseEnv, "team-secret")
	if _, _, err := runCommand(t, dir, "share", "team"); err != nil {
		t.Fatalf("share error = %v", err)
	}
	t.Setenv(passphraseEnv, "wrong-secret")
	if _, _, err := runCommand(t, dir, "adopt", "team"); err == nil {
		t.Fatal("adopt with wrong passphrase should error")
	}
}
