// +build mage

package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/pkg/errors"

	devtools "github.com/elastic/beats/v7/dev-tools/mage"
	"github.com/elastic/beats/v7/dev-tools/mage/target/build"
	"github.com/elastic/beats/v7/dev-tools/mage/target/common"
	"github.com/elastic/beats/v7/dev-tools/mage/target/pkg"
	"github.com/elastic/beats/v7/dev-tools/mage/target/unittest"
)

var beatPath string

var baseVer string
var topVer string

func init() {
	devtools.SetBuildVariableSources(devtools.DefaultBeatBuildVariableSources)

	devtools.BeatDescription = "sip call monitor & probe"
	devtools.BeatVendor = "intuitivelabs"
	devtools.BeatURL = "https://github.com/intuitivelabs/sipcmbeat/blob/master/README.md"
	devtools.BeatProjectType = devtools.CommunityProject
	devtools.CrossBuildMountModcache = true

	beatPath = os.Getenv("BEAT_PATH")
	if beatPath == "" {
		beatPath = "github.com/intuitivelabs/sipcmbeat"
	}

	baseVer, _ = sh.Output("git", "describe", "--tags",
		"--match=v[0-9]*.[0-9]*.[0-9]*", "--abbrev=0", "HEAD")
	topVer, _ = sh.Output("git", "describe", "--tags",
		"--match=v[0-9]*.[0-9]*.[0-9]*", "--dirty")
}

// Package packages the Beat for distribution.
// Use SNAPSHOT=true to build snapshots.
// Use PLATFORMS to control the target platforms.
func Package() {
	start := time.Now()
	defer func() { fmt.Println("package ran for", time.Since(start)) }()

	devtools.UseCommunityBeatPackaging()

	mg.Deps(Update)
	mg.Deps(build.CrossBuild, build.CrossBuildGoDaemon)
	mg.SerialDeps(devtools.Package, pkg.PackageTest)
}

// Update updates the generated files (aka make update).
func Update() error {
	return sh.Run("make", "update")
}

// Fields generates a fields.yml for the Beat.
func Fields() error {
	return devtools.GenerateFieldsYAML()
}

// Config generates both the short/reference/docker configs.
func Config() error {
	p := devtools.DefaultConfigFileParams()
	p.Templates = append(p.Templates, "_meta/config/*.tmpl")
	return devtools.Config(devtools.AllConfigTypes, p, ".")
}

// Clean cleans all generated files and build artifacts.
func Clean() error {
	return devtools.Clean()
}

// Check formats code, updates generated content, check for common errors, and
// checks for any modified files.
func Check() {
	common.Check()
}

// Fmt formats source code (.go and .py) and adds license headers.
func Fmt() {
	common.Fmt()
}

// Test runs all available tests
func Test() {
	mg.Deps(unittest.GoUnitTest)
}

// Build builds the Beat binary.
func Build() error {
	params := devtools.DefaultBuildArgs()
	params.Vars[beatPath+"/beater.baseVersion"] = baseVer
	params.Vars[beatPath+"/beater.topVersion"] = topVer
	return devtools.Build(params)
}

// CrossBuild cross-builds the beat for all target platforms.
func CrossBuild() error {
	return build.CrossBuild()
}

// BuildGoDaemon builds the go-daemon binary (use crossBuildGoDaemon).
func BuildGoDaemon() error {
	return build.BuildGoDaemon()
}

// GolangCrossBuild build the Beat binary inside of the golang-builder.
// Do not use directly, use crossBuild instead.
func GolangCrossBuild() error {
	if dep, found := crossBuildDeps[devtools.Platform.Name]; found {
		mg.Deps(dep)
	}

	params := devtools.DefaultGolangCrossBuildArgs()
	if flags, found := libpcapLDFLAGS[devtools.Platform.Name]; found {
		params.Env = map[string]string{
			"CGO_LDFLAGS": flags,
		}
	}
	if flags, found := libpcapCFLAGS[devtools.Platform.Name]; found {
		params.Env["CGO_CFLAGS"] = flags
	}
	params.Vars[beatPath+"/beater.baseVersion"] = baseVer
	params.Vars[beatPath+"/beater.topVersion"] = topVer

	return devtools.GolangCrossBuild(params)
}

// -----------------------------------------------------------------------------
// Customizations
// - Must compile libpcap or winpcap during cross-compilation.
// - On Linux libpcap is statically linked. Darwin and Windows are dynamic.

const (
	libpcapURL    = "https://s3.amazonaws.com/beats-files/deps/libpcap-1.8.1.tar.gz"
	libpcapSHA256 = "673dbc69fdc3f5a86fb5759ab19899039a8e5e6c631749e48dcd9c6f0c83541e"
)

const (
	linuxPcapLDFLAGS = "-L/libpcap/libpcap-1.8.1 -lpcap"
	linuxPcapCFLAGS  = "-I /libpcap/libpcap-1.8.1"
)

var libpcapLDFLAGS = map[string]string{
	"linux/386":      linuxPcapLDFLAGS,
	"linux/amd64":    linuxPcapLDFLAGS,
	"linux/arm64":    linuxPcapLDFLAGS,
	"linux/armv5":    linuxPcapLDFLAGS,
	"linux/armv6":    linuxPcapLDFLAGS,
	"linux/armv7":    linuxPcapLDFLAGS,
	"linux/mips":     linuxPcapLDFLAGS,
	"linux/mipsle":   linuxPcapLDFLAGS,
	"linux/mips64":   linuxPcapLDFLAGS,
	"linux/mips64le": linuxPcapLDFLAGS,
	"linux/ppc64le":  linuxPcapLDFLAGS,
	"linux/s390x":    linuxPcapLDFLAGS,
	"darwin/amd64":   "-lpcap",
	"windows/amd64":  "-L /libpcap/win/WpdPack/Lib/x64 -lwpcap",
	"windows/386":    "-L /libpcap/win/WpdPack/Lib -lwpcap",
}

var libpcapCFLAGS = map[string]string{
	"linux/386":      linuxPcapCFLAGS,
	"linux/amd64":    linuxPcapCFLAGS,
	"linux/arm64":    linuxPcapCFLAGS,
	"linux/armv5":    linuxPcapCFLAGS,
	"linux/armv6":    linuxPcapCFLAGS,
	"linux/armv7":    linuxPcapCFLAGS,
	"linux/mips":     linuxPcapCFLAGS,
	"linux/mipsle":   linuxPcapCFLAGS,
	"linux/mips64":   linuxPcapCFLAGS,
	"linux/mips64le": linuxPcapCFLAGS,
	"linux/ppc64le":  linuxPcapCFLAGS,
	"linux/s390x":    linuxPcapCFLAGS,
	"windows/amd64":  "-I /libpcap/win/WpdPack/Include",
	"windows/386":    "-I /libpcap/win/WpdPack/Include",
}

var crossBuildDeps = map[string]func() error{
	"linux/386":      buildLibpcapLinux386,
	"linux/amd64":    buildLibpcapLinuxAMD64,
	"linux/arm64":    buildLibpcapLinuxARM64,
	"linux/armv5":    buildLibpcapLinuxARMv5,
	"linux/armv6":    buildLibpcapLinuxARMv6,
	"linux/armv7":    buildLibpcapLinuxARMv7,
	"linux/mips":     buildLibpcapLinuxMIPS,
	"linux/mipsle":   buildLibpcapLinuxMIPSLE,
	"linux/mips64":   buildLibpcapLinuxMIPS64,
	"linux/mips64le": buildLibpcapLinuxMIPS64LE,
	"linux/ppc64le":  buildLibpcapLinuxPPC64LE,
	"linux/s390x":    buildLibpcapLinuxS390x,
	"windows/amd64":  installLibpcapWindowsAMD64,
	"windows/386":    installLibpcapWindows386,
}

// buildLibpcapFromSource builds libpcap from source because the library needs
// to be compiled with -fPIC.
// See https://github.com/elastic/beats/v7/pull/4217.
func buildLibpcapFromSource(params map[string]string) error {
	tarFile, err := devtools.DownloadFile(libpcapURL, "/libpcap")
	if err != nil {
		return errors.Wrap(err, "failed to download libpcap source")
	}

	if err = devtools.VerifySHA256(tarFile, libpcapSHA256); err != nil {
		return err
	}

	if err = devtools.Extract(tarFile, "/libpcap"); err != nil {
		return errors.Wrap(err, "failed to extract libpcap")
	}

	var configureArgs []string
	for k, v := range params {
		if strings.HasPrefix(k, "-") {
			delete(params, k)
			configureArgs = append(configureArgs, k+"="+v)
		}
	}

	// Use sh -c here because sh.Run does not expose a way to change the CWD.
	// This command only runs in Linux so this is fine.
	return sh.RunWith(params, "sh", "-c",
		"cd /libpcap/libpcap-1.8.1 && "+
			"./configure --enable-usb=no --enable-bluetooth=no --enable-dbus=no "+strings.Join(configureArgs, " ")+"&& "+
			"make")
}

func buildLibpcapLinux386() error {
	return buildLibpcapFromSource(map[string]string{
		"CFLAGS":  "-m32",
		"LDFLAGS": "-m32",
	})
}

func buildLibpcapLinuxAMD64() error {
	return buildLibpcapFromSource(map[string]string{})
}

func buildLibpcapLinuxARM64() error {
	return buildLibpcapFromSource(map[string]string{
		"--host":      "aarch64-unknown-linux-gnu",
		"--with-pcap": "linux",
	})
}

func buildLibpcapLinuxARMv5() error {
	return buildLibpcapFromSource(map[string]string{
		"--host":      "arm-linux-gnueabi",
		"--with-pcap": "linux",
	})
}

func buildLibpcapLinuxARMv6() error {
	return buildLibpcapFromSource(map[string]string{
		"--host":      "arm-linux-gnueabi",
		"--with-pcap": "linux",
	})
}

func buildLibpcapLinuxARMv7() error {
	return buildLibpcapFromSource(map[string]string{
		"--host":      "arm-linux-gnueabihf",
		"--with-pcap": "linux",
	})
}

func buildLibpcapLinuxMIPS() error {
	return buildLibpcapFromSource(map[string]string{
		"--host":      "mips-unknown-linux-gnu",
		"--with-pcap": "linux",
	})
}

func buildLibpcapLinuxMIPSLE() error {
	return buildLibpcapFromSource(map[string]string{
		"--host":      "mipsle-unknown-linux-gnu",
		"--with-pcap": "linux",
	})
}

func buildLibpcapLinuxMIPS64() error {
	return buildLibpcapFromSource(map[string]string{
		"--host":      "mips64-unknown-linux-gnu",
		"--with-pcap": "linux",
	})
}

func buildLibpcapLinuxMIPS64LE() error {
	return buildLibpcapFromSource(map[string]string{
		"--host":      "mips64le-unknown-linux-gnu",
		"--with-pcap": "linux",
	})
}

func buildLibpcapLinuxPPC64LE() error {
	return buildLibpcapFromSource(map[string]string{
		"--host":      "powerpc64le-linux-gnu",
		"--with-pcap": "linux",
	})
}

func buildLibpcapLinuxS390x() error {
	return buildLibpcapFromSource(map[string]string{
		"--host":      "s390x-ibm-linux-gnu",
		"--with-pcap": "linux",
	})
}

func installLibpcapWindowsAMD64() error {
	mg.SerialDeps(installWinpcap, generateWin64StaticWinpcap)
	return nil
}

func installLibpcapWindows386() error {
	return installWinpcap()
}

func installWinpcap() error {
	log.Println("Install Winpcap")
	const wpdpackURL = "https://www.winpcap.org/install/bin/WpdPack_4_1_2.zip"

	winpcapZip, err := devtools.DownloadFile(wpdpackURL, "/")
	if err != nil {
		return err
	}

	if err = devtools.Extract(winpcapZip, "/libpcap/win"); err != nil {
		return err
	}

	return nil
}

func generateWin64StaticWinpcap() error {
	log.Println(">> Generating 64-bit winpcap static lib")

	// Notes: We are using absolute path to make sure the files
	// are available for x-pack build.
	// Ref: https://github.com/elastic/beats/v7/issues/1259
	defer devtools.DockerChown(devtools.MustExpand("{{elastic_beats_dir}}/{{.BeatName}}/lib"))
	return devtools.RunCmds(
		// Requires mingw-w64-tools.
		[]string{"gendef", devtools.MustExpand("{{elastic_beats_dir}}/{{.BeatName}}/lib/windows-64/wpcap.dll")},
		[]string{"mv", "wpcap.def", devtools.MustExpand("{{ elastic_beats_dir}}/{{.BeatName}}/lib/windows-64/wpcap.def")},
		[]string{"x86_64-w64-mingw32-dlltool", "--as-flags=--64",
			"-m", "i386:x86-64", "-k",
			"--output-lib", "/libpcap/win/WpdPack/Lib/x64/libwpcap.a",
			"--input-def", devtools.MustExpand("{{elastic_beats_dir}}/{{.BeatName}}/lib/windows-64/wpcap.def")},
	)
}
