package envdiff

import (
	"reflect"
	"testing"
)

func TestCompare(t *testing.T) {
	result := Compare(
		map[string]string{
			"DATABASE_URL": "postgres://local",
			"API_KEY":      "one",
			"LEFT_ONLY":    "left",
			"SAME":         "same",
		},
		map[string]string{
			"DATABASE_URL": "postgres://local",
			"API_KEY":      "two",
			"RIGHT_ONLY":   "right",
			"SAME":         "same",
		},
	)

	if !reflect.DeepEqual(result.OnlyLeft, []string{"LEFT_ONLY"}) {
		t.Fatalf("OnlyLeft = %#v, want LEFT_ONLY", result.OnlyLeft)
	}
	if !reflect.DeepEqual(result.OnlyRight, []string{"RIGHT_ONLY"}) {
		t.Fatalf("OnlyRight = %#v, want RIGHT_ONLY", result.OnlyRight)
	}
	if !reflect.DeepEqual(result.Changed, []string{"API_KEY"}) {
		t.Fatalf("Changed = %#v, want API_KEY", result.Changed)
	}
	if result.SameCount != 2 {
		t.Fatalf("SameCount = %d, want 2", result.SameCount)
	}
	if result.DifferenceCount() != 3 {
		t.Fatalf("DifferenceCount = %d, want 3", result.DifferenceCount())
	}
}
