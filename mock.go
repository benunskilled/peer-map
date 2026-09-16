package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// mockPeers returns plausible getpeerinfo output for PEERMAP_MOCK=1, so the
// dashboard can be tried - and screenshotted - without a node.
//
// Every address is from the documentation ranges (RFC 5737 for IPv4, RFC 3849
// for IPv6), never a real host, so a store screenshot cannot show somebody's
// server. Those ranges have no location by definition, so demo peers carry
// their country directly; the real lookup is covered by the tests instead.
// The dashboard shows a "demo data" banner whenever this is in use.
func mockPeers(ctx context.Context) ([]rawPeer, error) {
	// Fixed seed: the same demo node every refresh, with a few inbound peers
	// swapped out each interval so the refresh is visible.
	r := rand.New(rand.NewSource(42))
	churn := rand.New(rand.NewSource(time.Now().Unix() / int64(interval/time.Second)))
	now := time.Now().Unix()

	// Weighted roughly like a real node's peer list: heavy on Germany, the US
	// and the big hosting countries.
	countries := []string{"DE", "DE", "DE", "DE", "US", "US", "US", "US", "NL", "NL", "FR", "FR", "FI",
		"GB", "CA", "CH", "AT", "SE", "PL", "CZ", "ES", "IT", "JP", "SG", "HK", "AU", "BR", "ZA", "IN", "RU"}
	clients := []string{"/Satoshi:31.1.0/", "/Satoshi:31.1.0/", "/Satoshi:30.0.0/", "/Satoshi:29.0.0/", "/Satoshi:28.1.0/", "/Knots:20250305/"}

	var out []rawPeer
	id := int64(1)
	v4, v6 := 0, 0
	addr := func(rr *rand.Rand, port int) (string, string) {
		if rr.Intn(4) == 0 {
			v6++
			return fmt.Sprintf("[2001:db8:%x::%x]:%d", rr.Intn(0xffff), v6, port), "ipv6"
		}
		v4++
		nets := []string{"192.0.2", "198.51.100", "203.0.113"}
		return fmt.Sprintf("%s.%d:%d", nets[v4%3], 1+(v4*37)%253, port), "ipv4"
	}
	add := func(typ string, inbound bool, network, a, cc string) {
		ping := 0.02 + r.Float64()*0.3
		out = append(out, rawPeer{
			ID: id, Addr: a, Network: network, ConnectionType: typ, Inbound: inbound,
			Subver: clients[r.Intn(len(clients))], PingTime: &ping,
			ConnTime: now - int64(r.Intn(86400*3)), Transport: []string{"v1", "v2"}[r.Intn(2)],
			BytesSent: int64(r.Intn(50 << 20)), BytesRecv: int64(r.Intn(200 << 20)),
			mockCountry: cc,
		})
		id++
	}
	public := func(rr *rand.Rand, typ string, inbound bool) {
		port := 8333
		if inbound {
			port = 40000 + rr.Intn(20000)
		}
		a, n := addr(rr, port)
		add(typ, inbound, n, a, countries[rr.Intn(len(countries))])
	}

	for i := 0; i < 8; i++ {
		public(r, "manual", false)
	}
	for i := 0; i < 10; i++ {
		public(r, "outbound-full-relay", false)
	}
	public(r, "block-relay-only", false)
	add("block-relay-only", false, "onion", "vww6ybal4bd7szmgncyruucpgfkqahzddi37ktceo3ah7ngmcopnpyyd.onion:8333", "")
	for i := 0; i < 22; i++ {
		src := r
		if i >= 19 {
			src = churn
		}
		public(src, "inbound", true)
	}
	add("inbound", true, "not_publicly_routable", "10.21.0.1:51234", "")
	add("inbound", true, "not_publicly_routable", "10.21.0.1:51980", "")
	add("inbound", true, "onion", "127.0.0.1:50122", "")
	add("inbound", true, "i2p", "ukeu3k5oycgaauneqgtnvselmt4yemvoilkln7jpvamvfx7dnkdq.b32.i2p:0", "")
	add("feeler", false, "ipv4", "192.0.2.250:8333", "DE") // must be hidden
	return out, nil
}
