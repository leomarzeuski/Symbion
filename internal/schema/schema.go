package schema

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const DefaultFilename = ".symbion.yaml"

type Schema struct {
	Project string    `yaml:"project"`
	Envs    []EnvSpec `yaml:"envs"`
}

type EnvSpec struct {
	Key         string `yaml:"key"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
	Secret      bool   `yaml:"secret"`
	Default     string `yaml:"default"`
	Deprecated  bool   `yaml:"deprecated"`
	Replacement string `yaml:"replacement"`
}

func New(project string) *Schema {
	project = strings.TrimSpace(project)
	if project == "" {
		project = "my-project"
	}

	return &Schema{
		Project: project,
		Envs:    []EnvSpec{},
	}
}

func Load(path string) (*Schema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var s Schema
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse schema: %w", err)
	}

	if s.Envs == nil {
		s.Envs = []EnvSpec{}
	}

	if err := s.Validate(); err != nil {
		return nil, err
	}

	return &s, nil
}

func Save(path string, s *Schema) error {
	if s == nil {
		return fmt.Errorf("schema is nil")
	}
	if s.Envs == nil {
		s.Envs = []EnvSpec{}
	}
	if err := s.Validate(); err != nil {
		return err
	}

	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal schema: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}

func (s *Schema) Validate() error {
	if strings.TrimSpace(s.Project) == "" {
		return fmt.Errorf("schema project is required")
	}

	seen := make(map[string]struct{}, len(s.Envs))
	for _, env := range s.Envs {
		key := strings.TrimSpace(env.Key)
		if key == "" {
			return fmt.Errorf("schema contains an env with an empty key")
		}
		if _, ok := seen[key]; ok {
			return fmt.Errorf("schema contains duplicate env key %q", key)
		}
		seen[key] = struct{}{}
	}

	return nil
}

func (s *Schema) AddKeys(keys []string) []string {
	if s.Envs == nil {
		s.Envs = []EnvSpec{}
	}

	existing := s.SpecByKey()
	added := make([]string, 0)
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := existing[key]; ok {
			continue
		}

		s.Envs = append(s.Envs, EnvSpec{
			Key:      key,
			Required: true,
		})
		existing[key] = EnvSpec{Key: key}
		added = append(added, key)
	}

	sort.Strings(added)
	return added
}

func (s Schema) SpecByKey() map[string]EnvSpec {
	specs := make(map[string]EnvSpec, len(s.Envs))
	for _, env := range s.Envs {
		specs[env.Key] = env
	}
	return specs
}

func (s Schema) Keys() []string {
	keys := make([]string, 0, len(s.Envs))
	for _, env := range s.Envs {
		keys = append(keys, env.Key)
	}
	sort.Strings(keys)
	return keys
}
