package main

import (
	"regexp"
	"strings"
)

// tellOf is a disguised peer's first reason in a few words, for the line
// under its pill (28.10.2026: the pill said Disguised, and only the opened
// row said why). It reads the reasons disguise.go writes, so each of them has
// a short form here; TestEveryDisguiseHasATell keeps the two together.
// LinkingLion's own reason is the pill itself, so its first other evidence
// is told, or that the address is a listed one.
func tellOf(about string, lion bool) string {
	lines := strings.Split(about, "\n")
	var reasons []string
	for _, l := range lines {
		if r, ok := strings.CutPrefix(l, "Probably: "); ok && !(lion && strings.HasPrefix(r, "LinkingLion")) {
			reasons = append(reasons, r)
		} else if r, ok := strings.CutPrefix(l, "Also: "); ok {
			reasons = append(reasons, r)
		}
	}
	claimed := ""
	if m := tellClaim.FindStringSubmatch(about); m != nil {
		claimed = m[1]
	}
	for _, r := range reasons {
		if t := shortReason(r, claimed); t != "" {
			return t
		}
	}
	if lion {
		return "listed address range"
	}
	return ""
}

var (
	tellAddrs    = regexp.MustCompile(`asked for addresses (\d+) times`)
	tellPings    = regexp.MustCompile(`pings every (\d+) second`)
	tellProtocol = regexp.MustCompile(`speaks protocol (\d+)`)
	tellLacked   = regexp.MustCompile(`it offers (.+), which Core ([0-9.]+) did not have`)
	tellClaim    = regexp.MustCompile(`^Claims: Bitcoin (?:Core |Knots \(Core )([0-9.]+)`)
)

// shortReason is one reason in a few words: what was measured, beside what
// the peer claims where the two only mean something together.
func shortReason(r, claimed string) string {
	if m := tellAddrs.FindStringSubmatch(r); m != nil {
		return "asks for addresses " + m[1] + "\u00d7"
	}
	if m := tellPings.FindStringSubmatch(r); m != nil {
		return "pings every " + m[1] + " s"
	}
	if m := tellLacked.FindStringSubmatch(r); m != nil && serviceShort[m[1]] != "" {
		return serviceShort[m[1]] + " for Core " + m[2]
	}
	if m := tellProtocol.FindStringSubmatch(r); m != nil {
		if claimed != "" {
			return "protocol " + m[1] + " for Core " + claimed
		}
		return "protocol " + m[1]
	}
	for _, t := range []struct{ in, tell string }{
		{"left out wtxidrelay", "no wtxidrelay"},
		{"offers no services", "offers no services"},
		{"lacks NETWORK_LIMITED", "no NETWORK_LIMITED"},
		{"asked for nothing", "asks for nothing"},
		{"newer protocol than this wallet", "newer protocol than the wallet"},
		{"offers blocks, which no wallet does", "offers blocks"},
		{"speaks v2, which no wallet", "offers v2"},
		{"v2 connections, which that software never had", "offers v2"},
		{"which the Bitcore library never does", "more services than Bitcore"},
		{"Sat0shi with a zero", "misspelt name"},
		{"Core calls itself /Satoshi:", "misspelt name"},
		{"Core writes its name between slashes", "misspelt name"},
		{"poses as Libre Relay", "Libre Relay bit, no matching release"},
		{"reached us at its own address", "gives its own address as ours"},
	} {
		if strings.Contains(r, t.in) {
			return t.tell
		}
	}
	return ""
}

// behaviourTell is, for a peer whose name says nothing (Other, No user
// agent), what otherAbout and noAgentAbout saw it do, in a few words for the
// line under its pill. Only what a real node does not do, or what says it
// acts like one: where real nodes do the same (headers and no transactions,
// addresses only, transactions fetched and none announced) it is "", and the
// line keeps what Core saw. TestEveryBehaviourHasATell reads those two
// functions.
func behaviourTell(about string) string {
	for _, t := range []struct{ in, tell string }{
		{"hears new transactions, fetches none", "fetches nothing"},
		{"asks for nothing", "asks for nothing"},
		{"asked for nothing", "asks for nothing"},
		{"a full node under a name its operator chose", "offers blocks"},
		{"Asks for our mempool", "asks for our mempool"},
		{"Pushes transactions without announcing them", "sends tx unannounced"},
		{"Asks for headers and addresses, like a full node", "headers + addresses"},
		{"Asks for headers and compact blocks, like a full node", "headers + compact blocks"},
	} {
		if strings.Contains(about, t.in) {
			return t.tell
		}
	}
	return ""
}
