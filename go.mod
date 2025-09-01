module github.com/intuitivelabs/sipcmbeat

go 1.23.0

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
	cloud.google.com/go v0.116.0
	cloud.google.com/go/compute/metadata v0.6.0
	cloud.google.com/go/datastore v1.20.0
	github.com/Microsoft/go-winio v0.6.2
	github.com/Shopify/sarama v0.0.0-00010101000000-000000000000
	github.com/armon/go-radix v1.0.0
	github.com/bradleyfalzon/ghinstallation v1.1.1
	github.com/cespare/xxhash/v2 v2.3.0
	github.com/containerd/containerd v1.7.27
	github.com/containerd/errdefs v0.3.0
	github.com/davecgh/go-spew v1.1.1
	github.com/dgrijalva/jwt-go v3.2.1-0.20190620180102-5e25c22bd5d6+incompatible
	github.com/dlclark/regexp2 v1.11.0
	github.com/docker/distribution v2.7.1+incompatible
	github.com/docker/docker v1.4.2-0.20170802015333-8af4db6f002a
	github.com/docker/go-connections v0.4.0
	github.com/docker/go-units v0.5.0
	github.com/dop251/goja v0.0.0-00010101000000-000000000000
	github.com/dop251/goja_nodejs v0.0.0-20171011081505-adff31b136e6
	github.com/dustin/go-humanize v0.0.0-20171111073723-bb3d318650d4
	github.com/eapache/go-resiliency v1.2.0
	github.com/eapache/go-xerial-snappy v0.0.0-20180814174437-776d5712da21
	github.com/eapache/queue v1.1.0
	github.com/elastic/beats/v7 v7.9.2
	github.com/elastic/ecs v1.5.0
	github.com/elastic/go-lumber v0.1.0
	github.com/elastic/go-seccomp-bpf v1.1.0
	github.com/elastic/go-structform v0.0.7
	github.com/elastic/go-sysinfo v1.3.0
	github.com/elastic/go-txfile v0.0.7
	github.com/elastic/go-ucfg v0.8.3
	github.com/elastic/go-windows v1.0.1
	github.com/elastic/gosigar v0.10.6-0.20200715000138-f115143bb233
	github.com/emicklei/go-restful/v3 v3.10.1
	github.com/fatih/color v1.18.0
	github.com/garyburd/redigo v1.0.1-0.20160525165706-b8dc90050f24
	github.com/go-logr/logr v1.4.2
	github.com/go-openapi/jsonpointer v0.19.6
	github.com/go-openapi/jsonreference v0.20.1
	github.com/go-openapi/swag v0.22.3
	github.com/go-sourcemap/sourcemap v2.1.2+incompatible
	github.com/gofrs/flock v0.12.1
	github.com/gofrs/uuid v3.3.0+incompatible
	github.com/gogo/protobuf v1.3.2
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da
	github.com/golang/protobuf v1.5.4
	github.com/golang/snappy v0.0.4
	github.com/google/gnostic v0.5.7-v3refs
	github.com/google/go-cmp v0.7.0
	github.com/google/go-github/v29 v29.0.2
	github.com/google/go-github/v31 v31.0.0
	github.com/google/go-querystring v1.0.0
	github.com/google/gofuzz v1.2.0
	github.com/google/gopacket v1.1.19
	github.com/google/s2a-go v0.1.9
	github.com/google/uuid v1.6.0
	github.com/googleapis/enterprise-certificate-proxy v0.3.4
	github.com/googleapis/gax-go/v2 v2.14.1
	github.com/hashicorp/errwrap v1.1.0
	github.com/hashicorp/go-cleanhttp v0.5.2
	github.com/hashicorp/go-multierror v1.1.0
	github.com/hashicorp/go-retryablehttp v0.6.6
	github.com/hashicorp/go-uuid v1.0.3
	github.com/hashicorp/go-version v1.7.0
	github.com/haya14busa/go-actions-toolkit v0.0.0-20200105081403-ca0307860f01
	github.com/imdario/mergo v0.3.6
	github.com/inconshreveable/mousetrap v1.1.0
	github.com/jcmturner/gofork v1.0.0
	github.com/joeshaw/multierror v0.0.0-20140124173710-69b34d4ec901
	github.com/josharian/intern v1.0.0
	github.com/json-iterator/go v1.1.12
	github.com/klauspost/compress v1.16.7
	github.com/klauspost/cpuid/v2 v2.0.9
	github.com/magefile/mage v1.14.0
	github.com/mailru/easyjson v0.7.7
	github.com/mattn/go-colorable v0.1.14
	github.com/mattn/go-isatty v0.0.20
	github.com/mattn/go-shellwords v1.0.10
	github.com/miekg/dns v1.1.15
	github.com/mitchellh/hashstructure v1.0.0
	github.com/mitchellh/iochan v1.0.0
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd
	github.com/modern-go/reflect2 v1.0.2
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.1.0
	github.com/oschwald/maxminddb-golang v1.8.0
	github.com/pierrec/lz4 v2.4.1+incompatible
	github.com/pkg/errors v0.9.1
	github.com/prometheus/procfs v0.10.1
	github.com/rcrowley/go-metrics v0.0.0-20200313005456-10cdbea86bc0
	github.com/reviewdog/errorformat v0.0.0-20200622091151-ac6101f62307
	github.com/reviewdog/reviewdog v0.10.2
	github.com/santhosh-tekuri/jsonschema v1.2.4
	github.com/sirupsen/logrus v1.9.3
	github.com/spf13/cobra v0.0.3
	github.com/spf13/pflag v1.0.6
	github.com/urso/go-bin v0.0.0-20180220135811-781c575c9f0e
	github.com/urso/magetools v0.0.0-20190919040553-290c89e0c230
	github.com/xanzy/go-gitlab v0.32.1
	github.com/zeebo/xxh3 v1.0.2
	github.com/intuitivelabs/anonymization v1.5.3
	github.com/intuitivelabs/bytescase v1.0.3
	github.com/intuitivelabs/bytespool v0.0.4
	github.com/intuitivelabs/calltr v1.1.15-0.20250901075216-d65b5f8604e7
	github.com/intuitivelabs/counters v0.3.3
	github.com/intuitivelabs/httpsp v0.0.10
	github.com/intuitivelabs/ipcrypt v1.1.0
	github.com/intuitivelabs/mallocs/qmalloc v0.0.4
	github.com/intuitivelabs/sipcallmon v0.8.24-0.20250901091400-b3f10b3a2547
	github.com/intuitivelabs/sipsp v1.1.6
	github.com/intuitivelabs/slog v0.0.3
	github.com/intuitivelabs/timestamp v0.0.4
	github.com/intuitivelabs/unsafeconv v0.0.3
	github.com/intuitivelabs/websocket v0.0.2
	github.com/intuitivelabs/wtimer v0.0.3
	go.elastic.co/apm v1.7.2
	go.elastic.co/apm/module/apmelasticsearch v1.7.2
	go.elastic.co/apm/module/apmhttp v1.7.2
	go.elastic.co/ecszap v0.2.0
	go.elastic.co/fastjson v1.1.0
	go.opencensus.io v0.24.0
	go.uber.org/atomic v1.7.0
	go.uber.org/multierr v1.6.0
	go.uber.org/zap v1.24.0
	golang.org/x/build v0.0.0-20200616162219-07bebbe343e9
	golang.org/x/crypto v0.36.0
	golang.org/x/lint v0.0.0-20200302205851-738671d3881b
	golang.org/x/mod v0.24.0
	golang.org/x/net v0.37.0
	golang.org/x/oauth2 v0.26.0
	golang.org/x/sync v0.12.0
	golang.org/x/sys v0.31.0
	golang.org/x/term v0.30.0
	golang.org/x/text v0.23.0
	golang.org/x/time v0.10.0
	golang.org/x/tools v0.30.0
	google.golang.org/api v0.223.0
	google.golang.org/genproto v0.0.0-20241118233622-e639e219e697
	google.golang.org/genproto/googleapis/api v0.0.0-20241209162323-e6fa225c2576
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250219182151-9fdb1cabc7b2
	google.golang.org/grpc v1.70.0
	google.golang.org/protobuf v1.36.5
	gopkg.in/inf.v0 v0.9.1
	gopkg.in/jcmturner/aescts.v1 v1.0.1
	gopkg.in/jcmturner/dnsutils.v1 v1.0.1
	gopkg.in/jcmturner/goidentity.v3 v3.0.0
	gopkg.in/jcmturner/gokrb5.v7 v7.5.0
	gopkg.in/jcmturner/rpc.v1 v1.1.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	honnef.co/go/tools v0.6.1
	howett.net/plist v0.0.0-20181124034731-591f970eefbb
	k8s.io/api v0.26.2
	k8s.io/apimachinery v0.27.4
	k8s.io/client-go v0.26.2
	k8s.io/klog/v2 v2.90.1
	k8s.io/kube-openapi v0.0.0-20230501164219-8b0f38b5fd1f
	k8s.io/utils v0.0.0-20230220204549-a5ecb0141aa5
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3
	sigs.k8s.io/yaml v1.3.0
)

require (
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/sdp/v3 v3.0.9 // indirect
)

tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint


require (
	cloud.google.com/go/compute v1.29.0 // indirect
	github.com/akavel/rsrc v0.9.0 // indirect
	github.com/josephspurrier/goversioninfo v1.2.0 // indirect
	github.com/jstemmer/go-junit-report v0.9.1 // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	google.golang.org/appengine v1.6.8 // indirect
)