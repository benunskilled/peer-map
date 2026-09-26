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
		"/Floresta:0.9.1/mandacaru:0.15.2/":        "Node (Floresta)",
		"/electrs:0.11.1/":                         "Indexer",
		"/Metrika-Bitnodes:0.1/":                   "Crawler",
		"/bitnodes.io:0.3/":                        "Crawler",
		"/GlobalNodeMap:2.1/":                      "Crawler",
		"/dsn.tm.kit.edu/bitcoin:0.9.99/":          "Research scanner",
		"/ckp2p:2.0/":                              "Pool node",
		// The operator's own word, in the comment part - above the software.
		"/Satoshi:29.1.0(PyBLOCK-POOL)/Knots:20250903/": "Pool node",
		"/Satoshi:31.0.0(pool)/":                        "Pool node",
		// As it arrives on a real node, with a URL after the Knots part.
		"/Satoshi:29.1.0(PyBLOCK-POOL)/Knots:20250903/https://pyblock.xyz:8443/": "Pool node",
		// PyBLOCK's node without the pool: a Knots node, not a pool.
		"/Satoshi:29.4.2(PyBLOCK-BIP110)/Knots:20260508/": "Node (Knots)",
		// "pool" outside the brackets is not a claim: this is an indexer.
		"/mempool:3.0.0/electrs:0.10.0/": "Indexer",
		"/Bitcoin ABC:0.14.5(EB8.0)/":    "Other chain",
		"/Floresta:0.9.1/":               "Node (Floresta)",
		"/btcwire:0.5.0/hemi-soak:1.0/":  "Other",
		"":                               "unknown",
		// The whole reason the observed flags exist. A zero in place of the o,
		// seen on four connections from rented machines in five regions. It
		// must not come out as Core, and it must not be quietly swallowed
		// either - "Other" is the honest answer to a name nobody recognises.
		"/Sat0shi:31.0.0/": "Other",
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
	names := []string{"Other", "unknown", fakeWalletKind}
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
		{"/bitcoinj:0.14.5/Bitcoin Wallet:5.42/", []string{"NETWORK", "WITNESS", "NETWORK_LIMITED", "P2P_V2"}, fakeWalletKind, ""},
		{"/breadwallet:0.6.5/", []string{"NETWORK"}, fakeWalletKind, ""},
		{"/bitcoinj:0.16.2/Bitcoin Wallet:9.26/", []string{}, "Wallet", "Schildbach"},
		{"/bitcoinj:0.16.2/Bitcoin Wallet:9.26/", nil, "Wallet", "Schildbach"},
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
