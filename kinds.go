package main

import (
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
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
	{"Disguised", regexp.MustCompile(`^/Sat0shi:|^/Satoshi[0-9X]+:|^Satoshi:`), true},
	// Mining pool software speaking the p2p protocol, or a node whose operator
	// says in the comment part of the agent - the bit in brackets - that it
	// belongs to a pool, e.g. "/Satoshi:29.1.0(PyBLOCK-POOL)/Knots:20250903/".
	// Only the brackets count: outside them "pool" is also in "mempool".
	// "mempool" inside the brackets does not count: "(mempool.guide)" is a
	// node's own label, not a pool (seen on the node this was written for).
	// /ckp2p:/ used to be here as "ckpool's relay", but nothing named ckp2p
	// exists in ckpool's source and nothing public explains it, so it is no
	// longer called a pool; see aboutRules for what was actually observed.
	{"Pool node", regexp.MustCompile(`(?i)ckpool|\([^)]*pool[^)]*\)`), true},
	// Clients of other chains that still dial Bitcoin's port.
	// Bitcoin Classic's "(EB8)" is Bitcoin Cash's block size; SuperBitcoin,
	// Irium and VDS are forks and altcoins of their own.
	{"Other chain", regexp.MustCompile(`(?i)Bitcoin ABC|BUCash|Bitcoin SV|BCHUnlimited|Bitcoin XT|Classic:|\(EB[0-9]|SuperBitcoin|iriumd|vds_`), false},
	// Network scanners, in two flavours: the ones run as a service and the
	// ones run by universities. Both announce themselves honestly.
	{"Research scanner", regexp.MustCompile(`(?i)kit\.edu|dsn\.tm|dsn\.kastel|\.ac\.|uni-`), false},
	// Crawlers, seeders and node counters: they collect addresses and look,
	// and several of them say so only as "scan", "seeder", "census" or
	// "monitor" - on one node 953 sessions of /bitcoin-seeder/ and 372 of
	// /btc-range-scan/ used to land in "Other".
	{"Crawler", regexp.MustCompile(`(?i)bitnodes|metrika|nodemap|crawl|scan|seeder|census|monitor|observatory|sonar|scout|nebula|watch|listener|argus|readonly|bitdash|logosnaut`), false},
	// Address indexers for wallets: they follow the chain but relay nothing.
	// "/mempool" at the start of a part of the agent: "/mempool/" and
	// "/mempool.space:1.0/" are the indexer, "(mempool.guide)" is a node label.
	{"Indexer", regexp.MustCompile(`(?i)electrs|electrum|esplora|/mempool`), false},
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
	// Other full-node software, named rather than left in "Other".
	{"Node (btcd)", regexp.MustCompile(`(?i)btcd:`), true},
	{"Node (libbitcoin)", regexp.MustCompile(`(?i)libbitcoin`), true},
	{"Node (bcoin)", regexp.MustCompile(`(?i)/bcoin:`), true},
	{"Node (Gocoin)", regexp.MustCompile(`(?i)gocoin`), true},
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
// "Disguised" rather than "Fake" (Ben, 08.10.2026): it says what happens - the
// peer passes itself off as something else - without claiming why.
const fakeWalletKind = "Disguised"

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
	{regexp.MustCompile(`(?i)ckp2p`), "Fetches blocks only, never transactions, from data centres; the operator is unknown and it is probably not part of ckpool"},
	{regexp.MustCompile(`(?i)NBitcoin`), "NBitcoin, a .NET Bitcoin library (behind BTCPay Server, among others)"},
	{regexp.MustCompile(`(?i)/bitcore:`), "Bitcore, BitPay's node and index server"},
	{regexp.MustCompile(`(?i)pyblock`), "PyBLOCK, a node dashboard"},
	{regexp.MustCompile(`(?i)bitnodes`), "Bitnodes, a public map of reachable nodes"},
	{regexp.MustCompile(`(?i)btcd:`), "btcd, a full node written in Go"},
	{regexp.MustCompile(`(?i)libbitcoin`), "libbitcoin, a full node written in C++, independent of Core"},
	{regexp.MustCompile(`(?i)/bcoin:`), "bcoin, a full node written in JavaScript"},
	{regexp.MustCompile(`(?i)gocoin`), "Gocoin, a full node and wallet written in Go"},
	{regexp.MustCompile(`(?i)Classic:|\(EB[0-9]`), "Bitcoin Classic: a Bitcoin Cash client"},
}

// fakeAbout says, for a disguised peer, what it claims and what it probably is. The
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

// LinkingLion opens connections to many nodes from a few address ranges and
// listens to transaction announcements, which may let it link transactions to
// the IP addresses of nodes. Named and tracked by 0xB10C, who runs
// peer-observer (github.com/peer-observer/peer-observer); the ranges are the
// ones its shared/src/util.rs checks, announced by AS54098. On the node this
// was written for, over 24 days, they held 2.1% of all connection time - about
// 4 of some 180 peers at any moment - but came back so often, each time from a
// new address and gone after 1.6 minutes on average, that 62% of all addresses
// Bitcoin Lab ever saw were theirs. Nearly all used the old wallet names
// fakeWalletKind already catches.
// It stays Disguised: the kind says what it does, the about says who.
// Current ranges from the bnoc banlist (github.com/bitcoin-noc/banlist,
// entities.toml, after PR #4 of 16.09.2026): the IPv6 range is one /64, not the
// provider's whole /32, and two single hosts come on top. One of them,
// 104.234.118.2, made 5,822 sessions on the node this was written for, with 82
// different fake user agents from that one address.
var linkingLion = []netip.Prefix{
	// used from 2025-12-05
	netip.MustParsePrefix("143.20.137.0/24"), netip.MustParsePrefix("31.58.215.0/24"),
	netip.MustParsePrefix("87.229.79.0/24"), netip.MustParsePrefix("2602:f5c0:0:ace::/64"),
	netip.MustParsePrefix("104.234.118.2/32"), netip.MustParsePrefix("69.17.52.1/32"),
	// used until the end of 2025
	netip.MustParsePrefix("162.218.65.0/24"), netip.MustParsePrefix("209.222.252.0/24"),
	netip.MustParsePrefix("91.198.115.0/24"), netip.MustParsePrefix("2604:d500:4:1::/64"),
}

func isLinkingLion(addr string) bool {
	a, err := netip.ParseAddr(hostOf(addr))
	if err != nil {
		return false
	}
	a = a.Unmap()
	for _, p := range linkingLion {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

const linkingLionAbout = "LinkingLion – connects to many nodes and may link transactions to their IP addresses (tracked by 0xB10C, b10c.me)"

var coreVersion = regexp.MustCompile(`[Ss]at[o0]shi[0-9X]*:([0-9.]+)`)

// What Bitcoin Core started to offer, and in which version. A peer calling
// itself an older Core (or a Knots built on it) that offers one of these
// is not that version. On one node 23,567 sessions came as Core 0.7 to 0.18
// offering P2P_V2 - from Core 26, 2023 - each gone after about 25 seconds.
var serviceSince = []struct {
	name, what string
	major      int
	minor      int
}{
	{"P2P_V2", "encrypted v2 connections", 26, 0},
	{"COMPACT_FILTERS", "compact block filters", 0, 21},
	{"NETWORK_LIMITED", "the pruned-node service", 0, 16},
	{"WITNESS", "SegWit", 0, 13},
}

var claimedCore = regexp.MustCompile(`^/Satoshi:([0-9]+)\.([0-9]+)`)

// Software with no version that ever spoke the v2 transport.
var neverV2 = regexp.MustCompile(`(?i)^/(bitcore|bcoin|Classic|Bitcoin ABC|BUCash|bitcoinj|BitCoinJ|breadwallet|MultiBit|libbitcoin)`)

// anachronism returns what a peer offers that the software it names never
// had, as a sentence for "Probably:", or "".
func anachronism(subver string, services []string) string {
	has := map[string]bool{}
	for _, s := range services {
		has[s] = true
	}
	if m := claimedCore.FindStringSubmatch(subver); m != nil {
		maj, _ := strconv.Atoi(m[1])
		mnr, _ := strconv.Atoi(m[2])
		for _, s := range serviceSince {
			if has[s.name] && (maj < s.major || maj == s.major && mnr < s.minor) {
				since := fmt.Sprintf("%d.%d", s.major, s.minor)
				if s.major > 0 {
					since = fmt.Sprint(s.major)
				}
				return fmt.Sprintf("a scanner - it offers %s, which Core %s.%s did not have (from Core %s)", s.what, m[1], m[2], since)
			}
		}
	}
	if has["P2P_V2"] && neverV2.MatchString(subver) {
		return "a scanner - it offers encrypted v2 connections, which that software never had"
	}
	return ""
}

// offersNothingAsCore is a peer naming Bitcoin Core (or Knots) that offers no
// service at all. Core always offers at least WITNESS and NETWORK or
// NETWORK_LIMITED, so the name is borrowed. On the node this was written for:
// 6,954 sessions in four weeks, 27% of everything that said "/Satoshi:", from
// 620 addresses, 73% of them on Amazon's network, and not one ever delivered
// a block first. A nil list - Core did not say - is not this.
func offersNothingAsCore(subver string, services []string) bool {
	return services != nil && len(services) == 0 && claimedCore.MatchString(subver)
}

const offersNothingAbout = "a crawler or scanner - it offers nothing, which Core always does"

// claimOf is the software a user agent names, readably: "/bcoin:v1.0.0/" is
// "bcoin v1.0.0", Core and Knots are spelled out.
func claimOf(subver string) string {
	if v := coreVersion.FindStringSubmatch(subver); v != nil {
		if strings.Contains(subver, "Knots:") {
			return "Bitcoin Knots (Core " + v[1] + ")"
		}
		return "Bitcoin Core " + v[1]
	}
	first, _, _ := strings.Cut(strings.Trim(subver, "/"), "/")
	return strings.Replace(first, ":", " ", 1)
}

func aboutOf(subver string) string {
	for _, r := range aboutRules {
		if r.re.MatchString(subver) {
			return r.text
		}
	}
	return ""
}

// noAgentKind is a peer that has not told its name. On one node that was
// 21,830 sessions in a fortnight, from 2,516 addresses, typically gone after
// 16 seconds: connections that only checked the port, or that are still
// being set up when the page asks.
const noAgentKind = "No user agent"

// kindOf returns the family name, noAgentKind for a peer that sent no user
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

// mempoolWord is removed before the kind rules look for "pool", so that a
// "(mempool.guide)" label does not make a node a pool node.
var mempoolWord = regexp.MustCompile(`(?i)mempool`)

func classify(subver string) (string, bool) {
	if subver == "" {
		return noAgentKind, true
	}
	for _, r := range kindRules {
		s := subver
		if r.name == "Pool node" {
			s = mempoolWord.ReplaceAllString(subver, "")
		}
		if r.re.MatchString(s) {
			return r.name, r.relays
		}
	}
	return "Other", true
}
