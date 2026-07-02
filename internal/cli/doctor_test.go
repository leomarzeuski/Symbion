package cli

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".symbion.yaml"),
		"project: demo\nenvs:\n  - key: API_KEY\n    required: true\n")
	writeFile(t, filepath.Join(dir, ".env"), "")
	writeFile(t, filepath.Join(dir, ".env.example"), "API_KEY=\n")

	out, _, err := runCommand(t, dir, "doctor", "--json")

	// API_KEY is required but missing from .env, so doctor exits 1 — but the
	// JSON is still written to stdout first.
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("expected exit 1, got %v", err)
	}

	var payload struct {
		Project      string   `json:"project"`
		MissingInEnv []string `json:"missing_in_env"`
		IssueCount   int      `json:"issue_count"`
		OK           bool     `json:"ok"`
	}
	if e := json.Unmarshal([]byte(out), &payload); e != nil {
		t.Fatalf("invalid JSON: %v\n%s", e, out)
	}
	if payload.Project != "demo" {
		t.Fatalf("project = %q, want demo", payload.Project)
	}
	if len(payload.MissingInEnv) != 1 || payload.MissingInEnv[0] != "API_KEY" {
		t.Fatalf("missing_in_env = %v, want [API_KEY]", payload.MissingInEnv)
	}
	if payload.IssueCount != 1 || payload.OK {
		t.Fatalf("issue_count=%d ok=%v, want 1/false", payload.IssueCount, payload.OK)
	}
	// empty collections serialize as [] (not null)
	if !strings.Contains(out, `"invalid_values": []`) {
		t.Fatalf("expected empty invalid_values as []:\n%s", out)
	}
}
