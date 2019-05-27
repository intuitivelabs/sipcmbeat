package main

import (
	"os"

	"andrei/sipcmbeat/cmd"

	_ "andrei/sipcmbeat/include"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
