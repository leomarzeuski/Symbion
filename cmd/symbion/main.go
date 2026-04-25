package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/leonardomarzeuski/symbion/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		var exitErr *cli.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}

		fmt.Fprintf(os.Stderr, "symbion: %v\n", err)
		os.Exit(2)
	}
}
