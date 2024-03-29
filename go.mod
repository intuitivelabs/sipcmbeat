module github.com/intuitivelabs/sipcmbeat

go 1.15

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v12.2.0+incompatible
	github.com/Shopify/sarama => github.com/elastic/sarama v1.19.1-0.20200629123429-0e7b69039eec
	github.com/cucumber/godog => github.com/cucumber/godog v0.8.1
	github.com/docker/docker => github.com/docker/engine v0.0.0-20191113042239-ea84732a7725
	github.com/docker/go-plugins-helpers => github.com/elastic/go-plugins-helpers v0.0.0-20200207104224-bdf17607b79f
	github.com/dop251/goja => github.com/andrewkroh/goja v0.0.0-20190128172624-dd2ac4456e20
	github.com/dop251/goja_nodejs => github.com/dop251/goja_nodejs v0.0.0-20171011081505-adff31b136e6
	github.com/fsnotify/fsevents => github.com/elastic/fsevents v0.0.0-20181029231046-e1d381a4d270
	github.com/fsnotify/fsnotify => github.com/adriansr/fsnotify v0.0.0-20180417234312-c9bbe1f46f1d
	github.com/insomniacslk/dhcp => github.com/elastic/dhcp v0.0.0-20200227161230-57ec251c7eb3 // indirect
	//github.com/intuitivelabs/anonymization => github.com/intuitivelabs/anonymization v1.4.2-0.20220516114438-8a491145a6db
	//github.com/intuitivelabs/sipcallmon => github.com/intuitivelabs/sipcallmon v0.8.15-0.20220516142113-7f841f75d7e8
	//	golang.org/x/tools => golang.org/x/tools v0.0.0-20200602230032-c00d67ef29d0 // release 1.14
	github.com/tonistiigi/fifo => github.com/containerd/fifo v0.0.0-20190816180239-bda0ff6ed73c
)

// NOTE: following replace recommended by beats, but breaks sipcallmon
//       (pcap OpenLive timeout seems to be ignored or set to very small
//        value => continuosly exiting pcap.ZeroCopyReadPacketData() and
//       eating 100% CPU.
//        github.com/google/gopacket seems more up-to-date
//  TODO: more checking for the best  version
// --andrei
// replace github.com/google/gopacket => github.com/adriansr/gopacket v1.1.18

require (
	github.com/akavel/rsrc v0.9.0 // indirect
	github.com/dlclark/regexp2 v1.4.0 // indirect
	github.com/dop251/goja v0.0.0-20201008094107-f97e50db25ec // indirect
	github.com/dop251/goja_nodejs v0.0.0-20200811150831-9bc458b4bbeb // indirect
	github.com/elastic/beats/v7 v7.9.2
	github.com/elastic/go-sysinfo v1.4.0 // indirect
	github.com/fatih/color v1.9.0 // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/intuitivelabs/anonymization v1.5.0
	github.com/intuitivelabs/calltr v1.1.12
	github.com/intuitivelabs/counters v0.3.1
	github.com/intuitivelabs/sipcallmon v0.8.18
	github.com/intuitivelabs/sipsp v1.1.5
	github.com/intuitivelabs/slog v0.0.2
	github.com/intuitivelabs/timestamp v0.0.3
	github.com/josephspurrier/goversioninfo v1.2.0 // indirect
	github.com/magefile/mage v1.15.0
	github.com/mattn/go-colorable v0.1.8 // indirect
	github.com/mitchellh/gox v1.0.1
	github.com/mitchellh/hashstructure v1.0.0 // indirect
	github.com/oschwald/maxminddb-golang v1.8.0
	github.com/pierrre/gotestcover v0.0.0-20160517101806-924dca7d15f0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/procfs v0.2.0 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20200313005456-10cdbea86bc0 // indirect
	github.com/reviewdog/reviewdog v0.10.2
	github.com/spf13/cobra v0.0.3
	github.com/stretchr/testify v1.7.0 // indirect
	github.com/tsg/go-daemon v0.0.0-20200207173439-e704b93fd89b
	go.elastic.co/apm v1.8.0 // indirect
	go.elastic.co/ecszap v0.2.0 // indirect
	go.elastic.co/fastjson v1.1.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.16.0 // indirect
	golang.org/x/lint v0.0.0-20200302205851-738671d3881b
	golang.org/x/sys v0.0.0-20201101102859-da207088b7d1 // indirect
	golang.org/x/tools v0.0.0-20201218024724-ae774e9781d2
	honnef.co/go/tools v0.0.1-2020.1.6 // indirect
	howett.net/plist v0.0.0-20200419221736-3b63eb3a43b5 // indirect
)
