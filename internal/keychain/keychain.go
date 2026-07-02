// Package keychain stores the Symbion encryption passphrase in the macOS
// Keychain via the `security` CLI. On non-macOS systems it is unavailable.
package keychain

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

const service = "symbion"

var osIsDarwin = func() bool { return runtime.GOOS == "darwin" }

var run = func(args ...string) ([]byte, error) {
	return exec.Command("security", args...).Output()
}

// Available reports whether Keychain storage is supported on this platform.
func Available() bool { return osIsDarwin() }

// Get returns the stored passphrase and whether one was found.
func Get() (string, bool, error) {
	if !Available() {
		return "", false, nil
	}
	out, err := run("find-generic-password", "-s", service, "-w")
	if err != nil {
		return "", false, nil // not found (or unreadable) — treat as absent
	}
	return strings.TrimRight(string(out), "\r\n"), true, nil
}

// Set stores (or replaces) the passphrase.
func Set(passphrase string) error {
	if !Available() {
		return fmt.Errorf("keychain storage is only supported on macOS")
	}
	if passphrase == "" {
		return fmt.Errorf("passphrase is empty")
	}
	_, err := run("add-generic-password", "-U", "-s", service, "-a", service, "-w", passphrase)
	return err
}

// Delete removes the stored passphrase.
func Delete() error {
	if !Available() {
		return fmt.Errorf("keychain storage is only supported on macOS")
	}
	_, err := run("delete-generic-password", "-s", service)
	return err
}
