package parser

import (
	"fmt"
	"os"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

var composeVariablePattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?:[^}]*)\}`)

type ComposeReferences struct {
	Files     []string
	Variables []string
}

func LoadComposeReferences(paths []string) (ComposeReferences, error) {
	seenFiles := make(map[string]struct{})
	seenVars := make(map[string]struct{})

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return ComposeReferences{}, err
		}

		var node yaml.Node
		if err := yaml.Unmarshal(data, &node); err != nil {
			return ComposeReferences{}, fmt.Errorf("parse compose file %s: %w", path, err)
		}

		seenFiles[path] = struct{}{}
		for _, variable := range ExtractComposeVariables(data) {
			seenVars[variable] = struct{}{}
		}
	}

	files := make([]string, 0, len(seenFiles))
	for file := range seenFiles {
		files = append(files, file)
	}
	sort.Strings(files)

	variables := make([]string, 0, len(seenVars))
	for variable := range seenVars {
		variables = append(variables, variable)
	}
	sort.Strings(variables)

	return ComposeReferences{
		Files:     files,
		Variables: variables,
	}, nil
}

func ExtractComposeVariables(data []byte) []string {
	matches := composeVariablePattern.FindAllSubmatch(data, -1)
	seen := make(map[string]struct{}, len(matches))

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		seen[string(match[1])] = struct{}{}
	}

	variables := make([]string, 0, len(seen))
	for variable := range seen {
		variables = append(variables, variable)
	}
	sort.Strings(variables)

	return variables
}
