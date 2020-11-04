// // Copyright 2019-2020 Intuitive Labs GmbH. All rights reserved.
// //
// // Use of this source code is governed by source-available license
// // that can be found in the LICENSE file in the root of the source
// // tree.

package beater

import (
	"fmt"
	//	"strconv"
	"os"
	//	"runtime/pprof"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/elastic/beats/v7/libbeat/logp"

	"github.com/intuitivelabs/calltr"
	"github.com/intuitivelabs/counters"
	"github.com/intuitivelabs/sipcallmon"
)

/*
type CounterIdx uint8

const (
	CntEvSkipped CounterIdx = iota
	CntEvNil
	CntEvSigs
	CntLast
)

type counters []expvar.Int
*/

// stats
var stats counters.Group

var cntEvPub counters.Handle
var cntEvSkipped counters.Handle
var cntEvBusy counters.Handle
var cntEvInvalid counters.Handle
var cntEvNilTotal counters.Handle
var cntEvNilConsec counters.Handle
var cntEvSigs counters.Handle
var cntEvTrunc counters.Handle
var cntEvErr counters.Handle
var cntEvMaxQ counters.Handle

func init() {
	cntDefs := [...]counters.Def{
		{&cntEvPub, 0, nil, nil, "published",
			"events sent/published"},
		{&cntEvSkipped, 0, nil, nil, "skipped",
			"events skipped due to slow output"},
		{&cntEvBusy, 0, nil, nil, "busy",
			"busy event entries"},
		{&cntEvInvalid, 0, nil, nil, "invalid",
			"invalid events entries, skipped"},
		{&cntEvNilTotal, 0, nil, nil, "nil_total",
			"emtpy events received (debugging)"},
		{&cntEvNilConsec, counters.CntMaxF, nil, nil, "nil_crt",
			"emtpy events received consecutively (debugging)"},
		{&cntEvSigs, 0, nil, nil, "signals",
			"new events signals received"},
		{&cntEvTrunc, 0, nil, nil, "truncated",
			"truncated event"},
		{&cntEvErr, 0, nil, nil, "error",
			"error preparing to send event"},
		{&cntEvMaxQ, 0, nil, nil, "max_queued",
			"maximum number of queued events"},
	}
	stats.Init("events", nil, len(cntDefs))
	if !stats.RegisterDefs(cntDefs[:]) {
		panic("failed to register stat counters")
	}
}

// Sipcmbeat configuration.
type Sipcmbeat struct {
	done   chan struct{}
	newEv  chan struct{}        // new events are signalled here
	evIdx  sipcallmon.EvRingIdx // curent position in the ring
	evRing *sipcallmon.EvRing
	wg     *sync.WaitGroup
	Config sipcallmon.Config
	client beat.Client
}

/*
func dbg_fileno() uintptr {
	file, _ := os.Open("/dev/zero")
	fd := file.Fd()
	file.Close()
	return fd
}
*/

// New creates an instance of sipcmbeat.
func New(b *beat.Beat, cfg *common.Config) (beat.Beater, error) {
	c := sipcallmon.DefaultConfig
	if c.MaxBlockedTo > 5*time.Second {
		c.MaxBlockedTo = 5 * time.Second // lower timeout to see Stop() sooner
	}
	if cfg != nil {
		if err := cfg.Unpack(&c); err != nil {
			return nil, fmt.Errorf("Error reading config file: %v", err)
		}
	}
	if err := sipcallmon.CfgCheck(&c); err != nil && cfg != nil {
		return nil, fmt.Errorf("Invalid Config: %v", err)
	}

	bt := &Sipcmbeat{
		done:   make(chan struct{}),
		newEv:  make(chan struct{}, 512),
		Config: c,
		wg:     &sync.WaitGroup{},
	}
	bt.evRing = &sipcallmon.EventsRing
	bt.evRing.Init(bt.Config.EvBufferSz)
	bt.evRing.SetEvSignal(bt.newEv)
	return bt, nil
}

// Run starts sipcmbeat.
func (bt *Sipcmbeat) Run(b *beat.Beat) error {
	logp.Info("sipcmbeat is running! Hit CTRL-C to stop it.")

	var err error
	bt.client, err = b.Publisher.Connect()
	if err != nil {
		return err
	}
	//f, err := os.Create("cpuprofile")
	//pprof.StartCPUProfile(f)
	bt.wg.Add(1)
	go bt.consumeEv()
	sipcallmon.Run(&bt.Config)
	//pprof.StopCPUProfile()
	return nil
}

// Stop stops sipcmbeat.
func (bt *Sipcmbeat) Stop() {

	sipcallmon.Stop()
	if bt.client != nil {
		bt.client.Close()
		bt.client = nil
	}
	close(bt.done)
	if bt.wg != nil {
		bt.wg.Wait()
		bt.wg = nil
	}
	bt.evRing.CloseEvSignal() // safe, since sipcallmon is already stopped
}

func (bt *Sipcmbeat) consumeEv() {
	defer bt.wg.Done()
waitsig:
	for {
		select {
		case <-bt.done:
			return
		case <-bt.newEv:
			stats.Inc(cntEvSigs)
			last := bt.evRing.LastIdx()
			stats.Max(cntEvMaxQ, counters.Val(last-bt.evIdx))
			for bt.evIdx != last {
				ev, nxtIdx, err := bt.evRing.Get(bt.evIdx)
				if ev != nil {
					bt.publishEv(ev)
					bt.evRing.Put(bt.evIdx)
					if stats.Get(cntEvNilConsec) != 0 {
						fmt.Fprintf(os.Stderr, "recovered from NIL ev[%d]: %p (last %d:%d) - %d cycles\n",
							bt.evIdx, ev, last, bt.evRing.LastIdx(),
							stats.Get(cntEvNilConsec))
					}
					stats.Set(cntEvNilConsec, 0)
					bt.evIdx = nxtIdx
				} else {
					stats.Inc(cntEvNilTotal)
					if stats.Inc(cntEvNilConsec) == 1 {
						fmt.Fprintf(os.Stderr, "GOT NIL ev[%d]: %p err %d (last %d:%d)\n",
							bt.evIdx, ev, err, last, bt.evRing.LastIdx())
					}
					switch err {
					case sipcallmon.ErrBusy:
						// busy (written on), wait for it (next signal)
						stats.Inc(cntEvBusy)
						continue waitsig
					case sipcallmon.ErrOutOfRange:
						skipped := nxtIdx - bt.evIdx
						fmt.Fprintf(os.Stderr, "WARNING: missed %d events"+
							" (%d:%d:%d)\n",
							skipped, bt.evIdx, last, bt.evRing.LastIdx())
						stats.Add(cntEvSkipped, counters.Val(skipped))
					case sipcallmon.ErrInvalid:
						// just ignore it
						stats.Inc(cntEvInvalid)
					}
					bt.evIdx = nxtIdx
				}
			}
		}
	}
}

//quick hack to avoid copying
func str(b []byte) (s string) {
	s = *(*string)(unsafe.Pointer(&b))
	return
}

// break dot separated label into multiple maps keys
// e.g.: sip.call_id => m[sip][call_id] = ...
func addFields(m common.MapStr, label string, val interface{}) bool {
	keys := strings.Split(label, ".")
	i := 0
	for ; i < len(keys)-1; i++ {
		if n, ok := m[keys[i]]; ok {
			if t, ok := n.(common.MapStr); ok {
				m = t
			} else {
				return false
			}
		} else {
			n := make(common.MapStr)
			m[keys[i]] = n
			m = n
		}
	}
	m[keys[i]] = val
	return true
}

func (bt *Sipcmbeat) publishEv(srcEv *calltr.EventData) {
	if bt.client == nil { // dev null
		return
	}
	if srcEv.Truncated {
		stats.Inc(cntEvTrunc)
	}
	// It looks like even.Publish() does not copy all the strings passed to it
	// and returns before consuming/building the message => all the strings
	// will point to a buffer that will be reused...
	//We copy here the event data, just in case.

	var ed calltr.EventData
	ed.Init(make([]byte, srcEv.Used)) // alloc a new buffer
	if !ed.Copy(srcEv) {
		logp.Err("ERROR: event copy failed (%d bytes)...\n", srcEv.Used)
		stats.Inc(cntEvErr)
		return
	}

	event := beat.Event{
		Timestamp: time.Now(),
		Fields: common.MapStr{
			//"type":    b.Info.Name,
			"type": ed.Type.String(),
			//		"sip.call_id": str(ed.CallID.Get(ed.Buf)),
		},
	}
	addFields(event.Fields, "sip.call_id", str(ed.CallID.Get(ed.Buf)))
	for i := 0; i < len(ed.Attrs); i++ {
		if !ed.Attrs[i].Empty() {
			ok := addFields(event.Fields, calltr.CallAttrIdx(i).String(),
				str(ed.Attrs[i].Get(ed.Buf)))
			if !ok {
				logp.Err("failed to add %q to Fields\n",
					calltr.CallAttrIdx(i).String())
				stats.Inc(cntEvErr)
			}
			//	event.Fields[calltr.CallAttrIdx(i).String()] =
			//		str(ed.Attrs[i].Get(ed.Buf))
		}
	}
	// some fields are added only to some events: handle this below
	switch ed.Type {
	case calltr.EvCallEnd:
		// add duration only on events that make sense, and only
		// if call-start is known. Use seconds.
		if !ed.StartTS.IsZero() {
			// (otherwise the current monitoring part will get confused)
			addFields(event.Fields, "event.duration",
				ed.TS.Sub(ed.StartTS)/time.Second)
		} else {
			// add a min_length field containing the minimum call duration^
			addFields(event.Fields, "event.min_length",
				ed.TS.Sub(sipcallmon.StartTS)/time.Second)
		}
		if ed.ReplStatus == 0 {
			// created by a BYE, no INVITE seen (no call-start)
			addFields(event.Fields, "sip.unmatched_invite", true)
		} else {
			// for CallEnd we do not add sip.response.status
			addFields(event.Fields, "sip.response.last", ed.ReplStatus)
		}
	case calltr.EvRegDel, calltr.EvRegExpired, calltr.EvSubDel:
		// add duration only on events that make sense, and only
		// if call-start is known. Use seconds.
		if !ed.StartTS.IsZero() {
			// (otherwise the current monitoring part will get confused)
			addFields(event.Fields, "event.lifetime",
				ed.TS.Sub(ed.StartTS)/time.Second)
		} else {
			// add a min_length field containing the minimum call duration^
			addFields(event.Fields, "event.min_lifetime",
				ed.TS.Sub(sipcallmon.StartTS)/time.Second)
		}
		addFields(event.Fields, "sip.response.last", ed.ReplStatus)
	case calltr.EvCallAttempt, calltr.EvCallStart, calltr.EvRegNew, calltr.EvSubNew:
		addFields(event.Fields, "sip.response.status", ed.ReplStatus)
	default:
		addFields(event.Fields, "sip.response.last", ed.ReplStatus)
	}
	addFields(event.Fields, "event.call_start", ed.StartTS)
	addFields(event.Fields, "client.transport", ed.ProtoF.ProtoName())
	addFields(event.Fields, "client.ip", ed.Src)
	addFields(event.Fields, "client.port", ed.SPort)
	addFields(event.Fields, "server.ip", ed.Dst)
	addFields(event.Fields, "server.port", ed.DPort)
	addFields(event.Fields, "dbg.state", ed.State.String())
	addFields(event.Fields, "dbg.prev_state", ed.PrevState.String())
	addFields(event.Fields, "dbg.fromtag", str(ed.FromTag.Get(ed.Buf)))
	addFields(event.Fields, "dbg.totag", str(ed.ToTag.Get(ed.Buf)))
	addFields(event.Fields, "dbg.lastev", ed.LastEv.String())
	addFields(event.Fields, "dbg.evflags", ed.EvFlags.String())
	addFields(event.Fields, "dbg.evgen", ed.EvGen.String())
	addFields(event.Fields, "dbg.created", ed.CreatedTS)
	addFields(event.Fields, "dbg.call_start", ed.StartTS)
	addFields(event.Fields, "dbg.cseq", ed.CSeq)
	addFields(event.Fields, "dbg.rcseq", ed.RCSeq)
	addFields(event.Fields, "dbg.forked", ed.ForkedTS)
	addFields(event.Fields, "dbg.call_flags", ed.CFlags)
	addFields(event.Fields, "dbg.req_no", ed.Reqs)
	addFields(event.Fields, "dbg.repl_no", ed.Repls)
	addFields(event.Fields, "dbg.req_retr", ed.ReqsRetr)
	addFields(event.Fields, "dbg.repl_retr", ed.ReplsRetr)
	addFields(event.Fields, "dbg.last_method", ed.LastMethod)
	addFields(event.Fields, "dbg.last_status", ed.LastStatus)
	addFields(event.Fields, "dbg.msg_trace", ed.LastMsgs.String())

	bt.client.Publish(event)
	stats.Inc(cntEvPub)
	//	logp.Info("Event sent")
}
