package beater

import (
	"strings"

	"github.com/elastic/beats/v7/libbeat/version"
	//	"github.com/intuitivelabs/sipcallmon"
)

// Name of this beat
const Name = "sipcmbeat"

var fallbackVer = "0.7.13" // fallback version
var fallbackTopVer = "unknown"

//const FullVer = Version + " sipcallmon " + sipcallmon.Version

var baseVersion = ""       // last released version
var topVersion = "unknown" // current top version (might contain commit id)

// CanonVersion returns the cannonical version (vN.N.N)
func CanonVersion() string {
	ver := baseVersion
	if baseVersion == "" || strings.EqualFold(baseVersion, "unknown") {
		ver = fallbackVer
	}
	if len(ver) > 1 && (ver[0] == 'V' || ver[0] == 'v') {
		// skip over v
		ver = ver[1:]
	}
	return ver
}

// CrtVersion returns the  current "top" version.
// It might include the commit id (git describe format) if it is not a
// "release" or it might have "unknown" appended to a version (not able
// to found out the exact version).
// If the top commit corresponds to a release (tagged with vN.N.N) it would
// return the tag (identical to CanonVersion()).
func CrtVersion() string {
	if topVersion == "" || strings.EqualFold(topVersion, "unknown") {
		topVersion = fallbackTopVer + "-unknown*"
	}
	if topVersion == "" || strings.EqualFold(topVersion, "unknown") {
		return CanonVersion() + "-unknown"
	}
	return topVersion
}

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
