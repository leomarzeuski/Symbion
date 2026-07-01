// Package validate checks environment values against a schema's type, enum,
// and pattern rules. Reasons never include the value itself, so validation
// stays safe to print for secret variables.
package validate

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/leonardomarzeuski/symbion/internal/schema"
)

// Value checks value against the spec's Type/Enum/Pattern rules. It returns a
// human-readable reason and false when the value is invalid. The reason never
// contains the value.
func Value(spec schema.EnvSpec, value string) (string, bool) {
	if reason, ok := checkType(spec.Type, value); !ok {
		return reason, false
	}
	if len(spec.Enum) > 0 && !contains(spec.Enum, value) {
		return fmt.Sprintf("must be one of %v", spec.Enum), false
	}
	if spec.Pattern != "" {
		re, err := regexp.Compile(spec.Pattern)
		if err != nil {
			return fmt.Sprintf("schema pattern is invalid: %v", err), false
		}
		if !re.MatchString(value) {
			return fmt.Sprintf("must match pattern %s", spec.Pattern), false
		}
	}
	return "", true
}

func checkType(typ, value string) (string, bool) {
	switch typ {
	case "", "string":
	case "int":
		if _, err := strconv.Atoi(value); err != nil {
			return "must be an integer", false
		}
	case "bool":
		if _, err := strconv.ParseBool(value); err != nil {
			return "must be a boolean", false
		}
	case "port":
		if n, err := strconv.Atoi(value); err != nil || n < 1 || n > 65535 {
			return "must be a port (1-65535)", false
		}
	case "url":
		if u, err := url.ParseRequestURI(value); err != nil || u.Scheme == "" || u.Host == "" {
			return "must be a URL", false
		}
	case "duration":
		if _, err := time.ParseDuration(value); err != nil {
			return "must be a duration (e.g. 30s, 5m)", false
		}
	}
	return "", true
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
