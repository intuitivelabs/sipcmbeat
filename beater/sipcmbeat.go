package beater

import (
	"fmt"
	//	"strconv"
	//"os"
	//	"runtime/pprof"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"

	"andrei/sipcallmon"
	"andrei/sipsp/calltr"
)

// Sipcmbeat configuration.
type Sipcmbeat struct {
	done   chan struct{}
	newEv  chan struct{} // new events are signalled here
	evIdx  int
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
	bt.evRing.Init(102400)
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
	}
	close(bt.done)
	bt.wg.Wait()
	bt.evRing.CloseEvSignal() // safe, since sipcallmon is already stopped
}

func (bt *Sipcmbeat) consumeEv() {
	defer bt.wg.Done()
	for {
		select {
		case <-bt.done:
			return
		case <-bt.newEv:
			// FIXME: idx overflow -> switch to uint
			for ; bt.evIdx != bt.evRing.LastIdx(); bt.evIdx++ {
				ev := bt.evRing.Get(bt.evIdx)
				//fmt.Printf("GOT ev[%d]: %p\n", bt.evIdx, ev)
				if ev != nil {
					bt.publishEv(ev)
					bt.evRing.Put(bt.evIdx)
				} else {
					fmt.Printf("GOT NIL ev[%d]: %p\n", bt.evIdx, ev)
					if bt.evRing.LastIdx()-bt.evIdx > bt.evRing.BufSize() {
						fmt.Printf("WARNING: missed %d events \n",
							bt.evRing.LastIdx()-bt.evIdx-bt.evRing.BufSize())
						bt.evIdx = bt.evRing.LastIdx() - bt.evRing.BufSize()
					}
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

func (bt *Sipcmbeat) publishEv(ed *calltr.EventData) {
	if bt.client == nil { // dev null
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
			}
			//	event.Fields[calltr.CallAttrIdx(i).String()] =
			//		str(ed.Attrs[i].Get(ed.Buf))
		}
	}
	addFields(event.Fields, "sip.response.status", ed.ReplStatus)
	addFields(event.Fields, "event.duration", ed.TS.Sub(ed.StartTS))
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
	addFields(event.Fields, "dbg.req_no", ed.Reqs)
	addFields(event.Fields, "dbg.repl_no", ed.Repls)

	bt.client.Publish(event)
	//	logp.Info("Event sent")
}
