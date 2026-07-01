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
