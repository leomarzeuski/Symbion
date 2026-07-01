package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// runProcess executes command[0] with command[1:] using env as the child's
// entire environment. Signals are forwarded to the child; the child's exit
// code is propagated exactly. It never writes to disk.
func runProcess(command []string, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	bin, err := exec.LookPath(command[0])
	if err != nil {
		fmt.Fprintf(stderr, "symbion: %s: command not found\n", command[0])
		return &ExitError{Code: 127}
	}

	proc := exec.Command(bin, command[1:]...)
	proc.Env = env
	proc.Stdin = stdin
	proc.Stdout = stdout
	proc.Stderr = stderr

	if err := proc.Start(); err != nil {
		fmt.Fprintf(stderr, "symbion: %s: %v\n", command[0], err)
		return &ExitError{Code: 126}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer signal.Stop(sigCh)
	go func() {
		for sig := range sigCh {
			_ = proc.Process.Signal(sig)
		}
	}()

	if err := proc.Wait(); err != nil {
		return &ExitError{Code: exitCodeFromError(err)}
	}
	return nil
}

// exitCodeFromError extracts a process exit code from an *exec.ExitError,
// mapping signal termination to the conventional 128+signal.
func exitCodeFromError(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			if status.Signaled() {
				return 128 + int(status.Signal())
			}
			return status.ExitStatus()
		}
		return exitErr.ExitCode()
	}
	return 1
}
