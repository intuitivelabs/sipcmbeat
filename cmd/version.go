package cmd

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	//"time"

	"github.com/elastic/beats/v7/libbeat/version"
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
		fmt.Printf("%s: %s (%s %s) libbeat: %s\n",
			beater.Name, beater.CrtVersion(),
			beater.CommitId(), beater.BuildTime(),
			version.GetDefaultVersion())
		printDepsVer(os.Stdout,
			[]string{"github.com/intuitivelabs"})
	} else if shortVer {
		fmt.Printf("%s\n", beater.CanonVersion())
	} else {
		beatVerInfo(cmd, args)
	}
	if depsVer {
		printDepsVer(os.Stdout, nil)
	}
}

func printDepsVer(w io.Writer, filter []string) {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, m := range bi.Deps[:] {
			path := m.Path
			if len(filter) > 0 {
				for _, f := range filter {
					if len(f) > 0 {
						l := len(f)
						if len(m.Path) > l && m.Path[:l] == f {
							if m.Path[l] == '/' {
								l++ // skip over '/'
							}
							path = m.Path[l:]
							break
						} else {
							path = "" // no match
						}
					}
				}
			}
			if path == "" {
				// filtered, skip
				continue
			}
			if m.Replace != nil {
				fmt.Fprintf(w, "  %-40s    %-20s",
					m.Replace.Path, m.Replace.Version)
				fmt.Fprintf(w, "  [r: %s]\n",
					path)
			} else {
				fmt.Fprintf(w, "  %-40s    %-20s\n",
					path, m.Version)
			}
		}
	}
}
