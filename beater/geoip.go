// Copyright 2021 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a source-available license
// that can be found in the LICENSE.txt file in the root of the source
// tree.

package beater

import (
	"errors"
	"strconv"
	"sync/atomic"
	"unsafe"

	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/oschwald/maxminddb-golang"

	"github.com/intuitivelabs/calltr"
	"github.com/intuitivelabs/counters"
)

type geoipLookupCounters struct {
	Ok        counters.Handle
	Err       counters.Handle
	DecodeErr counters.Handle
	EncErr    counters.Handle
	NotFound  counters.Handle
	NoCountry counters.Handle
	NoCity    counters.Handle
	//NoSubDiv  counters.Handle
}

type geoipCounters struct {
	DBreload    counters.Handle
	DBloadErr   counters.Handle
	InternalUpd counters.Handle
	Src         geoipLookupCounters
	Dst         geoipLookupCounters
}

type geoipDbStats struct {
	main   counters.Group
	lookup struct {
		src counters.Group
		dst counters.Group
	}
}

func (bt *Sipcmbeat) initGeoIPcounters() error {
	geoipCntDefs := [...]counters.Def{
		{&bt.geoipCnts.DBreload, 0, nil, nil, "db_reload",
			"db reloads"},
		{&bt.geoipCnts.DBloadErr, 0, nil, nil, "db_load_err",
			"db reload errors"},
		{&bt.geoipCnts.InternalUpd, 0, nil, nil, "db_update",
			"db internal local copy updated"},
	}
	srcCntDefs := [...]counters.Def{
		{&bt.geoipCnts.Src.Ok, 0, nil, nil, "lookup_ok",
			"successful lookups"},
		{&bt.geoipCnts.Src.Err, 0, nil, nil, "lookup_err",
			"failed lookups, db error"},
		{&bt.geoipCnts.Src.DecodeErr, 0, nil, nil, "decode_err",
			"decode db record error, but successful lookup"},
		{&bt.geoipCnts.Src.EncErr, 0, nil, nil, "encrypt_err",
			"record encrypt error"},
		{&bt.geoipCnts.Src.NotFound, 0, nil, nil, "lookup_miss",
			"empty lookups, nothing found"},
		{&bt.geoipCnts.Src.NoCountry, 0, nil, nil, "country_empty",
			"non-empty lookup with no country"},
		{&bt.geoipCnts.Src.NoCity, 0, nil, nil, "city_empty",
			"non-empty lookup with no city"},
		/*
			{&bt.geoipCnts.Src.NoSubDiv, 0, nil, nil, "subdiv_empty",
				"non-empty lookup with no subdivisions"},
		*/
	}
	dstCntDefs := [...]counters.Def{
		{&bt.geoipCnts.Dst.Ok, 0, nil, nil, "lookup_ok",
			"successful lookups"},
		{&bt.geoipCnts.Dst.Err, 0, nil, nil, "lookup_err",
			"failed lookups, db error"},
		{&bt.geoipCnts.Dst.DecodeErr, 0, nil, nil, "decode_err",
			"decode db record error, but successful lookup"},
		{&bt.geoipCnts.Dst.EncErr, 0, nil, nil, "encrypt_err",
			"record encrypt error"},
		{&bt.geoipCnts.Dst.NotFound, 0, nil, nil, "lookup_miss",
			"empty lookups, nothing found"},
		{&bt.geoipCnts.Dst.NoCountry, 0, nil, nil, "country_empty",
			"non-empty lookup with no country"},
		{&bt.geoipCnts.Dst.NoCity, 0, nil, nil, "city_empty",
			"non-empty lookup with no city"},
		/*
			{&bt.geoipCnts.Dst.NoSubDiv, 0, nil, nil, "subdiv_empty",
				"non-empty lookup with no subdivisions"},
		*/
	}
	bt.geoipStats.main.Init("geoip", nil, len(geoipCntDefs))
	if !bt.geoipStats.main.RegisterDefs(geoipCntDefs[:]) {
		return errors.New("failed to register geoip counters")
	}

	bt.geoipStats.lookup.src.Init("src", &bt.geoipStats.main, len(srcCntDefs))
	if !bt.geoipStats.lookup.src.RegisterDefs(srcCntDefs[:]) {
		return errors.New("failed to register src geoip counters")
	}
	bt.geoipStats.lookup.dst.Init("dst", &bt.geoipStats.main, len(dstCntDefs))
	if !bt.geoipStats.lookup.dst.RegisterDefs(dstCntDefs[:]) {
		return errors.New("failed to register dst geoip counters")
	}
	return nil
}

type GeoIPdbHandle struct {
	dbh    *maxminddb.Reader
	refCnt int64
	name   string
}

// Ref increases the internal reference counter and returns the new value.
func (gh *GeoIPdbHandle) Ref() int64 {
	return atomic.AddInt64(&gh.refCnt, 1)
}

// Unref decrements the internal reference counter and closes the db
// handler if it becomes 0.
// It returns  true if the db was closes.
func (gh *GeoIPdbHandle) Unref() bool {
	if atomic.AddInt64(&gh.refCnt, -1) == 0 {
		if gh.dbh != nil {
			if err := gh.dbh.Close(); err != nil {
				Log.ERR("maminddb Close failed: %v\n", err)
			}
			gh.dbh = nil
			gh.name = ""
		}
		return true
	}
	return false
}

// openGeoIpdb opens a geoip db file in maxminddb format.
// It returns an initialised GeoIpdbHandle (with refCnt == 1) or
// nil and a set error on error.
func openGeoIPdb(file string) (*GeoIPdbHandle, error) {
	db, err := maxminddb.Open(file)
	if err != nil {
		return nil, err
	}
	return &GeoIPdbHandle{dbh: db, refCnt: 1, name: file}, nil
}

// openGeoIPh opens a geoip DB, creates a handle for it and deref the
// previously used handle (if any).
// If open fails, it leave the current value unchanged.
func (bt *Sipcmbeat) openGeoIPh(file string) error {
	db, err := openGeoIPdb(file)
	if err != nil {
		bt.geoipStats.main.Inc(bt.geoipCnts.DBloadErr)
		return err
	}
	bt.geoipStats.main.Inc(bt.geoipCnts.DBreload)
	// change current handle
	bt.geoipChgLock.Lock()
	{
		pGeoIPh := (*unsafe.Pointer)(unsafe.Pointer(&bt.geoipH))
		h := (*GeoIPdbHandle)(atomic.LoadPointer(pGeoIPh))
		// geoipH = db ; refcnt is already 1 from open above
		atomic.StorePointer(pGeoIPh, (unsafe.Pointer)(db))
		if h != nil {
			h.Unref()
		}
	}
	bt.geoipChgLock.Unlock()
	return nil
}

func (bt *Sipcmbeat) getGeoIPh() *GeoIPdbHandle {
	var h *GeoIPdbHandle
	// lock
	bt.geoipChgLock.Lock()
	{
		p := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&bt.geoipH)))
		h = (*GeoIPdbHandle)(p)
		if h != nil {
			h.Ref()
		}
		// unlock
	}
	bt.geoipChgLock.Unlock()
	return h
}

func (bt *Sipcmbeat) updateGeoIPh(cached *GeoIPdbHandle) *GeoIPdbHandle {
	h := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&bt.geoipH)))
	if h == unsafe.Pointer(cached) {
		return cached
	}
	if cached != nil {
		cached.Unref() // release/free it
	}
	cached = bt.getGeoIPh()
	bt.geoipStats.main.Inc(bt.geoipCnts.InternalUpd)
	return cached
}

func (bt *Sipcmbeat) addGeoIPinfo(geoip *GeoIPdbHandle, event beat.Event,
	ed *calltr.EventData,
	encFlags *FormatFlags) {

	if geoip == nil || geoip.dbh == nil {
		return
	}
	var rec1, rec2 struct {
		Country struct {
			// GeoNameID uint   `maxminddb:"geoname_id"`
			ISOcode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
		City struct {
			GeoNameID uint `maxminddb:"geoname_id"`
			//		Names     map[string]string `maxminddb:"names"`
		} `maxminddb:"city"`
		/*
			Postal struct {
				Code string `maxminddb:"code"`
			} `maxminddb:"postal"`
		*/
		/*
			Subdivisions []struct {
				GeoNameID uint   `maxminddb:"geoname_id"`
				ISOcode   string `maxminddb:"iso_code"`
			} `maxminddb:"subdivisions"`
		*/
	}

	// src
	offs, err := geoip.dbh.LookupOffset(ed.Src)

	if err != nil {
		if (bt.geoipStats.lookup.src.Inc(bt.geoipCnts.Src.Err)%1000 == 0) &&
			Log.ERRon() {
			Log.ERR("geoip: failed to lookup src ip (%q): %v\n", ed.Src, err)
		}
	} else if offs == maxminddb.NotFound {
		v := bt.geoipStats.lookup.src.Inc(bt.geoipCnts.Src.NotFound)
		if (v%100000 == 0) && Log.DBGon() {
			Log.DBG("geoip: src ip lookup(%q) => no result\n", ed.Src)
		}
	} else {
		bt.geoipStats.lookup.src.Inc(bt.geoipCnts.Src.Ok)
		err = geoip.dbh.Decode(offs, &rec1)
		if err != nil {
			bt.geoipStats.lookup.src.Inc(bt.geoipCnts.Src.DecodeErr)
			if Log.ERRon() {
				Log.ERR("geoip: failed to decode record at offset %d for"+
					" src ip (%q): %v\n", offs, ed.Src, err)
			}
		} else {
			if len(rec1.Country.ISOcode) != 0 {
				isoCode := []byte(rec1.Country.ISOcode)
				if !bt.evAddEncBField(event, "geoip.src.iso_code", isoCode,
					bt.Config.UseIPAnonymization(),
					FormatCountryISOencF, encFlags) {
					bt.geoipStats.lookup.src.Inc(bt.geoipCnts.Src.EncErr)
				}
			} else {
				bt.geoipStats.lookup.src.Inc(bt.geoipCnts.Src.NoCountry)
			}
			if rec1.City.GeoNameID != 0 {
				cityID := []byte(
					strconv.FormatUint(uint64(rec1.City.GeoNameID), 10))
				if !bt.evAddEncBField(event, "geoip.src.city_id", cityID,
					bt.Config.UseIPAnonymization(),
					FormatCityIDencF, encFlags) {
					bt.geoipStats.lookup.src.Inc(bt.geoipCnts.Src.EncErr)
				}
			} else {
				bt.geoipStats.lookup.src.Inc(bt.geoipCnts.Src.NoCity)
			}
			/* subdivisions disabled
			if len(rec1.Subdivisions) > 0 {
				if len(rec1.Subdivisions[0].ISOcode) != 0 {
					isoCode := []byte(rec1.Subdivisions[0].ISOcode)
					if !bt.evAddEncBField(event, "geoip.src.sub1_code",
						isoCode,
						bt.Config.UseIPAnonymization(),
						FormatCityIDencF, encFlags) {
						bt.geoipStats.lookup.src.Inc(bt.geoipCnts.Src.EncErr)
					}
				}
				if len(rec1.Subdivisions) > 1 &&
					len(rec1.Subdivisions[1].ISOcode) != 0 {
					isoCode := []byte(rec1.Subdivisions[1].ISOcode)
					if !bt.evAddEncBField(event, "geoip.src.sub2_code",
						isoCode,
						bt.Config.UseIPAnonymization(),
						FormatCityIDencF, encFlags) {
						bt.geoipStats.lookup.src.Inc(bt.geoipCnts.Src.EncErr)
					}
				}
			} else {
				bt.geoipStats.lookup.src.Inc(bt.geoipCnts.Src.NoSubDiv)
			}
			*/
		}
	}

	// dst
	offs, err = geoip.dbh.LookupOffset(ed.Dst)
	if err != nil {
		if (bt.geoipStats.lookup.dst.Inc(bt.geoipCnts.Dst.Err)%1000 == 0) &&
			Log.ERRon() {
			Log.ERR("geoip: failed to lookup dst ip (%q): %v\n", ed.Dst, err)
		}
	} else if offs == maxminddb.NotFound {
		v := bt.geoipStats.lookup.dst.Inc(bt.geoipCnts.Dst.NotFound)
		if (v%100000 == 0) && Log.DBGon() {
			Log.DBG("geoip: dst ip lookup(%q) => no result\n", ed.Dst)
		}
	} else {
		bt.geoipStats.lookup.dst.Inc(bt.geoipCnts.Dst.Ok)
		err = geoip.dbh.Decode(offs, &rec2)
		if err != nil {
			bt.geoipStats.lookup.dst.Inc(bt.geoipCnts.Dst.DecodeErr)
			Log.ERR("geoip: failed to decode record at offset %d for"+
				" dst ip (%q): %v\n", offs, ed.Dst, err)
		} else {
			if len(rec2.Country.ISOcode) != 0 {
				isoCode := []byte(rec2.Country.ISOcode)
				if !bt.evAddEncBField(event, "geoip.dst.iso_code", isoCode,
					bt.Config.UseIPAnonymization() &&
						!bt.Config.ClearTxtCountryISO,
					FormatCountryISOencF, encFlags) {
					bt.geoipStats.lookup.dst.Inc(bt.geoipCnts.Dst.EncErr)
				}
			} else {
				bt.geoipStats.lookup.dst.Inc(bt.geoipCnts.Dst.NoCountry)
			}
			if rec2.City.GeoNameID != 0 {
				cityID := []byte(
					strconv.FormatUint(uint64(rec2.City.GeoNameID), 10))
				if !bt.evAddEncBField(event, "geoip.dst.city_id", cityID,
					bt.Config.UseIPAnonymization() &&
						!bt.Config.ClearTxtCityID,
					FormatCityIDencF, encFlags) {
					bt.geoipStats.lookup.dst.Inc(bt.geoipCnts.Dst.EncErr)
				}
			} else {
				bt.geoipStats.lookup.dst.Inc(bt.geoipCnts.Dst.NoCity)
			}
			/* subdivisions disabled
			if len(rec2.Subdivisions) > 0 {
				if len(rec2.Subdivisions[0].ISOcode) != 0 {
					isoCode := []byte(rec2.Subdivisions[0].ISOcode)
					if !bt.evAddEncBField(event, "geoip.dst.sub1_code",
						isoCode,
						bt.Config.UseIPAnonymization(),
						FormatCityIDencF, encFlags) {
						bt.geoipStats.lookup.dst.Inc(bt.geoipCnts.Dst.EncErr)
					}
				}
				if len(rec2.Subdivisions) > 1 &&
					len(rec2.Subdivisions[1].ISOcode) != 0 {
					isoCode := []byte(rec2.Subdivisions[1].ISOcode)
					if !bt.evAddEncBField(event, "geoip.dst.sub2_code",
						isoCode,
						bt.Config.UseIPAnonymization(),
						FormatCityIDencF, encFlags) {
						bt.geoipStats.lookup.dst.Inc(bt.geoipCnts.Dst.EncErr)
					}
				}
			} else {
				bt.geoipStats.lookup.dst.Inc(bt.geoipCnts.Dst.NoSubDiv)
			}
			*/
		}
	}
}
