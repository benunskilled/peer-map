package main

import (
	"net/netip"
	"regexp"
)

// Which mining pool a peer belongs to. A pool's node rarely says so in its
// user agent: ckpool's block relay calls itself /ckp2p:2.0/ for every pool
// that runs it, and a pool's own bitcoind is a plain /Satoshi:/. So the pool
// is named here only where it was proven, by address: on 27.10.2026 each of
// these addresses had connected to the node this was written for, and the
// same address either handed out mining jobs carrying the pool's coinbase tag
// to a stratum subscribe on port 3333 (the tag in brackets), or is what the
// pool's own stratum host name resolves to. Peers of a pool that are not on
// this list stay "Pool node" without a name: a pool moves its servers.
type poolNode struct {
	pool, proof string
}

var poolNodes = map[netip.Addr]poolNode{
	// AtlasPool runs ckpool at points of presence on Amazon. The tag names its
	// own site, not where the server is: the one in London says FRA.
	netip.MustParseAddr("3.11.221.101"):  {"AtlasPool", "hands out AtlasPool's mining jobs (AtlasPool/FRA)"},
	netip.MustParseAddr("63.184.166.44"): {"AtlasPool", "hands out AtlasPool's mining jobs (AtlasPool/FRA)"},
	netip.MustParseAddr("3.228.156.48"):  {"AtlasPool", "hands out AtlasPool's mining jobs (AtlasPool/IAD)"},
	netip.MustParseAddr("32.189.241.52"): {"AtlasPool", "hands out AtlasPool's mining jobs (AtlasPool/PDX)"},
	// ckpool tagged /pool.mutatrum.com/ on 3333, Public-Pool on 3334.
	netip.MustParseAddr("95.168.173.100"): {"mutatrum", "hands out mutatrum's mining jobs (/pool.mutatrum.com/)"},
	netip.MustParseAddr("66.85.130.202"):  {"Kano pool", "is stratum.kano.is, and hands out KanoPool's mining jobs"},
	netip.MustParseAddr("62.2.188.226"):   {"Blitzpool", "is blitzpool.yourdevice.ch, and hands out Blitzpool's mining jobs"},
	netip.MustParseAddr("179.27.118.130"): {"PyBLOCK", "is pool.pyblock.xyz"},
	netip.MustParseAddr("144.202.96.186"): {"MintSolo", "hands out MintSolo's mining jobs (MintSolo GSS)"},
	netip.MustParseAddr("149.28.149.108"): {"MintSolo", "hands out MintSolo's mining jobs (MintSolo GSS)"},
	// Says (xbtpool.io); xbtpool is ctrlpool now, and its jobs on 4334 are tagged CTRL.
	netip.MustParseAddr("161.97.128.46"): {"CTRL", "hands out ctrlpool's mining jobs (CTRL), formerly xbtpool"},
}

// A pool the user agent names itself, where no address says more.
var poolLabels = []struct {
	re   *regexp.Regexp
	pool string
}{
	{regexp.MustCompile(`(?i)\(PyBLOCK-POOL\)`), "PyBLOCK"},
	{regexp.MustCompile(`(?i)\(xbtpool\.io\)`), "CTRL"},
	{regexp.MustCompile(`(?i)^/pydroid-pool:`), "pydroid"},
}

// poolOf says which pool a peer belongs to, and how that is known: from the
// list of proven addresses, else from a pool the user agent names. The second
// result says the address proved it.
func poolOf(addr, subver string) (poolNode, bool) {
	if a, err := netip.ParseAddr(hostOf(addr)); err == nil {
		if n, ok := poolNodes[a.Unmap()]; ok {
			return n, true
		}
	}
	for _, l := range poolLabels {
		if l.re.MatchString(subver) {
			return poolNode{pool: l.pool}, false
		}
	}
	return poolNode{}, false
}
