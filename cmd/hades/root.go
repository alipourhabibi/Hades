package main

import (
	"github.com/spf13/cobra"
)

// newRootCmd returns the top-level Cobra command for the Hades CLI.
func newRootCmd(_ []string) (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "hades",
		Short: "Hades Schema Registry",
		Run: func(c *cobra.Command, args []string) {
			c.HelpFunc()(c, args)
		},
	}

	cmd.AddCommand(
		newServeCmd(),
	)

	return cmd, nil
}
