package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/leonardomarzeuski/symbion/internal/parser"
	"github.com/leonardomarzeuski/symbion/internal/schema"
)

type Inputs struct {
	Schema           schema.Schema
	Env              map[string]string
	EnvExample       map[string]string
	ComposeVariables []string
	ComposeFiles     []string
	EnvFileFound     bool
	EnvExampleFound  bool
	SchemaFileFound  bool
}

type DeprecatedUsage struct {
	Key         string
	Replacement string
}

type Report struct {
	Project             string
	TrackedVariables    int
	EnvFileFound        bool
	EnvExampleFound     bool
	SchemaFileFound     bool
	ComposeFiles        []string
	MissingInEnv        []string
	MissingInEnvExample []string
	MissingForCompose   []string
	ExtraInEnv          []string
	ExtraInEnvExample   []string
	DeprecatedInEnv     []DeprecatedUsage
}

func InspectProject(root string) (Report, error) {
	schemaPath := filepath.Join(root, schema.DefaultFilename)
	loadedSchema, err := schema.Load(schemaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Report{}, fmt.Errorf("%s not found; run symbion init or symbion scan first", schema.DefaultFilename)
		}
		return Report{}, err
	}

	env, envFound, err := parser.LoadEnvFile(filepath.Join(root, ".env"))
	if err != nil {
		return Report{}, err
	}

	envExample, envExampleFound, err := parser.LoadEnvFile(filepath.Join(root, ".env.example"))
	if err != nil {
		return Report{}, err
	}

	compose, err := parser.LoadComposeReferences(defaultComposePaths(root))
	if err != nil {
		return Report{}, err
	}

	return Analyze(Inputs{
		Schema:           *loadedSchema,
		Env:              env,
		EnvExample:       envExample,
		ComposeVariables: compose.Variables,
		ComposeFiles:     compose.Files,
		EnvFileFound:     envFound,
		EnvExampleFound:  envExampleFound,
		SchemaFileFound:  true,
	}), nil
}

func Analyze(input Inputs) Report {
	report := Report{
		Project:          input.Schema.Project,
		TrackedVariables: len(input.Schema.Envs),
		EnvFileFound:     input.EnvFileFound,
		EnvExampleFound:  input.EnvExampleFound,
		SchemaFileFound:  input.SchemaFileFound,
		ComposeFiles:     append([]string{}, input.ComposeFiles...),
	}

	specsByKey := input.Schema.SpecByKey()
	for _, spec := range input.Schema.Envs {
		if spec.Deprecated {
			if _, ok := input.Env[spec.Key]; ok {
				report.DeprecatedInEnv = append(report.DeprecatedInEnv, DeprecatedUsage{
					Key:         spec.Key,
					Replacement: spec.Replacement,
				})
			}
			continue
		}

		if spec.Required {
			if _, ok := input.Env[spec.Key]; !ok {
				report.MissingInEnv = append(report.MissingInEnv, spec.Key)
			}
		}

		if _, ok := input.EnvExample[spec.Key]; !ok {
			report.MissingInEnvExample = append(report.MissingInEnvExample, spec.Key)
		}
	}

	for _, variable := range input.ComposeVariables {
		if _, ok := input.Env[variable]; !ok {
			report.MissingForCompose = append(report.MissingForCompose, variable)
		}
	}

	for key := range input.Env {
		if _, ok := specsByKey[key]; !ok {
			report.ExtraInEnv = append(report.ExtraInEnv, key)
		}
	}

	for key := range input.EnvExample {
		if _, ok := specsByKey[key]; !ok {
			report.ExtraInEnvExample = append(report.ExtraInEnvExample, key)
		}
	}

	sort.Strings(report.ComposeFiles)
	sort.Strings(report.MissingInEnv)
	sort.Strings(report.MissingInEnvExample)
	sort.Strings(report.MissingForCompose)
	sort.Strings(report.ExtraInEnv)
	sort.Strings(report.ExtraInEnvExample)
	sortDeprecated(report.DeprecatedInEnv)

	return report
}

func (r Report) IssueCount() int {
	return len(r.MissingInEnv) +
		len(r.MissingInEnvExample) +
		len(r.MissingForCompose) +
		len(r.ExtraInEnv) +
		len(r.ExtraInEnvExample) +
		len(r.DeprecatedInEnv)
}

func (r Report) HasIssues() bool {
	return r.IssueCount() > 0
}

func defaultComposePaths(root string) []string {
	return []string{
		filepath.Join(root, "docker-compose.yml"),
		filepath.Join(root, "docker-compose.yaml"),
		filepath.Join(root, "compose.yml"),
		filepath.Join(root, "compose.yaml"),
	}
}

func sortDeprecated(values []DeprecatedUsage) {
	sort.Slice(values, func(i, j int) bool {
		return values[i].Key < values[j].Key
	})
}
