package cli

import (
	"fmt"
	"os"

	"github.com/leonardomarzeuski/symbion/internal/trust"
	"github.com/spf13/cobra"
)

func newAllowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "allow [dir]",
		Short: "Trust the current directory's .env for shell auto-load",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := trustDirArg(args)
			if err != nil {
				return err
			}
			store, err := trust.NewDefaultStore()
			if err != nil {
				return err
			}
			if err := store.Allow(dir); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Allowed %s for shell auto-load.\n", dir)
			return nil
		},
	}
}

func newDenyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "deny [dir]",
		Short: "Revoke shell auto-load trust for a directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := trustDirArg(args)
			if err != nil {
				return err
			}
			store, err := trust.NewDefaultStore()
			if err != nil {
				return err
			}
			if err := store.Deny(dir); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Revoked auto-load trust for %s.\n", dir)
			return nil
		},
	}
}

func trustDirArg(args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	return os.Getwd()
}
