// gengeo turns the DB-IP "IP to Country Lite" CSV files into geo/geo.bin, the
// compact lookup table the app embeds.
//
// Usage (from the repository root):
//
//	go run ./tools/gengeo -v4 dbip-country-ipv4.csv -v6 dbip-country-ipv6.csv -o geo/geo.bin
//
// The CSVs come from the npm package @ip-location-db/dbip-country, which
// republishes DB-IP's CC BY 4.0 data; see geo/SOURCE.md for the exact version
// and how to refresh it.
//
// Why a custom format instead of the .mmdb file: the app only needs a country
// code, and a sorted list of range starts can be binary-searched straight out
// of the embedded bytes with the standard library - no dependency, no copy
// into the heap. Adjacent ranges with the same country are merged first,
// which is what makes it small.
//
// File layout (all integers big-endian):
//
//	"PMGEO1"                 6 bytes magic
//	nCodes    uint16         then nCodes x 2 bytes ASCII country code; index 0 is "--" (no data)
//	n4        uint32         then n4 x uint32 range start, then n4 x uint8 code index
//	n6        uint32         then n6 x uint64 range start (upper 64 bits), then n6 x uint16 code index
//
// IPv6 is keyed on the upper 64 bits. A country boundary inside a single /64
// is not something a public peer address will sit on in practice; where two
// ranges collapse onto the same 64-bit start, the later one wins.
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/netip"
	"os"
	"sort"
)

type rng struct {
	start, end *big.Int
	cc         string
}

func main() {
	v4 := flag.String("v4", "", "dbip-country-ipv4.csv")
	v6 := flag.String("v6", "", "dbip-country-ipv6.csv")
	out := flag.String("o", "geo/geo.bin", "output file")
	flag.Parse()
	if *v4 == "" || *v6 == "" {
		log.Fatal("both -v4 and -v6 are required")
	}

	r4 := merge(read(*v4, 4))
	r6 := merge(read(*v6, 6))

	codes := []string{"--"}
	idx := map[string]int{"--": 0}
	for _, rs := range [][]rng{r4, r6} {
		for _, r := range rs {
			if _, ok := idx[r.cc]; !ok {
				idx[r.cc] = len(codes)
				codes = append(codes, r.cc)
			}
		}
	}
	if len(codes) > 255 {
		log.Fatalf("%d country codes do not fit the uint8 IPv4 index", len(codes))
	}

	type e4 struct {
		s uint32
		c uint8
	}
	type e6 struct {
		s uint64
		c uint16
	}
	var t4 []e4
	for _, r := range withGaps(r4, 32) {
		t4 = append(t4, e4{uint32(r.start.Uint64()), uint8(idx[r.cc])})
	}
	var t6 []e6
	for _, r := range withGaps(r6, 128) {
		s := new(big.Int).Rsh(r.start, 64).Uint64()
		if n := len(t6); n > 0 && t6[n-1].s == s {
			t6[n-1].c = uint16(idx[r.cc])
			continue
		}
		t6 = append(t6, e6{s, uint16(idx[r.cc])})
	}

	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	w := bufio.NewWriter(f)
	be := binary.BigEndian
	w.WriteString("PMGEO1")
	binary.Write(w, be, uint16(len(codes)))
	for _, c := range codes {
		w.WriteString(c)
	}
	binary.Write(w, be, uint32(len(t4)))
	for _, e := range t4 {
		binary.Write(w, be, e.s)
	}
	for _, e := range t4 {
		w.WriteByte(e.c)
	}
	binary.Write(w, be, uint32(len(t6)))
	for _, e := range t6 {
		binary.Write(w, be, e.s)
	}
	for _, e := range t6 {
		binary.Write(w, be, e.c)
	}
	if err := w.Flush(); err != nil {
		log.Fatal(err)
	}
	f.Close()
	st, _ := os.Stat(*out)
	fmt.Printf("codes=%d ipv4 ranges=%d ipv6 ranges=%d -> %s (%d bytes)\n", len(codes), len(t4), len(t6), *out, st.Size())
}

func read(path string, fam int) []rng {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	cr := csv.NewReader(bufio.NewReader(f))
	cr.FieldsPerRecord = 3
	var rs []rng
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		a, err1 := netip.ParseAddr(rec[0])
		b, err2 := netip.ParseAddr(rec[1])
		if err1 != nil || err2 != nil || (fam == 4) != a.Is4() || (fam == 4) != b.Is4() || len(rec[2]) != 2 {
			log.Fatalf("%s: bad row %q", path, rec)
		}
		rs = append(rs, rng{toInt(a), toInt(b), rec[2]})
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].start.Cmp(rs[j].start) < 0 })
	return rs
}

func toInt(a netip.Addr) *big.Int { return new(big.Int).SetBytes(a.AsSlice()) }

// merge joins ranges that touch and share a country.
func merge(rs []rng) []rng {
	var out []rng
	one := big.NewInt(1)
	for _, r := range rs {
		if n := len(out); n > 0 {
			last := &out[n-1]
			next := new(big.Int).Add(last.end, one)
			if last.cc == r.cc && next.Cmp(r.start) >= 0 {
				if r.end.Cmp(last.end) > 0 {
					last.end = r.end
				}
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

// withGaps inserts "--" entries wherever the data has a hole, so a lookup that
// lands between two ranges reports "no data" instead of the previous country.
func withGaps(rs []rng, bits uint) []rng {
	var out []rng
	one := big.NewInt(1)
	max := new(big.Int).Sub(new(big.Int).Lsh(one, bits), one)
	zero := big.NewInt(0)
	if len(rs) == 0 || rs[0].start.Cmp(zero) > 0 {
		out = append(out, rng{zero, zero, "--"})
	}
	for i, r := range rs {
		out = append(out, r)
		next := new(big.Int).Add(r.end, one)
		if r.end.Cmp(max) < 0 && (i+1 == len(rs) || rs[i+1].start.Cmp(next) > 0) {
			out = append(out, rng{next, next, "--"})
		}
	}
	return out
}
