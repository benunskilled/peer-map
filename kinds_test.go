package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestKindOf(t *testing.T) {
	for subver, want := range map[string]string{
		"/Satoshi:31.1.0/":                         "Node (Core)",
		"/Satoshi:29.3.0/Knots:20260507/":          "Node (Knots)",
		"/bitcoinj:0.16.2/Bitcoin Wallet:9.26/":    "Wallet",
		"/breadwallet:1.3.5/":                      "Wallet",
		"/btcwire:0.5.0/neutrino:0.17.1/":          "Lightning wallet",
		"/Rust BIP-157:0.6.0/rust-bitcoin:0.32.8/": "Light client",
		// Seen on a real node, all of them "Other" before.
		"/bitcoin-seeder:0.01/":          "Crawler",
		"/btc-range-scan:0.1.0/":         "Crawler",
		"/BTC-Nodes:2026-09-24/Sonar/":   "Crawler",
		"/census:0.1.7/":                 "Crawler",
		"/btc-node-observatory:0.1.0/":   "Crawler",
		"/go-bitnode-monitor:firstseen/": "Crawler",
		"/atlas-scout:0.1/":              "Crawler",
		"nebula/2.4.1-85b3e43":           "Crawler",
		"/onlytwentyone-crawler:0.1/":    "Crawler",
		// The whole reason the observed flags exist: a zero in place of the o,
		// seen from rented machines in five regions. It must not come out as
		// Core. It was "Other" until the Fake kind existed to say what it is.
		"/Sat0shi:31.0.0/":                              "Disguised",
		"/Satoshi2:0.18.3/":                             "Disguised",
		"/SatoshiX:0.18.0/":                             "Disguised",
		"Satoshi:22.0.0":                                "Disguised",
		"/Satoshi:22.0.0/":                              "Node (Core)",
		"/Floresta:0.9.1/mandacaru:0.15.2/":             "Node (Floresta)",
		"/electrs:0.11.1/":                              "Indexer",
		"/Metrika-Bitnodes:0.1/":                        "Crawler",
		"/bitnodes.io:0.3/":                             "Crawler",
		"/GlobalNodeMap:2.1/":                           "Crawler",
		"/dsn.tm.kit.edu/bitcoin:0.9.99/":               "Research scanner",
		"/ckp2p:2.0/":                                   "Pool node",
		"/CKCoinD:31.1.0/":                              "Pool node",
		"/rust-bitcoin:0.18/bluematt-tokio-client:0.1/": "Crawler",
		// The operator's own word, in the comment part - above the software.
		"/Satoshi:29.1.0(PyBLOCK-POOL)/Knots:20250903/": "Pool node",
		"/Satoshi:31.0.0(pool)/":                        "Pool node",
		// As it arrives on a real node, with a URL after the Knots part.
		"/Satoshi:29.1.0(PyBLOCK-POOL)/Knots:20250903/https://pyblock.xyz:8443/": "Pool node",
		// PyBLOCK's node without the pool: a Knots node, not a pool.
		"/Satoshi:29.4.2(PyBLOCK-BIP110)/Knots:20260508/": "Node (Knots)",
		// "pool" outside the brackets is not a claim: this is an indexer.
		"/mempool:3.0.0/electrs:0.10.0/": "Indexer",
		// "mempool" in the brackets is a node's label, not a pool (seen on a real node)
		"/Satoshi:29.4.2(mempool.guide)/Knots:20260508rc2/": "Node (Knots)",
		"/mempool/":                     "Indexer",
		"/mempool.space:1.0/":           "Indexer",
		"/Bitcoin ABC:0.14.5(EB8.0)/":   "Other chain",
		"/Floresta:0.9.1/":              "Node (Floresta)",
		"/btcwire:0.5.0/hemi-soak:1.0/": "Other",
		"":                              noAgentKind,
	} {
		if got := kindOf(subver); got != want {
			t.Errorf("%q: got %q want %q", subver, got, want)
		}
	}
}

// Knots contains "Satoshi" as well, so the order of the rules decides this
// one. A rule moved above Knots would break it silently.
func TestKnotsBeatsCore(t *testing.T) {
	if got := kindOf("/Satoshi:29.1.0/Knots:20250903/"); got != "Node (Knots)" {
		t.Errorf("got %q, want Node (Knots)", got)
	}
}

func TestObservedFlags(t *testing.T) {
	no, yes := false, true
	minusOne, height := int64(-1), int64(967308)

	cases := []struct {
		name                                string
		raw                                 rawPeer
		noServices, noTxRelay, chainUnknown bool
	}{
		{"nothing reported", rawPeer{}, false, false, false},
		{"a full node", rawPeer{
			ServicesNames: []string{"NETWORK", "WITNESS"}, RelayTxes: &yes, SyncedHeaders: &height,
		}, false, false, false},
		{"offers nothing, wants nothing, chain unknown", rawPeer{
			ServicesNames: []string{}, RelayTxes: &no, SyncedHeaders: &minusOne,
		}, true, true, true},
	}
	for _, c := range cases {
		c.raw.Addr = "1.0.0.1:8333"
		c.raw.ConnectionType = "inbound"
		c.raw.Inbound = true
		got := convert([]rawPeer{c.raw})
		if len(got) != 1 {
			t.Fatalf("%s: got %d peers", c.name, len(got))
		}
		p := got[0]
		if p.NoServices && !p.NoRelay {
			t.Errorf("%s: offers nothing but is not marked as unable to deliver", c.name)
		}
		if p.NoServices != c.noServices || p.NoTxRelay != c.noTxRelay || p.ChainUnknown != c.chainUnknown {
			t.Errorf("%s: got no_services=%v no_tx=%v chain_unknown=%v, want %v/%v/%v",
				c.name, p.NoServices, p.NoTxRelay, p.ChainUnknown, c.noServices, c.noTxRelay, c.chainUnknown)
		}
	}
}

// The red mark in the table, and the red pill in Bitcoin Lab, come from this
// one answer. It has to stay the same in both apps.
func TestRelaysBlocks(t *testing.T) {
	for subver, want := range map[string]bool{
		"/Satoshi:31.1.0/":                      true,
		"/Satoshi:29.3.0/Knots:20260507/":       true,
		"/ckp2p:2.0/":                           true,
		"/Floresta:0.9.1/":                      true,
		"/Sat0shi:31.0.0/":                      true, // unrecognised is not the same as known bad
		"":                                      true, // no user agent at all: we do not know
		"/bitcoinj:0.16.2/Bitcoin Wallet:9.26/": false,
		"/breadwallet:1.3.5/":                   false,
		"/btcwire:0.5.0/neutrino:0.17.1/":       false,
		"/electrs:0.11.1/":                      false,
		"/Metrika-Bitnodes:0.1/":                false,
		"/dsn.tm.kit.edu/bitcoin:0.9.99/":       false,
		"/Bitcoin ABC:0.14.5(EB8.0)/":           false,
	} {
		if got := relaysBlocks(subver); got != want {
			t.Errorf("%q: relaysBlocks = %v, want %v", subver, got, want)
		}
	}
}

// The page explains each kind in a sentence (KIND_WHY in web/app.js). A kind
// added here without a line there would show its filter with no reason given.
func TestEveryKindIsExplainedOnThePage(t *testing.T) {
	js, err := os.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"Other", noAgentKind, fakeWalletKind}
	for _, r := range kindRules {
		names = append(names, r.name)
	}
	for _, n := range names {
		if !strings.Contains(string(js), `"`+n+`": `) {
			t.Errorf("KIND_WHY in web/app.js has no line for %q", n)
		}
	}
}

// A 2016 wallet name on something offering blocks and v2 is not that wallet.
// The real Bitcoin Wallet, offering nothing, stays a wallet and says what it is.
func TestFakeWallet(t *testing.T) {
	cases := []struct {
		subver    string
		services  []string
		kind      string
		aboutSays string
	}{
		{"/bitcoinj:0.14.5/Bitcoin Wallet:5.42/", []string{"NETWORK", "WITNESS", "NETWORK_LIMITED", "P2P_V2"}, fakeWalletKind, "Probably: a node or scanner behind an old wallet name"},
		{"/breadwallet:0.6.5/", []string{"NETWORK"}, fakeWalletKind, "Probably: a node or scanner behind an old wallet name"},
		{"/bitcoinj:0.16.2/Bitcoin Wallet:9.26/", []string{}, "Wallet", "Schildbach"},
		{"/bitcoinj:0.16.2/Bitcoin Wallet:9.26/", nil, "Wallet", "Schildbach"},
		// Core's name misspelt: what it claims, and a guess from what it offers.
		{"/Sat0shi:31.0.0/", []string{}, fakeWalletKind, "Claims: Bitcoin Core 31.0.0\nProbably: not Bitcoin Core - it writes Sat0shi with a zero and offers no services"},
		{"/Satoshi2:0.18.3/", []string{"NETWORK", "WITNESS"}, fakeWalletKind, "Probably: not Bitcoin Core - Core calls itself /Satoshi:, this one /Satoshi2:"},
		{"/SatoshiX:0.18.0/", []string{"NETWORK", "WITNESS"}, fakeWalletKind, "Probably: not Bitcoin Core - Core calls itself /Satoshi:, this one /SatoshiX:"},
		{"Satoshi:22.0.0", []string{"NETWORK"}, fakeWalletKind, "Claims: Bitcoin Core 22.0.0\nProbably: not Bitcoin Core - Core writes its name between slashes: /Satoshi:22.0.0/"},
		// Only a wallet name can be fake: a node offering blocks is a node.
		{"/Satoshi:31.1.0/", []string{"NETWORK", "WITNESS", "NETWORK_LIMITED", "P2P_V2"}, "Node (Core)", ""},
	}
	for _, c := range cases {
		got := convert([]rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "inbound", Inbound: true,
			Subver: c.subver, ServicesNames: c.services}})[0]
		if got.Kind != c.kind {
			t.Errorf("%s %v: kind %q, want %q", c.subver, c.services, got.Kind, c.kind)
		}
		if c.aboutSays == "" && got.About != "" || !strings.Contains(got.About, c.aboutSays) {
			t.Errorf("%s %v: about %q, want it to say %q", c.subver, c.services, got.About, c.aboutSays)
		}
	}
}

// Core versions offering what they never had: the kind says Fake, and why.
// Real peers of their own version stay what they are.
func TestAnachronism(t *testing.T) {
	v2 := []string{"NETWORK", "WITNESS", "NETWORK_LIMITED", "P2P_V2"}
	cases := []struct {
		subver   string
		services []string
		kind     string
		says     string
	}{
		{"/Satoshi:0.14.2/", v2, "Disguised", "Claims: Bitcoin Core 0.14.2\nProbably: a scanner - it offers encrypted v2 connections, which Core 0.14 did not have (from Core 26)"},
		{"/Satoshi:25.1.0/", v2, "Disguised", "Core 25.1 did not have"},
		{"/Satoshi:0.15.1/", []string{"NETWORK", "WITNESS", "NETWORK_LIMITED"}, "Disguised", "pruned-node service, which Core 0.15"},
		{"/Satoshi:0.20.1/", []string{"NETWORK", "WITNESS", "COMPACT_FILTERS"}, "Disguised", "compact block filters"},
		{"/Satoshi:25.1.0/Knots:20231115/", v2, "Disguised", "Claims: Bitcoin Knots (Core 25.1.0)"},
		{"/bcoin:v1.0.0-beta.14/", v2, "Disguised", "Claims: bcoin v1.0.0-beta.14"},
		{"/Classic:1.3.4(EB8)/", v2, "Disguised", "never had"},
		{"/Satoshi:31.1.0/", v2, "Node (Core)", ""},
		{"/Satoshi:26.0.0/", v2, "Node (Core)", ""},
		{"/Satoshi:0.21.0/", []string{"NETWORK", "WITNESS", "COMPACT_FILTERS", "NETWORK_LIMITED"}, "Node (Core)", ""},
		{"/Satoshi:0.14.2/", []string{"NETWORK", "WITNESS"}, "Node (Core)", ""},
		{"/Satoshi:31.0.0/", []string{"NETWORK", "WITNESS"}, "Disguised", "lacks the pruned-node service"},
		{"/Satoshi:0.16.0/", []string{"NETWORK", "WITNESS"}, "Disguised", "since 0.16"},
		{"/Satoshi:0.15.1/", []string{"NETWORK", "WITNESS"}, "Node (Core)", ""},
		{"/Satoshi:0.19.0.1/", []string{"WITNESS"}, "Disguised", "lacks the pruned-node service"},
		{"/Satoshi:30.2.0/", []string{"WITNESS", "P2P_V2"}, "Disguised", "lacks the pruned-node service"},
		{"/Floresta:0.9.1/", []string{"WITNESS", "P2P_V2"}, "Node (Floresta)", "Floresta"},
		{"/libbitcoin:4.0.0/", []string{"NETWORK", "WITNESS"}, "Node (libbitcoin)", "libbitcoin"},
	}
	for _, c := range cases {
		got := convert([]rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "inbound", Inbound: true,
			Subver: c.subver, ServicesNames: c.services}})[0]
		if got.Kind != c.kind || !strings.Contains(got.About, c.says) {
			t.Errorf("%s %v: got %q / %q, want %q containing %q", c.subver, c.services, got.Kind, got.About, c.kind, c.says)
		}
	}
}

// Your own electrs reaches Core from inside Umbrel's network, and says so.
func TestOwnIndexer(t *testing.T) {
	got := convert([]rawPeer{{Addr: "10.21.21.10:40216", ConnectionType: "inbound", Inbound: true,
		Network: "not_publicly_routable", Subver: "/electrs:0.11.1/"}})[0]
	if got.Kind != "Indexer" || got.About != "Probably your own electrs 0.11.1 on this Umbrel" || got.App != "electrs" {
		t.Errorf("got %q / %q / %q", got.Kind, got.About, got.App)
	}
	got = convert([]rawPeer{{Addr: "10.21.22.199:40216", ConnectionType: "inbound", Inbound: true,
		Network: "not_publicly_routable", Subver: "/ElectrumX:1.16.0/"}})[0]
	if got.App != "ElectrumX" || got.About != "Probably your own ElectrumX 1.16.0 on this Umbrel" {
		t.Errorf("electrumx: got %q / %q", got.About, got.App)
	}
	got = convert([]rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "inbound", Inbound: true, Subver: ""}})[0]
	if got.Kind != noAgentKind || !strings.Contains(got.About, "checks the port") {
		t.Errorf("no agent: got %q / %q", got.Kind, got.About)
	}
}

func TestLinkingLion(t *testing.T) {
	for _, a := range []string{"143.20.137.5:51234", "[2602:f5c0:0:ace::72:300]:8333", "[2602:f5c0:0:ace::60]:8333",
		"104.234.118.2:41000", "69.17.52.1:8333", "91.198.115.9:1", "[::ffff:31.58.215.200]:8333", "199.116.84.20:8333"} {
		if !isLinkingLion(a) {
			t.Errorf("%s not recognised", a)
		}
	}
	// the rest of the provider's /32 and the neighbours of the single hosts are not LinkingLion
	for _, a := range []string{"143.20.138.5:8333", "[2604:d500:4:2::1]:8333", "[2602:f5c0:1::7]:8333",
		"104.234.118.3:8333", "69.17.52.2:8333", "10.21.0.4:40000", "abc.onion:8333", ""} {
		if isLinkingLion(a) {
			t.Errorf("%s wrongly recognised", a)
		}
	}
	got := convert([]rawPeer{
		{Addr: "87.229.79.10:40000", Subver: "/bitcoinj:0.14.4/Bitcoin Wallet:5.24/", ServicesNames: []string{"NETWORK", "P2P_V2"}, Inbound: true},
		{Addr: "87.229.79.11:40000", Subver: "", Inbound: true},
	})
	if len(got) != 2 {
		t.Fatalf("got %d peers, want 2", len(got))
	}
	// with a borrowed name: Disguised; without any name: still "No user agent"
	if got[0].Kind != fakeWalletKind || !strings.Contains(got[0].About, linkingLionAbout) {
		t.Errorf("%s: kind %q about %q", got[0].Addr, got[0].Kind, got[0].About)
	}
	if got[1].Kind != noAgentKind || !strings.Contains(got[1].About, linkingLionAbout) {
		t.Errorf("%s: kind %q about %q", got[1].Addr, got[1].Kind, got[1].About)
	}
}

// A peer naming Core that offers no service at all is not Core: Core always
// offers WITNESS and NETWORK or NETWORK_LIMITED (one node: 6,954 sessions,
// 73% on Amazon, none ever first). nil = Core did not say, which is not this.
func TestCoreOfferingNothing(t *testing.T) {
	no := []string{}
	got := convert([]rawPeer{{Addr: "198.51.100.7:40000", Inbound: true, ConnectionType: "inbound", Subver: "/Satoshi:31.0.0/", ServicesNames: no}})
	if len(got) != 1 || got[0].Kind != fakeWalletKind || got[0].About != "Claims: Bitcoin Core 31.0.0\nProbably: not Bitcoin Core - it offers no services at all; Core always offers NETWORK or NETWORK_LIMITED" {
		t.Fatalf("Core offering nothing: %+v", got)
	}
	for _, svc := range [][]string{nil, {"NETWORK", "WITNESS", "NETWORK_LIMITED"}, {"NETWORK_LIMITED", "WITNESS"}} {
		got := convert([]rawPeer{{Addr: "198.51.100.8:40000", Inbound: true, ConnectionType: "inbound", Subver: "/Satoshi:31.0.0/", ServicesNames: svc}})
		if len(got) != 1 || got[0].Kind != "Node (Core)" {
			t.Errorf("services %v: kind %q, want Node (Core)", svc, got[0].Kind)
		}
	}
	// a wallet offering nothing stays a wallet
	got = convert([]rawPeer{{Addr: "198.51.100.9:40000", Inbound: true, ConnectionType: "inbound", Subver: "/bitcoinj:0.16.2/Bitcoin Wallet:9.0/", ServicesNames: no}})
	if got[0].Kind != "Wallet" {
		t.Errorf("wallet offering nothing: %q", got[0].Kind)
	}
}

// Inbound from umbrelOS's Docker gateway is IPv6 with the address hidden;
// outbound or any other private address is not.
func TestHiddenIPv6(t *testing.T) {
	cases := []struct {
		addr    string
		inbound bool
		want    string
	}{
		{"10.21.0.1:51234", true, hiddenIPv6},
		{"10.21.0.1:51234", false, "not_publicly_routable"},
		{"10.21.22.10:40000", true, torHidden},
		{"10.21.22.11:40000", true, i2pHidden},
		{"10.21.21.10:40216", true, "not_publicly_routable"}, // an app on this Umbrel
	}
	for _, c := range cases {
		typ := "outbound-full-relay"
		if c.inbound {
			typ = "inbound"
		}
		got := convert([]rawPeer{{Addr: c.addr, Network: "not_publicly_routable", ConnectionType: typ, Inbound: c.inbound, Subver: "/Satoshi:31.0.0/"}})
		if len(got) != 1 || got[0].Network != c.want {
			t.Errorf("%s inbound=%v: network %+v, want %s", c.addr, c.inbound, got, c.want)
		}
		wantOp := "Your Umbrel's own network"
		if c.want != "not_publicly_routable" {
			wantOp = ""
		}
		if got[0].Country != "" || got[0].Operator != wantOp {
			t.Errorf("%s: looked up %q %q", c.addr, got[0].Country, got[0].Operator)
		}
	}
}

// Every LinkingLion peer is flagged for the header count, whatever kind its
// user agent made it; nothing else is.
func TestLinkingLionFlag(t *testing.T) {
	got := convert([]rawPeer{
		{Addr: "104.234.118.2:41000", Inbound: true, ConnectionType: "inbound", Subver: "/bitcoinj:0.14.4/Bitcoin Wallet:5.36/"},
		{Addr: "143.20.137.9:41000", Inbound: true, ConnectionType: "inbound", Subver: ""},
		{Addr: "198.51.100.7:8333", Inbound: true, ConnectionType: "inbound", Subver: "/Satoshi:31.0.0/", ServicesNames: []string{"NETWORK", "WITNESS"}},
	})
	if !got[0].LinkingLion || !got[1].LinkingLion || got[2].LinkingLion {
		t.Fatalf("flags: %v %v %v", got[0].LinkingLion, got[1].LinkingLion, got[2].LinkingLion)
	}
}

// Every Core release sends one protocol version: 70016 from 0.21 (70017 in
// development builds), 70015 or less before.
func TestProtocolMismatch(t *testing.T) {
	core := []string{"NETWORK", "WITNESS", "NETWORK_LIMITED"}
	cases := []struct {
		subver  string
		version int
		kind    string
		says    string
	}{
		{"/Satoshi:31.0.0/", 70015, "Disguised", "Claims: Bitcoin Core 31.0.0\nProbably: unknown software - it speaks protocol 70015, older than any Core since 0.21"},
		{"/Satoshi:0.21.0/", 70015, "Disguised", "protocol 70015"},
		{"/Satoshi:0.13.2/", 70016, "Disguised", "which Core 0.13 never did"},
		{"/Satoshi:25.1.0/Knots:20231115/", 70015, "Disguised", "Claims: Bitcoin Knots"},
		{"/Satoshi:31.1.0/", 70016, "Node (Core)", ""},
		{"/Satoshi:31.99.0/", 70017, "Node (Core)", ""},
		{"/Satoshi:0.21.0/", 70016, "Node (Core)", ""},
		{"/Satoshi:0.20.1/", 70015, "Node (Core)", ""},
		{"/Satoshi:0.20.99/", 70016, "Node (Core)", ""}, // a 0.21 development build
		{"/Satoshi:0.13.2/", 70014, "Node (Core)", ""},
		{"/Satoshi:31.1.0/", 0, "Node (Core)", ""},         // Core did not report it
		{"/Satoshi:1.14.5(2x)/", 70015, "Node (Core)", ""}, // not a Core release number: left alone
	}
	for _, c := range cases {
		svc := core
		if strings.HasPrefix(c.subver, "/Satoshi:0.13") {
			svc = []string{"NETWORK", "WITNESS"} // before NETWORK_LIMITED
		}
		got := convert([]rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "inbound", Inbound: true,
			Subver: c.subver, ServicesNames: svc, Version: c.version}})[0]
		if got.Kind != c.kind || !strings.Contains(got.About, c.says) {
			t.Errorf("%s %d: got %q / %q, want %q containing %q", c.subver, c.version, got.Kind, got.About, c.kind, c.says)
		}
	}
}

// bitcoinj has never sent more than 70013, so a wallet name on 70016 is not
// that wallet - even when it offers nothing, like the real one.
func TestBitcoinjTooNew(t *testing.T) {
	cases := []struct {
		subver  string
		version int
		kind    string
		about   string
	}{
		{"/bitcoinj:0.16.2/Bitcoin Wallet:9.26/", 70016, fakeWalletKind, "Claims: the wallet Bitcoin Wallet by Schildbach\nProbably: not a wallet - speaks a newer protocol than this wallet ever did"},
		{"/bitcoinj:0.16.5/Bitcoin Wallet:10.26/", 70015, fakeWalletKind, "Probably: not a wallet"},
		// Another program on bitcoinj could set its own number, a later
		// bitcoinj could raise it: both are left alone.
		{"/bitcoinj:0.16/", 70016, "Wallet", ""},
		{"/bitcoinj:0.18.0/Bitcoin Wallet:12.0/", 70016, "Wallet", "Schildbach"},
		{"/bitcoinj:0.16.2/Bitcoin Wallet:9.26/", 70013, "Wallet", "Schildbach"},
		{"/bitcoinj:0.14.6/", 70001, "Wallet", ""},
		// A version Core did not report says nothing either way.
		{"/bitcoinj:0.16.5/Bitcoin Wallet:10.26/", 0, "Wallet", "Schildbach"},
		// Neutrino sends 70013 or 70016 depending on its btcd: left alone.
		{"/btcwire:0.5.0/neutrino:0.17.1/", 70016, "Lightning wallet", "Neutrino"},
	}
	for _, c := range cases {
		got := convert([]rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "inbound", Inbound: true,
			Subver: c.subver, Version: c.version, ServicesNames: []string{}}})[0]
		if got.Kind != c.kind || !strings.Contains(got.About, c.about) {
			t.Errorf("%s %d: %q / %q, want %q / %q", c.subver, c.version, got.Kind, got.About, c.kind, c.about)
		}
	}
}

// Core's private broadcast and python-bitcoinlib say what they are; a peer
// that introduced itself without a name is told apart from one still silent.
func TestOtherAbouts(t *testing.T) {
	cases := []struct {
		subver  string
		version int
		about   string
	}{
		{"/pynode:0.0.1/", 70016, "Probably Bitcoin Core handing over a transaction privately"},
		{"/python-bitcoinlib:0.12.2/", 70016, "python-bitcoinlib"},
		{"", 70016, "Introduced itself but left its name empty"},
		{"", 0, "Has not said what it is"},
		{"/bitcoinj:0.17/", 70013, "bitcoinj, a Java library"},
		{"/dsn.kastel.kit.edu/bitcoin:28.0.0/", 70016, "KIT Karlsruhe's Bitcoin network monitoring"},
		{"/btcwire:0.5.0/:/", 70016, "A program on btcd's Go library that left its own name empty"},
		{"/GlobalNodeMap:2.1/", 70016, "Probably a node map or counter"},
		{"/reset-header-agent:1.0/", 70016, "Software with a name we don't recognise"},
	}
	for _, c := range cases {
		got := convert([]rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "inbound", Inbound: true,
			Subver: c.subver, Version: c.version, ServicesNames: []string{}}})[0]
		if !strings.HasPrefix(got.About, c.about) {
			t.Errorf("%q %d: about %q, want %q", c.subver, c.version, got.About, c.about)
		}
	}
}

// The time connected is added once it is a day or more, never on its own.
func TestStayedLong(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	cases := []struct {
		hours float64
		want  string
	}{
		{1.4, ""},
		{23.9, ""},
		{24, ", and has stayed connected for a day"},
		{278, ", and has stayed connected for 11 days"},
	}
	for _, c := range cases {
		start := now.Add(-time.Duration(c.hours * float64(time.Hour))).Unix()
		if got := stayedLong(start, now); got != c.want {
			t.Errorf("%.1f h: %q, want %q", c.hours, got, c.want)
		}
	}
	if got := stayedLong(0, now); got != "" {
		t.Errorf("no conntime: %q", got)
	}
}

// A name nothing knows still gets a line: a full node's if it offers blocks.
func TestOtherAboutOffersBlocks(t *testing.T) {
	got := convert([]rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "inbound", Inbound: true,
		Subver: "/GrayersDad:1.0/", Version: 70016, ServicesNames: []string{"NETWORK", "WITNESS"}}})[0]
	if got.Kind != "Other" || got.About != "Probably a full node under a name its operator chose" {
		t.Errorf("kind %q about %q", got.Kind, got.About)
	}
}

// bitcoinj without an app name, offering blocks, claims bitcoinj - not Core.
func TestPlainBitcoinjOfferingBlocks(t *testing.T) {
	got := convert([]rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "inbound", Inbound: true,
		Subver: "/bitcoinj:0.14.5/", Version: 70016, ServicesNames: []string{"NETWORK", "P2P_V2"}}})[0]
	want := "Claims: bitcoinj 0.14.5\nProbably: a node or scanner behind an old wallet name - it offers blocks, which no wallet does"
	if got.Kind != "Disguised" || got.About != want {
		t.Errorf("kind %q about %q", got.Kind, got.About)
	}
}

func TestBehaviourAbout(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	hour := now.Unix() - 3600
	cases := []struct {
		name      string
		subver    string
		transport string
		recv      map[string]int64
		connTime  int64
		want      string
	}{
		{"bitcoinj's ping under Core's name", "/Satoshi:31.1.0/", "v1", map[string]int64{"ping": 32 * 1800}, hour,
			"not Bitcoin Core - it pings every 2 seconds, Core every 2 minutes"},
		{"the same over v2", "/Satoshi:31.1.0/", "v2", map[string]int64{"ping": 29 * 1800}, hour,
			"not Bitcoin Core - it pings every 2 seconds, Core every 2 minutes"},
		{"faster than a second is said as one", "/Satoshi:31.1.0/", "v1", map[string]int64{"ping": 32 * 7200}, hour,
			"not Bitcoin Core - it pings every 1 second, Core every 2 minutes"},
		{"Core's own two minutes", "/Satoshi:31.1.0/", "v2", map[string]int64{"ping": 29 * 31}, hour, ""},
		{"a ping every 15 seconds is still left alone", "/Satoshi:31.1.0/", "v1", map[string]int64{"ping": 32 * 240}, hour, ""},
		{"a development build pinging every 10 seconds is left alone", "/Satoshi:31.99.0/", "v1", map[string]int64{"ping": 32 * 360}, hour, ""},
		{"every 5 seconds is still left alone", "/Satoshi:31.1.0/", "v1", map[string]int64{"ping": 32 * 720}, hour, ""},
		{"every 4 seconds is not", "/Satoshi:31.1.0/", "v1", map[string]int64{"ping": 32 * 900}, hour,
			"not Bitcoin Core - it pings every 4 seconds, Core every 2 minutes"},
		{"too young for a rate", "/Satoshi:31.1.0/", "v1", map[string]int64{"ping": 32 * 30}, now.Unix() - 60, ""},
		{"a crawler asking again and again", "/Satoshi:0.19.0.1/", "v1", map[string]int64{"getaddr": 24 * 58}, now.Unix() - 60,
			"a crawler - it asked for addresses 58 times, Core asks once"},
		{"the same over v2", "/Satoshi:30.0.0/", "v2", map[string]int64{"getaddr": 33 * 3}, hour,
			"a crawler - it asked for addresses 3 times, Core asks once"},
		{"Core asks once", "/Satoshi:31.1.0/", "v2", map[string]int64{"getaddr": 33}, hour, ""},
		{"twice over v2 is still left alone", "/Satoshi:31.1.0/", "v2", map[string]int64{"getaddr": 33 * 2}, hour, ""},
		{"twice is still left alone", "/Satoshi:31.1.0/", "v1", map[string]int64{"getaddr": 48}, hour, ""},
		{"Knots runs Core's code", "/Satoshi:29.3.0/Knots:20260507/", "v1", map[string]int64{"ping": 32 * 1800}, hour,
			"not Bitcoin Core - it pings every 2 seconds, Core every 2 minutes"},
		{"bitcoinj under its own name pings as it likes", "/bitcoinj:0.17/", "v1", map[string]int64{"ping": 32 * 1800}, hour, ""},
		{"a transport not settled yet counts nothing", "/Satoshi:31.1.0/", "detecting", map[string]int64{"ping": 32 * 1800, "getaddr": 24 * 9}, hour, ""},
		{"no bytes reported", "/Satoshi:31.1.0/", "v1", nil, hour, ""},
	}
	for _, c := range cases {
		if got := behaviourAbout(c.subver, c.transport, c.recv, c.connTime, now); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestBehaviourMakesCoreDisguised(t *testing.T) {
	conn := time.Now().Unix() - 3600
	got := convert([]rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "inbound", Inbound: true, Subver: "/Satoshi:31.1.0/",
		Transport: "v1", ConnTime: conn, ServicesNames: []string{"NETWORK", "WITNESS", "NETWORK_LIMITED", "P2P_V2"},
		BytesRecvPerMsg: map[string]int64{"ping": 32 * 1800}}})[0]
	if got.Kind != fakeWalletKind || got.About != "Claims: Bitcoin Core 31.1.0\nProbably: not Bitcoin Core - it pings every 2 seconds, Core every 2 minutes" {
		t.Errorf("got %q / %q", got.Kind, got.About)
	}
	// The same peer pinging like Core stays a node.
	got = convert([]rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "inbound", Inbound: true, Subver: "/Satoshi:31.1.0/",
		Transport: "v1", ConnTime: conn, ServicesNames: []string{"NETWORK", "WITNESS", "NETWORK_LIMITED", "P2P_V2"},
		BytesRecvPerMsg: map[string]int64{"ping": 32 * 31, "getaddr": 24}}})[0]
	if got.Kind != "Node (Core)" {
		t.Errorf("an honest Core became %q: %q", got.Kind, got.About)
	}
}

func TestWasabiIsAWallet(t *testing.T) {
	conn := time.Now().Unix() - 3600
	yes, no := true, false
	wasabi := rawPeer{Addr: "185.220.101.1:40000", ConnectionType: "inbound", Inbound: true, Version: 70016,
		Subver: "/Satoshi:29.3.0/", Transport: "v1", ConnTime: conn, ServicesNames: []string{}, RelayTxes: &yes,
		BytesRecvPerMsg: map[string]int64{"ping": 32 * 30, "getheaders": 1053 * 15, "getdata": 300000}}
	got := convert([]rawPeer{wasabi})[0]
	if got.Kind != "Wallet" || got.About != "Claims: Bitcoin Core 29.3.0\nProbably: Wasabi Wallet or a fork - one of its 13 borrowed Core names, offers nothing, wants transactions" || !got.NoRelay {
		t.Errorf("Wasabi: got %q / %q / norelay %v", got.Kind, got.About, got.NoRelay)
	}
	// Without transaction relay it is no wallet.
	r := wasabi
	r.RelayTxes = &no
	if got := convert([]rawPeer{r})[0]; got.Kind != fakeWalletKind {
		t.Errorf("no tx relay: got %q", got.Kind)
	}
	// A name outside Wasabi's list stays Disguised.
	r = wasabi
	r.Subver = "/Satoshi:31.0.0/"
	if got := convert([]rawPeer{r})[0]; got.Kind != fakeWalletKind {
		t.Errorf("31.0.0: got %q", got.Kind)
	}
	// Offering services, it is a node.
	r = wasabi
	r.ServicesNames = []string{"NETWORK", "WITNESS", "NETWORK_LIMITED", "P2P_V2"}
	if got := convert([]rawPeer{r})[0]; got.Kind != "Node (Core)" {
		t.Errorf("with services: got %q", got.Kind)
	}
	// Ginger's older list, and the one name on both.
	r = wasabi
	r.Subver = "/Satoshi:0.21.0/"
	if got := convert([]rawPeer{r})[0]; got.Kind != "Wallet" || got.About != "Claims: Bitcoin Core 0.21.0\nProbably: Ginger Wallet (or an old Wasabi) - one of its 31 borrowed Core names, offers nothing, wants transactions" {
		t.Errorf("Ginger: got %q / %q", got.Kind, got.About)
	}
	r.Subver = "/Satoshi:26.0.0/"
	if got := convert([]rawPeer{r})[0]; got.About != "Claims: Bitcoin Core 26.0.0\nProbably: Wasabi or Ginger Wallet - a borrowed Core name both use, offers nothing, wants transactions" {
		t.Errorf("26.0.0: got %q", got.About)
	}
	// Asking for addresses again and again is a crawler, whatever the name.
	r = wasabi
	r.BytesRecvPerMsg = map[string]int64{"getaddr": 24 * 20}
	if got := convert([]rawPeer{r})[0]; got.Kind != fakeWalletKind {
		t.Errorf("getaddr crawler: got %q", got.Kind)
	}
}

func TestSkippedWtxidrelayIsNotCore(t *testing.T) {
	conn := time.Now().Unix() - 3600
	all := []string{"NETWORK", "WITNESS", "NETWORK_LIMITED", "P2P_V2"}
	core := rawPeer{Addr: "1.0.0.1:8333", ConnectionType: "inbound", Inbound: true, Version: 70016,
		Subver: "/Satoshi:28.0.0/", Transport: "v1", ConnTime: conn, ServicesNames: all,
		BytesRecvPerMsg: map[string]int64{"version": 126, "wtxidrelay": 24, "sendaddrv2": 24, "verack": 24, "ping": 32 * 30}}
	if got := convert([]rawPeer{core})[0]; got.Kind != "Node (Core)" {
		t.Errorf("Core sending wtxidrelay: got %q / %q", got.Kind, got.About)
	}
	r := core
	r.BytesRecvPerMsg = map[string]int64{"version": 126, "verack": 24, "ping": 32 * 30}
	if got := convert([]rawPeer{r})[0]; got.Kind != fakeWalletKind ||
		got.About != "Claims: Bitcoin Core 28.0.0\nProbably: not Bitcoin Core - it left out wtxidrelay and sendaddrv2 when connecting; Core has sent both on every connection since 0.21" {
		t.Errorf("no wtxidrelay: got %q / %q", got.Kind, got.About)
	}
	// With sendaddrv2 sent, only wtxidrelay is named.
	r.BytesRecvPerMsg = map[string]int64{"version": 126, "sendaddrv2": 24, "verack": 24}
	if got := convert([]rawPeer{r})[0]; got.About != "Claims: Bitcoin Core 28.0.0\nProbably: not Bitcoin Core - it left out wtxidrelay when connecting; Core has sent it on every connection since 0.21" {
		t.Errorf("only wtxidrelay missing: got %q", got.About)
	}
	// Knots is Core underneath and sends it too.
	r.Subver = "/Satoshi:29.2.0/Knots:20251110/"
	if got := convert([]rawPeer{r})[0]; got.Kind != fakeWalletKind {
		t.Errorf("Knots without wtxidrelay: got %q", got.Kind)
	}
	// Still in its handshake: nothing to judge yet.
	r = core
	r.BytesRecvPerMsg = map[string]int64{"version": 126}
	if got := convert([]rawPeer{r})[0]; got.Kind != "Node (Core)" {
		t.Errorf("before verack: got %q", got.Kind)
	}
	// Core before 0.21 never sent it.
	r = core
	r.Subver, r.Version, r.ServicesNames = "/Satoshi:0.20.1/", 70015, []string{"NETWORK", "WITNESS", "NETWORK_LIMITED"}
	r.BytesRecvPerMsg = map[string]int64{"version": 126, "verack": 24}
	if got := convert([]rawPeer{r})[0]; got.Kind != "Node (Core)" {
		t.Errorf("Core 0.20: got %q / %q", got.Kind, got.About)
	}
	// No byte counts at all, as from an older Core: no judgement.
	r = core
	r.BytesRecvPerMsg = nil
	if got := convert([]rawPeer{r})[0]; got.Kind != "Node (Core)" {
		t.Errorf("no counts: got %q", got.Kind)
	}
}

func TestWasabiPeerSearch(t *testing.T) {
	no, yes := false, true
	search := rawPeer{Addr: "185.220.101.1:40000", ConnectionType: "inbound", Inbound: true, Version: 70016,
		Subver: "/Satoshi:27.0.0/", Transport: "v1", ConnTime: time.Now().Unix() - 5, ServicesNames: []string{}, RelayTxes: &no,
		BytesRecvPerMsg: map[string]int64{"version": 126, "sendaddrv2": 24, "verack": 24, "getaddr": 24}}
	got := convert([]rawPeer{search})[0]
	if got.Kind != "Crawler" || got.About != "Claims: Bitcoin Core 27.0.0\nProbably: Wasabi Wallet's peer search - one of its 13 borrowed Core names, offers nothing, asks only for addresses" {
		t.Errorf("peer search: got %q / %q", got.Kind, got.About)
	}
	for name, change := range map[string]func(r *rawPeer){
		"sendcmpct instead of sendaddrv2": func(r *rawPeer) {
			r.BytesRecvPerMsg = map[string]int64{"version": 126, "sendcmpct": 33, "verack": 24, "getaddr": 24}
		},
		"Ginger's name only": func(r *rawPeer) { r.Subver = "/Satoshi:0.21.0/" },
		"protocol 70015":     func(r *rawPeer) { r.Version = 70015 },
	} {
		r := search
		change(&r)
		if got := convert([]rawPeer{r})[0]; got.Kind == "Crawler" || strings.Contains(got.About, "Wasabi") {
			t.Errorf("%s: got %q / %q", name, got.Kind, got.About)
		}
	}
	// A wallet that sends wtxidrelay is not on NBitcoin.
	r := search
	r.RelayTxes = &yes
	r.BytesRecvPerMsg = map[string]int64{"version": 126, "wtxidrelay": 24, "sendaddrv2": 24, "verack": 24}
	if got := convert([]rawPeer{r})[0]; got.Kind == "Wallet" {
		t.Errorf("wtxidrelay wallet: got %q / %q", got.Kind, got.About)
	}
}

func TestLibreRelayAndGarbageman(t *testing.T) {
	bit := []string{"NETWORK", "WITNESS", "NETWORK_LIMITED", "P2P_V2", "UNKNOWN[2^29]"}
	for _, c := range []struct{ subver, kind, about string }{
		{"/Satoshi:30.2.0/", "Node (Libre Relay)", "Libre Relay, a fork of Core that also relays transactions Core and Knots filter out"},
		{"/Satoshi:31.1.0/", "Node (Libre Relay)", "Libre Relay, a fork of Core that also relays transactions Core and Knots filter out"},
		{"/Satoshi:29.3.0/", fakeWalletKind, "Claims: Libre Relay\nProbably: Garbageman - a Knots fork that poses as Libre Relay and drops what it gets"},
		{"/Satoshi:29.1.0/", "Node (Libre Relay)", "Libre Relay - or Garbageman, a Knots fork that poses as it"},
		{"/Satoshi:28.1.0/", "Node (Libre Relay)", "Libre Relay - or Garbageman, a Knots fork that poses as it"},
		{"/Satoshi:29.1.0/Knots:20250903/", fakeWalletKind, "Claims: Libre Relay\nProbably: Garbageman - a Knots fork that poses as Libre Relay and drops what it gets"},
	} {
		got := convert([]rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "inbound", Inbound: true, Version: 70016,
			Subver: c.subver, Transport: "v2", ServicesNames: bit}})[0]
		if got.Kind != c.kind || got.About != c.about {
			t.Errorf("%s: got %q / %q", c.subver, got.Kind, got.About)
		}
	}
	// Without the bit it is plain Core.
	got := convert([]rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "inbound", Inbound: true, Version: 70016,
		Subver: "/Satoshi:30.2.0/", Transport: "v2", ServicesNames: bit[:4]}})[0]
	if got.Kind != "Node (Core)" {
		t.Errorf("no bit: got %q", got.Kind)
	}
}

func TestNamesSeenOnce(t *testing.T) {
	for subver, want := range map[string]string{
		"/dnsseedrs:0.2.0/":      "Crawler",
		"/ckh-netsurvey:1.0/":    "Crawler",
		"/bread:2.1/":            "Wallet",
		"//":                     noAgentKind,
		"/:1.1.0-dev+c1771608d/": noAgentKind,
		"/Satoshi:31.1.0/":       "Node (Core)",
	} {
		if got := kindOf(subver); got != want {
			t.Errorf("%s: got %q, want %q", subver, got, want)
		}
	}
	got := convert([]rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "inbound", Inbound: true, Version: 70016, Subver: "//", ServicesNames: []string{}}})[0]
	if !strings.HasPrefix(got.About, "Introduced itself but left its name empty") {
		t.Errorf("// about: %q", got.About)
	}
}

func TestP2aburnAbout(t *testing.T) {
	if got := aboutOf("/p2aburn:1.0/"); !strings.HasPrefix(got, "p2aburn, a bot that sweeps P2A anchors") || kindOf("/p2aburn:1.0/") != "Other" {
		t.Errorf("got %q / %q", kindOf("/p2aburn:1.0/"), got)
	}
}

func TestOtherSaysWhatItDoes(t *testing.T) {
	no, yes := false, true
	for _, c := range []struct {
		svc   []string
		relay *bool
		recv  map[string]int64
		sent  map[string]int64
		want  string
	}{
		{[]string{"NETWORK", "WITNESS"}, &yes, nil, nil, "Probably a full node under a name its operator chose"},
		{[]string{}, &yes, map[string]int64{"tx": 90 * 3}, nil, "Unknown software that pushes transactions without announcing them"},
		{[]string{}, &yes, map[string]int64{"tx": 90, "inv": 61}, nil, "Software with a name we don't recognise"},
		{[]string{}, &no, map[string]int64{"getheaders": 1053, "sendheaders": 24}, nil, "Unknown software that follows new blocks: asks for headers, wants no transactions"},
		{[]string{}, &no, map[string]int64{"getaddr": 24}, nil, "Unknown software that collects addresses"},
		{[]string{}, &yes, map[string]int64{"getaddr": 24, "getdata": 1117166, "pong": 15936}, map[string]int64{"inv": 17521264, "tx": 5292757}, "Unknown software that listens to transactions: fetches every one it hears of, announces none"},
		// A VerifiedX validator before it had a name here: asks for blocks,
		// is sent blocks, no transactions.
		{[]string{}, &yes, map[string]int64{"getdata": 1647, "getheaders": 354039}, map[string]int64{"block": 21269099, "inv": 565744}, "Software with a name we don't recognise"},
		{[]string{}, &no, nil, nil, "Software with a name we don't recognise"},
	} {
		if got := otherAbout(c.svc, c.relay, c.recv, c.sent); got != c.want {
			t.Errorf("%v %v: got %q, want %q", c.svc, c.recv, got, c.want)
		}
	}
}

func TestClientsFoundOn24October(t *testing.T) {
	for subver, want := range map[string]string{
		"/P2Pool:17.0/":    "Pool node",
		"/pydroid-pool:3/": "Pool node",
		"/Satoshi:29.4.2(mempool.guide)/Knots:20260508rc2/": "Node (Knots)",
		"/Ghost:1.11.42/":                        "Node (Ghost)",
		"/CamlCoin:0.1.0/":                       "Node (other)",
		"/rustoshi:0.1.0/":                       "Node (other)",
		"/tbcd:2.1.0-dev/":                       "Node (other)",
		"/blueberry:2026.08.17/":                 "Wallet",
		"/BitcoinKit:0.1.0/":                     "Wallet",
		"Yuki / 0.0.1":                           "Light client",
		"/VerifiedX:9.0.1/":                      "Light client",
		"/WalletKit:0.1.0/":                      "Wallet",
		"/btcwire:0.4.1/Satoshi:0.19/":           "Research scanner",
		"/p2p-observer:0.1.0/":                   "Crawler",
		"/bip324-probe:0.1.0/":                   "Crawler",
		"/crawlrs:0.2.0/":                        "Crawler",
		"/symphony:0.0.1/":                       "Indexer",
		"/btcwire:0.5.0/utreexod:0.5.0/":         "Node (other)",
		"NBXplorer-4096":                         "Indexer",
		"/bitcoin-s:1.9.12/":                     "Light client",
		"Stacks/2.1":                             "Light client",
		"/btcwire:0.4.1/lit:v0.1/":               "Lightning wallet",
		"/nakamoto:0.3.0/":                       "Light client",
		"/btcwire:0.5.0/spvwallet:0.1.0/":        "Wallet",
		"/bread:2.1/":                            "Wallet",
		"/bitcoinj:0.16.3/Elastum:0.1z/":         "Wallet",
		"/LionSeed:0.1.0/":                       "Crawler",
		"/gnoban:1.3.1/":                         "Crawler",
		"/Aurum:0.12.4/":                         "Other chain",
		"/bitcoinj:0.15.10.bisq.16/Bisq:1.10.9/": "Wallet",
	} {
		if got := kindOf(subver); got != want {
			t.Errorf("%s: got %q, want %q", subver, got, want)
		}
	}
	if got := aboutOf("/bitcoinj:0.15.10.bisq.16/Bisq:1.10.9/"); !strings.HasPrefix(got, "Bisq") {
		t.Errorf("Bisq about: %q", got)
	}
	for subver, want := range map[string]string{
		"/WalletKit:0.1.0/":              "Unstoppable Wallet on iPhone",
		"/btcwire:0.4.1/Satoshi:0.19/":   "Pool Detective",
		"/Denovo:0.7.0/":                 "Denovo",
		"/programmingbitcoin:0.1/":       "Code from Jimmy Song",
		"/programmingblockchain:0.1/":    "Code from Jimmy Song",
		"/bip324-ts:0.0.1/":              "bip324-ts",
		"/bread:2.1/":                    "BRD's wallet core",
		"/breadwallet:0.6.7/":            "BRD (breadwallet)",
		"/bitcoinj:0.16.3/Elastum:0.1z/": "Names Elastum",
	} {
		if got := aboutOf(subver); !strings.HasPrefix(got, want) {
			t.Errorf("%s about: %q", subver, got)
		}
	}
	if got := aboutOf("libbtc 0.1"); !strings.HasPrefix(got, "libbtc") {
		t.Errorf("libbtc about: %q", got)
	}
}

func TestVirtusCrawlerBehindCore23(t *testing.T) {
	no := false
	r := rawPeer{Addr: "198.51.100.7:50000", ConnectionType: "inbound", Inbound: true, Version: 70015,
		Subver: "/Satoshi:23.0.0/", ServicesNames: []string{}, RelayTxes: &no,
		BytesRecvPerMsg: map[string]int64{"version": 126, "verack": 24}}
	got := convert([]rawPeer{r})[0]
	if got.Kind != "Crawler" || !strings.Contains(got.About, "p2p-crawler") {
		t.Errorf("virtu's crawler: got %q / %q", got.Kind, got.About)
	}
	r.Version = 70016
	if got := convert([]rawPeer{r})[0]; strings.Contains(got.About, "p2p-crawler") {
		t.Errorf("70016 is not the crawler: %q", got.About)
	}
}
