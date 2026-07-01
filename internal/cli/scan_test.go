package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/leonardomarzeuski/symbion/internal/schema"
)

func TestScanCodeAddsDiscoveredKeys(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".symbion.yaml"), "project: demo\nenvs: []\n")
	writeFile(t, filepath.Join(dir, "app.js"), "const x = process.env.SERVICE_URL;\n")

	out, errOut, err := runCommand(t, dir, "scan", "--code")
	if err != nil {
		t.Fatalf("scan --code error = %v, stderr = %s", err, errOut)
	}
	if !strings.Contains(out, "SERVICE_URL") {
		t.Fatalf("expected SERVICE_URL in output:\n%s", out)
	}

	s, err := schema.Load(filepath.Join(dir, ".symbion.yaml"))
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	for _, e := range s.Envs {
		if e.Key == "SERVICE_URL" {
			return
		}
	}
	t.Fatal("SERVICE_URL not added to schema")
}
