# Peer Map

[![Tests](https://github.com/benunskilled/peer-map/actions/workflows/ci.yml/badge.svg)](https://github.com/benunskilled/peer-map/actions/workflows/ci.yml) [![Release](https://img.shields.io/github/v/release/benunskilled/peer-map)](https://github.com/benunskilled/peer-map/releases)

**Ever wondered who all these connections are? Peer Map shows where your node's peers are, who runs them and what they are.**

Your live peers on a world map: their location, the provider that hosts them, and what they run – nodes, pool nodes, wallets, crawlers or scanners. Manual, outbound and inbound connections stay separate.

![Peer Map dashboard](https://raw.githubusercontent.com/benunskilled/bitcoin-lab-community-store/main/bitcoinlab-peermap/overview.png)

## Explore your node's neighbourhood

Start with the world view, then zoom in to split country markers into states, provinces and regions. Click a country or region to filter the peer tables, or toggle connection types to focus on one part of your node's network.

The three groups show how each connection got there:

| Group | What it means |
|---|---|
| **Manual** | Connections established through Core's `addnode` interface |
| **Outbound** | Connections Core opened automatically – eight full-relay and two block-relay-only |
| **Inbound** | Connections another peer opened to your node |

Each table includes the address, location, hosting provider, software, network, P2P transport, ping and time connected. The outbound table also shows which of the two kinds each connection is.

## Look beyond the map

**See which networks your peers use.** Three peers in three different countries may still be hosted by the same provider. Sort by provider to bring those connections together. The count of providers and countries above each table shows how widely your manual peers, for example, are spread.

**Understand the mix of software your peers run.** Peer Map recognises Core, Knots and other node software such as btcd, as well as pools, wallets, light clients, indexers, crawlers, research software and fakes. Each connection group shows a summary of that mix, with categories not expected to relay blocks marked in the tables.

The labels come from each peer's own user agent, so they are what the peer claims. Next to them, Peer Map shows what Core itself observed: whether the peer offers services, relays transactions and follows a chain Core knows. Together, these details give you a fuller picture of who's connected.

**Keep unplaced peers in view.** Tor, I2P and CJDNS peers have no public-IP location to plot. On Umbrel, Docker hides the real address of inbound IPv6 peers. These connections are counted in a separate panel beside the map.

The map uses approximate IP locations, with country labels generally more reliable than regional positions.

![The peer tables, one per connection group](https://raw.githubusercontent.com/benunskilled/bitcoin-lab-community-store/main/bitcoinlab-peermap/2.png)

## Follow a block through your node

With [Bitcoin Lab](https://github.com/benunskilled/bitcoin-lab) installed on the same node, a card above the map follows the newest block: which pool mined it and which of your peers delivered it first. With Stratum Race switched on and your own pool added to it, the card also follows the block all the way to your pool's mining job. The peers that delivered it are starred on the map and turn orange in the peer tables for two minutes; every peer that has ever delivered a block first stays tinted green. Without Bitcoin Lab the card does not appear.

![Block 968,560 from the winning hash to the peer that delivered it first](https://raw.githubusercontent.com/benunskilled/bitcoin-lab-community-store/main/bitcoinlab-peermap/block-to-job.gif)

*Block 968,560 on my node, from the winning hash to the peer that delivered it first. [Watch its whole route to the mining job](https://github.com/user-attachments/assets/e50a460f-b903-4b9e-9668-a80b3573a03c).*

On that route the card draws each stretch to scale – from the first job any pool sent, through your peer, Core and the block template, to your own pool's job – and next to it the typical route over the last hundred blocks. You see which stretch takes the time. The template stretch also shows how many transactions it carried – that is what your own pool's time mostly depends on.

## Local data, quiet operation

Peer Map reads your current connections. The map, geolocation and ASN data are bundled with the app, so looking up a peer does not send its address to an external service. Block details come from Bitcoin Lab on the same node, fetched alongside the peer list.

The dashboard requests updates only while it is visible. Background tabs stop polling, and a dashboard with no mouse or keyboard activity for 30 minutes pauses until you resume it.

There is no background collection job or stored peer history. The app is a single static Go binary using only the standard library, and on my node, with around 200 peers connected, it uses about 27 MB of RAM.

## Install on Umbrel

Add the [Bitcoin Peer Lab community store](https://github.com/benunskilled/bitcoin-lab-community-store) and install **Peer Map**. It requires the official **Bitcoin Node** app. Tested with Bitcoin Core 31.1. Open it from Umbrel or at `<your-umbrel>:8791`.

Curious which of your peers delivers new blocks first? [Bitcoin Lab](https://github.com/benunskilled/bitcoin-lab), in the same store, measures it. Both apps work on their own and complement each other.

## Data and attribution

- Geolocation: [DB-IP](https://db-ip.com), IP to City Lite, CC BY 4.0 — data from August 2026 (IPv4) and September 2026 (IPv6).
- Network providers: DB-IP, IP to ASN Lite, CC BY 4.0 — data from September 2026.
- World map: Natural Earth, public domain.

See [geo/SOURCE.md](geo/SOURCE.md) and [asn/SOURCE.md](asn/SOURCE.md) for data versions and update instructions.

## Licence

MIT — see [LICENSE](LICENSE).
