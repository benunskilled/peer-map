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
		{"3.11.221.101:33764", "/ckp2p:2.0/", "Pool node", "AtlasPool", "AtlasPool's block relay (ckp2p)"},
		// A pool's plain bitcoind becomes a pool node.
		{"66.85.130.202:8333", "/Satoshi:28.2.0/", "Pool node", "KanoPool", "KanoPool's own Bitcoin node"},
		// ckp2p elsewhere: a pool node, whose pool is not known.
		{"91.145.114.152:60964", "/ckp2p:2.0/", "Pool node", "", "ckp2p, ckpool's block relay"},
		// The user agent names the pool.
		{"203.0.113.9:8333", "/Satoshi:29.1.0(PyBLOCK-POOL)/Knots:20250903/", "Pool node", "PyBLOCK", ""},
		{"203.0.113.9:8333", "/pydroid-pool:3/", "Pool node", "pydroid", ""},
		// A name in brackets does not make an ordinary node a pool's.
		{"203.0.113.9:8333", "/Satoshi:31.1.0/", "Node (Core)", "", ""},
	} {
		p := convert([]rawPeer{{ID: 1, Addr: c.addr, ConnectionType: "inbound", Inbound: true, Subver: c.subver, Version: 70016}})[0]
		if p.Kind != c.kind || p.Pool != c.pool || !strings.HasPrefix(p.About, c.about) || (c.about != "" && strings.Contains(p.About, "This address")) {
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

// The proof goes to the panel, apart from the short line the list shows.
func TestPoolProof(t *testing.T) {
	p := convert([]rawPeer{{ID: 1, Addr: "3.228.156.48:33764", ConnectionType: "inbound", Inbound: true, Subver: "/ckp2p:2.0/", Version: 70016}})[0]
	if p.PoolProof != "This address hands out AtlasPool's mining jobs (AtlasPool/IAD)" {
		t.Errorf("got %q", p.PoolProof)
	}
}

// A pool node no rule describes still gets a line.
func TestUndescribedKindsGetALine(t *testing.T) {
	for _, ua := range []string{"/pydroid-pool:3/", "/p2p-watcher:0.1.0/"} {
		p := convert([]rawPeer{{ID: 1, Addr: "203.0.113.9:8333", ConnectionType: "inbound", Inbound: true, Subver: ua, Version: 70016}})[0]
		if p.About == "" {
			t.Errorf("%s (%s): no line", ua, p.Kind)
		}
	}
}
