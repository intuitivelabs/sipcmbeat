package cmd

import (
	"github.com/intuitivelabs/sipcmbeat/beater"

	cmd "github.com/elastic/beats/v7/libbeat/cmd"
	"github.com/elastic/beats/v7/libbeat/cmd/instance"
)

// RootCmd to handle beats cli
var RootCmd = cmd.GenRootCmdWithSettings(beater.New,
	instance.Settings{Name: beater.Name, Version: beater.Version})
