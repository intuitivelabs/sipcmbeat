package main

import (
	"fmt"
	"os"

	"github.com/intuitivelabs/sipcmbeat/cmd"

	_ "github.com/intuitivelabs/sipcmbeat/include"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		// does not seem to be ever reached (os.Exit() on error from New?)
		fmt.Fprintf(os.Stderr, "ERROR: %+v\n", err)
		os.Exit(1)
	}
}
