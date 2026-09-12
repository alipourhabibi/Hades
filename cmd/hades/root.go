package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newRootCmd returns the top-level Cobra command for the Hades CLI.
func newRootCmd(_ []string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "hades",
		Short:   "Hades Schema Registry",
		Version: fmt.Sprintf("%s (%s)", version, commit),
		Run: func(c *cobra.Command, args []string) {
			c.HelpFunc()(c, args)
		},
	}

	cmd.AddCommand(
		newServeCmd(),
	)

	return cmd
}
