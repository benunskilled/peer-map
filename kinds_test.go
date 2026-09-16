package main

import "testing"

func TestKindOf(t *testing.T) {
	for subver, want := range map[string]string{
		"/Satoshi:31.1.0/":                      "Core",
		"/Satoshi:29.3.0/Knots:20260507/":       "Knots",
		"/bitcoinj:0.16.2/Bitcoin Wallet:9.26/": "Wallet",
		"/breadwallet:1.3.5/":                   "Wallet",
		"/btcwire:0.5.0/neutrino:0.17.1/":       "Light client",
		"/electrs:0.11.1/":                      "Indexer",
		"/Metrika-Bitnodes:0.1/":                "Crawler",
		"/bitnodes.io:0.3/":                     "Crawler",
		"/GlobalNodeMap:2.1/":                   "Crawler",
		"/dsn.tm.kit.edu/bitcoin:0.9.99/":       "Research",
		"/ckp2p:2.0/":                           "Pool",
		"/Bitcoin ABC:0.14.5(EB8.0)/":           "Other chain",
		"/Floresta:0.9.1/":                      "Other",
		"/btcwire:0.5.0/hemi-soak:1.0/":         "Other",
		"":                                      "unknown",
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
	if got := kindOf("/Satoshi:29.1.0(PyBLOCK-POOL)/Knots:20250903/"); got != "Knots" {
		t.Errorf("got %q, want Knots", got)
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
