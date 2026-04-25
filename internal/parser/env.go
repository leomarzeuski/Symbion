package parser

import (
	"fmt"
	"os"
	"sort"

	"github.com/joho/godotenv"
)

func LoadEnvFile(path string) (map[string]string, bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, false, nil
		}
		return nil, false, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, true, fmt.Errorf("parse %s: %w", path, err)
	}
	values, err := ParseEnv(data)
	if err != nil {
		return nil, true, fmt.Errorf("parse %s: %w", path, err)
	}

	return values, true, nil
}

func ParseEnv(data []byte) (map[string]string, error) {
	return godotenv.Unmarshal(string(data))
}

func SortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
