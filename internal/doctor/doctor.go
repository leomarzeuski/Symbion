package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/leonardomarzeuski/symbion/internal/parser"
	"github.com/leonardomarzeuski/symbion/internal/schema"
	"github.com/leonardomarzeuski/symbion/internal/validate"
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
	IgnoreEnv        bool
}

type DeprecatedUsage struct {
	Key         string `json:"key"`
	Replacement string `json:"replacement"`
}

type ValueViolation struct {
	Key    string `json:"key"`
	Reason string `json:"reason"`
}

type Report struct {
	Project             string            `json:"project"`
	TrackedVariables    int               `json:"tracked_variables"`
	EnvFileFound        bool              `json:"env_file_found"`
	EnvExampleFound     bool              `json:"env_example_found"`
	SchemaFileFound     bool              `json:"schema_file_found"`
	ComposeFiles        []string          `json:"compose_files"`
	MissingInEnv        []string          `json:"missing_in_env"`
	MissingInEnvExample []string          `json:"missing_in_env_example"`
	MissingForCompose   []string          `json:"missing_for_compose"`
	ExtraInEnv          []string          `json:"extra_in_env"`
	ExtraInEnvExample   []string          `json:"extra_in_env_example"`
	DeprecatedInEnv     []DeprecatedUsage `json:"deprecated_in_env"`
	InvalidValues       []ValueViolation  `json:"invalid_values"`
}

func InspectProject(root string) (Report, error) {
	return inspect(root, false)
}

// InspectProjectSchemaOnly validates schema/.env.example drift while ignoring
// .env (useful in CI, where .env is absent).
func InspectProjectSchemaOnly(root string) (Report, error) {
	return inspect(root, true)
}

func inspect(root string, ignoreEnv bool) (Report, error) {
	schemaPath := filepath.Join(root, schema.DefaultFilename)
	loadedSchema, err := schema.Load(schemaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Report{}, fmt.Errorf("%s not found; run symbion init or symbion scan first", schema.DefaultFilename)
		}
		return Report{}, err
	}

	env, envFound, err := parser.LoadLocalEnv(root)
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
		IgnoreEnv:        ignoreEnv,
	}), nil
}

func Analyze(input Inputs) Report {
	report := Report{
		Project:             input.Schema.Project,
		TrackedVariables:    len(input.Schema.Envs),
		EnvFileFound:        input.EnvFileFound,
		EnvExampleFound:     input.EnvExampleFound,
		SchemaFileFound:     input.SchemaFileFound,
		ComposeFiles:        append([]string{}, input.ComposeFiles...),
		MissingInEnv:        []string{},
		MissingInEnvExample: []string{},
		MissingForCompose:   []string{},
		ExtraInEnv:          []string{},
		ExtraInEnvExample:   []string{},
		DeprecatedInEnv:     []DeprecatedUsage{},
		InvalidValues:       []ValueViolation{},
	}

	specsByKey := input.Schema.SpecByKey()
	for _, spec := range input.Schema.Envs {
		if spec.Deprecated {
			if !input.IgnoreEnv {
				if _, ok := input.Env[spec.Key]; ok {
					report.DeprecatedInEnv = append(report.DeprecatedInEnv, DeprecatedUsage{
						Key:         spec.Key,
						Replacement: spec.Replacement,
					})
				}
			}
			continue
		}

		if !input.IgnoreEnv && spec.Required {
			if _, ok := input.Env[spec.Key]; !ok {
				report.MissingInEnv = append(report.MissingInEnv, spec.Key)
			}
		}

		if _, ok := input.EnvExample[spec.Key]; !ok {
			report.MissingInEnvExample = append(report.MissingInEnvExample, spec.Key)
		}
	}

	if !input.IgnoreEnv {
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
	}

	for key := range input.EnvExample {
		if _, ok := specsByKey[key]; !ok {
			report.ExtraInEnvExample = append(report.ExtraInEnvExample, key)
		}
	}

	if !input.IgnoreEnv {
		for _, spec := range input.Schema.Envs {
			if spec.Deprecated {
				continue
			}
			value, ok := input.Env[spec.Key]
			if !ok {
				continue
			}
			if spec.Type == "" && len(spec.Enum) == 0 && spec.Pattern == "" {
				continue
			}
			if reason, valid := validate.Value(spec, value); !valid {
				report.InvalidValues = append(report.InvalidValues, ValueViolation{Key: spec.Key, Reason: reason})
			}
		}
	}

	sort.Strings(report.ComposeFiles)
	sort.Strings(report.MissingInEnv)
	sort.Strings(report.MissingInEnvExample)
	sort.Strings(report.MissingForCompose)
	sort.Strings(report.ExtraInEnv)
	sort.Strings(report.ExtraInEnvExample)
	sortDeprecated(report.DeprecatedInEnv)
	sort.Slice(report.InvalidValues, func(i, j int) bool {
		return report.InvalidValues[i].Key < report.InvalidValues[j].Key
	})

	return report
}

func (r Report) IssueCount() int {
	return len(r.MissingInEnv) +
		len(r.MissingInEnvExample) +
		len(r.MissingForCompose) +
		len(r.ExtraInEnv) +
		len(r.ExtraInEnvExample) +
		len(r.DeprecatedInEnv) +
		len(r.InvalidValues)
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
