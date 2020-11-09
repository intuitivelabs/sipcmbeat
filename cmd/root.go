package cmd

import (
	"github.com/intuitivelabs/sipcmbeat/beater"

	cmd "github.com/elastic/beats/v7/libbeat/cmd"
	"github.com/elastic/beats/v7/libbeat/cmd/instance"
	"github.com/intuitivelabs/sipcallmon"
)

// Name of this beat
const Name = "sipcmbeat"

const Version = "0.6.13"
const FullVer = Version + " sipcallmon " + sipcallmon.Version

// RootCmd to handle beats cli
var RootCmd = cmd.GenRootCmdWithSettings(beater.New, instance.Settings{Name: Name, Version: FullVer})
