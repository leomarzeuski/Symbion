package cli

import (
	"os"
	"strings"
	"testing"
)

// TestMain keeps the whole cli test suite off the real OS keychain by default.
// Individual tests override keychainGet/keychainSet locally as needed.
func TestMain(m *testing.M) {
	keychainGet = func() (string, bool, error) { return "", false, nil }
	os.Exit(m.Run())
}

func TestOptionalPassphraseEnvWins(t *testing.T) {
	t.Setenv(passphraseEnv, "fromenv")
	orig := keychainGet
	defer func() { keychainGet = orig }()
	keychainGet = func() (string, bool, error) { return "fromkeychain", true, nil }

	if got := string(optionalPassphrase()); got != "fromenv" {
		t.Fatalf("optionalPassphrase = %q, want fromenv", got)
	}
}

func TestOptionalPassphraseKeychainFallback(t *testing.T) {
	t.Setenv(passphraseEnv, "")
	orig := keychainGet
	defer func() { keychainGet = orig }()
	keychainGet = func() (string, bool, error) { return "fromkeychain", true, nil }

	if got := string(optionalPassphrase()); got != "fromkeychain" {
		t.Fatalf("optionalPassphrase = %q, want fromkeychain", got)
	}
}

func TestPassphraseSetReadsStdin(t *testing.T) {
	t.Setenv(passphraseEnv, "")
	orig := keychainSet
	defer func() { keychainSet = orig }()
	var stored string
	keychainSet = func(p string) error { stored = p; return nil }

	var out, errOut strings.Builder
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"passphrase", "set"})
	cmd.SetIn(strings.NewReader("mypass\n"))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("passphrase set: %v", err)
	}
	if stored != "mypass" {
		t.Fatalf("stored = %q, want mypass", stored)
	}
}
