package validate

import (
	"testing"

	"github.com/leonardomarzeuski/symbion/internal/schema"
)

func TestValueTypes(t *testing.T) {
	cases := []struct {
		typ   string
		value string
		valid bool
	}{
		{"", "anything", true},
		{"string", "anything", true},
		{"int", "42", true},
		{"int", "4.2", false},
		{"int", "x", false},
		{"bool", "true", true},
		{"bool", "0", true},
		{"bool", "yes", false},
		{"port", "5432", true},
		{"port", "0", false},
		{"port", "70000", false},
		{"url", "postgres://localhost:5432/db", true},
		{"url", "not a url", false},
		{"duration", "30s", true},
		{"duration", "banana", false},
	}
	for _, c := range cases {
		_, ok := Value(schema.EnvSpec{Type: c.typ}, c.value)
		if ok != c.valid {
			t.Errorf("Value(type=%q, %q) valid=%v, want %v", c.typ, c.value, ok, c.valid)
		}
	}
}

func TestValueEnum(t *testing.T) {
	spec := schema.EnvSpec{Enum: []string{"dev", "staging", "prod"}}
	if _, ok := Value(spec, "staging"); !ok {
		t.Error("staging should be valid")
	}
	if _, ok := Value(spec, "qa"); ok {
		t.Error("qa should be invalid")
	}
}

func TestValuePattern(t *testing.T) {
	spec := schema.EnvSpec{Pattern: `^sk-[a-z0-9]+$`}
	if _, ok := Value(spec, "sk-abc123"); !ok {
		t.Error("sk-abc123 should match")
	}
	if _, ok := Value(spec, "nope"); ok {
		t.Error("nope should not match")
	}
}

func TestValueReasonNeverIncludesValue(t *testing.T) {
	reason, ok := Value(schema.EnvSpec{Type: "int"}, "supersecret")
	if ok {
		t.Fatal("expected invalid")
	}
	if reason == "" {
		t.Fatal("expected a reason")
	}
	if got := reason; got != "must be an integer" {
		t.Fatalf("reason = %q, want %q (must not leak the value)", got, "must be an integer")
	}
}
