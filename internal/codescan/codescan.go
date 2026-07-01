// Package codescan walks a project's source files and extracts the environment
// variable names referenced in code (process.env.X, os.Getenv("X"), ...). It is
// best-effort and pattern-based: it favors precision (quoted/qualified forms)
// over catching every possible reference.
package codescan

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

var patterns = []*regexp.Regexp{
	regexp.MustCompile(`process\.env\.([A-Za-z_][A-Za-z0-9_]*)`),                // JS/TS: process.env.FOO
	regexp.MustCompile(`process\.env\[['"]([A-Za-z_][A-Za-z0-9_]*)['"]\]`),      // JS/TS: process.env['FOO']
	regexp.MustCompile(`import\.meta\.env\.([A-Za-z_][A-Za-z0-9_]*)`),           // Vite: import.meta.env.FOO
	regexp.MustCompile(`os\.Getenv\(\s*"([A-Za-z_][A-Za-z0-9_]*)"`),             // Go: os.Getenv("FOO")
	regexp.MustCompile(`os\.LookupEnv\(\s*"([A-Za-z_][A-Za-z0-9_]*)"`),          // Go: os.LookupEnv("FOO")
	regexp.MustCompile(`os\.environ\[['"]([A-Za-z_][A-Za-z0-9_]*)['"]\]`),       // Py: os.environ['FOO']
	regexp.MustCompile(`os\.environ\.get\(\s*['"]([A-Za-z_][A-Za-z0-9_]*)['"]`), // Py: os.environ.get('FOO')
	regexp.MustCompile(`os\.getenv\(\s*['"]([A-Za-z_][A-Za-z0-9_]*)['"]`),       // Py: os.getenv('FOO')
	regexp.MustCompile(`\bENV\[['"]([A-Za-z_][A-Za-z0-9_]*)['"]\]`),             // Ruby: ENV['FOO']
	regexp.MustCompile(`\bENV\.fetch\(\s*['"]([A-Za-z_][A-Za-z0-9_]*)['"]`),     // Ruby: ENV.fetch('FOO')
}

var codeExtensions = map[string]bool{
	".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".mjs": true, ".cjs": true,
	".go": true, ".py": true, ".rb": true,
}

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true,
	"target": true, ".venv": true, "venv": true, "__pycache__": true, ".next": true,
	".idea": true, ".symbion": true,
}

const maxFileSize = 1 << 20 // 1 MiB

// Scan walks root and returns the sorted, de-duplicated environment variable
// names referenced in supported source files.
func Scan(root string) ([]string, error) {
	found := map[string]struct{}{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !codeExtensions[filepath.Ext(d.Name())] {
			return nil
		}
		if info, ierr := d.Info(); ierr != nil || info.Size() > maxFileSize {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		for _, re := range patterns {
			for _, m := range re.FindAllSubmatch(data, -1) {
				if len(m) > 1 {
					found[string(m[1])] = struct{}{}
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(found))
	for k := range found {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}
