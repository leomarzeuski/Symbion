package cli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitRunArgs(t *testing.T) {
	tests := []struct {
		name        string
		dash        int
		args        []string
		wantProfile string
		wantCommand []string
		wantErr     bool
	}{
		{"nothing", -1, nil, "", nil, false},
		{"profile only", -1, []string{"staging"}, "staging", nil, false},
		{"missing dash before command", -1, []string{"npm", "start"}, "", nil, true},
		{"command only", 0, []string{"npm", "start"}, "", []string{"npm", "start"}, false},
		{"profile and command", 1, []string{"staging", "npm", "run"}, "staging", []string{"npm", "run"}, false},
		{"too many before dash", 2, []string{"a", "b", "cmd"}, "", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, command, err := splitRunArgs(tt.dash, tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if profile != tt.wantProfile {
				t.Errorf("profile = %q, want %q", profile, tt.wantProfile)
			}
			if strings.Join(command, ",") != strings.Join(tt.wantCommand, ",") {
				t.Errorf("command = %v, want %v", command, tt.wantCommand)
			}
		})
	}
}

func TestRunDryRunMasksSecrets(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFile(t, filepath.Join(dir, ".symbion.yaml"),
		"project: billing-api\nenvs:\n  - key: API_KEY\n    secret: true\n")
	writeFile(t, filepath.Join(dir, ".env"), "API_KEY=super-secret\nPORT=3000\n")

	out, errOut, err := runCommand(t, dir, "run", "--dry-run")
	if err != nil {
		t.Fatalf("run --dry-run error = %v, stderr = %s", err, errOut)
	}
	if strings.Contains(out, "super-secret") {
		t.Fatalf("dry-run leaked secret value:\n%s", out)
	}
	if !strings.Contains(out, "API_KEY=********") {
		t.Fatalf("expected masked API_KEY, got:\n%s", out)
	}
	if !strings.Contains(out, "PORT=3000") {
		t.Fatalf("expected visible PORT, got:\n%s", out)
	}
}

func TestRunStrictFailsOnMissingRequired(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFile(t, filepath.Join(dir, ".symbion.yaml"),
		"project: billing-api\nenvs:\n  - key: SYMBION_NEEDS_THIS\n    required: true\n")
	writeFile(t, filepath.Join(dir, ".env"), "PORT=3000\n")

	_, errOut, err := runCommand(t, dir, "run", "--strict", "--dry-run")
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("expected exit 1, got %v", err)
	}
	if !strings.Contains(errOut, "SYMBION_NEEDS_THIS") {
		t.Fatalf("expected missing var in stderr, got %q", errOut)
	}
}

func TestRunNoCommandErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFile(t, filepath.Join(dir, ".symbion.yaml"), "project: billing-api\nenvs: []\n")
	writeFile(t, filepath.Join(dir, ".env"), "PORT=3000\n")

	_, _, err := runCommand(t, dir, "run", "local")
	if err == nil {
		t.Fatalf("expected error when no command and no --dry-run")
	}
}

func TestRunInjectsProfileEndToEnd(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFile(t, filepath.Join(dir, ".symbion.yaml"), "project: billing-api\nenvs: []\n")
	// GO_WANT_HELPER_PROCESS travels through the profile so the re-exec'd test
	// binary acts as the child helper.
	writeFile(t, filepath.Join(dir, ".env"), "INJECTED_VAR=from-profile\nGO_WANT_HELPER_PROCESS=1\n")

	if _, errOut, err := runCommand(t, dir, "save", "local"); err != nil {
		t.Fatalf("save error = %v, stderr = %s", err, errOut)
	}
	// Overwrite .env to prove the value comes from the profile, not .env.
	writeFile(t, filepath.Join(dir, ".env"), "INJECTED_VAR=from-dotenv\nGO_WANT_HELPER_PROCESS=1\n")

	args := append([]string{"run", "local", "--"}, helperCommand("printenv", "INJECTED_VAR")...)
	out, errOut, err := runCommand(t, dir, args...)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, errOut)
	}
	if !strings.Contains(out, "from-profile") {
		t.Fatalf("expected child to see profile value, got:\n%s", out)
	}
}
