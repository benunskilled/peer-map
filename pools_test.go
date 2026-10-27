package main

import (
	"strings"
	"testing"
)

func TestPoolNodes(t *testing.T) {
	for _, c := range []struct {
		addr, subver, kind, pool, about string
	}{
		// A proven address names the pool; ckp2p stays a relay.
		{"3.11.221.101:33764", "/ckp2p:2.0/", "Pool node", "AtlasPool", "AtlasPool's block relay, ckpool's ckp2p: passes on blocks only, never transactions. This address hands out AtlasPool's mining jobs (AtlasPool/FRA)"},
		// A pool's plain bitcoind becomes a pool node.
		{"66.85.130.202:8333", "/Satoshi:28.2.0/", "Pool node", "Kano pool", "Kano pool's own Bitcoin node. This address is stratum.kano.is, and hands out KanoPool's mining jobs"},
		// ckp2p elsewhere: a pool node, whose pool is not known.
		{"91.145.114.152:60964", "/ckp2p:2.0/", "Pool node", "", "ckp2p, ckpool's block relay"},
		// The user agent names the pool.
		{"203.0.113.9:8333", "/Satoshi:29.1.0(PyBLOCK-POOL)/Knots:20250903/", "Pool node", "PyBLOCK", ""},
		{"203.0.113.9:8333", "/pydroid-pool:3/", "Pool node", "pydroid", ""},
		// A name in brackets does not make an ordinary node a pool's.
		{"203.0.113.9:8333", "/Satoshi:31.1.0/", "Node (Core)", "", ""},
	} {
		p := convert([]rawPeer{{ID: 1, Addr: c.addr, ConnectionType: "inbound", Inbound: true, Subver: c.subver, Version: 70016}})[0]
		if p.Kind != c.kind || p.Pool != c.pool || !strings.HasPrefix(p.About, c.about) {
			t.Errorf("%s %s: got %q %q %q", c.addr, c.subver, p.Kind, p.Pool, p.About)
		}
	}
}

// A Disguised peer on a pool's address keeps what it is: only pool and
// node kinds take the pool's name.
func TestPoolNodeKeepsDisguised(t *testing.T) {
	p := convert([]rawPeer{{ID: 1, Addr: "66.85.130.202:8333", ConnectionType: "inbound", Inbound: true, Subver: "/bitcoinj:0.16.5/Bitcoin Wallet:10.26/", Version: 70016, ServicesNames: []string{"NETWORK", "WITNESS"}}})[0]
	if p.Pool != "" {
		t.Errorf("got pool %q on %q", p.Pool, p.Kind)
	}
}
