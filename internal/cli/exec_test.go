package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestHelperProcess is not a real test: when GO_WANT_HELPER_PROCESS=1 the test
// binary re-executes itself and behaves as the child process under test.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := helperArgs()
	if len(args) == 0 {
		os.Exit(0)
	}
	switch args[0] {
	case "printenv":
		if len(args) > 1 {
			fmt.Fprint(os.Stdout, os.Getenv(args[1]))
		}
		os.Exit(0)
	case "exitcode":
		code := 0
		if len(args) > 1 {
			code, _ = strconv.Atoi(args[1])
		}
		os.Exit(code)
	default:
		os.Exit(0)
	}
}

func helperArgs() []string {
	for i, a := range os.Args {
		if a == "--" {
			return os.Args[i+1:]
		}
	}
	return nil
}

func helperCommand(args ...string) []string {
	base := []string{os.Args[0], "-test.run=^TestHelperProcess$", "--"}
	return append(base, args...)
}

func TestRunProcessInjectsEnv(t *testing.T) {
	var out bytes.Buffer
	env := append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "SYMBION_TEST_VAR=hello")
	err := runProcess(helperCommand("printenv", "SYMBION_TEST_VAR"), env, nil, &out, io.Discard)
	if err != nil {
		t.Fatalf("runProcess error = %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "hello" {
		t.Fatalf("child saw SYMBION_TEST_VAR=%q, want hello", got)
	}
}

func TestRunProcessPropagatesExitCode(t *testing.T) {
	env := append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	err := runProcess(helperCommand("exitcode", "3"), env, nil, io.Discard, io.Discard)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *ExitError, got %v", err)
	}
	if exitErr.Code != 3 {
		t.Fatalf("exit code = %d, want 3", exitErr.Code)
	}
}

func TestRunProcessSuccessReturnsNil(t *testing.T) {
	env := append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	if err := runProcess(helperCommand("exitcode", "0"), env, nil, io.Discard, io.Discard); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestRunProcessCommandNotFound(t *testing.T) {
	err := runProcess([]string{"symbion-nonexistent-xyz"}, os.Environ(), nil, io.Discard, io.Discard)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 127 {
		t.Fatalf("expected exit 127, got %v", err)
	}
}
