package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLocalEnvOverlay(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("FOO=a\nBAR=b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env.local"), []byte("FOO=override\nBAZ=c\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	env, found, err := LoadLocalEnv(dir)
	if err != nil || !found {
		t.Fatalf("LoadLocalEnv found=%v err=%v", found, err)
	}
	if env["FOO"] != "override" {
		t.Errorf("FOO = %q, want override (.env.local wins)", env["FOO"])
	}
	if env["BAR"] != "b" {
		t.Errorf("BAR = %q, want b", env["BAR"])
	}
	if env["BAZ"] != "c" {
		t.Errorf("BAZ = %q, want c (.env.local only)", env["BAZ"])
	}
}

func TestLoadLocalEnvNoLocal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("FOO=a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env, found, err := LoadLocalEnv(dir)
	if err != nil || !found || env["FOO"] != "a" {
		t.Fatalf("LoadLocalEnv = %#v, found=%v, err=%v", env, found, err)
	}
}

func TestLoadLocalEnvMissing(t *testing.T) {
	env, found, err := LoadLocalEnv(t.TempDir())
	if err != nil || found || len(env) != 0 {
		t.Fatalf("LoadLocalEnv missing = %#v, found=%v, err=%v", env, found, err)
	}
}
