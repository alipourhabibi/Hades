package main

import (
	"github.com/spf13/cobra"
)

func newRootCmd(args []string) (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "hades",
		Short: "Hades Schema Registry",
		Run: func(c *cobra.Command, args []string) {
			c.HelpFunc()(c, args)
		},
	}

	flags := cmd.PersistentFlags()
	err := flags.Parse(args)
	if err != nil {
		return nil, err
	}

	cmd.AddCommand(
		newServeCmd(),
	)

	return cmd, nil
}
