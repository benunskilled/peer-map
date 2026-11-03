package main

import (
	"strings"
	"testing"
	"time"
)

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
		{"", 70016, "Introduced itself without a name"},
		{"", 0, "Has not said what it is"},
		{"/bitcoinj:0.17/", 70013, "bitcoinj, a Java library"},
		{"/dsn.kastel.kit.edu/bitcoin:28.0.0/", 70016, "KIT Karlsruhe's Bitcoin network monitoring"},
		{"/btcwire:0.5.0/:/", 70016, "A program on btcd's Go library that left its own name empty"},
		{"/GlobalNodeMap:2.1/", 70016, "Probably a node map or counter"},
		{"/reset-header-agent:1.0/", 70016, "Probably a header client for RESET"},
		{"/RESET-Headers:1.0/", 70016, "Probably a header client for RESET"},
		{"/btcwire:0.5.0/", 70016, "A program on btcd's Go library that set no name of its own"},
		{"/btcwire:0.5.0/peer:1.0.0/", 70016, "Probably a program built from the example code in btcd's Go library"},
		{"/mantle-replay-iouring:0.1.0/", 70016, "Mantle's replay tool"},
		{"/mantle-discover:0.1.0/", 70016, "Mantle's peer finder"},
		{"/mantle-rank-peers:0.1.0/", 70016, "Mantle's speed test"},
		{"/btcwire:0.4.1/Satoshi:0.19/", 70016, "Pool Detective"},
		{"/jwrd.net:0.9.99.99/", 70016, "Probably a node on JWRD Computing's own bitcoind"},
		{"/curl:8.5.0/", 70015, "Names curl, the download tool"},
	}
	for _, c := range cases {
		got := convert([]rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "inbound", Inbound: true,
			Subver: c.subver, Version: c.version, ServicesNames: []string{}}})[0]
		if !strings.HasPrefix(got.About, c.about) {
			t.Errorf("%q %d: about %q, want %q", c.subver, c.version, got.About, c.about)
		}
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
		{[]string{}, &yes, map[string]int64{"tx": 90 * 3}, nil, "Pushes transactions without announcing them"},
		{[]string{}, &yes, map[string]int64{"tx": 90, "inv": 61}, nil, "Software with a name we don't recognise"},
		{[]string{}, &no, map[string]int64{"getheaders": 1053, "sendheaders": 24}, nil, "Asks for headers and wants no transactions: a program that tracks the chain"},
		{[]string{}, &no, map[string]int64{"getaddr": 24}, nil, "Asks for addresses, not for headers or blocks: a crawler"},
		{[]string{}, &yes, map[string]int64{"getaddr": 24, "getdata": 1117166, "pong": 15936}, map[string]int64{"inv": 17521264, "tx": 5292757}, "Fetches transactions from your node and announces none: a program collecting them"},
		// A VerifiedX validator before it had a name here: asks for blocks,
		// is sent blocks, no transactions.
		{[]string{}, &yes, map[string]int64{"getdata": 1647, "getheaders": 354039}, map[string]int64{"block": 21269099, "inv": 565744}, "Software with a name we don't recognise"},
		{[]string{}, &no, nil, nil, "Software with a name we don't recognise"},
	} {
		if got := otherAbout(c.svc, c.relay, c.recv, c.sent, 0, time.Now()); got != c.want {
			t.Errorf("%v %v: got %q, want %q", c.svc, c.recv, got, c.want)
		}
	}
}

// An unknown name that stays and asks for nothing is said to only listen,
// ahead of whatever its services suggest.
func TestOtherAboutOnlyListens(t *testing.T) {
	now := time.Now()
	yes, no := true, false
	quiet := map[string]int64{"version": 126, "verack": 24, "ping": 32, "pong": 32 * 40, "sendaddrv2": 24}
	hour := now.Unix() - 3600
	if got := otherAbout(nil, &yes, quiet, map[string]int64{"inv": 120}, hour, now); got != "Only listens: hears new transactions, fetches none" {
		t.Errorf("wants transactions: %q", got)
	}
	// Nothing was announced to it, so it cannot be said to hear anything.
	if got := otherAbout(nil, &yes, quiet, nil, hour, now); got != "Only listens: asks for nothing" {
		t.Errorf("nothing announced: %q", got)
	}
	if got := otherAbout(nil, &no, quiet, nil, hour, now); got != "Only listens: wants no transactions, asks for nothing" {
		t.Errorf("wants none: %q", got)
	}
	// Listing NETWORK does not make a silent peer a full node.
	if got := otherAbout([]string{"NETWORK"}, &yes, quiet, nil, hour, now); !strings.Contains(got, "Only listens") {
		t.Errorf("NETWORK but silent: %q", got)
	}
	// Not long enough yet, or it did ask: the older lines stand.
	if got := otherAbout(nil, &yes, quiet, nil, now.Unix()-60, now); strings.Contains(got, "only listens") {
		t.Errorf("one minute: %q", got)
	}
	asks := map[string]int64{"version": 126, "verack": 24, "getaddr": 24}
	if got := otherAbout(nil, &yes, asks, nil, hour, now); got != "Asks for addresses, not for headers or blocks: a node already in sync, or a crawler" {
		t.Errorf("collects addresses: %q", got)
	}
}

// What unknown software did, said plainly: asking for the mempool, downloading
// blocks, wanting headers announced.
func TestOtherAboutWhatItDid(t *testing.T) {
	now := time.Now()
	yes, no := true, false
	cases := []struct {
		name  string
		relay *bool
		recv  map[string]int64
		sent  map[string]int64
		want  string
	}{
		{"mempool", &yes, map[string]int64{"version": 126, "verack": 24, "mempool": 24}, nil,
			"Asks for our mempool"},
		{"asks for headers", &no, map[string]int64{"version": 126, "getheaders": 1061}, nil,
			"Asks for headers and wants no transactions: a block-only connection, or a program that tracks the chain"},
		{"only wants them announced", &no, map[string]int64{"version": 126, "sendheaders": 24}, nil,
			"Asks for headers and wants no transactions: a block-only connection, or a program that tracks the chain"},
	}
	for _, c := range cases {
		if got := otherAbout(nil, c.relay, c.recv, c.sent, 0, now); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// A peer with a version but no name says what it did next.
func TestNoAgentAbout(t *testing.T) {
	now := time.Now()
	hour := now.Unix() - 3600
	cases := []struct {
		name string
		recv map[string]int64
		conn int64
		want string
	}{
		{"a full node without a name", map[string]int64{"version": 126, "verack": 24, "getheaders": 1061, "getaddr": 24, "sendcmpct": 33}, hour,
			"Asks for headers and addresses, like a full node does"},
		{"addresses only", map[string]int64{"version": 126, "verack": 24, "getaddr": 24, "pong": 32}, hour,
			"Asks for addresses, not for headers or blocks"},
		{"quiet for an hour", map[string]int64{"version": 126, "verack": 24, "pong": 32 * 30}, hour,
			"Introduced itself, then asked for nothing"},
		{"just arrived", map[string]int64{"version": 126, "verack": 24}, now.Unix() - 5,
			"Introduced itself without a name; nothing it did tells what it is"},
		{"no counters", nil, hour,
			"Introduced itself without a name; nothing it did tells what it is"},
	}
	for _, c := range cases {
		if got := noAgentAbout(c.recv, c.conn, now); got != c.want {
			t.Errorf("%s: got %q", c.name, got)
		}
	}
}

// A fixed sentence says what a name stands for; what the peer did in the
// session is still said under it, and a session that showed nothing adds
// nothing.
func TestFixedSentenceKeepsTheSession(t *testing.T) {
	yes := true
	for _, ua := range []string{"/curl:8.5.0/", "urbit", "/dexcash-nodes:0.1.0/", "/bip157-fetch:0.1/"} {
		got := convertAt([]rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "inbound", Inbound: true, Subver: ua, Version: 70016,
			ServicesNames: []string{}, RelayTxes: &yes, BytesRecvPerMsg: map[string]int64{"mempool": 24}}}, time.Now())[0]
		if !strings.Contains(got.About, "Asks for our mempool") {
			t.Errorf("%s: the session is lost: %q", ua, got.About)
		}
		quiet := convertAt([]rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "inbound", Inbound: true, Subver: ua, Version: 70016,
			ServicesNames: []string{}, RelayTxes: &yes}}, time.Now())[0]
		if strings.Contains(quiet.About, "In this session") || strings.Contains(quiet.About, unknownAbout) && aboutOf(ua) != "" {
			t.Errorf("%s: a quiet session added a line: %q", ua, quiet.About)
		}
	}
}

func TestObservedPart(t *testing.T) {
	for in, want := range map[string]string{
		"Only listens: hears new transactions, fetches none": "Hears new transactions, fetches none",
		"Only listens: asks for nothing":                     "Asks for nothing",
		"Asks for headers and wants no transactions: a block-only connection, or a program that tracks the chain": "Asks for headers and wants no transactions",
		"Asks for our mempool":                        "Asks for our mempool",
		"Pushes transactions without announcing them": "Pushes transactions without announcing them",
	} {
		if got := observedPart(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
