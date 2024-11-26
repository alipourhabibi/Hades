package main

import (
	"os"
)

func main() {
	cmd, err := newRootCmd(os.Args[1:])
	if err != nil {
		os.Exit(1)
	}

	err = cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
