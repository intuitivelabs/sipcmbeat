// Copyright 2021 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a source-available license
// that can be found in the LICENSE.txt file in the root of the source
// tree.

package beater

import (
	"github.com/elastic/beats/v7/libbeat/beat"

	"github.com/intuitivelabs/sipsp"
)

// returns false if there was some error encrypting (no field added)
func (bt *Sipcmbeat) evAddEncBField(event beat.Event,
	field string,
	val []byte,
	doEnc bool,
	setEncFlg FormatFlags, // flags set if field encrypted
	encFlags *FormatFlags) bool {
	if doEnc {
		buf := newAnonymizationBuf(len(val))
		encVal, isEnc, err :=
			bt.getEncContent(buf, val,
				sipsp.PField{Offs: 0, Len: sipsp.OffsT(len(val))})
		if err != nil {
			Log.ERR("enc error for field %q: %s\n",
				field, err.Error())
			return false
		} else {
			val = encVal
			if isEnc && (encFlags != nil) {
				*encFlags |= setEncFlg
			}
		}
	}
	addFields(event.Fields, field, str(val))
	return true
}
