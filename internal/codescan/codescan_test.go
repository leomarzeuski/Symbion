package codescan

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanFindsEnvKeys(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "app.js"), "const a = process.env.API_URL; const b = process.env['SECRET_KEY'];\n")
	write(t, filepath.Join(root, "main.go"), "os.Getenv(\"DATABASE_URL\")\nos.LookupEnv(\"REDIS_URL\")\n")
	write(t, filepath.Join(root, "svc.py"), "os.environ['CACHE_TTL']\nos.getenv('QUEUE_NAME')\n")
	write(t, filepath.Join(root, "vite.ts"), "import.meta.env.VITE_TOKEN\n")
	// should be ignored: vendored dir + non-code file
	write(t, filepath.Join(root, "node_modules", "dep.js"), "process.env.IGNORED_DEP\n")
	write(t, filepath.Join(root, "README.md"), "process.env.NOT_CODE\n")

	keys, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan error = %v", err)
	}

	want := []string{"API_URL", "CACHE_TTL", "DATABASE_URL", "QUEUE_NAME", "REDIS_URL", "SECRET_KEY", "VITE_TOKEN"}
	if len(keys) != len(want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys = %v, want %v", keys, want)
		}
	}
	for _, k := range keys {
		if k == "IGNORED_DEP" || k == "NOT_CODE" {
			t.Fatalf("should not include %q", k)
		}
	}
}
