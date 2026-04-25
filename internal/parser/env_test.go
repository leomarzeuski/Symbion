package parser

import (
	"path/filepath"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "valid", ".env")

	values, found, err := LoadEnvFile(path)
	if err != nil {
		t.Fatalf("LoadEnvFile() error = %v", err)
	}
	if !found {
		t.Fatal("expected .env to be found")
	}

	if values["DATABASE_URL"] == "" {
		t.Fatal("expected DATABASE_URL to be parsed")
	}
	if values["API_KEY"] != "dev-api-key" {
		t.Fatalf("API_KEY = %q, want dev-api-key", values["API_KEY"])
	}
}

func TestLoadEnvFileMissing(t *testing.T) {
	values, found, err := LoadEnvFile(filepath.Join(t.TempDir(), ".env"))
	if err != nil {
		t.Fatalf("LoadEnvFile() error = %v", err)
	}
	if found {
		t.Fatal("expected missing file to be reported as not found")
	}
	if len(values) != 0 {
		t.Fatalf("values length = %d, want 0", len(values))
	}
}
