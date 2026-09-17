package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/benunskilled/peer-map/geo"
)

// mockPeers returns plausible getpeerinfo output for PEERMAP_MOCK=1, so the
// dashboard can be tried - and screenshotted - without a node.
//
// Every address is from the documentation ranges (RFC 5737 for IPv4, RFC 3849
// for IPv6), never a real host, so a store screenshot cannot show somebody's
// server. Those ranges have no location by definition, so demo peers carry
// their location directly; the real lookup is covered by the tests instead.
// The dashboard shows a "demo data" banner whenever this is in use.
func mockPeers(ctx context.Context) ([]rawPeer, error) {
	// Fixed seed: the same demo node every refresh, with a few inbound peers
	// swapped out each interval so the refresh is visible.
	r := rand.New(rand.NewSource(42))
	churn := rand.New(rand.NewSource(time.Now().Unix() / int64(interval/time.Second)))
	now := time.Now().Unix()

	// Weighted roughly like a real node's peer list: heavy on Germany, the US
	// and the big hosting countries, with several regions each.
	places := [][2]string{
		{"DE", "Hesse"}, {"DE", "Bavaria"}, {"DE", "Berlin"}, {"DE", "North Rhine-Westphalia"}, {"DE", "Saxony"},
		{"US", "Virginia"}, {"US", "California"}, {"US", "Oregon"}, {"US", "New York"}, {"US", "Texas"},
		{"NL", "North Holland"}, {"NL", "North Holland"}, {"FR", "Ile-de-France"}, {"FI", "Uusimaa"},
		{"GB", "England"}, {"CA", "Quebec"}, {"CA", "Ontario"}, {"CH", "Zurich"}, {"AT", "Vienna"},
		{"SE", "Stockholm"}, {"PL", "Mazovia"}, {"CZ", "Prague"}, {"ES", "Madrid"}, {"IT", "Lombardy"},
		{"JP", "Tokyo"}, {"SG", ""}, {"HK", "Kowloon"}, {"AU", "New South Wales"}, {"AU", "Victoria"},
		{"AU", "Queensland"}, {"AU", "Western Australia"}, {"BR", "Sao Paulo"}, {"ZA", "Gauteng"}, {"IN", "Maharashtra"},
	}
	locs := make([]geo.Location, 0, len(places))
	for _, pl := range places {
		if loc, ok := geo.FindRegion(pl[0], pl[1]); ok {
			locs = append(locs, loc)
		} else {
			locs = append(locs, geo.Location{Country: pl[0]})
		}
	}
	// A real node hears from more than Bitcoin Core. The spread here is taken
	// from one evening on an actual node, so the demo shows the Kind column
	// doing its job instead of a wall of "Core".
	clients := []string{
		"/Satoshi:31.1.0/", "/Satoshi:31.1.0/", "/Satoshi:31.1.0/", "/Satoshi:30.0.0/",
		"/Satoshi:29.0.0/", "/Satoshi:28.1.0/", "/Satoshi:29.3.0/Knots:20260507/",
		"/bitcoinj:0.16.2/Bitcoin Wallet:9.26/", "/breadwallet:1.3.5/",
		"/btcwire:0.5.0/neutrino:0.17.1/", "/electrs:0.11.1/", "/Metrika-Bitnodes:0.1/",
		"/dsn.tm.kit.edu/bitcoin:0.9.99/", "/ckp2p:2.0/", "/Floresta:0.9.1/",
	}

	nodeClients := []string{}
	for _, c := range clients {
		if relaysBlocks(c) {
			nodeClients = append(nodeClients, c)
		}
	}

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
	add := func(typ string, inbound bool, network, a string, loc *geo.Location) {
		ping := 0.02 + r.Float64()*0.3
		// Only inbound gets the full spread. You never dial OUT to a phone
		// wallet or a crawler - an outbound or manual connection is one this
		// node chose, and it chooses nodes.
		pool := clients
		if !inbound {
			pool = nodeClients
		}
		subver := pool[r.Intn(len(pool))]
		// The observed flags follow the software rather than a coin toss,
		// because that is how they behave on a real node: a wallet or a
		// crawler offers nothing and Core never learns its chain, while a
		// full node does both. The rare last case is the interesting one -
		// something calling itself Core that behaves like neither.
		services, relay, headers := []string{"NETWORK", "WITNESS"}, true, int64(967000+r.Intn(400))
		switch kindOf(subver) {
		case "Wallet", "Light client", "Crawler", "Research", "Indexer":
			services, headers = []string{}, -1
			relay = r.Intn(3) > 0
		case "Core":
			if r.Intn(20) == 0 {
				services, relay, headers = []string{}, false, -1
			}
		}
		out = append(out, rawPeer{
			ID: id, Addr: a, Network: network, ConnectionType: typ, Inbound: inbound,
			Subver: subver, PingTime: &ping,
			ConnTime: now - int64(r.Intn(86400*3)), Transport: []string{"v1", "v2"}[r.Intn(2)],
			BytesSent: int64(r.Intn(50 << 20)), BytesRecv: int64(r.Intn(200 << 20)),
			ServicesNames: services, RelayTxes: &relay, SyncedHeaders: &headers,
			mockLoc: loc,
		})
		id++
	}
	public := func(rr *rand.Rand, typ string, inbound bool) {
		port := 8333
		if inbound {
			port = 40000 + rr.Intn(20000)
		}
		a, n := addr(rr, port)
		loc := locs[rr.Intn(len(locs))]
		add(typ, inbound, n, a, &loc)
	}

	for i := 0; i < 8; i++ {
		public(r, "manual", false)
	}
	for i := 0; i < 10; i++ {
		public(r, "outbound-full-relay", false)
	}
	public(r, "block-relay-only", false)
	add("block-relay-only", false, "onion", "vww6ybal4bd7szmgncyruucpgfkqahzddi37ktceo3ah7ngmcopnpyyd.onion:8333", nil)
	for i := 0; i < 22; i++ {
		src := r
		if i >= 19 {
			src = churn
		}
		public(src, "inbound", true)
	}
	add("inbound", true, "not_publicly_routable", "10.21.0.1:51234", nil)
	add("inbound", true, "not_publicly_routable", "10.21.0.1:51980", nil)
	add("inbound", true, "onion", "127.0.0.1:50122", nil)
	add("inbound", true, "i2p", "ukeu3k5oycgaauneqgtnvselmt4yemvoilkln7jpvamvfx7dnkdq.b32.i2p:0", nil)
	add("feeler", false, "ipv4", "192.0.2.250:8333", &locs[0]) // must be hidden
	return out, nil
}
