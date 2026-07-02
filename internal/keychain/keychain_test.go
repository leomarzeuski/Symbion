package keychain

import (
	"errors"
	"reflect"
	"testing"
)

func stub(t *testing.T, darwin bool, fn func(args ...string) ([]byte, error)) {
	t.Helper()
	origDarwin, origRun := osIsDarwin, run
	t.Cleanup(func() { osIsDarwin, run = origDarwin, origRun })
	osIsDarwin = func() bool { return darwin }
	run = fn
}

func TestGetSetDelete(t *testing.T) {
	var gotArgs []string
	stub(t, true, func(args ...string) ([]byte, error) {
		gotArgs = args
		return []byte("s3cret\n"), nil
	})

	val, ok, err := Get()
	if err != nil || !ok || val != "s3cret" {
		t.Fatalf("Get = %q, %v, %v; want s3cret,true,nil", val, ok, err)
	}
	if gotArgs[0] != "find-generic-password" {
		t.Fatalf("Get args = %v", gotArgs)
	}

	if err := Set("newpass"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	want := []string{"add-generic-password", "-U", "-s", "symbion", "-a", "symbion", "-w", "newpass"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("Set args = %v, want %v", gotArgs, want)
	}

	if err := Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if gotArgs[0] != "delete-generic-password" {
		t.Fatalf("Delete args = %v", gotArgs)
	}
}

func TestGetNotFound(t *testing.T) {
	stub(t, true, func(args ...string) ([]byte, error) { return nil, errors.New("exit status 44") })
	if val, ok, err := Get(); ok || err != nil || val != "" {
		t.Fatalf("Get not found = %q, %v, %v; want \"\",false,nil", val, ok, err)
	}
}

func TestUnavailable(t *testing.T) {
	stub(t, false, func(args ...string) ([]byte, error) {
		t.Fatal("run should not be called when unavailable")
		return nil, nil
	})
	if _, ok, _ := Get(); ok {
		t.Fatal("Get should be false when unavailable")
	}
	if err := Set("x"); err == nil {
		t.Fatal("Set should error when unavailable")
	}
}
