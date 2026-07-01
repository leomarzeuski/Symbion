package trust

import (
	"os"
	"path/filepath"
	"testing"
)

func writeEnv(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
}

func TestAllowIsTrustedDenyLifecycle(t *testing.T) {
	store := Store{Root: t.TempDir()}
	project := t.TempDir()
	writeEnv(t, project, "FOO=bar\n")

	if err := store.Allow(project); err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if ok, err := store.IsTrusted(project); err != nil || !ok {
		t.Fatalf("IsTrusted after Allow = %v, %v; want true", ok, err)
	}
	writeEnv(t, project, "FOO=changed\n") // editing .env re-blocks
	if ok, _ := store.IsTrusted(project); ok {
		t.Fatal("IsTrusted after edit = true; want false")
	}
	if err := store.Allow(project); err != nil {
		t.Fatalf("re-Allow: %v", err)
	}
	if err := store.Deny(project); err != nil {
		t.Fatalf("Deny: %v", err)
	}
	if ok, _ := store.IsTrusted(project); ok {
		t.Fatal("IsTrusted after Deny = true; want false")
	}
}

func TestAllowWithoutEnvErrors(t *testing.T) {
	store := Store{Root: t.TempDir()}
	if err := store.Allow(t.TempDir()); err == nil {
		t.Fatal("Allow without .env should error")
	}
}

func TestIsTrustedUnknownDir(t *testing.T) {
	store := Store{Root: t.TempDir()}
	if ok, err := store.IsTrusted(t.TempDir()); err != nil || ok {
		t.Fatalf("IsTrusted unknown = %v, %v; want false, nil", ok, err)
	}
}
