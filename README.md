# sipcmbeat

This software collects unencrypted SIP (RFC3261) traffic and generates events
that describe SIP signaling transactions. The events comply to ECS
https://www.elastic.co/guide/en/ecs/current/ecs-reference.html and include
SIP-specific extensions. They can be further used for security analytics, CDR
postprocessing, compliance audits, and troubleshooting.

Futher information is available at
https://www.intuitivelabs.com/technology/sipcmbeat


# Reference of Available Events

The events are aggregated in order to provide a reasonably abstracted
view of dialog status without details of transaction processing and
reliability handling. The following events are available:

- **call-attempt** -- a SIP INVITE was received that was responded with
  a negative answer (other than an authentication failure) or timed out
- **call-start** -- a SIP INVITE received a positive answer and 
  established a dialog
- **call-end** -- a SIP BYE terminated a call
- **reg-new** -- a new SIP contact was registered for an AoR; 
  re-registrations do not trigger an event
- **reg-del** -- a SIP contact was unregistered
- **reg-expired** -- a SIP contact expired
- **auth-failed** -- a SIP request failed to authentication
  (the initial 401/407 without credentials doesn't count, only
   the subsequent request does)
- **other-failed** -- a non-INVITE not covered by any of the above events,
 failed with a negative reply
- **other-timeout** - a non-INVITE not covered by any of the above events,
 failed due to timeout (no final reply received)
- **other-ok** - a non-INVITE not covered by any of the above events, succeeded
 (2xx reply)
- **parse-error** - a potential SIP message could not be parsed
- **msg-probe** - non-SIP message (too small or starting with non-ASCII)

Note that generation of any of the above events can be disabled via the config
 file (e.g. ```event_types_blst: ["msg-probe"]```) or at runtime (e.g.
 ```http://127.0.0.1:8080/events/blst```).

The event fields are documented [here](https://github.com/intuitivelabs/sipcmbeat/blob/master/docs/fields.asciidoc#sipcmbeat-fields).


# Features

* anonymization and encryption support for the relevant event fields

* event rate reporting and rate based blacklisting

* event type blacklisting support

* vxlan auto-decapsulation

* periodic statistics/counters events


# Known limitations

* The software collects data as "seen on the net" to produce events. 
  The events may be incomplete or entirely missing if:
  - traffic is encrypted
  - sipcmbeat hasn't been running or hasn't been running long enough.
    Attempts are made in this case to reconstruct dialog and registration
    state, produce as-complete-as-possible events from subsequent SIP 
    messages, and label the events as such. Specific examples
    include lack of INVITE elements (request URI) when only BYE 
    was received, or incomplete information abound bindings
    captured in REGISTER responses but not present in the 
    original REGISTERs.
* TCP and UDP are supported transport protocols, SCTP is not.
* The implementation supports RFC3261 and attempts to be rather
  true to the standard. Thus  events often contain hints to
  non-compliant SIP traffic as seen on the net so that users 
  can trust the events to fingerpoint at possible problems. 
  Notwithstanding that there are some empirical cases where 
  implementations are known not to implement the spec and 
  still work more or less normally. In such a cases we try 
  to avoid too many events even if it may conceal a problem 
  in the traffic. Examples of such include delayed 
  re-registrations (this happens unfortunately quite often 
  when server side enforces frequent re-registers), or failure 
  to keep constant Call-Id during re-registrations.


## Bugs and feature requests

If you are sure you found a bug or have a feature request, open an issue on
[Github](https://github.com/intuitivelabs/sipcmbeat/issues).

# Licensing

The source code in this repository is licensed either under the Intuitive Labs license (beater/sipcmbeat.go) or the Apache License Version 2.0.

The Intuitive Labs go modules on which sipcmbeat depends are licensed either
 under the Intuitive Labs license or a BSD-style license (see each module
  LICENSE.txt file).

The Intuitive Labs license permits use of source code for non-commercial
purposes. Commercial uses require a commercial license. See
[INTUITIVE_LABS-LICENSE.txt](./INTUITIVE_LABS-LICENSE.txt) for specific license
terms.


# Use of Open-source Software

[Beat Generator](https://github.com/elastic/beats)
 has been used under terms of Apache License
 (see https://www.elastic.co/guide/en/beats/devguide/current/newbeat-generate.html).

# Contributing

contributions are welcome and subject to Contributor Licence
Agreement (CLA).


# Getting Started with sipcmbeat

Ensure that this folder is at the following location:
`${GOPATH}/src/github.com/intuitivelabs/sipcmbeat/`

### Requirements

* [Golang](https://golang.org/dl/) 1.15

### Build

To build the binary for sipcmbeat run the command below. This will generate a
binary in the same directory with the name sipcmbeat.

```
make
```


### Run

To run sipcmbeat with debugging output enabled, run:

```
./sipcmbeat -c sipcmbeat.yml -e -d "*"
```

To overwrite some config file settings from the command line use
 -E sipcmbeat.\<option\_name\>, but make sure that the option is overwritten
 adter loading the config file: all the -E must come in the command line
 after -c config_file.yml.

```
./sipcmbeat -c sipcmbeat.yml -E sipcmbeat.event_buffer_size=1024
```

For replaying a pcap file, instead of live capture run:

```
./sipcmbeat -c sipcmbeat.yml ./sipcmbeat -c sipmcbeat_test.yml -E sipcmbeat.replay=true -E sipcmbeat.pcap="test.pcap" -E sipcmbeat.run_forever=true
```

To overwrite the output, writing the events to a file instead of sending
 them to ES, use something like:

```
./sipcmbeat -c sipcmbeat.yml -E output.logstash.enabled=false -E output.file.enabled=true -E output.file.path="/tmp" -E output.file.filename="sipcmbeat_log" -E output.file.pretty=true
```

### Config

* [Default Config](https://raw.githubusercontent.com/intuitivelabs/sipcmbeat/master/_meta/config/beat.reference.yml.tmpl)

### Built-in Web Interface

The built-in web interface runs by default on 127.0.0.1:8080. It can be
 disabled by setting  http\_port to 0 in the config.

Documentation for the web interface is available [here](https://github.com/intuitivelabs/sipcallmon/tree/master/cmd/sipcm#http-url-paths).

### Test

To test sipcmbeat, run the following command:

```
make testsuite
```

alternatively:
```
make unit-tests
make system-tests
make integration-tests
make coverage-report
```

The test coverage is reported in the folder `./build/coverage/`

### Update

Each beat has a template for the mapping in elasticsearch and a documentation for the fields
which is automatically generated based on `fields.yml` by running the following command.

```
make update
```


### Cleanup

To clean  sipcmbeat source code, run the following command:

```
make fmt
```

To clean up the build directory and generated artifacts, run:

```
make clean
```


### Clone

To clone sipcmbeat from the git repository, run the following commands:

```
mkdir -p ${GOPATH}/src/github.com/intuitivelabs/sipcmbeat
git clone https://github.com/intuitivelabs/sipcmbeat ${GOPATH}/src/github.com/intuitivelabs/sipcmbeat
```


For further development, check out the [beat developer guide](https://www.elastic.co/guide/en/beats/libbeat/current/new-beat.html).


## Packaging

The beat frameworks provides tools to crosscompile and package your beat for different platforms. This requires [docker](https://www.docker.com/) and vendoring as described above. To build packages of your beat, run the following command:

```
make release
```

This will fetch and create all images required for the build process. The whole process to finish can take several minutes.
