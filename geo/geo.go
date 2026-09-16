// Package geo maps an IP address to an ISO 3166-1 alpha-2 country code using
// the table in geo.bin (built by tools/gengeo from DB-IP's IP to Country Lite
// data, CC BY 4.0 - https://db-ip.com).
//
// The table is embedded in the binary and searched in place: nothing is
// decoded into the heap, and the kernel only pages in the few hundred bytes a
// binary search actually touches.
package geo

import (
	_ "embed"
	"encoding/binary"
	"errors"
	"net/netip"
	"sort"
)

//go:embed geo.bin
var raw []byte

type table struct {
	codes  []byte // 2 bytes per code
	n4     int
	s4, c4 []byte
	n6     int
	s6, c6 []byte
}

var tbl table

func init() {
	t, err := parse(raw)
	if err != nil {
		panic("geo.bin: " + err.Error())
	}
	tbl = t
}

func parse(b []byte) (table, error) {
	var t table
	be := binary.BigEndian
	if len(b) < 8 || string(b[:6]) != "PMGEO1" {
		return t, errors.New("bad magic")
	}
	p := 6
	need := func(n int) error {
		if p+n > len(b) {
			return errors.New("truncated")
		}
		return nil
	}
	nc := int(be.Uint16(b[p:]))
	p += 2
	if err := need(nc * 2); err != nil {
		return t, err
	}
	t.codes = b[p : p+nc*2]
	p += nc * 2
	if err := need(4); err != nil {
		return t, err
	}
	t.n4 = int(be.Uint32(b[p:]))
	p += 4
	if err := need(t.n4 * 5); err != nil {
		return t, err
	}
	t.s4 = b[p : p+t.n4*4]
	p += t.n4 * 4
	t.c4 = b[p : p+t.n4]
	p += t.n4
	if err := need(4); err != nil {
		return t, err
	}
	t.n6 = int(be.Uint32(b[p:]))
	p += 4
	if err := need(t.n6 * 10); err != nil {
		return t, err
	}
	t.s6 = b[p : p+t.n6*8]
	p += t.n6 * 8
	t.c6 = b[p : p+t.n6*2]
	p += t.n6 * 2
	if p != len(b) {
		return t, errors.New("trailing bytes")
	}
	return t, nil
}

// Country returns the two-letter country code for a public address, or ""
// when the table has no data for it.
func Country(a netip.Addr) string {
	a = a.Unmap()
	be := binary.BigEndian
	var ci int
	switch {
	case a.Is4():
		v := be.Uint32(a.AsSlice())
		i := sort.Search(tbl.n4, func(i int) bool { return be.Uint32(tbl.s4[i*4:]) > v }) - 1
		if i < 0 {
			return ""
		}
		ci = int(tbl.c4[i])
	case a.Is6():
		b := a.As16()
		v := be.Uint64(b[:8])
		i := sort.Search(tbl.n6, func(i int) bool { return be.Uint64(tbl.s6[i*8:]) > v }) - 1
		if i < 0 {
			return ""
		}
		ci = int(be.Uint16(tbl.c6[i*2:]))
	default:
		return ""
	}
	if ci == 0 || ci*2+2 > len(tbl.codes) {
		return ""
	}
	return string(tbl.codes[ci*2 : ci*2+2])
}

// Ranges reports the table sizes, for the startup log line.
func Ranges() (v4, v6 int) { return tbl.n4, tbl.n6 }
