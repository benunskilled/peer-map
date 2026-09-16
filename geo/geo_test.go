package geo

import (
	"encoding/json"
	"net/netip"
	"os"
	"testing"
)

// vectors.json holds 600 addresses drawn at random from the source CSVs
// (300 IPv4, 300 IPv6) with the country the CSV lists for them.
func TestAgainstSourceCSV(t *testing.T) {
	b, err := os.ReadFile("testdata/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vs [][2]string
	if err := json.Unmarshal(b, &vs); err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		if got := Country(netip.MustParseAddr(v[0])); got != v[1] {
			t.Errorf("%s: got %q want %q", v[0], got, v[1])
		}
	}
}

func TestKnownRows(t *testing.T) {
	for ip, want := range map[string]string{
		"1.0.0.1":         "AU", // first rows of dbip-country-ipv4.csv
		"1.0.2.200":       "CN",
		"2001::1":         "US", // dbip-country-ipv6.csv
		"::ffff:1.0.0.1":  "AU", // v4-mapped
		"10.21.0.1":       "",   // private: not in the data
		"192.168.178.124": "",
		"fd00::1":         "",
	} {
		if got := Country(netip.MustParseAddr(ip)); got != want {
			t.Errorf("%s: got %q want %q", ip, got, want)
		}
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := parse([]byte("nope")); err == nil {
		t.Fatal("expected error")
	}
	if _, err := parse(raw[:len(raw)-1]); err == nil {
		t.Fatal("expected error for truncated table")
	}
}

func BenchmarkCountry(b *testing.B) {
	a := netip.MustParseAddr("2a01:4f8::1")
	for i := 0; i < b.N; i++ {
		Country(a)
	}
}
