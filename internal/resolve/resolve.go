// Package resolve merges layered environment sources (schema defaults, the
// ambient shell, and a profile or .env) into a single ordered set of
// variables, marking which ones are secret. It performs no I/O.
package resolve

import (
	"sort"
	"strings"
)

type Source string

const (
	SourceShell   Source = "shell"
	SourceProfile Source = "profile"
	SourceEnvFile Source = ".env"
	SourceDefault Source = "default"
	SourceSafeMin Source = "safe-min"
)

// Var is one fully-resolved environment variable.
type Var struct {
	Key    string
	Value  string
	Secret bool
	Source Source
}

// Options controls how Resolve merges its layers.
type Options struct {
	InheritShell bool              // false when --isolated
	Override     bool              // true = source (profile/.env) wins over shell
	SourceName   Source            // SourceProfile or SourceEnvFile, for attribution
	Defaults     map[string]string // schema documented non-empty defaults
	SecretKeys   map[string]bool   // schema keys with secret: true
}

// ToEnviron converts resolved Vars into a KEY=VALUE slice for exec.Cmd.Env.
func ToEnviron(vars []Var) []string {
	out := make([]string, 0, len(vars))
	for _, v := range vars {
		out = append(out, v.Key+"="+v.Value)
	}
	return out
}

var safeMinKeys = []string{"PATH", "HOME", "LANG", "TERM", "TMPDIR", "USER", "SHELL", "PWD"}

// safeMinimum returns the OS essentials a child needs to run, pulled from the
// real shell: a fixed allowlist plus every LC_* locale variable.
func safeMinimum(shell map[string]string) map[string]string {
	out := make(map[string]string)
	for _, k := range safeMinKeys {
		if v, ok := shell[k]; ok {
			out[k] = v
		}
	}
	for k, v := range shell {
		if strings.HasPrefix(k, "LC_") {
			out[k] = v
		}
	}
	return out
}

var sensitiveMarkers = []string{
	"SECRET", "TOKEN", "PASSWORD", "PASSWD", "PASS", "API_KEY", "APIKEY",
	"PRIVATE", "CREDENTIAL", "ACCESS_KEY", "SESSION", "AUTH",
}

// looksSensitive reports whether a variable name suggests a secret value.
func looksSensitive(key string) bool {
	upper := strings.ToUpper(key)
	for _, marker := range sensitiveMarkers {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

// parseEnviron turns a KEY=VALUE slice (os.Environ form) into a map, splitting
// on the first '=' and skipping entries without one.
func parseEnviron(environ []string) map[string]string {
	m := make(map[string]string, len(environ))
	for _, kv := range environ {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			m[kv[:i]] = kv[i+1:]
		}
	}
	return m
}

// layer is one contributor to the merged environment. Later layers win.
type layer struct {
	name   Source
	values map[string]string
}

// Resolve merges the layers implied by opts and returns the result sorted by
// Key. Precedence, lowest to highest:
//
//	default:       [defaults, shell, source]        (source/profile wins)
//	--no-override: [defaults, source, shell]         (shell wins)
//	--isolated:    [safeMinimum(shell), defaults, source]
func Resolve(shell []string, source map[string]string, opts Options) []Var {
	layers := buildLayers(shell, source, opts)

	type entry struct {
		value  string
		source Source
	}
	merged := make(map[string]entry)
	for _, l := range layers {
		for k, v := range l.values {
			merged[k] = entry{value: v, source: l.name}
		}
	}

	vars := make([]Var, 0, len(merged))
	for k, e := range merged {
		vars = append(vars, Var{
			Key:    k,
			Value:  e.value,
			Source: e.source,
			Secret: opts.SecretKeys[k] || looksSensitive(k),
		})
	}
	sort.Slice(vars, func(i, j int) bool { return vars[i].Key < vars[j].Key })
	return vars
}

func buildLayers(shell []string, source map[string]string, opts Options) []layer {
	shellMap := parseEnviron(shell)

	sourceName := opts.SourceName
	if sourceName == "" {
		sourceName = SourceEnvFile
	}
	defaults := layer{name: SourceDefault, values: opts.Defaults}
	src := layer{name: sourceName, values: source}

	if !opts.InheritShell {
		return []layer{
			{name: SourceSafeMin, values: safeMinimum(shellMap)},
			defaults,
			src,
		}
	}

	shellLayer := layer{name: SourceShell, values: shellMap}
	if opts.Override {
		return []layer{defaults, shellLayer, src}
	}
	return []layer{defaults, src, shellLayer}
}
