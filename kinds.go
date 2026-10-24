package main

import (
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
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
	// /CKCoinD:/ is ckcoind, ckolivas' mining fork of Core (UA_NAME in its
	// clientversion.cpp). /ckp2p:/ is ckpool's own block relay: ckp2p.c on
	// the p2pcrawler and nodrop branches of github.com/ckolivas/ckpool (not
	// on master, which is why it was once taken for someone else's).
	// P2Pool's bitcoind connection says /P2Pool:x/ (p2pool/bitcoin/p2p.py), and
	// a name that is itself a pool - /pydroid-pool:3/ - is one too.
	{"Pool node", regexp.MustCompile(`(?i)ckpool|^/ckp2p:|^/CKCoinD:|\([^)]*pool[^)]*\)|^/P2Pool:|^/[^:/(]*pool[^:/(]*:`), true},
	// Clients of other chains that still dial Bitcoin's port.
	// Bitcoin Classic's "(EB8)" is Bitcoin Cash's block size; SuperBitcoin,
	// Irium and VDS are forks and altcoins of their own.
	{"Other chain", regexp.MustCompile(`(?i)Bitcoin ABC|BUCash|Bitcoin SV|BCHUnlimited|Bitcoin XT|Classic:|\(EB[0-9]|SuperBitcoin|iriumd|vds_|^/Aurum:`), false},
	// Network scanners, in two flavours: the ones run as a service and the
	// ones run by universities. Both announce themselves honestly.
	{"Research scanner", regexp.MustCompile(`(?i)kit\.edu|dsn\.tm|dsn\.kastel|\.ac\.|uni-`), false},
	// Crawlers, seeders and node counters: they collect addresses and look,
	// and several of them say so only as "scan", "seeder", "census" or
	// "monitor" - on one node 953 sessions of /bitcoin-seeder/ and 372 of
	// /btc-range-scan/ used to land in "Other".
	{"Crawler", regexp.MustCompile(`(?i)bitnodes|bluematt-tokio-client|metrika|nodemap|crawl|scan|seeder|census|monitor|observatory|sonar|scout|nebula|watch|listener|argus|readonly|bitdash|logosnaut|dnsseed|survey|lionseed|gnoban`), false},
	// Address indexers for wallets: they follow the chain but relay nothing.
	// "/mempool" at the start of a part of the agent: "/mempool/" and
	// "/mempool.space:1.0/" are the indexer, "(mempool.guide)" is a node label.
	{"Indexer", regexp.MustCompile(`(?i)electrs|electrum|esplora|/mempool`), false},
	// SPV and mobile wallets. bitcoinj is the Android wallet's library.
	{"Wallet", regexp.MustCompile(`(?i)bitcoinj|breadwallet|/bread:|bither|multibit|wasabi|Bitcoin Wallet|^/blueberry:|^/winnow:|^/BitcoinKit:|^/Ladybug:|^/hodlwallet:`), false},
	// BIP157 light clients: neutrino, which Lightning wallets on lnd use, and
	// Kyoto, which announces itself as "Rust BIP-157". btcwire alone is only
	// the Go p2p library, which says nothing about the program using it, so it
	// stays out here and is caught by "Other" below.
	{"Light client", regexp.MustCompile(`(?i)neutrino|BIP-157|kyoto|^Yuki /`), false},
	// A small Utreexo node. Named so it is not lost in "Other", and counted as
	// relaying like any node - Bitcoin Lab has no rule for it either.
	{"Node (Floresta)", regexp.MustCompile(`(?i)floresta`), true},
	// Other full-node software, named rather than left in "Other".
	{"Node (btcd)", regexp.MustCompile(`(?i)btcd:`), true},
	{"Node (libbitcoin)", regexp.MustCompile(`(?i)libbitcoin`), true},
	{"Node (bcoin)", regexp.MustCompile(`(?i)/bcoin:`), true},
	{"Node (Gocoin)", regexp.MustCompile(`(?i)gocoin`), true},
	// Bitcoin Ghost (github.com/bitcoin-ghost/ghost): ghostd, a fork of Core
	// v30, UA_NAME "Ghost" in clientversion.cpp.
	{"Node (Ghost)", regexp.MustCompile(`^/Ghost:`), true},
	// Full nodes written from scratch, each sending its own name (checked in
	// its source): the hashhog fleet of ten languages (github.com/hashhog),
	// reardencode/rbitcoin, AmazingAng/rbtc, bitcoinecho/bitcoin-echo,
	// BobClawblaw/bitcoinmachinecode, kernel-node/kernel-node, and tbcd, the
	// Hemi network's Bitcoin daemon (hemilabs/heminetwork).
	{"Node (other)", regexp.MustCompile(`(?i)^/(CamlCoin|rustoshi|LunarBlock|Haskoin|beamchain|blockbrew|clearbit|nimrod|bitcoin-hybrid|rbitcoin|rbtc|BitcoinEcho|BitcoinMachineCode|kernel-node|tbcd):`), true},
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
// "Disguised" rather than "Fake": it says what happens - the
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

// bitcoinjTooNew is a peer naming Bitcoin Wallet (Schildbach's Android app)
// that speaks a newer protocol than its bitcoinj ever has: bitcoinj's version
// message sends a constant, 70001 up to 0.14, 70012 in 0.15 and 70013 from
// 0.16 to 0.17.1 and master (ProtocolVersion CURRENT). The app has never set
// its own in fifteen years of history. Only bitcoinj up to 0.17 counts, so a
// later bitcoinj that raises the number is not taken for a fake, and only the
// app: another program on bitcoinj could change the number itself. On the node
// this was written for, in four days: every bitcoinj offering nothing sent
// 70013 or 70001, except two sessions of "Bitcoin Wallet 9.26" with 70016
// from one university address, connected for eleven days without asking for
// a single header.
const bitcoinjMaxProtocol = 70013

var walletOnOldBitcoinj = regexp.MustCompile(`(?i)^/bitcoinj:0\.(1[0-7]|[0-9])[./][^/]*/Bitcoin Wallet:`)

func bitcoinjTooNew(subver string, version int) bool {
	return walletOnOldBitcoinj.MatchString(subver) && version > bitcoinjMaxProtocol
}

const bitcoinjTooNewAbout = "not a wallet - speaks a newer protocol than this wallet ever did"

// stayedLong adds how long the peer has been connected once that is a day or
// more: the real wallet, a phone app, stayed 1.4 hours at most on the node
// this was written for, the fake one eleven days. Only said alongside the
// protocol, which is the proof - a phone left open for days is possible.
func stayedLong(connTime int64, now time.Time) string {
	if connTime <= 0 {
		return ""
	}
	days := int(now.Sub(time.Unix(connTime, 0)).Hours() / 24)
	switch {
	case days < 1:
		return ""
	case days == 1:
		return ", and has stayed connected for a day"
	}
	return fmt.Sprintf(", and has stayed connected for %d days", days)
}

// What a piece of software is, in a sentence, for the peers somebody is
// likely to ask about. Only software that says who it is: the kind already
// covers the rest.
var aboutRules = []struct {
	re   *regexp.Regexp
	text string
}{
	{regexp.MustCompile(`(?i)seeder|dnsseed`), "A DNS seeder: collects addresses of reachable nodes for new nodes to start from"},
	{regexp.MustCompile(`(?i)census|survey|observatory|monitor|sonar`), "A node census: counts and watches reachable nodes"},
	{regexp.MustCompile(`(?i)/Bisq:`), "Bisq, a decentralised exchange; its wallet runs on bitcoinj"},
	{regexp.MustCompile(`^/P2Pool:`), "P2Pool, a peer-to-peer mining pool: miners share the work without a pool operator"},
	{regexp.MustCompile(`^/Ghost:`), "Bitcoin Ghost, a fork of Core v30 that filters data-carrying transactions; its operators can join a decentralised mining pool"},
	{regexp.MustCompile(`(?i)^/CamlCoin:`), "CamlCoin, a Bitcoin full node written in OCaml (hashhog)"},
	{regexp.MustCompile(`(?i)^/rustoshi:`), "rustoshi, a Bitcoin full node written in Rust (hashhog)"},
	{regexp.MustCompile(`(?i)^/LunarBlock:`), "LunarBlock, a Bitcoin full node written in Lua (hashhog)"},
	{regexp.MustCompile(`^/Haskoin:`), "A Bitcoin full node written in Haskell (hashhog)"},
	{regexp.MustCompile(`(?i)^/beamchain:`), "beamchain, a Bitcoin full node written in Erlang (hashhog)"},
	{regexp.MustCompile(`(?i)^/blockbrew:`), "blockbrew, a Bitcoin full node written in Go (hashhog)"},
	{regexp.MustCompile(`(?i)^/clearbit:`), "clearbit, a Bitcoin full node written in Zig (hashhog)"},
	{regexp.MustCompile(`(?i)^/nimrod:`), "nimrod, a Bitcoin full node written in Nim (hashhog)"},
	{regexp.MustCompile(`(?i)^/bitcoin-hybrid:`), "ouroboros, a Bitcoin full node written in Rust and Python (hashhog)"},
	{regexp.MustCompile(`(?i)^/rbitcoin:`), "rbitcoin, a Bitcoin full node written in Rust (reardencode)"},
	{regexp.MustCompile(`(?i)^/rbtc:`), "rbtc, Bitcoin Core rewritten in Rust"},
	{regexp.MustCompile(`(?i)^/BitcoinEcho:`), "Bitcoin Echo, a full node in plain C, meant to be frozen as it is"},
	{regexp.MustCompile(`(?i)^/BitcoinMachineCode:`), "Bitcoin Machine Code, a node written in x86 assembly, entirely AI-generated"},
	{regexp.MustCompile(`(?i)^/kernel-node:`), "kernel-node, a node built on libbitcoinkernel, Core's validation engine as a library"},
	{regexp.MustCompile(`(?i)^/tbcd:`), "tbcd, the Bitcoin daemon of the Hemi network"},
	{regexp.MustCompile(`(?i)hemi-soak`), "A soak test of the Hemi network's Bitcoin code"},
	{regexp.MustCompile(`(?i)^/blueberry:`), "Blueberry, BlueWallet's new wallet that talks to nodes directly"},
	{regexp.MustCompile(`(?i)^/winnow:`), "Winnow, an iPhone wallet that talks to nodes directly; its census dials every reachable node daily under the same name"},
	{regexp.MustCompile(`(?i)^/BitcoinKit:`), "BitcoinKit, a wallet library (Unstoppable Wallet and others)"},
	{regexp.MustCompile(`(?i)^/Ladybug:`), "Ladybug, a wallet built on bitcoinj"},
	{regexp.MustCompile(`(?i)^/hodlwallet:`), "HODL Wallet, a phone wallet"},
	{regexp.MustCompile(`^Yuki /`), "Yuki, a light client by Impervious built on Kyoto"},
	{regexp.MustCompile(`(?i)^/LionSeed:`), "LionSeed, a DNS seeder that lists only the nodes of one fork"},
	{regexp.MustCompile(`(?i)^/gnoban:`), "gnoban, a tool that scores nodes and bans the ones its operator does not want"},
	{regexp.MustCompile(`^libbtc `), "libbtc, a small C library for Bitcoin wallets and tools"},
	{regexp.MustCompile(`(?i)^/haskoin-btc:`), "Haskoin, a Haskell library for Bitcoin wallets and tools"},
	{regexp.MustCompile(`(?i)^/sashimi:`), "sashimi, small Unix tools for Bitcoin by josibake"},
	{regexp.MustCompile(`(?i)^/bridge:`), "Probably castor, Floresta's Utreexo bridge node"},
	{regexp.MustCompile(`(?i)^/rust-sha2-p2p-client/`), "gnostr-bitcoin, a terminal monitor of the Bitcoin network from the gnostr project"},
	{regexp.MustCompile(`(?i)^/Gnostr:`), "Names gnostr, the git+nostr tools; offers Libre Relay's service bit and came only through Tor"},
	{regexp.MustCompile(`(?i)^/Aurum:`), "Aurumcoin, an altcoin built on Bitcoin Core 0.12"},
	{regexp.MustCompile(`Bitcoin Wallet:`), "Bitcoin Wallet by Schildbach, an Android app"},
	{regexp.MustCompile(`(?i)breadwallet|/bread:`), "BRD (breadwallet), a phone wallet shut down in 2022"},
	{regexp.MustCompile(`(?i)multibit`), "MultiBit, a desktop wallet discontinued in 2017"},
	{regexp.MustCompile(`(?i)bither`), "Bither, a wallet for phone and desktop"},
	{regexp.MustCompile(`(?i)wasabi`), "Wasabi Wallet, a desktop wallet"},
	// The library alone, without a wallet app's name after it. After the
	// wallet rules above, which also start with bitcoinj.
	{regexp.MustCompile(`(?i)^/bitcoinj:`), "bitcoinj, a Java library that wallets and other tools use to talk to nodes"},
	{regexp.MustCompile(`(?i)neutrino`), "Neutrino: Lightning wallets on lnd, such as Blixt or Zeus"},
	{regexp.MustCompile(`(?i)BIP-157|kyoto`), "Kyoto, a light client for wallets built with BDK"},
	{regexp.MustCompile(`(?i)floresta`), "Floresta, a lightweight Utreexo node"},
	{regexp.MustCompile(`(?i)electrs`), "electrs, the address index behind Electrum-style wallets"},
	{regexp.MustCompile(`(?i)^/CKCoinD:`), "ckcoind, a mining fork of Bitcoin Core used with ckpool"},
	{regexp.MustCompile(`(?i)bluematt-tokio-client`), "dnsseed-rust, the crawler behind a DNS seeder"},
	{regexp.MustCompile(`(?i)ckp2p`), "ckp2p, ckpool's block relay: passes on blocks only, never transactions"},
	{regexp.MustCompile(`(?i)NBitcoin`), "NBitcoin, a .NET Bitcoin library (behind BTCPay Server, among others)"},
	{regexp.MustCompile(`(?i)/bitcore:`), "Bitcore, BitPay's node and index server"},
	// Core's private broadcast (v31): it opens a connection over Tor or I2P,
	// hands over one of its own transactions and leaves, introducing itself
	// with this fixed name and no services so nobody can tell which node sent
	// it (net_processing.cpp, IsPrivateBroadcastConn). The one seen on the
	// node this was written for came from a Tor exit, v2, offering nothing.
	{regexp.MustCompile(`^/pynode:0\.0\.1/$`), "Probably Bitcoin Core handing over a transaction privately, then leaving"},
	// m4ycon on bnoc (#198): it sweeps the P2A anchors of a fleet that minted
	// rune 1:0 in chains of transactions; on the node this was written for it
	// pushed unannounced 66-byte transactions, the size of such a sweep.
	{regexp.MustCompile(`(?i)^/p2aburn:`), "p2aburn, a bot that sweeps P2A anchors (outputs anyone may spend) behind a Runes minting fleet, pushing the transactions unannounced"},
	{regexp.MustCompile(`(?i)python-bitcoinlib`), "python-bitcoinlib, a Python library scripts use to talk to nodes"},
	{regexp.MustCompile(`(?i)pyblock`), "PyBLOCK, a node dashboard"},
	{regexp.MustCompile(`(?i)bitnodes`), "Bitnodes, a public map of reachable nodes"},
	// Seen from one netcup address, offering nothing and gone after a second;
	// the name fits a node map, but nothing public says whose.
	{regexp.MustCompile(`(?i)GlobalNodeMap`), "Probably a node map or counter"},
	// dsn.kastel.kit.edu/bitcoin, the DSN group at KIT.
	{regexp.MustCompile(`(?i)^/dsn\.[a-z]+\.kit\.edu/`), "KIT Karlsruhe's Bitcoin network monitoring, measuring the network since 2015"},
	// btcd's wire library with the app name after it left empty.
	{regexp.MustCompile(`^/btcwire:[^/]*/:/$`), "A program on btcd's Go library that left its own name empty"},
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
	wallet := walletAbout != ""
	if strings.HasPrefix(walletAbout, "bitcoinj,") {
		// The library without an app name claims bitcoinj, not Core or a wallet.
		claim = claimOf(subver)
	} else if wallet {
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
	case wallet && blocks:
		guess = "a node or scanner behind an old wallet name - it offers blocks, which no wallet does"
	case wallet && v2:
		guess = "a scanner behind an old wallet name - it speaks v2, which no wallet of that age does"
	case misspeltCore(subver) != "":
		guess = "not Bitcoin Core - " + misspeltCore(subver)
		if services != nil && len(services) == 0 {
			guess += " and offers no services"
		}
	default:
		guess = "unknown software"
	}
	return "Claims: " + claim + "\nProbably: " + guess
}

var coreNameWithSuffix = regexp.MustCompile(`^/(Satoshi[0-9X]+):`)

// misspeltCore says how a name imitating Core's differs from it, for the
// three spellings the Disguised rule catches, or "".
func misspeltCore(subver string) string {
	switch {
	case strings.HasPrefix(subver, "/Sat0shi:"):
		return "it writes Sat0shi with a zero"
	case coreNameWithSuffix.MatchString(subver):
		return "Core calls itself /Satoshi:, this one /" + coreNameWithSuffix.FindStringSubmatch(subver)[1] + ":"
	case strings.HasPrefix(subver, "Satoshi:"):
		return "Core writes its name between slashes: /" + strings.Trim(subver, "/") + "/"
	}
	return ""
}

// LinkingLion opens connections to many nodes from a few address ranges and
// listens to transaction announcements, which may let it link transactions to
// the IP addresses of nodes. Named and tracked by 0xB10C, who runs
// peer-observer (github.com/peer-observer/peer-observer); the ranges are the
// ones its shared/src/util.rs checked at first, announced by AS54098. On the node this
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
	// listed by bnoc, no longer announced since 2026-03-30
	netip.MustParsePrefix("199.116.84.0/24"),
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

// The credit is not repeated on every card: the hover tooltip and the Disguised
// explanation above the list carry it (LION_CREDIT in web/app.js).
const linkingLionAbout = "LinkingLion – connects to many nodes and may link transactions to their IP addresses"

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
	// Core 0.16 and later always offer NETWORK_LIMITED, pruned or not, so a
	// newer Core offering something but not that is not Core. Seen on one
	// node as a "/Satoshi:31.0.0/" fleet offering NETWORK and WITNESS only,
	// and as "/Satoshi:0.19.0.1/" offering WITNESS alone.
	if len(services) > 0 && !has["NETWORK_LIMITED"] && coreAtLeast(subver, 0, 16) {
		return "unknown software - it lacks the pruned-node service, which Core has always offered since 0.16"
	}
	if has["P2P_V2"] && neverV2.MatchString(subver) {
		return "a scanner - it offers encrypted v2 connections, which that software never had"
	}
	return ""
}

// coreRelease reads a user agent that names a real Core release number:
// 0.x up to 0.21, then 22 and later. "/Satoshi:1.14.5(2x)/" (btc1) and other
// numbers Core never used return ok == false and are left alone.
func coreRelease(subver string) (maj, mnr int, ok bool) {
	m := claimedCore.FindStringSubmatch(subver)
	if m == nil {
		return 0, 0, false
	}
	maj, _ = strconv.Atoi(m[1])
	mnr, _ = strconv.Atoi(m[2])
	return maj, mnr, maj == 0 || maj >= 22
}

func coreAtLeast(subver string, major, minor int) bool {
	maj, mnr, ok := coreRelease(subver)
	return ok && (maj > major || maj == major && mnr >= minor)
}

// protocolMismatch is a peer whose protocol version does not match the Core
// release it names: Core 0.21 to 31 send 70016 (later ones 70017 or more),
// every release before sends 70015 or less. Development builds (x.99) are
// left alone: "0.20.99" was already 0.21 under way. On one node, in four
// days: 12,100 LinkingLion sessions naming Core 0.8 to 0.20 with 70016, a
// "/Satoshi:31.0.0/" fleet with 70015 - and no real Core node either way.
// devBuild is a Core development build: 0.20.99 (0.21 under way) or 31.99.0.
var devBuild = regexp.MustCompile(`^/Satoshi:[0-9]+\.([0-9]+\.)?99[./]`)

func protocolMismatch(subver string, version int) string {
	maj, mnr, ok := coreRelease(subver)
	if !ok || version == 0 || devBuild.MatchString(subver) {
		return ""
	}
	newer := maj >= 22 || mnr >= 21
	switch {
	case newer && version < 70016:
		return fmt.Sprintf("unknown software - it speaks protocol %d, older than any Core since 0.21", version)
	case !newer && version >= 70016:
		return fmt.Sprintf("unknown software - it speaks protocol %d, which Core %d.%d never did", version, maj, mnr)
	}
	return ""
}

// skipsWtxidrelay is a peer naming Core 0.21 or later that finished its
// handshake without sending wtxidrelay. Every Core since 0.21 sends it right
// after the version message to a peer speaking 70016 or later
// (net_processing.cpp, WTXID_RELAY_VERSION; unchanged in 25.1 and 28.1), and
// getpeerinfo counts it under bytesrecv_per_msg. On the node this was written
// for, on 24.10.2026: all 18 outbound Core and Knots peers and 74 inbound ones
// sent it; of those that did not, 26 were Wasabi, two followed blocks for
// hours offering nothing, and two passed for nodes.
func skipsWtxidrelay(subver string, version int, recv map[string]int64) bool {
	return recv != nil && recv["verack"] > 0 && recv["wtxidrelay"] == 0 &&
		version >= 70016 && coreAtLeast(subver, 0, 21)
}

// skippedHandshakeAbout names what the peer left out. sendaddrv2 is sent
// under the same condition as wtxidrelay, so it is named when it is missing too.
func skippedHandshakeAbout(recv map[string]int64) string {
	if recv["sendaddrv2"] == 0 {
		return "not Bitcoin Core - it left out wtxidrelay and sendaddrv2 when connecting; Core has sent both on every connection since 0.21"
	}
	return "not Bitcoin Core - it left out wtxidrelay when connecting; Core has sent it on every connection since 0.21"
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

// offersNothingAbout names the services Core always offers, rather than
// guessing what the peer is.
const offersNothingAbout = "not Bitcoin Core - it offers no services at all; Core always offers NETWORK or NETWORK_LIMITED"

// Libre Relay (github.com/petertodd/bitcoin, branches libre-relay-v28.0 to
// v31.1) is Core under Core's own name with service bit 29 added
// (NODE_LIBRE_RELAY in protocol.h). Core lists that bit as "UNKNOWN[2^29]".
// Libre Relay opens extra connections to nodes advertising it and drops them
// at once when the bit is missing ("does not offer libre relay as expected").
// Garbageman (github.com/chrisguida/bitcoin, branches garbageman-v28,
// garbageman-v29, garbageman-29.3) is a Knots fork with the Knots part of
// its name commented out, so it says "/Satoshi:29.1.0/" or 29.3.0, and it
// advertises the same bit to draw Libre Relay nodes in and drop what they
// send. Libre Relay has no 29.3 and Garbageman nothing past 29, so the
// version tells them apart except up to 29.1. On the node this was written
// for, 21. to 24.10.2026: 21 connections with the bit from 12 addresses,
// 18 of them as 30.2.0, all gone straight after the version message.
const libreRelayService = "UNKNOWN[2^29]"

// libreRelay is the kind and about for a Core name offering bit 29, or "", "".
func libreRelay(subver string, services []string) (string, string) {
	if !slices.Contains(services, libreRelayService) {
		return "", ""
	}
	m := claimedCore.FindStringSubmatch(subver)
	if m == nil {
		return "", ""
	}
	maj, _ := strconv.Atoi(m[1])
	mnr, _ := strconv.Atoi(m[2])
	garbageman := "Claims: Libre Relay\nProbably: Garbageman - a Knots fork that poses as Libre Relay and drops what it gets"
	switch {
	case strings.Contains(subver, "Knots:"):
		// Libre Relay is Core; only a Garbageman built with its Knots name left in says Knots.
		return fakeWalletKind, garbageman
	case maj >= 30:
		return "Node (Libre Relay)", "Libre Relay, a fork of Core that also relays transactions Core and Knots filter out"
	case maj == 29 && mnr >= 2:
		return fakeWalletKind, garbageman
	}
	return "Node (Libre Relay)", "Libre Relay - or Garbageman, a Knots fork that poses as it"
}

// Wasabi Wallet 2.8 connects to nodes itself, always through Tor since 2.8.2
// (PR #15003), so it reaches a clearnet node from a Tor exit. Each connection
// names one of these Core versions at random (Constants.UserAgents in
// WalletWasabi/Helpers/Constants.cs, used by P2pConnectionManager.cs), offers
// nothing and asks for transactions. On the node this was written for, on
// 24.10.2026: 27 such peers at once, 23 of them from Tor exits, every version
// from this list and none outside it, each fetching transactions and headers
// for hours, some also compact block filters. In five weeks before: 1,420
// such sessions, 73% from Tor exits. A crawler borrowing one of these names
// usually turns transaction relay off; one that keeps it on is counted here
// as a wallet. Forks carry lists of their own in between
// (Zecure: 18 names from 0.20.0 to 28.1.0, each on this list or Ginger's),
// so the about says "or a fork".
var wasabiAgents = map[string]bool{
	"/Satoshi:30.2.0/": true, "/Satoshi:30.0.0/": true,
	"/Satoshi:29.3.0/": true, "/Satoshi:29.2.0/": true, "/Satoshi:29.0.0/": true,
	"/Satoshi:28.1.0/": true, "/Satoshi:28.0.0/": true,
	"/Satoshi:27.2.0/": true, "/Satoshi:27.1.0/": true, "/Satoshi:27.0.0/": true,
	"/Satoshi:26.2.0/": true, "/Satoshi:26.1.0/": true, "/Satoshi:26.0.0/": true,
}

// Ginger Wallet, a fork of Wasabi, does the same with the older list Wasabi
// had before 2.5 (Constants.UserAgents in GingerPrivacy/GingerWallet), also
// through Tor. On the node this was written for, in five weeks: 161 sessions
// with a name only on this list, offering nothing and relaying transactions,
// 89% from Tor exits, the names spread evenly over the list.
var gingerAgents = map[string]bool{
	"/Satoshi:26.0.0/": true, "/Satoshi:25.1.0/": true, "/Satoshi:25.0.0/": true, "/Satoshi:24.2.0/": true,
	"/Satoshi:24.1.0/": true, "/Satoshi:24.0.1/": true, "/Satoshi:24.0.0/": true, "/Satoshi:23.2.0/": true,
	"/Satoshi:23.1.0/": true, "/Satoshi:23.0.0/": true, "/Satoshi:22.1.0/": true, "/Satoshi:22.0.0/": true,
	"/Satoshi:0.21.2/": true, "/Satoshi:0.21.1/": true, "/Satoshi:0.21.0/": true, "/Satoshi:0.20.2/": true,
	"/Satoshi:0.20.1/": true, "/Satoshi:0.20.0/": true, "/Satoshi:0.19.1/": true, "/Satoshi:0.19.0.1/": true,
	"/Satoshi:0.19.0/": true, "/Satoshi:0.18.1/": true, "/Satoshi:0.18.0/": true, "/Satoshi:0.17.2/": true,
	"/Satoshi:0.17.1/": true, "/Satoshi:0.17.0.1/": true, "/Satoshi:0.17.0/": true, "/Satoshi:0.16.3/": true,
	"/Satoshi:0.16.2/": true, "/Satoshi:0.16.1/": true, "/Satoshi:0.16.0/": true,
}

// Wasabi also searches for nodes: each wallet runs ten crawlers that connect
// through Tor with the same random name and transaction relay off, take the
// address list and leave within 15 seconds (CreateCrawler in
// P2pConnectionManager.cs). Ginger has no such crawler. Both speak through
// NBitcoin, whose handshake sends sendaddrv2 and verack and nothing else
// (Node.cs): no wtxidrelay, no sendcmpct. In peer-observer's record of the
// node this was written for, 21. to 24.10.2026: 3,247 such searches, the 13
// names 228 to 289 times each, every one with start height 0 and asking for
// addresses, and 402 wallet connections, 20 to 41 per name. Another 101 with
// these names sent sendcmpct instead of sendaddrv2, unevenly spread: other
// software, left Disguised.

// behindCoreName is the kind and about for Wasabi, Ginger and Wasabi's peer
// search, or "", "". recv is bytesrecv_per_msg. NBitcoin speaks 70016.
func behindCoreName(subver string, version int, services []string, relayTxes *bool, recv map[string]int64) (string, string) {
	if services == nil || len(services) > 0 || relayTxes == nil || version < 70016 {
		return "", ""
	}
	// Anything beyond NBitcoin's handshake is other software.
	if recv["wtxidrelay"] > 0 || recv["sendcmpct"] > 0 {
		return "", ""
	}
	claim := "Claims: " + claimOf(subver) + "\nProbably: "
	if !*relayTxes {
		if wasabiAgents[subver] && recv["sendaddrv2"] > 0 {
			return "Crawler", claim + fmt.Sprintf("Wasabi Wallet's peer search - one of its %d borrowed Core names, offers nothing, asks only for addresses", len(wasabiAgents))
		}
		return "", ""
	}
	switch {
	case wasabiAgents[subver] && gingerAgents[subver]:
		return "Wallet", claim + "Wasabi or Ginger Wallet - a borrowed Core name both use, offers nothing, wants transactions"
	case wasabiAgents[subver]:
		return "Wallet", claim + fmt.Sprintf("Wasabi Wallet or a fork - one of its %d borrowed Core names, offers nothing, wants transactions", len(wasabiAgents))
	case gingerAgents[subver]:
		return "Wallet", claim + fmt.Sprintf("Ginger Wallet (or an old Wasabi) - one of its %d borrowed Core names, offers nothing, wants transactions", len(gingerAgents))
	}
	return "", ""
}

// What a peer does, not what it says. Core pings every two minutes
// (PING_INTERVAL in net_processing.cpp) and asks a peer for addresses once per
// connection (m_getaddr_sent); a peer naming Core and doing either far more
// often runs other code. bitcoinj pings every two seconds, crawlers ask again
// and again. The limits leave a wide margin: a ping every 5 seconds, three
// requests. Not 10: a Core development build (31.99) was seen pinging every
// 10 seconds, still Core's code.
const (
	behaviourMinAge = 10 * 60 // seconds connected before a ping rate means anything
	pingFastSeconds = 5
	getaddrTooMany  = 3
)

// msgCount turns Core's bytes per message type back into a count, for
// messages that always have the same size. Core adds the 24-byte header on v1
// and, on v2, the message type plus BIP324's 20 bytes of length and tag
// (m_raw_message_size in net.cpp). The type is one byte for messages with a
// BIP324 short id, such as ping, and 13 bytes for those without, such as
// getaddr (V2_MESSAGE_IDS in net.cpp). Anything else, "detecting" included,
// is not counted.
func msgCount(bytes int64, payload int64, transport string, shortID bool) int64 {
	switch transport {
	case "v1":
		return bytes / (payload + 24)
	case "v2":
		if shortID {
			return bytes / (payload + 1 + 20)
		}
		return bytes / (payload + 13 + 20)
	}
	return 0
}

// behaviourAbout is why a peer naming Core is not Core, judged by what it has
// sent, or "" when its behaviour fits. recv is getpeerinfo's bytesrecv_per_msg.
func behaviourAbout(subver, transport string, recv map[string]int64, connTime int64, now time.Time) string {
	if !claimedCore.MatchString(subver) || recv == nil {
		return ""
	}
	if n := msgCount(recv["getaddr"], 0, transport, false); n >= getaddrTooMany {
		return fmt.Sprintf("a crawler - it asked for addresses %d times, Core asks once", n)
	}
	age := now.Unix() - connTime
	if connTime <= 0 || age < behaviourMinAge {
		return ""
	}
	if n := msgCount(recv["ping"], 8, transport, true); n > 0 && age/n < pingFastSeconds {
		every := age / n
		unit := "seconds"
		if every <= 1 {
			every, unit = 1, "second"
		}
		return fmt.Sprintf("not Bitcoin Core - it pings every %d %s, Core every 2 minutes", every, unit)
	}
	return ""
}

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

// otherAbout is the line for a name none of the rules know, so no peer is left
// without one. Said as what we know, not as what it is: others may know it.
func otherAbout(services []string, relayTxes *bool, recv map[string]int64) string {
	for _, s := range services {
		if s == "NETWORK" {
			return "Probably a full node under a name its operator chose"
		}
	}
	// What it did, where Core counted it: a name nobody knows still says
	// little, its messages say more. Seen on the node this was written for,
	// 21. to 24.10.2026: /pwsh-pusher/ and /p2aburn/ pushing transactions
	// nobody asked for, a family of /reset-header-.../ names and others
	// fetching only headers, /Bitcoin:1.0.0/ asking for addresses each time,
	// /relay-geo:0.1/ fetching every transaction and announcing none.
	switch {
	case recv["tx"] > 0 && recv["inv"] == 0:
		return "Unknown software that pushes transactions without announcing them"
	case relayTxes != nil && !*relayTxes && (recv["getheaders"] > 0 || recv["sendheaders"] > 0) && recv["getdata"] == 0:
		return "Unknown software that follows new blocks: asks for headers, wants no transactions"
	case relayTxes != nil && *relayTxes && recv["getdata"] > 0 && recv["inv"] == 0 && recv["tx"] == 0:
		return "Unknown software that listens to transactions: fetches every one it hears of, announces none"
	case recv["getaddr"] > 0 && recv["getheaders"] == 0 && recv["getdata"] == 0:
		return "Unknown software that collects addresses"
	}
	return "Software with a name we don't recognise"
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

// emptyName is a user agent with nothing in it but slashes, or a version
// without a name before it: "//" came 523 times in three days from two
// addresses, "/:1.1.0-dev+c1771608d/" ten times - nothing said, as with none.
var emptyName = regexp.MustCompile(`^/*$|^/:[^/]*/$`)

func classify(subver string) (string, bool) {
	if subver == "" || emptyName.MatchString(subver) {
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
