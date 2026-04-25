package output

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/leonardomarzeuski/symbion/internal/doctor"
)

type theme struct {
	enabled bool
}

func PrintDoctorReport(w io.Writer, report doctor.Report) {
	p := printer{
		w: w,
		theme: theme{
			enabled: colorEnabled(),
		},
	}

	p.banner()
	p.printf("%s\n", p.accent("$ symbion doctor", cyan))
	p.printf("\n%s\n", p.accent("Symbion Doctor Report", cyan))
	p.printf("%s\n", p.accent("---------------------", cyan))
	p.printf("%s %s\n\n", p.accent("Project:", purple), report.Project)

	p.section("Schema Status")
	p.statusLine(report.SchemaFileFound, ".symbion.yaml loaded")
	p.statusLine(true, fmt.Sprintf("%d tracked variables", report.TrackedVariables))
	p.printf("\n")

	p.section("Files")
	p.statusLine(report.EnvFileFound, ".env loaded")
	p.statusLine(report.EnvExampleFound, ".env.example loaded")
	p.statusLine(len(report.ComposeFiles) > 0, composeFilesLabel(report.ComposeFiles))
	p.printf("\n")

	p.section("Checks")
	p.checkList("Missing in .env", report.MissingInEnv)
	p.checkList("Missing in .env.example", report.MissingInEnvExample)
	p.deprecatedList(report.DeprecatedInEnv)
	p.checkList("Missing for docker-compose", report.MissingForCompose)
	p.checkList("Extra in .env", report.ExtraInEnv)
	p.checkList("Extra in .env.example", report.ExtraInEnvExample)
	p.printf("\n")

	p.section("Summary")
	if report.HasIssues() {
		p.printf("  %s\n", p.warn(pluralize(report.IssueCount(), "issue")+" found"))
		p.printf("  Review: %s, %s and %s\n", p.accent(".env", cyan), p.accent(".env.example", cyan), p.accent(schemaFilename, cyan))
	} else {
		p.printf("  %s\n", p.ok("All checks passed"))
		p.printf("  Your local environment contract is in sync.\n")
	}

	p.printf("\n")
	p.footer()
}

const schemaFilename = ".symbion.yaml"

const banner = `
  _____ __  ____  ______  ____  ____  _   __
 / ___// / / /  |/  / __ )/  _/ __ \/ | / /
 \__ \/ / / / /|_/ / __  |/ // / / /  |/ /
___/ / /_/ / /  / / /_/ // // /_/ / /|  /
/____/\__, /_/  /_/_____/___/\____/_/ |_/
     /____/
Local Environment Intelligence
`

type printer struct {
	w     io.Writer
	theme theme
}

const (
	reset  = "\033[0m"
	cyan   = "\033[36m"
	green  = "\033[32m"
	purple = "\033[35m"
	yellow = "\033[33m"
)

func (p printer) banner() {
	p.printf("%s\n", p.accent(strings.TrimRight(banner, "\n"), cyan))
}

func (p printer) section(title string) {
	p.printf("%s\n", p.accent(title, green))
}

func (p printer) statusLine(ok bool, text string) {
	if ok {
		p.printf("  %s %s\n", p.ok("[OK]"), text)
		return
	}
	p.printf("  %s %s\n", p.warn("[!]"), text)
}

func (p printer) checkList(label string, items []string) {
	if len(items) == 0 {
		p.printf("  %s %s: none\n", p.ok("[OK]"), label)
		return
	}

	if len(items) == 1 {
		p.printf("  %s %s: %s\n", p.warn("[!]"), label, p.warn(items[0]))
		return
	}

	p.printf("  %s %s:\n", p.warn("[!]"), label)
	for _, item := range items {
		p.printf("      - %s\n", p.warn(item))
	}
}

func (p printer) deprecatedList(items []doctor.DeprecatedUsage) {
	if len(items) == 0 {
		p.printf("  %s Deprecated variables in .env: none\n", p.ok("[OK]"))
		return
	}

	if len(items) == 1 {
		p.printf("  %s Deprecated variables in .env: %s\n", p.warn("[!]"), p.deprecated(items[0]))
		return
	}

	p.printf("  %s Deprecated variables in .env:\n", p.warn("[!]"))
	for _, item := range items {
		p.printf("      - %s\n", p.deprecated(item))
	}
}

func (p printer) deprecated(item doctor.DeprecatedUsage) string {
	if item.Replacement == "" {
		return p.warn(item.Key)
	}

	return fmt.Sprintf("%s -> use %s", p.warn(item.Key), p.ok(item.Replacement))
}

func (p printer) footer() {
	p.printf("%s %s  %s  %s\n",
		p.accent("Common commands:", purple),
		p.accent("symbion init", green),
		p.accent("symbion scan", cyan),
		p.accent("symbion doctor", purple),
	)
}

func (p printer) ok(value string) string {
	return p.accent(value, green)
}

func (p printer) warn(value string) string {
	return p.accent(value, yellow)
}

func (p printer) accent(value string, color string) string {
	if !p.theme.enabled {
		return value
	}
	return color + value + reset
}

func (p printer) printf(format string, args ...any) {
	fmt.Fprintf(p.w, format, args...)
}

func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	term := os.Getenv("TERM")
	return term != "" && term != "dumb"
}

func composeFilesLabel(files []string) string {
	if len(files) == 0 {
		return "docker-compose/compose file not found"
	}

	names := make([]string, 0, len(files))
	for _, file := range files {
		names = append(names, filepath.Base(file))
	}

	return "compose files loaded: " + strings.Join(names, ", ")
}

func pluralize(count int, singular string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %ss", count, singular)
}
