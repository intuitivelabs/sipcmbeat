package cmd

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	//"time"

	"github.com/elastic/beats/v7/libbeat/version"
	"github.com/intuitivelabs/sipcallmon"
	"github.com/intuitivelabs/sipcmbeat/beater"
	"github.com/spf13/cobra"
)

var longVer bool
var shortVer bool
var depsVer bool

// original "standard" beat version info will be saved here
var beatVerInfo func(cmd *cobra.Command, args []string)

func init() {
	//	RootCmd.VersionCmd.PostRun = func(cmd *cobra.Command, args []string) { }
	beatVerInfo = RootCmd.VersionCmd.Run
	RootCmd.VersionCmd.Run = printVerInfo
	RootCmd.VersionCmd.Flags().BoolVarP(&longVer, "long", "l", false,
		"long version")
	RootCmd.VersionCmd.Flags().BoolVarP(&shortVer, "short", "s", false,
		"short version")
	RootCmd.VersionCmd.Flags().BoolVarP(&depsVer, "deps", "x", false,
		"dependencies version")
}

func printVerInfo(cmd *cobra.Command, args []string) {
	if longVer {
		fmt.Printf("%s: %s (%s %s) sipcallmon: %s libbeat: %s\n",
			beater.Name, beater.Version,
			beater.CommitId(), beater.BuildTime(),
			sipcallmon.Version,
			version.GetDefaultVersion())
	} else if shortVer {
		fmt.Printf("%s\n", beater.Version)
	} else {
		beatVerInfo(cmd, args)
	}
	if depsVer {
		printDepsVer(os.Stdout)
	}
}

func printDepsVer(w io.Writer) {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, m := range bi.Deps[:] {
			if m.Replace != nil {
				fmt.Fprintf(w, "  %-40s    %-20s",
					m.Replace.Path, m.Replace.Version)
				fmt.Fprintf(w, "  [r: %s]\n",
					m.Path)
			} else {
				fmt.Fprintf(w, "  %-40s    %-20s\n",
					m.Path, m.Version)
			}
		}
	}
}
