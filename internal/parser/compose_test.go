package parser

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestExtractComposeVariables(t *testing.T) {
	content := []byte(`
services:
  api:
    environment:
      DATABASE_URL: ${DATABASE_URL}
      REDIS_URL: ${REDIS_URL:-redis://localhost:6379}
      API_KEY: ${API_KEY?required}
      DUPLICATE_DATABASE_URL: ${DATABASE_URL}
`)

	got := ExtractComposeVariables(content)
	want := []string{"API_KEY", "DATABASE_URL", "REDIS_URL"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractComposeVariables() = %#v, want %#v", got, want)
	}
}

func TestLoadComposeReferences(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "valid", "docker-compose.yml")

	refs, err := LoadComposeReferences([]string{path})
	if err != nil {
		t.Fatalf("LoadComposeReferences() error = %v", err)
	}

	want := []string{"API_KEY", "DATABASE_URL", "REDIS_URL"}
	if !reflect.DeepEqual(refs.Variables, want) {
		t.Fatalf("refs.Variables = %#v, want %#v", refs.Variables, want)
	}
	if len(refs.Files) != 1 {
		t.Fatalf("refs.Files length = %d, want 1", len(refs.Files))
	}
}
