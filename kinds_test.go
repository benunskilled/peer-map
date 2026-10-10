package main

import (
	"os"
	"strings"
	"testing"
)

func TestKindOf(t *testing.T) {
	for subver, want := range map[string]string{
		"/Satoshi:31.1.0/":                         "Node (Core)",
		"/Satoshi:29.3.0/Knots:20260507/":          "Node (Knots)",
		"/bitcoinj:0.16.2/Bitcoin Wallet:9.26/":    "Wallet",
		"/breadwallet:1.3.5/":                      "Wallet",
		"/btcwire:0.5.0/neutrino:0.17.1/":          "Light client",
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
		"/Sat0shi:31.0.0/":                  "Disguised",
		"/Satoshi2:0.18.3/":                 "Disguised",
		"/SatoshiX:0.18.0/":                 "Disguised",
		"Satoshi:22.0.0":                    "Disguised",
		"/Satoshi:22.0.0/":                  "Node (Core)",
		"/Floresta:0.9.1/mandacaru:0.15.2/": "Node (Floresta)",
		"/electrs:0.11.1/":                  "Indexer",
		"/Metrika-Bitnodes:0.1/":            "Crawler",
		"/bitnodes.io:0.3/":                 "Crawler",
		"/GlobalNodeMap:2.1/":               "Crawler",
		"/dsn.tm.kit.edu/bitcoin:0.9.99/":   "Research scanner",
		"/ckp2p:2.0/":                       "Other",
		// The operator's own word, in the comment part - above the software.
		"/Satoshi:29.1.0(PyBLOCK-POOL)/Knots:20250903/": "Pool node",
		"/Satoshi:31.0.0(pool)/":                        "Pool node",
		// As it arrives on a real node, with a URL after the Knots part.
		"/Satoshi:29.1.0(PyBLOCK-POOL)/Knots:20250903/https://pyblock.xyz:8443/": "Pool node",
		// PyBLOCK's node without the pool: a Knots node, not a pool.
		"/Satoshi:29.4.2(PyBLOCK-BIP110)/Knots:20260508/": "Node (Knots)",
		// "pool" outside the brackets is not a claim: this is an indexer.
		"/mempool:3.0.0/electrs:0.10.0/": "Indexer",
		// "mempool" in the brackets is a node's label, not a pool (seen on Ben's node)
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
		{"/Sat0shi:31.0.0/", []string{}, fakeWalletKind, "Claims: Bitcoin Core 31.0.0\nProbably: a crawler"},
		{"/Satoshi2:0.18.3/", []string{"NETWORK", "WITNESS"}, fakeWalletKind, "Probably: a scanner or node that hides"},
		// Only a wallet name can be fake: a node offering blocks is a node.
		{"/Satoshi:31.1.0/", []string{"NETWORK", "P2P_V2"}, "Node (Core)", ""},
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
	if got.Kind != "Indexer" || got.About != "Probably your own electrs 0.11.1 on this Umbrel" {
		t.Errorf("got %q / %q", got.Kind, got.About)
	}
	got = convert([]rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "inbound", Inbound: true, Subver: ""}})[0]
	if got.Kind != noAgentKind || !strings.Contains(got.About, "checks the port") {
		t.Errorf("no agent: got %q / %q", got.Kind, got.About)
	}
}

func TestLinkingLion(t *testing.T) {
	for _, a := range []string{"143.20.137.5:51234", "[2602:f5c0:0:ace::72:300]:8333", "[2602:f5c0:0:ace::60]:8333",
		"104.234.118.2:41000", "69.17.52.1:8333", "91.198.115.9:1", "[::ffff:31.58.215.200]:8333"} {
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
// offers WITNESS and NETWORK or NETWORK_LIMITED (Ben's node: 6,954 sessions,
// 73% on Amazon, none ever first). nil = Core did not say, which is not this.
func TestCoreOfferingNothing(t *testing.T) {
	no := []string{}
	got := convert([]rawPeer{{Addr: "198.51.100.7:40000", Inbound: true, ConnectionType: "inbound", Subver: "/Satoshi:31.0.0/", ServicesNames: no}})
	if len(got) != 1 || got[0].Kind != fakeWalletKind || !strings.Contains(got[0].About, offersNothingAbout) {
		t.Fatalf("Core offering nothing: %+v", got)
	}
	for _, svc := range [][]string{nil, {"NETWORK", "WITNESS"}, {"NETWORK_LIMITED", "WITNESS"}} {
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
		{"10.21.22.10:40000", true, "not_publicly_routable"}, // Tor proxy
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
		if got[0].Country != "" || got[0].Operator != "" {
			t.Errorf("%s: looked up %q %q", c.addr, got[0].Country, got[0].Operator)
		}
	}
}
