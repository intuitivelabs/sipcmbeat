package main

import (
	//	"fmt"
	"os"

	// "github.com/intuitivelabs/sipcallmon"
	//"github.com/intuitivelabs/sipcmbeat/beater"
	"github.com/intuitivelabs/sipcmbeat/cmd"

	_ "github.com/intuitivelabs/sipcmbeat/include"
)

func main() {
	/*
		cfg := sipcallmon.DefaultConfig
		cfg.Iface = "eth0"
		cfg.BPF = "port 5060"
		cfg.Verbose = true
		bt, err := beater.New(nil, nil)
		if err != nil {
			fmt.Printf("error: cfg: %v\n", err)
			os.Exit(1)
		}
		bt.(*beater.Sipcmbeat).Config = cfg
		bt.Run(nil)
		//sipcallmon.Run(&cfg)
	*/
	if err := cmd.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
