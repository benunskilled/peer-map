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
// Order matters: the first match wins, so a more specific pattern goes above
// a more general one (Knots before Core - a Knots user agent contains
// "Satoshi" too).
var kindRules = []struct {
	name string
	re   *regexp.Regexp
}{
	// Mining pool software speaking the p2p protocol.
	{"Pool", regexp.MustCompile(`(?i)ckp2p|ckpool`)},
	// Clients of other chains that still dial Bitcoin's port.
	{"Other chain", regexp.MustCompile(`(?i)Bitcoin ABC|BUCash|Bitcoin SV|BCHUnlimited|Bitcoin XT`)},
	// Network scanners, in two flavours: the ones run as a service and the
	// ones run by universities. Both announce themselves honestly.
	{"Research", regexp.MustCompile(`(?i)kit\.edu|dsn\.tm|dsn\.kastel|\.ac\.|uni-`)},
	{"Crawler", regexp.MustCompile(`(?i)bitnodes|metrika|nodemap|crawler|scanner`)},
	// Address indexers for wallets: they follow the chain but relay nothing.
	{"Indexer", regexp.MustCompile(`(?i)electrs|electrum|esplora|mempool`)},
	// SPV and mobile wallets. bitcoinj is the Android wallet's library.
	{"Wallet", regexp.MustCompile(`(?i)bitcoinj|breadwallet|bither|multibit|wasabi|Bitcoin Wallet`)},
	// BIP157 light clients. btcwire alone is only the Go p2p library, which
	// says nothing about the program using it, so it stays out here and is
	// caught by "Other" below.
	{"Light client", regexp.MustCompile(`(?i)neutrino`)},
	{"Knots", regexp.MustCompile(`(?i)Knots`)},
	{"Core", regexp.MustCompile(`^/Satoshi:`)},
}

// kindOf returns the family name, "unknown" for a peer that sent no user
// agent at all, and "Other" for one whose agent matches nothing here - which
// is a normal answer, not a failure: the network runs plenty of software this
// list has never heard of.
func kindOf(subver string) string {
	if subver == "" {
		return "unknown"
	}
	for _, r := range kindRules {
		if r.re.MatchString(subver) {
			return r.name
		}
	}
	return "Other"
}
