package main

import (
	"regexp"
	"strings"
)

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
	// Bitcoin Core's name, misspelt: "/Sat0shi:" with a zero, "/Satoshi2:",
	// "/SatoshiX:", or "Satoshi:22.0.0" without the slashes Core always sends.
	// Real Core writes exactly "/Satoshi:x.y.z/". On the node this was written
	// for, "/Sat0shi:31.0.0/" alone came 475 times in a fortnight. First, so
	// nothing below reads it as Core. What it does is unknown, so it is not
	// marked as unable to relay.
	{"Fake", regexp.MustCompile(`^/Sat0shi:|^/Satoshi[0-9X]+:|^Satoshi:`), true},
	// Mining pool software speaking the p2p protocol, or a node whose operator
	// says in the comment part of the agent - the bit in brackets - that it
	// belongs to a pool, e.g. "/Satoshi:29.1.0(PyBLOCK-POOL)/Knots:20250903/".
	// Only the brackets count: outside them "pool" is also in "mempool".
	{"Pool node", regexp.MustCompile(`(?i)ckp2p|ckpool|\([^)]*pool[^)]*\)`), true},
	// Clients of other chains that still dial Bitcoin's port.
	{"Other chain", regexp.MustCompile(`(?i)Bitcoin ABC|BUCash|Bitcoin SV|BCHUnlimited|Bitcoin XT`), false},
	// Network scanners, in two flavours: the ones run as a service and the
	// ones run by universities. Both announce themselves honestly.
	{"Research scanner", regexp.MustCompile(`(?i)kit\.edu|dsn\.tm|dsn\.kastel|\.ac\.|uni-`), false},
	// Crawlers, seeders and node counters: they collect addresses and look,
	// and several of them say so only as "scan", "seeder", "census" or
	// "monitor" - on one node 953 sessions of /bitcoin-seeder/ and 372 of
	// /btc-range-scan/ used to land in "Other".
	{"Crawler", regexp.MustCompile(`(?i)bitnodes|metrika|nodemap|crawl|scan|seeder|census|monitor|observatory|sonar|scout|nebula`), false},
	// Address indexers for wallets: they follow the chain but relay nothing.
	{"Indexer", regexp.MustCompile(`(?i)electrs|electrum|esplora|mempool`), false},
	// SPV and mobile wallets. bitcoinj is the Android wallet's library.
	{"Wallet", regexp.MustCompile(`(?i)bitcoinj|breadwallet|bither|multibit|wasabi|Bitcoin Wallet`), false},
	// BIP157 light clients: neutrino, which Lightning wallets on lnd use, and
	// Kyoto, which announces itself as "Rust BIP-157". btcwire alone is only
	// the Go p2p library, which says nothing about the program using it, so it
	// stays out here and is caught by "Other" below.
	{"Light client", regexp.MustCompile(`(?i)neutrino|BIP-157|kyoto`), false},
	// A small Utreexo node. Named so it is not lost in "Other", and counted as
	// relaying like any node - Bitcoin Lab has no rule for it either.
	{"Node (Floresta)", regexp.MustCompile(`(?i)floresta`), true},
	{"Node (Knots)", regexp.MustCompile(`(?i)Knots`), true},
	{"Node (Core)", regexp.MustCompile(`^/Satoshi:`), true},
}

// fakeWalletKind is for a peer whose user agent names a wallet while it
// offers what no wallet can: blocks (NODE_NETWORK) or the encrypted v2
// transport, which no wallet had when these names were current. On the node
// this was written for, some 33,000 inbound sessions in a fortnight came as
// Bitcoin Wallet 4.x/5.x from 2016 (BlackBerry builds among them), MultiBit
// and breadwallet 0.6 - each version about as often as the next, gone after
// some 20 seconds, from over 800 addresses, and nearly all offering
// NETWORK and P2P_V2. The real Bitcoin Wallet 9 to 11 offered nothing. So
// this is a claim contradicted by an observation, and the kind says so
// rather than counting them as wallets. They still pass no blocks on to
// this node, so they keep the wallet's no-relay mark.
const fakeWalletKind = "Fake"

func offersWhatNoWalletCan(services []string) bool {
	for _, s := range services {
		if s == "NETWORK" || s == "P2P_V2" {
			return true
		}
	}
	return false
}

// What a piece of software is, in a sentence, for the peers somebody is
// likely to ask about. Only software that says who it is: the kind already
// covers the rest.
var aboutRules = []struct {
	re   *regexp.Regexp
	text string
}{
	{regexp.MustCompile(`(?i)seeder`), "A DNS seeder: collects addresses of reachable nodes for new nodes to start from"},
	{regexp.MustCompile(`(?i)census|observatory|monitor|sonar`), "A node census: counts and watches reachable nodes"},
	{regexp.MustCompile(`Bitcoin Wallet:`), "Bitcoin Wallet by Schildbach, an Android app"},
	{regexp.MustCompile(`(?i)breadwallet|/bread:`), "BRD (breadwallet), a phone wallet shut down in 2022"},
	{regexp.MustCompile(`(?i)multibit`), "MultiBit, a desktop wallet discontinued in 2017"},
	{regexp.MustCompile(`(?i)bither`), "Bither, a wallet for phone and desktop"},
	{regexp.MustCompile(`(?i)wasabi`), "Wasabi Wallet, a desktop wallet"},
	{regexp.MustCompile(`(?i)neutrino`), "Neutrino: Lightning wallets on lnd, such as Blixt or Zeus"},
	{regexp.MustCompile(`(?i)BIP-157|kyoto`), "Kyoto, a light client for wallets built with BDK"},
	{regexp.MustCompile(`(?i)floresta`), "Floresta, a lightweight Utreexo node"},
	{regexp.MustCompile(`(?i)electrs`), "electrs, the address index behind Electrum-style wallets"},
	{regexp.MustCompile(`(?i)ckp2p`), "ckpool's p2p relay, run next to solo mining pools"},
	{regexp.MustCompile(`(?i)pyblock`), "PyBLOCK, a node dashboard"},
	{regexp.MustCompile(`(?i)bitnodes`), "Bitnodes, a public map of reachable nodes"},
}

// fakeAbout says, for a Fake, what it claims and what it probably is. The
// claim comes from its user agent, the guess from what Core observed of it -
// the one part nobody can write into a string.
func fakeAbout(subver, walletAbout string, services []string) string {
	claim := "Bitcoin Core"
	if v := coreVersion.FindStringSubmatch(subver); v != nil {
		claim += " " + v[1]
	}
	if walletAbout != "" {
		claim, _, _ = strings.Cut(walletAbout, ",")
		claim = "the wallet " + claim
	}
	var guess string
	blocks, v2 := false, false
	for _, s := range services {
		blocks = blocks || s == "NETWORK"
		v2 = v2 || s == "P2P_V2"
	}
	switch {
	case walletAbout != "" && blocks:
		guess = "a node or scanner behind an old wallet name - it offers blocks, which no wallet does"
	case walletAbout != "" && v2:
		guess = "a scanner behind an old wallet name - it speaks v2, which no wallet of that age does"
	case services != nil && len(services) == 0:
		guess = "a crawler - it offers nothing"
	case blocks:
		guess = "a scanner or node that hides what it runs - it offers blocks"
	default:
		guess = "unknown software"
	}
	return "Claims: " + claim + "\nProbably: " + guess
}

var coreVersion = regexp.MustCompile(`[Ss]at[o0]shi[0-9X]*:([0-9.]+)`)

func aboutOf(subver string) string {
	for _, r := range aboutRules {
		if r.re.MatchString(subver) {
			return r.text
		}
	}
	return ""
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
