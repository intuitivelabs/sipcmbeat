// Copyright 2019-2020 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a source-available license
// that can be found in the INTUITIVE_LABS-LICENSE.txt file in the
// root of the source tree.

// Publish statistics

package beater

import (
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/common"

	"github.com/intuitivelabs/counters"
)

const cntLongFormat = 128

const statsMinTick = 500 * time.Millisecond

type statsGrpIntvl struct {
	name  string
	grp   *counters.Group
	intvl time.Duration // send interval
	last  time.Time     // last sent time
	oos   uint          // out-of-sync count
}

func (bt *Sipcmbeat) initStatsGrps() {
	// helper function for finding the greatest common denominator
	// (works only for positive values)
	gcd := func(d1, d2 time.Duration) time.Duration {
		for d2 != 0 {
			r := d1 % d2
			d1 = d2
			d2 = r
		}
		return d1
	}

	now := time.Now()
	statsCntGrps := ([]statsGrpIntvl)(nil)
	minIntvl := bt.Config.StatsInterval
	if minIntvl < 0 {
		// no default interval
		minIntvl = 0
	}
	for _, g := range bt.Config.StatsGrps {
		gname := g.Name
		var grp *counters.Group
		if gname == "none" || gname == "-" {
			continue
		}
		if gname == "all" || gname == "*" {
			grp = &counters.RootGrp
			gname = "all"
		} else {
			grp, _ = counters.RootGrp.GetSubGroupDot(gname)
		}
		if grp != nil {
			intvl := g.Intvl
			if intvl == -1 {
				intvl = bt.Config.StatsInterval
			}
			if intvl > 0 {
				if intvl < statsMinTick {
					intvl = statsMinTick
				}
				statsCntGrps = append(statsCntGrps,
					statsGrpIntvl{
						name:  gname,
						grp:   grp,
						intvl: intvl,
						last:  now,
					})
				minIntvl = gcd(intvl, minIntvl)
			}
		}
	}
	if minIntvl > 0 && minIntvl < statsMinTick {
		minIntvl = statsMinTick
	}
	//bt.statsTick = minIntvl
	atomic.StoreInt64((*int64)(unsafe.Pointer(&bt.statsTick)), int64(minIntvl))
	// TODO: atomic, change to timestamp
	bt.lastStatsEv = now
	pStatsRepGrps := (*unsafe.Pointer)(unsafe.Pointer(&bt.statsRepGrps))
	atomic.StorePointer(pStatsRepGrps, (unsafe.Pointer)(&statsCntGrps))
}

// publishStats will publish all the counters stats.
// The parameters are the current time and timer error
func (bt *Sipcmbeat) publishCounters(ts time.Time, terr time.Duration) {
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

	flags := counters.PrRec | counters.PrFullName |
		cntLongFormat /* | counters.PrDesc */
	now := ts
	p := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&bt.statsRepGrps)))
	statsGrps := *((*[]statsGrpIntvl)(p))
	added := 0
	for i := 0; i < len(statsGrps); i++ {
		g := statsGrps[i].grp
		/* note that all the time comparisons and time.Sub() will use the
		   process "monotonic" time and not the system time (see go time docs).
		   However attempting to print the time (convert to string) will
		   produce the real wall time. Each Time value in go holds
		   both the wall time and the monotonic per process clock.
		   time.Add() will add the duration to both internal Time fields.

		   On linux  hibernate/sleep would not cause
		   the process time to jump (the process clock will not account
		   for the sleep period), but suspending the process would work as
		   expected (the per process clock will account for the "suspended"
		   time).
		*/
		if g == nil || statsGrps[i].intvl == 0 ||
			statsGrps[i].last.After(now.Add(-statsGrps[i].intvl+terr)) {
			continue
		}

		statsGrps[i].last = statsGrps[i].last.Add(statsGrps[i].intvl)
		// resync if diff too big or in the future
		if (statsGrps[i].last.Sub(now) > terr) ||
			(now.Sub(statsGrps[i].last) > (bt.statsTick + terr)) {
			statsGrps[i].oos++
			// if out-of-sync more then 3 times in a row or the difference
			// is really big => re-sync "last" ( => skipping older stats)
			if statsGrps[i].oos > 3 ||
				(now.Sub(statsGrps[i].last) >= 2*statsGrps[i].intvl) {
				// resync
				statsGrps[i].last = now
				statsGrps[i].oos = 0
			}
		} else {
			statsGrps[i].oos = 0
		}

		addGroup(cntHash, g, flags)
		if flags&counters.PrRec != 0 {
			addSubGroups(cntHash, g, flags)
		}
		added++
	}

	if added == 0 && (bt.Config.StatsInterval <= 0 ||
		now.Sub(bt.lastStatsEv) < (bt.Config.StatsInterval-terr)) {
		// no event if no counters added and time since last event
		// < StatsInterval or StatsInterval disabled
		return
	}
	// version fields
	bt.addVersionToEv(event)

	bt.client.Publish(event)
	bt.stats.Inc(bt.cnts.EvPub)
	bt.stats.Inc(bt.cnts.EvStats)
	bt.lastStatsEv = now
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
