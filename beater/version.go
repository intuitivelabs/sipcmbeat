package beater

import (
	"github.com/elastic/beats/v7/libbeat/version"
	"github.com/intuitivelabs/sipcallmon"
)

// Name of this beat
const Name = "sipcmbeat"

const Version = "0.7.0"
const FullVer = Version + " sipcallmon " + sipcallmon.Version

func BuildTime() string {
	bt := version.BuildTime()
	bTime := "unknown"
	if !bt.IsZero() {
		//bTime = bt.String()
		bTime = bt.Format("2006/01/02 15:04 MST")
	}
	return bTime
}

func CommitId() string {
	commit := version.Commit()
	if len(commit) > 8 {
		return commit[:8]
	}
	return commit
}
