package schema

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadSchema(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "valid", DefaultFilename)

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.Project != "billing-api" {
		t.Fatalf("Project = %q, want billing-api", loaded.Project)
	}

	want := []string{"API_KEY", "DATABASE_URL", "REDIS_URL"}
	if !reflect.DeepEqual(loaded.Keys(), want) {
		t.Fatalf("Keys() = %#v, want %#v", loaded.Keys(), want)
	}
}

func TestAddKeysPreservesExistingMetadata(t *testing.T) {
	s := &Schema{
		Project: "billing-api",
		Envs: []EnvSpec{
			{
				Key:         "API_KEY",
				Description: "Existing metadata",
				Required:    true,
				Secret:      true,
			},
		},
	}

	added := s.AddKeys([]string{"API_KEY", "DATABASE_URL"})
	if !reflect.DeepEqual(added, []string{"DATABASE_URL"}) {
		t.Fatalf("added = %#v, want DATABASE_URL", added)
	}

	if s.Envs[0].Description != "Existing metadata" {
		t.Fatalf("metadata was not preserved")
	}
	if len(s.Envs) != 2 {
		t.Fatalf("len(Envs) = %d, want 2", len(s.Envs))
	}
	if !s.Envs[1].Required {
		t.Fatalf("new env should be required by default")
	}
}
