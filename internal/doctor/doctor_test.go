package doctor

import (
	"path/filepath"
	"testing"

	"github.com/leonardomarzeuski/symbion/internal/schema"
)

func TestInspectProjectValid(t *testing.T) {
	report := inspectFixture(t, "valid")

	if report.HasIssues() {
		t.Fatalf("expected no issues, got %#v", report)
	}
	if report.TrackedVariables != 3 {
		t.Fatalf("TrackedVariables = %d, want 3", report.TrackedVariables)
	}
}

func TestInspectProjectMissingEnv(t *testing.T) {
	report := inspectFixture(t, "missing-env")

	assertContains(t, report.MissingInEnv, "API_KEY")
	if report.IssueCount() != 1 {
		t.Fatalf("IssueCount = %d, want 1", report.IssueCount())
	}
}

func TestInspectProjectMissingEnvExample(t *testing.T) {
	report := inspectFixture(t, "missing-example")

	assertContains(t, report.MissingInEnvExample, "API_KEY")
	if report.IssueCount() != 1 {
		t.Fatalf("IssueCount = %d, want 1", report.IssueCount())
	}
}

func TestInspectProjectExtraEnv(t *testing.T) {
	report := inspectFixture(t, "extra-env")

	assertContains(t, report.ExtraInEnv, "UNUSED_FLAG")
	if report.IssueCount() != 1 {
		t.Fatalf("IssueCount = %d, want 1", report.IssueCount())
	}
}

func TestInspectProjectComposeMissing(t *testing.T) {
	report := inspectFixture(t, "compose-missing")

	assertContains(t, report.MissingForCompose, "REDIS_URL")
	if report.IssueCount() != 1 {
		t.Fatalf("IssueCount = %d, want 1", report.IssueCount())
	}
}

func TestInspectProjectDeprecatedReplacement(t *testing.T) {
	report := inspectFixture(t, "deprecated")

	if len(report.DeprecatedInEnv) != 1 {
		t.Fatalf("DeprecatedInEnv length = %d, want 1", len(report.DeprecatedInEnv))
	}
	if report.DeprecatedInEnv[0].Key != "OLD_API_KEY" {
		t.Fatalf("deprecated key = %q, want OLD_API_KEY", report.DeprecatedInEnv[0].Key)
	}
	if report.DeprecatedInEnv[0].Replacement != "API_KEY" {
		t.Fatalf("replacement = %q, want API_KEY", report.DeprecatedInEnv[0].Replacement)
	}
	if report.IssueCount() != 1 {
		t.Fatalf("IssueCount = %d, want 1", report.IssueCount())
	}
}

func inspectFixture(t *testing.T, name string) Report {
	t.Helper()

	root := filepath.Join("..", "..", "testdata", name)
	report, err := InspectProject(root)
	if err != nil {
		t.Fatalf("InspectProject(%q) error = %v", name, err)
	}

	return report
}

func assertContains(t *testing.T, values []string, want string) {
	t.Helper()

	for _, value := range values {
		if value == want {
			return
		}
	}

	t.Fatalf("%#v does not contain %q", values, want)
}

func TestAnalyzeInvalidValues(t *testing.T) {
	report := Analyze(Inputs{
		Schema: schema.Schema{
			Project: "p",
			Envs: []schema.EnvSpec{
				{Key: "PORT", Type: "port"},
				{Key: "ENVIRONMENT", Enum: []string{"dev", "prod"}},
				{Key: "DATABASE_URL", Type: "url"},
			},
		},
		Env: map[string]string{
			"PORT":         "70000",
			"ENVIRONMENT":  "qa",
			"DATABASE_URL": "postgres://localhost:5432/db",
		},
		EnvExample:      map[string]string{"PORT": "", "ENVIRONMENT": "", "DATABASE_URL": ""},
		SchemaFileFound: true,
		EnvFileFound:    true,
	})

	if len(report.InvalidValues) != 2 {
		t.Fatalf("InvalidValues = %#v, want 2 (PORT, ENVIRONMENT)", report.InvalidValues)
	}
	if report.InvalidValues[0].Key != "ENVIRONMENT" || report.InvalidValues[1].Key != "PORT" {
		t.Fatalf("keys/order = %#v, want ENVIRONMENT then PORT", report.InvalidValues)
	}
	if report.IssueCount() != 2 {
		t.Fatalf("IssueCount = %d, want 2", report.IssueCount())
	}
}
