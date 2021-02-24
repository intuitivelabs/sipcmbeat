// Copyright 2019-2020 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a source-available license
// that can be found in the INTUITIVE_LABS-LICENSE.txt file in the
// root of the source tree.

package beater

import (
	"encoding/hex"
	"fmt"
	//	"strconv"
	"os"
	//	"runtime/pprof"
	"crypto/subtle"
	"net"
	"reflect"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/elastic/beats/v7/libbeat/logp"

	"github.com/pkg/errors"

	"github.com/intuitivelabs/anonymization"
	"github.com/intuitivelabs/calltr"
	"github.com/intuitivelabs/counters"
	"github.com/intuitivelabs/sipcallmon"
)

// FormatFlags defines event structure or field encoding flags.
type FormatFlags uint8

const FormatNoneF FormatFlags = iota

// rest of the flags starting from 1
const (
	FormatCltIPencF = (FormatFlags)(1) << iota
	FormatSrvIPencF
	FormatCallIDencF
	FormatURIencF
)

type statCounters struct {
	EvPub       counters.Handle
	EvSkipped   counters.Handle
	EvBusy      counters.Handle
	EvInvalid   counters.Handle
	EvNilTotal  counters.Handle
	EvNilConsec counters.Handle
	EvSigs      counters.Handle
	EvTrunc     counters.Handle
	EvErr       counters.Handle
	EvMaxQ      counters.Handle
}

type ackCounters struct {
	EvPubAdd        counters.Handle
	EvPubDropFilter counters.Handle
	EvPubAck        counters.Handle
	EvBatchAck      counters.Handle
}

type publishCounters struct {
	EvPubOk       counters.Handle
	EvPubFiltered counters.Handle
	EvPubDropped  counters.Handle
}

// returns a list of all struct tags.
// if no tag is found, the field name will be returned.
// sub-structs tags will be of the form parent.child.tag1
func getStructTags(t reflect.Type) []string {
	if t.Kind() != reflect.Struct {
		return nil
	}
	lst := make([]string, 0, 48)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("config")
		if len(tag) == 0 {
			tag = f.Name
		}
		lst = append(lst, tag)
		if f.Type.Kind() == reflect.Struct {
			chldTags := getStructTags(f.Type)
			for _, subtag := range chldTags {
				lst = append(lst, tag+"."+subtag)
			}
		}
	}
	return lst
}

// check if all the options loaded from the configuration correspond
// to defined config option.
// Returns the first unknown option or "" if all are defined.
func unknownCfgOption(cfg *common.Config) string {
	cfgFlds := cfg.GetFields()
	defFlds := getStructTags(reflect.TypeOf((*sipcallmon.Config)(nil)).Elem())
	//fmt.Printf("DBG: loaded cfg fields(%d): %v\n", len(cfgFlds), cfgFlds)
	//fmt.Printf("DBG: defined cfg fields(%d): %v\n", len(defFlds), defFlds)

cfg_val_chk:
	for _, o := range cfgFlds {
		for _, d := range defFlds {
			if o == d {
				// found
				continue cfg_val_chk
			}
		}
		return o
	}
	return ""
}

// implements beat.ACKer interface, used for counting publish ACK stats
type acker struct {
	// some counters
	stats counters.Group
	cnts  ackCounters
}

// AddEvent is part of beat.ACKer interface: called after the processors
// have handled the event (before output). If the event was dropped by
// a processor, `published` will be set to _false_ (contrary to the docs)
func (a acker) AddEvent(event beat.Event, published bool) {
	if published {
		a.stats.Inc(a.cnts.EvPubAdd)
	} else {
		a.stats.Inc(a.cnts.EvPubDropFilter)
	}
}

// ACKEvents is part of beat.ACKer interface: number of ACKed events from
// the output and the pipeline.
// (total events - processor dropped events?)
func (a acker) ACKEvents(n int) {
	a.stats.Add(a.cnts.EvPubAck, counters.Val(n))
	a.stats.Set(a.cnts.EvBatchAck, counters.Val(n))
}

// Close is part of beat.ACKer interface: informs that the client used to
// publish _to_ the pipeline has been closed (in our case sipcmbeat?)
func (a acker) Close() {
}

// implements beat.ClientEventer interface, used for various event publish
// stats (some of them are very similar fo the ones from acker)
type eventer struct {
	// some counters
	stats counters.Group
	cnts  publishCounters
}

// Closing implements the  beat.ClientEventer interface:
// indicates the client is being shutdown next
func (p eventer) Closing() {
}

// Closed implements the  beat.ClientEventer interface:
// indicates the client the client being fully shutdown
func (p eventer) Closed() {
}

// Published implements the  beat.ClientEventer interface:
// event has been successfully forwarded to the publisher pipeline
func (p eventer) Published() {
	p.stats.Inc(p.cnts.EvPubOk)
}

// FilteredOut implements the  beat.ClientEventer interface:
// event has been filtered out/dropped by processors
func (p eventer) FilteredOut(beat.Event) {
	p.stats.Inc(p.cnts.EvPubFiltered)
}

// DroppedOnPublish implements the  beat.ClientEventer interface:
// event has been dropped, while waiting for the queue
func (p eventer) DroppedOnPublish(beat.Event) {
	p.stats.Inc(p.cnts.EvPubDropped)
}

// Sipcmbeat configuration.
type Sipcmbeat struct {
	done     chan struct{}
	newEv    chan struct{}        // new events are signalled here
	evIdx    sipcallmon.EvRingIdx // curent position in the ring
	evRing   *sipcallmon.EvRing
	wg       *sync.WaitGroup
	Config   sipcallmon.Config
	ipcipher *anonymization.Ipcipher
	client   beat.Client
	// stats
	stats   counters.Group
	cnts    statCounters
	ackCnts ackCounters
	pubCnts publishCounters
}

/*
func dbg_fileno() uintptr {
	file, _ := os.Open("/dev/zero")
	fd := file.Fd()
	file.Close()
	return fd
}
*/
func (bt *Sipcmbeat) initCounters() error {
	cntDefs := [...]counters.Def{
		{&bt.cnts.EvPub, 0, nil, nil, "published",
			"events attempted to be published" +
				" (see also published_ack)"},
		{&bt.cnts.EvSkipped, 0, nil, nil, "skipped",
			"events skipped due to slow output"},
		{&bt.cnts.EvBusy, 0, nil, nil, "busy",
			"busy event entries"},
		{&bt.cnts.EvInvalid, 0, nil, nil, "invalid",
			"invalid events entries, skipped"},
		{&bt.cnts.EvNilTotal, 0, nil, nil, "nil_total",
			"empty events received (debugging)"},
		{&bt.cnts.EvNilConsec, counters.CntMaxF, nil, nil, "nil_crt",
			"empty events received consecutively (debugging)"},
		{&bt.cnts.EvSigs, 0, nil, nil, "signals",
			"new events signals received"},
		{&bt.cnts.EvTrunc, 0, nil, nil, "truncated",
			"truncated event"},
		{&bt.cnts.EvErr, 0, nil, nil, "error",
			"error preparing to send event"},
		{&bt.cnts.EvMaxQ, 0, nil, nil, "max_queued",
			"maximum number of queued events"},
		// acker based counters
		{&bt.ackCnts.EvPubAdd, 0, nil, nil, "publish_add",
			"events queued for transport/client publish"},
		{&bt.ackCnts.EvPubDropFilter, 0, nil, nil, "publish_filtered",
			"events filtered out by the output pipeline processors"},
		{&bt.ackCnts.EvPubAck, 0, nil, nil, "publish_ack",
			"events published and acknowledged"},
		{&bt.pubCnts.EvPubOk, 0, nil, nil, "publish_ok",
			"events published ok (should be equivalent to publish_ack)"},
		{&bt.ackCnts.EvBatchAck, counters.CntMaxF, nil, nil, "batch_acks",
			"event acks received in a batch (debugging)"},
		{&bt.pubCnts.EvPubFiltered, 0, nil, nil, "publish_filtered2",
			"events filtered out by the output pipeline processors" +
				" (debugging)"},
		{&bt.pubCnts.EvPubDropped, 0, nil, nil, "publish_dropped",
			"events dropped waiting to be sent"},
	}
	bt.stats.Init("events", nil, len(cntDefs))
	if !bt.stats.RegisterDefs(cntDefs[:]) {
		return errors.New("initCounters: failed to register stat counters")
	}
	return nil
}

func (bt *Sipcmbeat) initEncryption() error {
	var key [16]byte

	if len(bt.Config.EncryptionPassphrase) > 0 {
		// generate encryption key from passphrase
		anonymization.GenerateKeyFromPassphraseAndCopy(bt.Config.EncryptionPassphrase, key[:])
	} else {
		// copy the configured key into the one used during realtime processing
		if decoded, err := hex.DecodeString(bt.Config.EncryptionKey); err != nil {
			return err
		} else {
			subtle.ConstantTimeCopy(1, key[:], decoded)
		}
	}

	if ipcipher, err := anonymization.NewCipher(key[:]); err != nil {
		return err
	} else {
		bt.ipcipher = ipcipher.(*anonymization.Ipcipher)
	}

	return nil
}

// New creates an instance of sipcmbeat.
func New(b *beat.Beat, cfg *common.Config) (beat.Beater, error) {
	c := sipcallmon.GetDefaultCfg()
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

	if o := unknownCfgOption(cfg); o != "" {
		return nil, fmt.Errorf("Unknown config option: %q", o)
	}

	bt := &Sipcmbeat{
		done:   make(chan struct{}),
		newEv:  make(chan struct{}, 512),
		Config: c,
		wg:     &sync.WaitGroup{},
	}

	if err := sipcallmon.Init(&bt.Config); err != nil {
		return nil, fmt.Errorf("sipcallmon: %v", err)
	}
	bt.evRing = &sipcallmon.EventsRing
	bt.evRing.SetEvSignal(bt.newEv)
	if bt.Config.UseAnonymization() {
		if err := bt.initEncryption(); err != nil {
			return nil, fmt.Errorf("Invalid configuration for encryption: %v", err)
		}
	}
	if err := bt.initCounters(); err != nil {
		return nil, errors.WithMessage(err, "sipcmbeat.New")
	}
	return bt, nil
}

// Run starts sipcmbeat.
func (bt *Sipcmbeat) Run(b *beat.Beat) error {
	logp.Info("sipcmbeat is running! Hit CTRL-C to stop it.")

	var err error

	// init ack "callback interface"
	ackH := acker{
		stats: bt.stats, // same counter group
		cnts:  bt.ackCnts,
	}
	pubEventsH := eventer{
		stats: bt.stats, // same counter group
		cnts:  bt.pubCnts,
	}

	// beats config for creating the pipelin
	clientCfg := beat.ClientConfig{
		// possible values: DefaultGuarantees, OutputChooses,
		// GuaranteedSend, DropIfFull
		PublishMode: beat.GuaranteedSend, // retry unitl ACK
		// WaitClose: max duration to wait for an ACK (req. some ACK cfg opt)
		ACKHandler: ackH,
		Events:     pubEventsH,
	}
	bt.client, err = b.Publisher.ConnectWith(clientCfg)
	if err != nil {
		return err
	}
	//f, err := os.Create("cpuprofile")
	//pprof.StartCPUProfile(f)
	bt.wg.Add(1)
	go bt.consumeEv()
	err = sipcallmon.Run(&bt.Config)
	if err != nil {
		return err
	}
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
			bt.stats.Inc(bt.cnts.EvSigs)
			last := bt.evRing.LastIdx()
			bt.stats.Max(bt.cnts.EvMaxQ, counters.Val(last-bt.evIdx))
			for bt.evIdx != last {
				ev, nxtIdx, err := bt.evRing.Get(bt.evIdx)
				if ev != nil {
					bt.publishEv(ev)
					bt.evRing.Put(bt.evIdx)
					if bt.stats.Get(bt.cnts.EvNilConsec) != 0 {
						fmt.Fprintf(os.Stderr, "recovered from NIL ev[%d]: %p (last %d:%d) - %d cycles\n",
							bt.evIdx, ev, last, bt.evRing.LastIdx(),
							bt.stats.Get(bt.cnts.EvNilConsec))
					}
					bt.stats.Set(bt.cnts.EvNilConsec, 0)
					bt.evIdx = nxtIdx
				} else {
					bt.stats.Inc(bt.cnts.EvNilTotal)
					if bt.stats.Inc(bt.cnts.EvNilConsec) == 1 {
						fmt.Fprintf(os.Stderr, "GOT NIL ev[%d]: %p err %d (last %d:%d)\n",
							bt.evIdx, ev, err, last, bt.evRing.LastIdx())
					}
					switch err {
					case sipcallmon.ErrBusy:
						// busy (written on), wait for it (next signal)
						bt.stats.Inc(bt.cnts.EvBusy)
						continue waitsig
					case sipcallmon.ErrOutOfRangeLow:
						skipped := nxtIdx - bt.evIdx
						fmt.Fprintf(os.Stderr, "WARNING: missed %d events"+
							" (%d:%d:%d)\n",
							skipped, bt.evIdx, last, bt.evRing.LastIdx())
						bt.stats.Add(bt.cnts.EvSkipped, counters.Val(skipped))
					case sipcallmon.ErrOutOfRangeHigh:
						fmt.Fprintf(os.Stderr, "ERROR: ouf of range high:"+
							" (%d:%d:%d, nxt: %d)\n",
							bt.evIdx, last, bt.evRing.LastIdx(), nxtIdx)
					case sipcallmon.ErrLast:
						fmt.Fprintf(os.Stderr, "ERROR: ring end:"+
							" (%d:%d:%d, nxt: %d)\n",
							bt.evIdx, last, bt.evRing.LastIdx(), nxtIdx)
					case sipcallmon.ErrInvalid:
						// just ignore it
						bt.stats.Inc(bt.cnts.EvInvalid)
					default:
						fmt.Fprintf(os.Stderr, "BUG: error %d not handled"+
							" (%d:%d/%d, nxt: %d)\n",
							err, bt.evIdx, last, bt.evRing.LastIdx(), nxtIdx)
					}
					if (bt.evIdx + 1) != nxtIdx {
						// skipped some indexes, make sure last is updated
						// (in case nxtIdx point past the original "last")
						// Note:  could be moved to the above error checks
						//        for ErrOutOfRangeHigh and ErrLast
						last = bt.evRing.LastIdx()
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
		bt.stats.Inc(bt.cnts.EvTrunc)
	}
	// It looks like even.Publish() does not copy all the strings passed to it
	// and returns before consuming/building the message => all the strings
	// will point to a buffer that will be reused...
	//We copy here the event data, just in case.

	var ed calltr.EventData
	ed.Init(make([]byte, srcEv.Used)) // alloc a new buffer
	if !ed.Copy(srcEv) {
		logp.Err("ERROR: event copy failed (%d bytes)...\n", srcEv.Used)
		bt.stats.Inc(bt.cnts.EvErr)
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
	var fFlags FormatFlags
	addFields(event.Fields, "sip.call_id", str(ed.CallID.Get(ed.Buf)))
	for i := 0; i < len(ed.Attrs); i++ {
		if !ed.Attrs[i].Empty() {
			ok := addFields(event.Fields, calltr.CallAttrIdx(i).String(),
				str(ed.Attrs[i].Get(ed.Buf)))
			if !ok {
				logp.Err("failed to add %q to Fields\n",
					calltr.CallAttrIdx(i).String())
				bt.stats.Inc(bt.cnts.EvErr)
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
	case calltr.EvCallAttempt, calltr.EvCallStart, calltr.EvRegNew, calltr.EvSubNew, calltr.EvAuthFailed:
		addFields(event.Fields, "sip.response.status", ed.ReplStatus)
	default:
		addFields(event.Fields, "sip.response.last", ed.ReplStatus)
	}
	addFields(event.Fields, "event.call_start", ed.StartTS)
	addFields(event.Fields, "client.transport", ed.ProtoF.ProtoName())
	if bt.Config.UseIPAnonymization() {
		c := make([]byte, len(ed.Src))
		bt.ipcipher.Encrypt(c, ed.Src)
		addFields(event.Fields, "client.ip", net.IP(c[:]))
		fFlags |= FormatCltIPencF
	} else {
		addFields(event.Fields, "client.ip", ed.Src)
	}
	addFields(event.Fields, "client.port", ed.SPort)
	if bt.Config.UseIPAnonymization() {
		c := make([]byte, len(ed.Dst))
		bt.ipcipher.Encrypt(c, ed.Dst)
		addFields(event.Fields, "server.ip", net.IP(c[:]))
		fFlags |= FormatSrvIPencF
	} else {
		addFields(event.Fields, "server.ip", ed.Dst)
	}
	addFields(event.Fields, "server.port", ed.DPort)
	// rate
	addFields(event.Fields, "rate.exceeded", ed.Rate.ExCnt)
	addFields(event.Fields, "rate.ex_diff", ed.Rate.ExCntDiff)
	addFields(event.Fields, "rate.crt", ed.Rate.Rate)
	addFields(event.Fields, "rate.lim", ed.Rate.MaxR)
	// rate.period is stored in milliseconds (epoch_millis)
	addFields(event.Fields, "rate.period", ed.Rate.Intvl.Nanoseconds()/1000000)
	addFields(event.Fields, "rate.since", ed.Rate.T)
	addFields(event.Fields, "rate.key", ed.Type.String()+":"+ed.Src.String())

	// dbg
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

	addFields(event.Fields, "fflags", fFlags)

	bt.client.Publish(event)
	bt.stats.Inc(bt.cnts.EvPub)
	//	logp.Info("Event sent")
}
