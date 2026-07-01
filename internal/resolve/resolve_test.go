package resolve

import (
	"reflect"
	"testing"
)

func TestLooksSensitive(t *testing.T) {
	cases := map[string]bool{
		"API_KEY":      true,
		"DB_PASSWORD":  true,
		"secret_token": true, // case-insensitive
		"GITHUB_TOKEN": true,
		"AUTH_HEADER":  true,
		"SESSION_ID":   true,
		"PORT":         false,
		"DATABASE_URL": false,
		"HOSTNAME":     false,
	}
	for key, want := range cases {
		if got := looksSensitive(key); got != want {
			t.Errorf("looksSensitive(%q) = %v, want %v", key, got, want)
		}
	}
}

func TestParseEnviron(t *testing.T) {
	got := parseEnviron([]string{"FOO=bar", "EQ=a=b", "NOEQUAL", "EMPTY="})
	want := map[string]string{"FOO": "bar", "EQ": "a=b", "EMPTY": ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseEnviron = %#v, want %#v", got, want)
	}
}

func TestSafeMinimum(t *testing.T) {
	shell := map[string]string{
		"PATH": "/bin", "HOME": "/home/u", "LANG": "en", "LC_ALL": "C",
		"APP_SECRET": "nope", "RANDOM_VAR": "nope",
	}
	got := safeMinimum(shell)
	want := map[string]string{"PATH": "/bin", "HOME": "/home/u", "LANG": "en", "LC_ALL": "C"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("safeMinimum = %#v, want %#v", got, want)
	}
}

func TestToEnviron(t *testing.T) {
	got := ToEnviron([]Var{{Key: "A", Value: "1"}, {Key: "B", Value: "x=y"}})
	want := []string{"A=1", "B=x=y"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ToEnviron = %#v, want %#v", got, want)
	}
}

func findVar(vars []Var, key string) (Var, bool) {
	for _, v := range vars {
		if v.Key == key {
			return v, true
		}
	}
	return Var{}, false
}

func TestResolveDefaultProfileWins(t *testing.T) {
	shell := []string{"FOO=shell", "PATH=/bin", "OTHER=keep"}
	source := map[string]string{"FOO": "profile", "BAR": "profile"}
	opts := Options{
		InheritShell: true, Override: true, SourceName: SourceProfile,
		Defaults:   map[string]string{"BAZ": "def"},
		SecretKeys: map[string]bool{},
	}

	vars := Resolve(shell, source, opts)

	// sorted by key
	if vars[0].Key != "BAR" || vars[len(vars)-1].Key != "PATH" {
		t.Fatalf("not sorted by key: %#v", vars)
	}
	foo, _ := findVar(vars, "FOO")
	if foo.Value != "profile" || foo.Source != SourceProfile {
		t.Errorf("FOO = %#v, want profile/profile", foo)
	}
	baz, _ := findVar(vars, "BAZ")
	if baz.Value != "def" || baz.Source != SourceDefault {
		t.Errorf("BAZ = %#v, want def/default", baz)
	}
	other, ok := findVar(vars, "OTHER")
	if !ok || other.Source != SourceShell {
		t.Errorf("OTHER = %#v, want kept from shell", other)
	}
}

func TestResolveNoOverrideShellWins(t *testing.T) {
	shell := []string{"FOO=shell"}
	source := map[string]string{"FOO": "profile", "BAR": "profile"}
	opts := Options{InheritShell: true, Override: false, SourceName: SourceProfile}

	vars := Resolve(shell, source, opts)

	foo, _ := findVar(vars, "FOO")
	if foo.Value != "shell" || foo.Source != SourceShell {
		t.Errorf("FOO = %#v, want shell wins", foo)
	}
	bar, _ := findVar(vars, "BAR")
	if bar.Value != "profile" {
		t.Errorf("BAR = %#v, want profile", bar)
	}
}

func TestResolveIsolatedDropsAmbient(t *testing.T) {
	shell := []string{"PATH=/bin", "OTHER=drop"}
	source := map[string]string{"FOO": "profile"}
	opts := Options{InheritShell: false, Override: true, SourceName: SourceProfile}

	vars := Resolve(shell, source, opts)

	if _, ok := findVar(vars, "OTHER"); ok {
		t.Error("OTHER should be dropped in isolated mode")
	}
	path, ok := findVar(vars, "PATH")
	if !ok || path.Source != SourceSafeMin {
		t.Errorf("PATH = %#v, want kept via safe-min", path)
	}
	foo, _ := findVar(vars, "FOO")
	if foo.Value != "profile" {
		t.Errorf("FOO = %#v, want profile", foo)
	}
}

func TestResolveMarksSecrets(t *testing.T) {
	source := map[string]string{"MY_TOKEN": "t", "PLAIN": "p", "DOCUMENTED": "d"}
	opts := Options{
		InheritShell: false, Override: true, SourceName: SourceEnvFile,
		SecretKeys: map[string]bool{"DOCUMENTED": true},
	}

	vars := Resolve(nil, source, opts)

	tok, _ := findVar(vars, "MY_TOKEN")
	if !tok.Secret {
		t.Error("MY_TOKEN should be secret via heuristic")
	}
	doc, _ := findVar(vars, "DOCUMENTED")
	if !doc.Secret {
		t.Error("DOCUMENTED should be secret via schema")
	}
	plain, _ := findVar(vars, "PLAIN")
	if plain.Secret {
		t.Error("PLAIN should not be secret")
	}
}
