package main

import "regexp"

// What kind of software a peer runs, read off the user agent it sent in its
// version message.
//
// This is what the peer SAYS it is, and a user agent is a free-text field the
// other side chooses. On the node this was written for, four connections
// announced themselves as "/Sat0shi:31.0.0/" - Bitcoin Core with a zero in
// place of the o - from rented machines in five regions. So the kind is a
// label, not evidence.
//
// The counterweight is in the three observed flags on Peer, which nobody can
// write into a string: what the peer offers, whether it wants transactions,
// and whether Core has ever learned which chain it is on. A peer calling
// itself Core while offering nothing and having no known chain is the
// interesting case, and it is visible only because the two are shown side by
// side.
//
// The name answers the question somebody actually has, which is not "what
// software is this" but "is this a node that could hand me a block". So the
// class comes first and the software after it: "Node (Core)", not "Core".
//
// Order matters: the first match wins, so a more specific pattern goes above
// a more general one (Knots before Core - a Knots user agent contains
// "Satoshi" too).
// relays says whether this kind of software passes blocks on at all. It is the
// same distinction Bitcoin Lab paints red in its peer list, and it has to stay
// the same in both apps: a colour that means two different things in two
// windows of the same node is worse than no colour.
//
// A wallet has nothing to relay, a crawler is there to look, an indexer serves
// somebody else's wallet. Measured across 2,270 peers on one node: 688 of them
// ran software like this, 790 observations between them, and not one block
// delivered first. Unknown software counts as relaying - not knowing is not the
// same as knowing it cannot.
var kindRules = []struct {
	name   string
	re     *regexp.Regexp
	relays bool
}{
	// Mining pool software speaking the p2p protocol.
	{"Pool node", regexp.MustCompile(`(?i)ckp2p|ckpool`), true},
	// Clients of other chains that still dial Bitcoin's port.
	{"Other chain", regexp.MustCompile(`(?i)Bitcoin ABC|BUCash|Bitcoin SV|BCHUnlimited|Bitcoin XT`), false},
	// Network scanners, in two flavours: the ones run as a service and the
	// ones run by universities. Both announce themselves honestly.
	{"Research scanner", regexp.MustCompile(`(?i)kit\.edu|dsn\.tm|dsn\.kastel|\.ac\.|uni-`), false},
	{"Crawler", regexp.MustCompile(`(?i)bitnodes|metrika|nodemap|crawler|scanner`), false},
	// Address indexers for wallets: they follow the chain but relay nothing.
	{"Indexer", regexp.MustCompile(`(?i)electrs|electrum|esplora|mempool`), false},
	// SPV and mobile wallets. bitcoinj is the Android wallet's library.
	{"Wallet", regexp.MustCompile(`(?i)bitcoinj|breadwallet|bither|multibit|wasabi|Bitcoin Wallet`), false},
	// BIP157 light clients. btcwire alone is only the Go p2p library, which
	// says nothing about the program using it, so it stays out here and is
	// caught by "Other" below.
	{"Light client", regexp.MustCompile(`(?i)neutrino`), false},
	{"Node (Knots)", regexp.MustCompile(`(?i)Knots`), true},
	{"Node (Core)", regexp.MustCompile(`^/Satoshi:`), true},
}

// kindOf returns the family name, "unknown" for a peer that sent no user
// agent at all, and "Other" for one whose agent matches nothing here - which
// is a normal answer, not a failure: the network runs plenty of software this
// list has never heard of.
func kindOf(subver string) string {
	name, _ := classify(subver)
	return name
}

// relaysBlocks is false only for software known not to pass blocks on. Anything
// unrecognised, and anything that sent no user agent at all, counts as relaying.
func relaysBlocks(subver string) bool {
	_, relays := classify(subver)
	return relays
}

func classify(subver string) (string, bool) {
	if subver == "" {
		return "unknown", true
	}
	for _, r := range kindRules {
		if r.re.MatchString(subver) {
			return r.name, r.relays
		}
	}
	return "Other", true
}
