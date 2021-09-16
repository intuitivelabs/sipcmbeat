// Copyright 2019-2020 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a source-available license
// that can be found in the INTUITIVE_LABS-LICENSE.txt file in the
// root of the source tree.

// Publish statistics

package beater

import (
	"time"

	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/common"

	"github.com/intuitivelabs/counters"
)

const cntLongFormat = 128

// publishStats will publish all the counters stats.
func (bt *Sipcmbeat) publishCounters() {
	if bt.client == nil { // dev null
		return
	}
	// It looks like event.Publish() does not copy all the strings passed to it
	// and returns before consuming/building the message => all the strings
	// will point to a buffer that will be reused...

	cntHash := make(common.MapStr)
	event := beat.Event{
		Timestamp: time.Now(),
		Fields: common.MapStr{
			"type":     "counters",
			"counters": cntHash,
		},
	}

	g := &counters.RootGrp
	// or g:= coutners.RootGrp.GetSubGroupDot("foo.bar") for foo.bar only

	flags := counters.PrRec | counters.PrFullName |
		cntLongFormat /* | counters.PrDesc */
	addGroup(cntHash, g, flags)
	if flags&counters.PrRec != 0 {
		addSubGroups(cntHash, g, flags)
	}

	// version fields
	bt.addVersionToEv(event)

	bt.client.Publish(event)
	bt.stats.Inc(bt.cnts.EvPub)
	bt.stats.Inc(bt.cnts.EvStats)
}

// addCounter adds the specified counter (g.h) to the event fields (m).
// Returns true on success.
func addCounter(m common.MapStr,
	g *counters.Group, h counters.Handle, flags int) bool {

	f := g.GetFlags(h)

	if flags&cntLongFormat != 0 {
		// Format:
		//    foo = {
		//         val: val
		//         max: max
		//         min: min
		//         desc: desc
		//    }
		// (any of the fields might be missing, it depends on the flags and
		//  the counter type/flags)
		name := g.GetFullName(h)
		if f&counters.CntHideVal == 0 {
			addFields(m, name+".val", g.Get(h))
		}
		if f&counters.CntMinF != 0 {
			min := g.GetMin(h)
			if min == counters.Val(^uint64(0)) {
				min = 0
			}
			addFields(m, name+".min", min)
		}
		if f&counters.CntMaxF != 0 {
			addFields(m, name+".max", g.GetMax(h))
		}
		if flags&counters.PrDesc != 0 {
			addFields(m, name+".desc", g.GetDesc(h))
		}
		return true
	}
	// else "brief" format:

	// Format:
	//    foo = val
	//    foo_max = max
	//    foo_min = min
	//    foo_desc = desc
	var name string
	if flags&counters.PrFullName != 0 {
		name = g.GetFullName(h)
	} else {
		name = g.GetName(h)
	}

	if f&counters.CntHideVal == 0 {
		addFields(m, name, g.Get(h))
	}
	if f&counters.CntMinF != 0 {
		min := g.GetMin(h)
		if min == counters.Val(^uint64(0)) {
			min = 0
		}
		if f&counters.CntHideVal == 0 || f&counters.CntMaxF != 0 {
			addFields(m, name+"_min", min)
		} else {
			addFields(m, name, min)
		}
	}
	if f&counters.CntMaxF != 0 {
		if f&counters.CntHideVal == 0 || f&counters.CntMinF != 0 {
			addFields(m, name+"_max", g.GetMax(h))
		} else {
			addFields(m, name, g.GetMax(h))
		}
	}
	if flags&counters.PrDesc != 0 {
		addFields(m, name+"_desc", g.GetDesc(h))
	}
	return true
}

func addGroup(m common.MapStr, g *counters.Group,
	flags int) bool {

	var i int
	for i = 0; i < g.CntNo(); i++ {
		addCounter(m, g, counters.Handle(i), flags)
	}
	return i != 0
}

// addSubGroups adds the subgroups corresponding to a group
// if counters.PrRec is set it will recursively print all the subgroups
func addSubGroups(m common.MapStr, g *counters.Group, flags int) bool {
	var ret bool
	n := g.GetSubGroupsNo()
	subgr := make([]*counters.Group, 0, n)
	g.GetSubGroups(&subgr)
	// no need to sort, the events fields are unsorted anyway
	for _, sg := range subgr {
		res := addGroup(m, sg, flags)
		ret = ret || res
		// rec
		if flags&counters.PrRec != 0 && sg.GetSubGroupsNo() > 0 {
			addSubGroups(m, sg, flags)
		}
	}
	return ret
}
