package main

import (
	"regexp"
	"strings"
	"time"
)

// What a piece of software is, in a sentence, for the peers somebody is
// likely to ask about. Only software that says who it is: the kind already
// covers the rest.
var aboutRules = []struct {
	re   *regexp.Regexp
	text string
}{
	{regexp.MustCompile(`(?i)seeder|dnsseed`), "A DNS seeder: collects addresses of reachable nodes for new nodes to start from"},
	{regexp.MustCompile(`(?i)census|survey|observatory|monitor|sonar`), "A node census: counts and watches reachable nodes"},
	// One address, 142 sessions in four days, each gone after its version
	// message. Elastum was an exchange; elastum.io no longer resolves.
	{regexp.MustCompile(`(?i)/Elastum:`), "Names Elastum, a former exchange; talks to nodes through bitcoinj"},
	// 26-27.10.2026: one address, five sessions in 40 minutes under protocol
	// 70001 with no services, saying our node is 127.0.0.1:8332. Two ended after
	// the version message; three took 7 to 16 MB of blocks each. Not found
	// anywhere, so the text says only what it names itself.
	{regexp.MustCompile(`^BTC RPC Proxy `), "Names itself an RPC proxy for Bitcoin; no public source"},
	{regexp.MustCompile(`(?i)/Bisq:`), "Bisq, a decentralised exchange; its wallet runs on bitcoinj"},
	{regexp.MustCompile(`^/P2Pool:`), "P2Pool, a peer-to-peer mining pool: miners share the work without a pool operator"},
	{regexp.MustCompile(`^/BitcoinRocks:`), "BitcoinRocks, an experimental fork of Core using RocksDB and compressed block storage"},
	{regexp.MustCompile(`(?i)^/EMPORION:`), "EMPORION, a Bitcoin header client for verifying OpenTimestamps proofs; later renamed SYMMARTYS"},
	{regexp.MustCompile(`(?i)^/SYMMARTYS:`), "SYMMARTYS, formerly EMPORION: follows Bitcoin headers to verify OpenTimestamps proofs"},
	{regexp.MustCompile(`(?i)^/bip110-observer:`), "BIP-110 Observer, a crawler that collects peer user agents and advertised services"},
	// Only the name points to favillakey.io; the site documents no P2P
	// connections, so the kind stays Other.
	{regexp.MustCompile(`^/FavillaKEY:`), "Named like Favilla KEY, a Nostr and Bitcoin signing device (favillakey.io); what it connects for is not documented"},
	{regexp.MustCompile(`^/Ghost:`), "Bitcoin Ghost, a fork of Core v30 that filters data-carrying transactions; its operators can join a decentralised mining pool"},
	{regexp.MustCompile(`(?i)^/CamlCoin:`), "CamlCoin, a Bitcoin full node written in OCaml (hashhog)"},
	{regexp.MustCompile(`(?i)^/rustoshi:`), "rustoshi, a Bitcoin full node written in Rust (hashhog)"},
	{regexp.MustCompile(`(?i)^/LunarBlock:`), "LunarBlock, a Bitcoin full node written in Lua (hashhog)"},
	{regexp.MustCompile(`^/Haskoin:`), "A Bitcoin full node written in Haskell (hashhog)"},
	{regexp.MustCompile(`(?i)^/beamchain:`), "beamchain, a Bitcoin full node written in Erlang (hashhog)"},
	{regexp.MustCompile(`(?i)^/blockbrew:`), "blockbrew, a Bitcoin full node written in Go (hashhog)"},
	{regexp.MustCompile(`(?i)^/clearbit:`), "clearbit, a Bitcoin full node written in Zig (hashhog)"},
	{regexp.MustCompile(`(?i)^/nimrod:`), "nimrod, a Bitcoin full node written in Nim (hashhog)"},
	{regexp.MustCompile(`(?i)^/hotbuns:`), "hotbuns, a Bitcoin full node written in TypeScript on Bun (hashhog)"},
	{regexp.MustCompile(`(?i)^/bitcoin-hybrid:`), "ouroboros, a Bitcoin full node written in Rust and Python (hashhog)"},
	{regexp.MustCompile(`(?i)^/rbitcoin:`), "rbitcoin, a Bitcoin full node written in Rust (reardencode)"},
	{regexp.MustCompile(`(?i)^/rbtc:`), "rbtc, Bitcoin Core rewritten in Rust"},
	{regexp.MustCompile(`(?i)^/BitcoinEcho:`), "Bitcoin Echo, a full node in plain C, meant to be frozen as it is"},
	{regexp.MustCompile(`(?i)^/BitcoinMachineCode:`), "Bitcoin Machine Code, a node written in x86 assembly, entirely AI-generated"},
	{regexp.MustCompile(`(?i)^/kernel-node:`), "kernel-node, a node built on libbitcoinkernel, Core's validation engine as a library"},
	{regexp.MustCompile(`(?i)^/bitcoinkernel-node:`), "bitcoinkernel-node, a small full node built on libbitcoinkernel, Core's validation engine as a library"},
	{regexp.MustCompile(`(?i)^/satd:`), "satd, a Bitcoin full node written in Rust that works like Core"},
	{regexp.MustCompile(`(?i)^/tbcd:`), "tbcd, the Bitcoin daemon of the Hemi network"},
	{regexp.MustCompile(`(?i)hemi-soak`), "A soak test of the Hemi network's Bitcoin code"},
	{regexp.MustCompile(`(?i)^/blueberry:`), "Blueberry, BlueWallet's new wallet that talks to nodes directly"},
	{regexp.MustCompile(`(?i)^/winnow:`), "Winnow, an iPhone wallet that talks to nodes directly"},
	{regexp.MustCompile(`(?i)^/BitcoinKit:`), "BitcoinKit, a wallet library (Unstoppable Wallet on Android and others)"},
	{regexp.MustCompile(`^/WalletKit:`), "Unstoppable Wallet on iPhone"},
	{regexp.MustCompile(`(?i)^/Ladybug:`), "Ladybug, a wallet built on bitcoinj"},
	{regexp.MustCompile(`(?i)^/hodlwallet:`), "HODL Wallet, a phone wallet"},
	{regexp.MustCompile(`^Yuki /`), "Yuki, a light client by Impervious built on Kyoto"},
	{regexp.MustCompile(`^/VerifiedX:`), "A VerifiedX validator: a sidechain that holds bitcoin and follows Bitcoin's chain for it"},
	{regexp.MustCompile(`(?i)^/LionSeed:`), "LionSeed, a DNS seeder that lists only the nodes of one fork"},
	{regexp.MustCompile(`(?i)^/gnoban:`), "gnoban, a tool that scores nodes and bans the ones its operator does not want"},
	{regexp.MustCompile(`^libbtc `), "libbtc, a small C library for Bitcoin wallets and tools"},
	{regexp.MustCompile(`(?i)^/haskoin-btc:`), "Haskoin, a Haskell library for Bitcoin wallets and tools"},
	{regexp.MustCompile(`(?i)^/sashimi:`), "sashimi, small Unix tools for Bitcoin by josibake"},
	{regexp.MustCompile(`(?i)^/bridge:`), "Probably castor, Floresta's Utreexo bridge node"},
	{regexp.MustCompile(`(?i)^/rust-sha2-p2p-client/`), "gnostr-bitcoin, a terminal monitor of the Bitcoin network from the gnostr project"},
	{regexp.MustCompile(`(?i)^/Gnostr:`), "Names gnostr, the git+nostr tools"},
	{regexp.MustCompile(`(?i)^/Aurum:`), "Aurumcoin, an altcoin built on Bitcoin Core 0.12"},
	{regexp.MustCompile(`(?i)/Purity:`), "Bitcoin Purity, a fork of Knots that made BIP110 permanent in a hard fork at block 961637"},
	{regexp.MustCompile(`Bitcoin Wallet:`), "Bitcoin Wallet by Schildbach, an Android app"},
	// breadwallet-core sends "/bread:" with its own version, 2.1 since 2019
	// (BRPeer.h), also in BRD's later WalletKit (blockset-corp/walletkit).
	{regexp.MustCompile(`^/bread:`), "BRD's wallet core: the BRD app, shut down in 2022, or a wallet built on its code"},
	{regexp.MustCompile(`(?i)breadwallet`), "BRD (breadwallet), a phone wallet shut down in 2022"},
	{regexp.MustCompile(`(?i)multibit`), "MultiBit, a desktop wallet discontinued in 2017"},
	{regexp.MustCompile(`(?i)bither`), "Bither, a wallet for phone and desktop"},
	{regexp.MustCompile(`(?i)wasabi`), "Wasabi Wallet, a desktop wallet"},
	// The library alone, without a wallet app's name after it. After the
	// wallet rules above, which also start with bitcoinj.
	{regexp.MustCompile(`(?i)^/bitcoinj:`), "bitcoinj, a Java library that wallets and other tools use to talk to nodes"},
	// neutrino sends a version fixed in its own code, not lnd's: 0.12.0-beta
	// from neutrino 0.12 to 0.16 (lnd 0.13 to 0.20), 0.17.1 since neutrino
	// 0.17.1 (lnd 0.21, June 2026; useragent.go in lightninglabs/neutrino).
	// Zeus 13.2 ships lnd 0.21.4, Blixt 0.9 lnd 0.20, and neither changes the
	// name; lnd on a server in neutrino mode says the same, so an app is only an example.
	{regexp.MustCompile(`(?i)/neutrino:0\.17\.`), "lnd 0.21 or newer, without a node of its own (neutrino), e.g. Zeus"},
	{regexp.MustCompile(`(?i)/neutrino:0\.12\.0-beta/`), "Older lnd, without a node of its own (neutrino), e.g. Blixt or an older Zeus"},
	{regexp.MustCompile(`(?i)neutrino`), "Neutrino: Lightning wallets on lnd, such as Blixt or Zeus"},
	{regexp.MustCompile(`(?i)BIP-157|kyoto`), "Kyoto, a light client for wallets built with BDK"},
	{regexp.MustCompile(`(?i)floresta`), "Floresta, a lightweight Utreexo node"},
	{regexp.MustCompile(`(?i)electrs`), "electrs, the address index behind Electrum-style wallets"},
	{regexp.MustCompile(`(?i)^/CKCoinD:`), "ckcoind, a mining fork of Bitcoin Core used with ckpool"},
	{regexp.MustCompile(`(?i)bluematt-tokio-client`), "dnsseed-rust, the crawler behind a DNS seeder"},
	{regexp.MustCompile(`(?i)ckp2p`), "ckp2p, ckpool's block relay: passes on blocks only, never transactions"},
	// Relay networks with a name of their own: bloXroute's gateway builds
	// "bloXroute Gateway <version>" (bxgateway, 2022); Matt Corallo's
	// RelayNode adapters send /RelayNetworkClient:42/ and
	// /RelayNetworkServer:42/ (2013-2017); Falcon was seen as
	// /Cornell-Falcon-Network:0.1.0/ (bips discussion, March 2017).
	{regexp.MustCompile(`(?i)^bloXroute Gateway`), "bloXroute Gateway, a relay network's gateway: passes on blocks and transactions"},
	{regexp.MustCompile(`^/RelayNetwork(Client|Server):`), "Matt Corallo's old Relay Network, which ran until 2017"},
	{regexp.MustCompile(`^/Cornell-Falcon-Network:`), "Falcon, Cornell's block relay network from 2016"},
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
	{regexp.MustCompile(`(?i)^/p2aburn:`), "p2aburn, a bot that sweeps P2A anchors (outputs anyone may spend) behind a Runes minting fleet"},
	{regexp.MustCompile(`(?i)python-bitcoinlib`), "python-bitcoinlib, a Python library scripts use to talk to nodes"},
	{regexp.MustCompile(`(?i)pyblock`), "PyBLOCK, a node dashboard"},
	// Projects whose name matches; none of them publishes the exact string.
	// Urbit reaches Bitcoin through its %btc-provider (github.com/urbit/urbit);
	// Symbiosis Finance runs a Bitcoin bridge (symbiosis.finance/bridge-btc);
	// DexCash Network is a blockchain of its own (dexcash.org). curl is the
	// download tool and does not speak Bitcoin's protocol, so a peer naming it
	// borrows the name.
	{regexp.MustCompile(`^urbit$`), "Probably Urbit's Bitcoin software (%btc-provider)"},
	{regexp.MustCompile(`/Symbiosis Bitcoin Bridge:`), "Probably the Bitcoin bridge of Symbiosis Finance, built on btcd's Go library"},
	{regexp.MustCompile(`^/curl:`), "Names curl, the download tool; curl itself does not speak Bitcoin's protocol"},
	{regexp.MustCompile(`^/dexcash-nodes:`), "Named like DexCash Network, a blockchain of its own"},
	{regexp.MustCompile(`(?i)^/alt-bitnodes:`), "Named after Bitnodes, the public node map"},
	{regexp.MustCompile(`(?i)bitnodes`), "Bitnodes, a public map of reachable nodes"},
	// Seen from one netcup address, offering nothing and gone after a second;
	// the name fits a node map, but nothing public says whose.
	{regexp.MustCompile(`(?i)GlobalNodeMap`), "Probably a node map or counter"},
	// illuminodes/mantle: the node says "/mantle:0.1.0/", its replay tool,
	// which fetches mainnet blocks from peers to validate them, says
	// "/mantle-replay-iouring:0.1.0/" (crates/fetcher/src/lib.rs); beside it
	// "/mantle-discover:0.1.0/" collects addresses with getaddr
	// (replay/src/discover_peers.rs) and "/mantle-rank-peers:0.1.0/" times
	// how fast a node answers its version (replay/src/rank_peers.rs).
	{regexp.MustCompile(`(?i)^/mantle:`), "Mantle, a Bitcoin full node written from scratch in Rust (illuminodes)"},
	{regexp.MustCompile(`(?i)^/mantle-replay`), "Mantle's replay tool: fetches blocks from nodes to check them with Mantle, a Bitcoin node written from scratch in Rust"},
	{regexp.MustCompile(`(?i)^/mantle-discover`), "Mantle's peer finder: asks nodes for addresses of other nodes"},
	{regexp.MustCompile(`(?i)^/mantle-rank-peers`), "Mantle's speed test: times how fast a node answers"},
	// btcd's wire package sends "/btcwire:0.5.0/" when a program sets no name
	// of its own (wire/msgversion.go), and its peer package's example sets
	// "peer" 1.0.0 (peer/example_test.go). On the node this was written for,
	// 21. to 25.10.2026, the bare name asked for addresses and headers and
	// wanted no transactions.
	{regexp.MustCompile(`^/btcwire:[^/]*/$`), "A program on btcd's Go library that set no name of its own"},
	{regexp.MustCompile(`^/btcwire:[^/]*/peer:1\.0\.0/$`), "Probably a program built from the example code in btcd's Go library, keeping its name"},
	// RESET (Trac-Systems/reset) is a coin clocked by Bitcoin's blocks; a
	// share of its reward goes to whoever brings in a new block first. Its
	// agents fetch from mempool.space unless told otherwise and its code has
	// no P2P client, so these names are a guess from the name, the behaviour
	// and the timing: its repository was created on 15.10.2026, and Bitcoin
	// Lab's record of the node this was written for, from 13.9., shows the
	// first reset-* name on 20.10.; by 25.10. there were seven, asking only
	// for headers, most wanting no transactions.
	{regexp.MustCompile(`(?i)^/reset-`), "Probably a header client for RESET, a coin clocked by Bitcoin's blocks that pays a small reward to whoever brings in a new block first"},
	// JWRD Computing (jwrd.net) maintains its own bitcoind of the old 0.5
	// line, which writes version 99999 as 0.9.99.99; nothing public says it
	// sends this name.
	{regexp.MustCompile(`^/jwrd\.net:`), "Probably a node on JWRD Computing's own bitcoind, a fork of an early version"},
	// dsn.kastel.kit.edu/bitcoin, the DSN group at KIT.
	{regexp.MustCompile(`^/btcwire:[^/]*/Satoshi:`), "Pool Detective, by MIT's Digital Currency Initiative: watches what mining pools do"},
	{regexp.MustCompile(`(?i)/lit:v`), "lit, a Lightning node by MIT's Digital Currency Initiative, no longer developed"},
	// observer/src/protocol.rs in 0xB10C/p2p-observer, work in progress in
	// October 2026; extractors/p2p in peer-observer/peer-observer, which a
	// node connects to itself when told to (addnode).
	// src/sync/stages/pull/peer.rs in maestro-org/maestro-symphony;
	// src/crawl.rs in achow101/dnsseedrs; Satscraft/bip324-probe.
	// /your-fork:/ has no sentence of its own (see kinds.go). Bitcoin Lab's
	// record of the node this was written for, 13.9. to 25.10.2026: 241 sessions from 187 addresses, offering nothing, no
	// transactions, gone after 18 seconds at the median; the same handshake
	// as Bitnodes' crawler (version, sendaddrv2, verack, then getaddr). A
	// session that stays, like Bitnodes' own, asks for nothing: getpeerinfo
	// on 31.10.2026, one of 22 hours, no getaddr.
	{regexp.MustCompile(`^/symphony:`), "Maestro Symphony, a Bitcoin indexer and API server"},
	{regexp.MustCompile(`^/crawlrs:`), "dnsseedrs, a DNS seeder by Ava Chow, a Bitcoin Core maintainer"},
	{regexp.MustCompile(`^/bip324-probe:`), "bip324-probe, a tool that checks whether nodes offer encrypted v2 connections"},
	{regexp.MustCompile(`^/p2p-observer:`), "p2p-observer, 0xB10C's tool that connects to nodes to watch the network"},
	{regexp.MustCompile(`^/p2p-extractor:`), "peer-observer's p2p extractor: nodes are told to connect to it, and it records what they send"},
	{regexp.MustCompile(`^/AgenticBitcoin:`), "AgenticBitcoin, Bitcoin rewritten in Rust by AI agents"},
	{regexp.MustCompile(`^/SatoshiSharp:`), "SatoshiSharp, parts of Bitcoin written in C#"},
	{regexp.MustCompile(`^NBXplorer-`), "NBXplorer, the indexer behind BTCPay Server"},
	// NodeConstants.scala in bitcoin-s/bitcoin-s; indexer.rs in stacks-core.
	{regexp.MustCompile(`^/bitcoin-s:`), "bitcoin-s, a Bitcoin toolkit in Scala with a light client on compact block filters"},
	{regexp.MustCompile(`^Stacks/`), "A Stacks node: follows Bitcoin's blocks for the Stacks chain built on it"},
	{regexp.MustCompile(`/utreexod:`), "utreexod, a Utreexo full node built on btcd"},
	{regexp.MustCompile(`^/python-bitcoinlib:`), "python-bitcoinlib, a Python library for scripts and tests"},
	{regexp.MustCompile(`^/nakamoto:`), "nakamoto, a light client on compact block filters"},
	{regexp.MustCompile(`(?i)/spvwallet:`), "spvwallet, the wallet of OpenBazaar, a marketplace closed in 2021"},
	// Autarkysoft's C# toolkit (Configuration.cs); it pushes transactions
	// under "/Satoshi:0.22.0/" instead, so only its own name lands here.
	{regexp.MustCompile(`^/Denovo:`), "Denovo, a Bitcoin toolkit in C#: pushes transactions, mines on testnet"},
	// network.py in jimmysong/programmingbitcoin and the course exercises.
	{regexp.MustCompile(`^/programming(bitcoin|blockchain):`), "Code from Jimmy Song's book or course Programming Bitcoin"},
	{regexp.MustCompile(`^/bip324-ts:`), "bip324-ts, a TypeScript library for encrypted v2 connections"},
	{regexp.MustCompile(`(?i)^/dsn\.[a-z]+\.kit\.edu/`), "KIT Karlsruhe's Bitcoin network monitoring, measuring the network since 2015"},
	// btcd's wire library with the app name after it left empty.
	{regexp.MustCompile(`^/btcwire:[^/]*/:/$`), "A program on btcd's Go library that left its own name empty"},
	{regexp.MustCompile(`(?i)btcd:`), "btcd, a full node written in Go"},
	{regexp.MustCompile(`(?i)libbitcoin`), "libbitcoin, a full node written in C++, independent of Core"},
	{regexp.MustCompile(`(?i)/bcoin:`), "bcoin, a full node written in JavaScript"},
	{regexp.MustCompile(`(?i)gocoin`), "Gocoin, a full node and wallet written in Go"},
	// gosuda/bitcoin-rs, crates/primitives/src/version.rs: "/bitcoin-rs:<version>/".
	{regexp.MustCompile(`^/bitcoin-rs:`), "bitcoin-rs, a full node written in Rust, independent of Core"},
	// No public source. On the node this was written for, 28.10. to
	// 01.11.2026: 23 sessions from 16 addresses, all in Cloudflare's network
	// (AS13335), none sending BIP 331's sendpackages, gone after 11 s (median).
	// Only the name is said: what one node saw is no rule for all of them.
	{regexp.MustCompile(`^/cf-package-relay:`), "Named after package relay, which passes related transactions on together (BIP 331)"},
	// Bitcoin Cash clients carry its block size as "(EB8.0)" too, so they
	// come before Classic.
	{regexp.MustCompile(`(?i)Bitcoin ABC`), "Bitcoin ABC, a Bitcoin Cash client, eCash's node since 2020"},
	{regexp.MustCompile(`(?i)BUCash|BCHUnlimited`), "Bitcoin Unlimited, a Bitcoin Cash client"},
	{regexp.MustCompile(`(?i)Bitcoin SV`), "Bitcoin SV, the node of BSV, which split from Bitcoin Cash in 2018"},
	{regexp.MustCompile(`(?i)Bitcoin XT`), "Bitcoin XT, a fork of Core for bigger blocks that went on to Bitcoin Cash"},
	{regexp.MustCompile(`(?i)SuperBitcoin`), "Super Bitcoin, a fork of Bitcoin from December 2017 with a chain of its own"},
	{regexp.MustCompile(`(?i)iriumd`), "Irium, an altcoin with a chain of its own"},
	{regexp.MustCompile(`(?i)vds[_ ]`), "VDS, an altcoin with a chain of its own"},
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
		guess = "a node or scanner behind an old wallet name - it speaks v2, which no wallet of that age does"
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

// sessionLine is the line for a name none of the rules know, so no peer is left
// without one. Said as what we know, not as what it is: others may know it.
func sessionLine(services []string, relayTxes *bool, recv, sent map[string]int64, connTime int64, now time.Time) (line string, saysNoTx bool) {
	// A peer that has been connected for a while and asked for nothing is no
	// full node, whatever services it lists. Seen 21. to 26.10.2026: /your-fork:0.1/
	// and /alt-bitnodes:0.1/ staying for hours, hearing of transactions
	// and never fetching one, /pydroid-pool:3/ wanting neither.
	if quietAfterHandshake(recv, connTime, now) {
		if wantsTx(relayTxes) {
			// "Hears" only if we announced something to it.
			if sent["inv"] > 0 {
				return "Only listens: hears new transactions, fetches none", false
			}
			return "Only listens: asks for nothing", false
		}
		return "Only listens: wants no transactions, asks for nothing", true
	}
	for _, s := range services {
		if s == "NETWORK" {
			return operatorNamedAbout, false
		}
	}
	// What it did, where Core counted it: a name nobody knows still says
	// little, its messages say more. Seen on the node this was written for,
	// 21. to 24.10.2026: /pwsh-pusher/ and /p2aburn/ pushing transactions
	// nobody asked for, others
	// fetching only headers, /Bitcoin:1.0.0/ asking for addresses each time,
	// /relay-geo:0.1/ fetching every transaction and announcing none.
	// A real node always offers a service, so one that offers none cannot be
	// the node reading below; nil means Core did not say.
	maybeNode := services == nil || len(services) > 0
	or := func(node, other string) string {
		if maybeNode {
			return node + ", or " + other
		}
		return other
	}
	switch {
	// BIP 35: "send me your whole mempool". Seen 22. to 26.10.2026 as /curl:8.5.0/,
	// twenty times from one address, each right after the handshake.
	case recv["mempool"] > 0:
		return "Asks for our mempool", false
	case recv["tx"] > 0 && recv["inv"] == 0:
		return "Pushes transactions without announcing them", false
	// Headers and no transactions is also what a real node does on a
	// connection for blocks only (2.2% of 2,553 Core and Knots sessions on
	// one node, 16. to 28.10.2026), so both readings are given. One that asks
	// for addresses too runs with blocksonly=1 if it is a node (noTxWhy).
	case refusesTx(relayTxes) && (recv["getheaders"] > 0 || recv["sendheaders"] > 0) && recv["getdata"] == 0 && recv["getaddr"] > 0:
		return "Asks for headers and addresses and wants no transactions: " + or("a node running blocksonly", "a program that tracks the chain"), true
	case refusesTx(relayTxes) && (recv["getheaders"] > 0 || recv["sendheaders"] > 0) && recv["getdata"] == 0:
		return "Asks for headers and wants no transactions: " + or("a block-only connection", "a program that tracks the chain"), true
	// Asking for data alone is no sign of listening: a peer that fetches
	// blocks asks the same way. Only transactions sent to it count.
	case wantsTx(relayTxes) && recv["getdata"] > 0 && sent["tx"] > 0 && recv["inv"] == 0 && recv["tx"] == 0:
		// 0.5% of the real nodes above did the same: one that hears of
		// everything from others first has nothing to announce.
		return "Fetches transactions from your node and announces none: " + or("a node that hears of them elsewhere first", "a program collecting them"), false
	case recv["getaddr"] > 0 && recv["getheaders"] == 0 && recv["getdata"] == 0:
		// 6.2% of the real nodes above did the same: one in sync asks for no headers.
		return "Asks for addresses, not for headers or blocks: " + or("a node already in sync", "a crawler"), false
	}
	return unknownAbout, false
}

// otherAbout is sessionLine's sentence alone.
func otherAbout(services []string, relayTxes *bool, recv, sent map[string]int64, connTime int64, now time.Time) string {
	line, _ := sessionLine(services, relayTxes, recv, sent, connTime, now)
	return line
}

// The two lines of otherAbout that say nothing the peer did: a guess from
// its services, and no guess at all.
const (
	operatorNamedAbout = "Probably a full node under a name its operator chose"
	unknownAbout       = "Software with a name we don't recognise"
)

// observedPart is otherAbout's line without the guess at what such a peer
// is: under a sentence that already says what it is, only what it did
// belongs. "Only listens: asks for nothing" keeps what follows the colon,
// "Asks for headers ...: a program that tracks the chain" what comes before.
func observedPart(line string) string {
	if rest, ok := strings.CutPrefix(line, "Only listens: "); ok {
		return strings.ToUpper(rest[:1]) + rest[1:]
	}
	before, _, _ := strings.Cut(line, ": ")
	return before
}

func aboutOf(subver string) string {
	for _, r := range aboutRules {
		if r.re.MatchString(subver) {
			return r.text
		}
	}
	return ""
}

// noAgentAbout is what a peer that sent its version but left its name empty
// did next, in a few words. Seen 21. to 26.10.2026, 429 such sessions: 113
// asked for headers and addresses like a full node, 43 asked for addresses
// and nothing else, the rest said nothing or only answered pings.
func noAgentAbout(recv map[string]int64, connTime int64, now time.Time) string {
	// The pill says it has no name; the sentence says what it did.
	switch {
	case recv["getheaders"] > 0 && recv["getaddr"] > 0:
		return "Asks for headers and addresses, like a full node does"
	case recv["getheaders"] > 0 && recv["sendcmpct"] > 0:
		return "Asks for headers and compact blocks, like a full node does"
	case recv["getaddr"] > 0 && recv["getheaders"] == 0 && recv["getdata"] == 0:
		return "Asks for addresses, not for headers or blocks"
	case quietAfterHandshake(recv, connTime, now):
		return "Introduced itself, then asked for nothing"
	}
	return "Introduced itself without a name; nothing it did tells what it is"
}
