// Copyright 2019-2020 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a source-available license
// that can be found in the INTUITIVE_LABS-LICENSE.txt file in the
// root of the source tree.

package beater

import (
	"encoding/hex"
	"fmt"
	"strconv"
	//	"runtime/pprof"
	"crypto"
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
	"github.com/intuitivelabs/sipsp"
	"github.com/intuitivelabs/slog"
	//	"github.com/intuitivelabs/timestamp"
)

// FormatFlags defines event structure or field encoding flags.
type FormatFlags uint16

const FormatNoneF FormatFlags = iota

// rest of the flags starting from 1
const (
	FormatCltIPencF = (FormatFlags)(1) << iota
	FormatSrvIPencF
	FormatCallIDencF
	FormatURIencF
	FormatReasonAencF    // reason attr. is enc
	FormatCountryISOencF // country iso is enc
	FormatCityIDencF     // city id is enc
	FormatUAencF         // user-agent is enc
	FormatIpcipherF      // ipcipher is used
)

type statCounters struct {
	EvPub       counters.Handle
	EvStats     counters.Handle
	EvSkipped   counters.Handle
	EvBusy      counters.Handle
	EvInvalid   counters.Handle
	EvNilTotal  counters.Handle
	EvNilConsec counters.Handle
	EvSigs      counters.Handle
	EvTrunc     counters.Handle
	EvPreDrop   counters.Handle
	EvErr       counters.Handle
	EvCallIDErr counters.Handle
	EvEInfoErr  counters.Handle
	EvAttrErr   [calltr.AttrLast]counters.Handle // error adding field/attr
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

// "local" log, default options (pre-config)
var Log slog.Log = slog.New(slog.LERR, slog.LbackTraceS|slog.LlocInfoS,
	slog.LStdErr)

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
func unknownCfgOption(cfg *common.Config, c *sipcallmon.Config) string {
	for _, ign := range c.CfgIgnUnknown {
		if strings.EqualFold(ign, "all") {
			return ""
		}
	}
	cfgFlds := cfg.GetFields()
	defFlds := getStructTags(reflect.TypeOf((*sipcallmon.Config)(nil)).Elem())

cfg_val_chk:
	for _, o := range cfgFlds {
		for _, d := range defFlds {
			if o == d {
				// found
				continue cfg_val_chk
			}
		}
		for _, ign := range c.CfgIgnUnknown {
			if o == ign {
				// found, safe to ignore
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
	done   chan struct{}
	newEv  chan struct{}        // new events are signalled here
	evIdx  sipcallmon.EvRingIdx // curent position in the ring
	evRing *sipcallmon.EvRing
	wg     *sync.WaitGroup

	pStatsInfo publishStatsInfo // stats event publish internal stuff

	Config    sipcallmon.Config
	ipcipher  *anonymization.Ipcipher
	validator anonymization.Validator
	client    beat.Client
	// stats
	stats   counters.Group
	cnts    statCounters
	ackCnts ackCounters
	pubCnts publishCounters
	// geoip
	geoipStats   geoipDbStats
	geoipCnts    geoipCounters
	geoipChgLock sync.Mutex // lock held when updating or getting geoipH
	geoipH       *GeoIPdbHandle
	// version info
	BaseVer string
	LongVer string
	CommitH string
	BuildT  string
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
			"total events attempted to be published" +
				" (see also published_ack)"},
		{&bt.cnts.EvStats, 0, nil, nil, "stats",
			"statistics events attempted to be published" +
				" (part of \"published\")"},
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
		{&bt.cnts.EvMaxQ, 0, nil, nil, "max_queued",
			"maximum number of queued events"},
		// acker based counters
		{&bt.ackCnts.EvPubAdd, 0, nil, nil, "publish_add",
			"events that passed the output pipeline processors filters"},
		{&bt.ackCnts.EvPubDropFilter, 0, nil, nil, "publish_filtered",
			"events filtered out by the output pipeline processors"},
		{&bt.ackCnts.EvPubAck, 0, nil, nil, "publish_ack",
			"events published and acknowledged by the remote side"},
		{&bt.pubCnts.EvPubOk, 0, nil, nil, "publish_ok",
			"events successfully forwarded to the publisher pipeline" +
				" (should be the same as publish_add)"},
		{&bt.ackCnts.EvBatchAck, counters.CntMaxF, nil, nil, "batch_acks",
			"event acks received in a batch (debugging)"},
		{&bt.pubCnts.EvPubFiltered, 0, nil, nil, "publish_filtered2",
			"events filtered out by the output pipeline processors" +
				" (debugging)"},
		{&bt.pubCnts.EvPubDropped, 0, nil, nil, "publish_dropped",
			"events dropped waiting for the queue (libbeat)"},
		// drop & error counters
		{&bt.cnts.EvPreDrop, 0, nil, nil, "pre_drop",
			"events dropped before attempting to publish them (blst a.s.o)"},
		{&bt.cnts.EvErr, 0, nil, nil, "error",
			"critical error preparing to send event, event dropped"},
		{&bt.cnts.EvCallIDErr, 0, nil, nil, "err_callid",
			"error adding the sip.call_id field to the event"},
		{&bt.cnts.EvEInfoErr, 0, nil, nil, "err_einfo",
			"error adding the err_info field to the event"},
	}
	bt.stats.Init("events", nil, len(cntDefs)+len(bt.cnts.EvAttrErr))
	if !bt.stats.RegisterDefs(cntDefs[:]) {
		return errors.New("initCounters: failed to register stat counters")
	}
	// register the counters for EvAttrErr (error adding attr to ev. field)
	for i := 0; i < len(bt.cnts.EvAttrErr); i++ {
		aName := calltr.CallAttrIdx(i).String()
		cName := "err_" + strings.ReplaceAll(aName, ".", "_")
		_, ok := bt.stats.RegisterDef(
			&counters.Def{&bt.cnts.EvAttrErr[i], 0, nil, nil,
				cName,
				"error adding the " + aName + " field/attr to the event"})
		if !ok {
			return fmt.Errorf("failed to register stat counter %q", cName)
		}
	}
	return bt.initGeoIPcounters()
}

// lookup key into the keystore and return the associated value or error
func keystoreVal(b *beat.Beat, key string) (string, error) {
	ks := b.Keystore
	if ks == nil {
		return "", errors.New("keystoreVal: un-intialized keystore")
	}
	ss, err := ks.Retrieve(key)
	if err != nil {
		return "", errors.WithMessage(err,
			"keystoreVal: failed to retrieve keystore key "+key)
	}
	secret, err := ss.Get()
	if err != nil {
		return "", errors.WithMessage(err,
			"keystoreVal:  failed to get secure string")
	}
	return string(secret), nil
}

func (bt *Sipcmbeat) initEncryption(b *beat.Beat) error {
	var encKey [anonymization.EncryptionKeyLen]byte
	var authKey [anonymization.AuthenticationKeyLen]byte
	const ksPrefix = "keystore:"

	salt := bt.Config.EncryptionValSalt
	if len(salt) == 0 {
		return errors.New("initEncryption: \"encryption_salt\" for" +
			" password validation is missing")
	} else if len(salt) >= len(ksPrefix) && strings.HasPrefix(salt, ksPrefix) {
		// starts with the keystore prefix -> look for the salt in the
		// keystore
		var err error
		ksSalt := salt[len(ksPrefix):]
		if salt, err = keystoreVal(b, ksSalt); err != nil {
			return errors.WithMessage(err, "initEncryption: salt")
		}
	}
	if len(bt.Config.EncryptionPassphrase) > 0 {
		// generate encryption key from passphrase
		pass := bt.Config.EncryptionPassphrase
		if len(pass) >= len(ksPrefix) && strings.HasPrefix(pass, ksPrefix) {
			// starts with the keystore prefix -> look for the pass in the
			// keystore
			var err error
			ksKey := pass[len(ksPrefix):]
			if pass, err = keystoreVal(b, ksKey); err != nil {
				return errors.WithMessage(err, "initEncryption: passphrase")
			}
		}
		anonymization.GenerateKeyFromPassphraseAndCopy(pass,
			anonymization.EncryptionKeyLen, encKey[:])
	} else {
		cfgKey := bt.Config.EncryptionKey
		if len(cfgKey) >= len(ksPrefix) && strings.HasPrefix(cfgKey, ksPrefix) {
			// starts with the keystore prefix -> look for the key in the
			// keystore
			var err error
			ksKey := cfgKey[len(ksPrefix):]
			if cfgKey, err = keystoreVal(b, ksKey); err != nil {
				return errors.WithMessage(err, "initEncryption: key")
			}
		}
		// copy the configured key into the one used during realtime processing
		if decoded, err := hex.DecodeString(cfgKey); err != nil {
			return err
		} else {
			subtle.ConstantTimeCopy(1, encKey[:], decoded)
		}
	}

	// generate authentication (HMAC) key from encryption key
	anonymization.GenerateKeyFromBytesAndCopy(encKey[:], anonymization.AuthenticationKeyLen, authKey[:])
	// validation code is the first 5 bytes of HMAC(SHA256) of random nonce; each thread needs its own validator!
	if validator, err := anonymization.NewKeyValidator(crypto.SHA256, authKey[:],
		5 /*length*/, salt, anonymization.NonceNone, false /*withNonce*/, true /*pre-allocated HMAC*/); err != nil {
		return err
	} else {
		bt.validator = validator
	}

	if ipcipher, err := anonymization.NewCipher(encKey[:]); err != nil {
		return err
	} else {
		bt.ipcipher = ipcipher.(*anonymization.Ipcipher)
	}

	// initialize the IP Prefix-preserving anonymization
	_ = anonymization.NewPanIPv4(encKey[:])

	// initialize the URI CBC based encryption
	anonymization.InitUriKeysFromMasterKey(encKey[:])
	_ = anonymization.NewUriCBC(anonymization.GetUriKeys())

	// initialize the Call-ID CBC based encryption
	anonymization.InitCallIdKeysFromMasterKey(encKey[:])
	_ = anonymization.NewCallIdCBC(anonymization.GetCallIdKeys())

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
	if err := sipcallmon.CfgFix(&c); err != nil && cfg != nil {
		return nil, fmt.Errorf("Invalid Config: error fixing: %v", err)
	}
	if err := sipcallmon.CfgCheck(&c); err != nil && cfg != nil {
		return nil, fmt.Errorf("Invalid Config: %v", err)
	}

	if o := unknownCfgOption(cfg, &c); o != "" {
		return nil, fmt.Errorf("Unknown config option: %q", o)
	}

	bt := &Sipcmbeat{
		done:    make(chan struct{}),
		newEv:   make(chan struct{}, 512),
		Config:  c,
		wg:      &sync.WaitGroup{},
		BaseVer: CanonVersion(),
		LongVer: CrtVersion(),
		CommitH: CommitId(),
		BuildT:  BuildTime(),
	}

	// init sipcmbeat logging
	slog.Init(&Log, slog.LogLevel(c.LogLev), slog.LogOptions(c.LogOpt),
		slog.LStdErr)

	if err := sipcallmon.Init(&bt.Config); err != nil {
		return nil, fmt.Errorf("sipcallmon: %v", err)
	}
	bt.evRing = &sipcallmon.EventsRing
	bt.evRing.SetEvSignal(bt.newEv)
	if bt.Config.UseAnonymization() {
		if err := bt.initEncryption(b); err != nil {
			return nil, fmt.Errorf("Invalid configuration for encryption: %v", err)
		}
	}
	if err := bt.initCounters(); err != nil {
		return nil, errors.WithMessage(err, "sipcmbeat.New")
	}

	if bt.Config.GeoIPdb != "" {
		if err := bt.openGeoIPh(bt.Config.GeoIPdb); err != nil {
			return nil, fmt.Errorf("failed to open geoip db %q: %w\n",
				bt.Config.GeoIPdb, err)
		}
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
	bt.initStatsGrps()
	if bt.pStatsInfo.statsTick > 0 {
		bt.pStatsInfo.statsT = time.NewTicker(bt.pStatsInfo.statsTick)
	} else {
		// init with null channel (== disabled)
		bt.pStatsInfo.statsT = &time.Ticker{}
	}
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
	if bt.pStatsInfo.statsT != nil && bt.pStatsInfo.statsT.C != nil {
		bt.pStatsInfo.statsT.Stop()
	}
	if bt.wg != nil {
		bt.wg.Wait()
		bt.wg = nil
	}
	bt.evRing.CloseEvSignal() // safe, since sipcallmon is already stopped
}

func (bt *Sipcmbeat) consumeEv() {
	defer bt.wg.Done()
	geoipHandle := bt.getGeoIPh()
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
					// update geoip handle (if needed)
					geoipHandle = bt.updateGeoIPh(geoipHandle)
					bt.publishEv(geoipHandle, ev)
					bt.evRing.Put(bt.evIdx)
					if bt.stats.Get(bt.cnts.EvNilConsec) != 0 {
						if Log.INFOon() {
							Log.INFO("recovered from NIL ev[%d]:"+
								" %p (last %d:%d) - %d cycles\n",
								bt.evIdx, ev, last, bt.evRing.LastIdx(),
								bt.stats.Get(bt.cnts.EvNilConsec))
						}
					}
					bt.stats.Set(bt.cnts.EvNilConsec, 0)
					bt.evIdx = nxtIdx
				} else {
					bt.stats.Inc(bt.cnts.EvNilTotal)
					if bt.stats.Inc(bt.cnts.EvNilConsec) == 1 {
						if Log.INFOon() {
							Log.INFO("GOT NIL ev[%d]:"+
								" %p err %d (last %d:%d)\n",
								bt.evIdx, ev, err, last, bt.evRing.LastIdx())
						}
					}
					switch err {
					case sipcallmon.ErrBusy:
						// busy (written on), wait for it (next signal)
						bt.stats.Inc(bt.cnts.EvBusy)
						continue waitsig
					case sipcallmon.ErrOutOfRangeLow:
						skipped := nxtIdx - bt.evIdx
						if Log.DBGon() {
							Log.DBG("missed %d events (%d:%d:%d)\n",
								skipped, bt.evIdx, last, bt.evRing.LastIdx())
						}
						bt.stats.Add(bt.cnts.EvSkipped, counters.Val(skipped))
					case sipcallmon.ErrOutOfRangeHigh:
						Log.ERR("ouf of range high: (%d:%d:%d, nxt: %d)\n",
							bt.evIdx, last, bt.evRing.LastIdx(), nxtIdx)
					case sipcallmon.ErrLast:
						Log.ERR("ring end: (%d:%d:%d, nxt: %d)\n",
							bt.evIdx, last, bt.evRing.LastIdx(), nxtIdx)
					case sipcallmon.ErrInvalid:
						// just ignore it
						bt.stats.Inc(bt.cnts.EvInvalid)
					default:
						Log.BUG("error %d not handled (%d:%d/%d, nxt: %d)\n",
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
		case ts, ok := <-bt.pStatsInfo.statsT.C:
			if ok {
				bt.publishCounters(ts, bt.pStatsInfo.statsTick/1000)
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

// allocates a buffer which can be used for anonymizing sip header fields
func newAnonymizationBuf(l int) []byte {
	if l < 32 {
		l = 32
	}
	return make([]byte, 3*l)
}

func (bt *Sipcmbeat) getCallID(dst, src []byte, callID sipsp.PField, encFlags *FormatFlags) ([]byte, error) {
	if bt.Config.UseCallIDAnonymization() && (len(src) > 0) {
		// anonymize Call-ID
		//anonymization.DbgOn()
		ac := anonymization.AnonymPField{
			PField: callID,
		}
		if err := ac.Anonymize(dst, src); err != nil {
			return nil, fmt.Errorf("Call-ID field processing error: %w", err)
		}
		if encFlags != nil {
			*encFlags |= FormatCallIDencF
		}
		return ac.PField.Get(dst), nil
	}
	// pass through
	return callID.Get(src), nil
}

// getEncContent returns the passed header content encrypted, if any
// anonymization/encryption is enabled.
// dst is a storage buffer for the encrypted result, in which the
// returned slice will point in the ecrypted case, src is the source
// buffer and content is a PField pointing inside src with the part to
// encrypt/process.
// It returns the processed content (encrypted or raw), a flag set to true
// if it was encrypted and an error (nil on success).
func (bt *Sipcmbeat) getEncContent(
	dst, src []byte,
	content sipsp.PField) ([]byte, bool, error) {

	if bt.Config.UseAnonymization() && (len(src) > 0) {
		// anonymize src, the same way as Call-ID
		ac := anonymization.AnonymPField{
			PField: content,
		}
		if err := ac.Anonymize(dst, src); err != nil {
			return nil, false,
				fmt.Errorf("field content processing error: %w", err)
		}
		// ac.Anonymize above will change ac.PField to point to the enc
		// version
		return ac.PField.Get(dst), true, nil
	}
	// pass through
	return content.Get(src), false, nil
}

func (bt *Sipcmbeat) getURI(attr calltr.CallAttrIdx, dst, src []byte,
	encFlags *FormatFlags) ([]byte, error) {
	if bt.Config.UseURIAnonymization() {
		if attr == calltr.AttrContact && len(src) == 1 && src[0] == '*' {
			// Contact: *  -> leave it unencrypte
			// pass through
			return src[:], nil
		}
		// anonymize URI
		var uri sipsp.PsipURI
		if err, _ := sipsp.ParseURI(src, &uri); err != 0 {
			return nil, err
		}
		//anonymization.DbgOn()
		au := anonymization.AnonymURI(uri)
		if err := au.Anonymize(dst, src, true); err != nil {
			return nil, err
		}
		if encFlags != nil {
			*encFlags |= FormatURIencF
		}
		return (*sipsp.PsipURI)(&au).Flat(dst), nil
	}
	// pass through
	return src[:], nil
}

// return event source ip (possibly encrypted) and sets encFlags
func (bt *Sipcmbeat) getSrcIP(ed *calltr.EventData, encFlags *FormatFlags) net.IP {
	if bt.Config.UseIPAnonymization() {
		c := make([]byte, len(ed.Src))
		if encFlags != nil {
			*encFlags |= FormatCltIPencF
		}
		if bt.Config.UseIpcipher() || ed.Src.To4() == nil {
			bt.ipcipher.Encrypt(c, ed.Src)
			if encFlags != nil {
				*encFlags |= FormatIpcipherF
			}
		} else {
			anonymization.GetPan4().Encrypt(c, ed.Src)
		}
		return net.IP(c[:])
	}
	return ed.Src
}

// return event destination ip (possibly encrypted) and sets encFlags
func (bt *Sipcmbeat) getDstIP(ed *calltr.EventData, encFlags *FormatFlags) net.IP {
	if bt.Config.UseIPAnonymization() {
		c := make([]byte, len(ed.Dst))
		if encFlags != nil {
			*encFlags |= FormatSrvIPencF
		}
		if bt.Config.UseIpcipher() || ed.Dst.To4() == nil {
			bt.ipcipher.Encrypt(c, ed.Dst)
			if encFlags != nil {
				*encFlags |= FormatIpcipherF
			}
		} else {
			anonymization.GetPan4().Encrypt(c, ed.Dst)
		}
		return net.IP(c[:])
	}
	return ed.Dst
}

func (bt *Sipcmbeat) publishEv(geoipH *GeoIPdbHandle, srcEv *calltr.EventData) {
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
		Log.ERR("event copy failed (%d bytes)...\n", srcEv.Used)
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
	var encFlags FormatFlags

	var callIDBuf []byte
	if bt.Config.UseCallIDAnonymization() {
		callIDBuf = newAnonymizationBuf(len(ed.CallID.Get(ed.Buf)))
	}
	callID, err := bt.getCallID(callIDBuf, ed.Buf, ed.CallID, &encFlags)
	if err != nil {
		logp.Err("failed to add sip.call_id to Fields: %s\n",
			err.Error())
		bt.stats.Inc(bt.cnts.EvCallIDErr)
	} else {
		addFields(event.Fields, "sip.call_id", str(callID))
	}
add_attrs:
	for i := 0; i < len(ed.Attrs); i++ {
		if !ed.Attrs[i].Empty() {
			switch calltr.CallAttrIdx(i) {
			case calltr.AttrToURI, calltr.AttrFromURI, calltr.AttrContact,
				calltr.AttrRURI, calltr.AttrPAI1, calltr.AttrPAI2:
				var (
					uriBuf []byte
					err    error
				)
				uri := ed.Attrs[i].Get(ed.Buf)
				if bt.Config.UseURIAnonymization() {
					uriBuf = newAnonymizationBuf(len(uri))
					if uri, err = bt.getURI(calltr.CallAttrIdx(i),
						uriBuf, uri, &encFlags); err != nil {
						// reuse EvAttrErr counter for URI enc. errs:
						bt.stats.Inc(bt.cnts.EvAttrErr[i])
						// obey configured event type blacklist
						if sipcallmon.EventsRing.Blacklisted(calltr.EvParseErr) {
							// TODO: counter? normally
							//      sipcallmon.evrStats.Inc(evrCnts.blstType)
							// drop counter
							bt.stats.Inc(bt.cnts.EvPreDrop)
							return
						}
						// force a parse-error event
						// overwrite event type
						event.Fields["type"] = calltr.EvParseErr.String()
						// create an error reason
						const maxURIlen = 60
						errR := make([]byte, 0, 256)
						errR = append(errR, "URI ERROR for "...)
						errR = append(errR,
							calltr.CallAttrIdx(i).String()...)
						errR = append(errR, " \""...)
						val := ed.Attrs[i].Get(ed.Buf)
						if len(val) > maxURIlen {
							errR = append(errR, val[:maxURIlen]...)
							errR = append(errR, "..."...)
						} else {
							errR = append(errR, val...)
						}
						errR = append(errR, "\" : "...)
						errR = append(errR, err.Error()...)
						// add (or overwrite) the reason, always encrypted
						//  (because here we are in enc. mode and we add
						//   the bad uri to it)
						ok := bt.evAddEncBField(event,
							"err_info", errR,
							true, // always enc.
							FormatReasonAencF, &encFlags)
						if !ok {
							logp.Err("failed to add enc. err_info to Fields\n")
							bt.stats.Inc(bt.cnts.EvEInfoErr)
						}
						/*
							logp.Err("failed to add %q to Fields: %s for %q\n",
								calltr.CallAttrIdx(i).String(), err.Error(),
								ed.Attrs[i].Get(ed.Buf))
								// skip
								continue
						*/
						// stop here, don't try adding more attrs
						break add_attrs
					}
				}
				ok := addFields(event.Fields, calltr.CallAttrIdx(i).String(),
					str(uri))
				if !ok {
					logp.Err("failed to add %q : %q to Fields\n",
						calltr.CallAttrIdx(i).String(), uri)
					bt.stats.Inc(bt.cnts.EvAttrErr[i])
				}
			case calltr.AttrUA, calltr.AttrUAS:
				if !bt.evAddEncBField(event,
					calltr.CallAttrIdx(i).String(),
					ed.Attrs[i].Get(ed.Buf),
					bt.Config.UseUAAnonymization(), FormatUAencF, &encFlags) {
					bt.stats.Inc(bt.cnts.EvAttrErr[i])
					continue // skip over this attr
				}
			case calltr.AttrReason:
				if ed.Type == calltr.EvParseErr {
					var reasonBuf []byte
					if bt.Config.UseAnonymization() {
						reasonBuf =
							newAnonymizationBuf(len(ed.Attrs[i].Get(ed.Buf)))
					}
					reason, isEnc, err :=
						bt.getEncContent(reasonBuf, ed.Buf, ed.Attrs[i])
					if err != nil {
						logp.Err("failed to add parse-error %q to Fields: %s"+
							" for %q\n",
							calltr.CallAttrIdx(i).String(), err.Error(),
							ed.Attrs[i].Get(ed.Buf))
						bt.stats.Inc(bt.cnts.EvAttrErr[i])
						continue
					}
					if isEnc {
						encFlags |= FormatReasonAencF
					}
					ok := addFields(event.Fields, "err_info", str(reason))
					if !ok {
						logp.Err("failed to add err_info : %q to Fields\n",
							reason)
						bt.stats.Inc(bt.cnts.EvEInfoErr)
					}
					// added, skip
					break
				}
				fallthrough
			default:
				ok := addFields(event.Fields, calltr.CallAttrIdx(i).String(),
					str(ed.Attrs[i].Get(ed.Buf)))
				if !ok {
					logp.Err("failed to add %q to Fields\n",
						calltr.CallAttrIdx(i).String())
					bt.stats.Inc(bt.cnts.EvAttrErr[i])
				}
				//	event.Fields[calltr.CallAttrIdx(i).String()] =
				//		str(ed.Attrs[i].Get(ed.Buf))
			}
		}
	}
	// some fields are added only to some events: handle this below
	switch ed.Type {
	case calltr.EvCallEnd:
		// add duration only on events that make sense, and only
		// if call-start is known. Use seconds.
		if !ed.FinReplTS.IsZero() {
			// (otherwise the current monitoring part will get confused)
			addFields(event.Fields, "event.duration",
				ed.TS.Sub(ed.FinReplTS)/time.Second)
		} else {
			// add a min_length field containing the minimum call duration^
			addFields(event.Fields, "event.min_length",
				ed.TS.SubTime(sipcallmon.StartTS)/time.Second)
		}
		if ed.CFlags&calltr.CFForcedTimeout != 0 {
			addFields(event.Fields, "sip.originator", "timeout-terminated")
		} else if ed.CFlags&calltr.CFCalleeTerminated != 0 {
			addFields(event.Fields, "sip.originator", "callee-terminated")
		} else {
			addFields(event.Fields, "sip.originator", "caller-terminated")
		}
		fallthrough // continue with common fileds for call-start & attempt
	case calltr.EvCallStart, calltr.EvCallAttempt:
		// hack for using different event field name for call-end last resp.
		if ed.Type == calltr.EvCallEnd {
			if ed.ReplStatus == 0 {
				// created by a BYE, no INVITE seen (no call-start)
				addFields(event.Fields, "sip.unmatched_invite", true)
			} else {
				// for CallEnd we do not add sip.response.status
				addFields(event.Fields, "sip.response.last", ed.ReplStatus)
			}
		} else {
			// last response for call-start or call-attempt
			addFields(event.Fields, "sip.response.status", ed.ReplStatus)
		}
		// post dial delay, time between request and 18x
		pdd := time.Duration(0)
		if !ed.EarlyDlgTS.IsZero() {
			pdd = ed.EarlyDlgTS.Sub(ed.CreatedTS)
		}
		// ring time: delay between 18x and final response
		rt := time.Duration(0)
		if !ed.FinReplTS.IsZero() {
			if !ed.EarlyDlgTS.IsZero() {
				rt = ed.FinReplTS.Sub(ed.EarlyDlgTS)
			} else {
				// no 18x =>  0 ring time and pdd = final repl time
				pdd = ed.FinReplTS.Sub(ed.CreatedTS)
			}
		}
		addFields(event.Fields, "sip.pdd", pdd/time.Millisecond)
		addFields(event.Fields, "sip.ring_time", rt/time.Millisecond)
		// note: fr_delay == pdd+ring_time can also be computed from
		//       dbg.call_start - dbg.created

	case calltr.EvRegDel, calltr.EvRegExpired, calltr.EvSubDel:
		// add duration only on events that make sense, and only
		// if call-start is known. Use seconds.
		if !ed.FinReplTS.IsZero() {
			// (otherwise the current monitoring part will get confused)
			addFields(event.Fields, "event.lifetime",
				ed.TS.Sub(ed.FinReplTS)/time.Second)
		} else {
			// add a min_length field containing the minimum call duration^
			addFields(event.Fields, "event.min_lifetime",
				ed.TS.SubTime(sipcallmon.StartTS)/time.Second)
		}
		addFields(event.Fields, "sip.response.last", ed.ReplStatus)

	case calltr.EvRegNew, calltr.EvSubNew, calltr.EvAuthFailed:
		addFields(event.Fields, "sip.response.status", ed.ReplStatus)
		frd := time.Duration(0) // final reply delay, time till final reply
		if !ed.FinReplTS.IsZero() {
			frd = ed.FinReplTS.Sub(ed.CreatedTS)
		}
		addFields(event.Fields, "sip.fr_delay", frd/time.Millisecond)

	default:
		addFields(event.Fields, "sip.response.last", ed.ReplStatus)
	}
	// fromtag & totag moved from dbg
	addFields(event.Fields, "sip.fromtag", str(ed.FromTag.Get(ed.Buf)))
	addFields(event.Fields, "sip.totag", str(ed.ToTag.Get(ed.Buf)))

	addFields(event.Fields, "event.call_start", ed.FinReplTS)
	addFields(event.Fields, "client.transport", ed.ProtoF.ProtoName())
	addFields(event.Fields, "client.ip", bt.getSrcIP(&ed, &encFlags))
	addFields(event.Fields, "client.port", ed.SPort)
	addFields(event.Fields, "server.ip", bt.getDstIP(&ed, &encFlags))
	addFields(event.Fields, "server.port", ed.DPort)
	// rate
	addFields(event.Fields, "rate.exceeded", ed.Rate.ExCnt)
	addFields(event.Fields, "rate.ex_diff", ed.Rate.ExCntDiff)
	addFields(event.Fields, "rate.crt", ed.Rate.Rate)
	addFields(event.Fields, "rate.lim", ed.Rate.MaxR)
	// rate.period is stored in milliseconds (epoch_millis)
	addFields(event.Fields, "rate.period", ed.Rate.Intvl.Nanoseconds()/1000000)
	addFields(event.Fields, "rate.since", ed.Rate.T)
	addFields(event.Fields, "rate.key", ed.Type.String()+":"+
		bt.getSrcIP(&ed, &encFlags).String())

	// dbg
	addFields(event.Fields, "dbg.state", ed.State.String())
	addFields(event.Fields, "dbg.prev_state", ed.PrevState.String())
	addFields(event.Fields, "dbg.lastev", ed.LastEv.String())
	addFields(event.Fields, "dbg.evflags", ed.EvFlags.String())
	addFields(event.Fields, "dbg.evgen", ed.EvGen.String())
	addFields(event.Fields, "dbg.created", ed.CreatedTS)
	addFields(event.Fields, "dbg.call_start", ed.FinReplTS)
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

	// geoip info
	if bt.Config.GeoIPLookup {
		bt.addGeoIPinfo(geoipH, event, &ed, &encFlags)
	}

	// encrypted flags
	if encFlags != 0 {
		if bt.validator != nil {
			// the precomputed validation code cand be used as long nonce is NOT used
			addFields(event.Fields, "encrypt_flags", strconv.Itoa(int(encFlags)))
			addFields(event.Fields, "encrypt", bt.validator.Code())
		}
	} else {
		addFields(event.Fields, "encrypt_flags", "0")
	}

	// version fields
	bt.addVersionToEv(event)

	bt.client.Publish(event)
	bt.stats.Inc(bt.cnts.EvPub)
	//	logp.Info("Event sent")
}

func (bt *Sipcmbeat) addVersionToEv(event beat.Event) {
	addFields(event.Fields, "agent.version_long", bt.LongVer)
	addFields(event.Fields, "agent.version_base", bt.BaseVer)
	addFields(event.Fields, "agent.commit_hash", bt.CommitH)
	addFields(event.Fields, "agent.build_time", bt.BuildT)
}
