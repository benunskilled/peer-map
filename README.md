# Peer Map

**Get to know the connections behind your Bitcoin node.**

Peer Map puts your live peers on a world map and shows their locations, hosting providers, software and advertised services. Manual, outbound and inbound connections stay separate, so you can see at a glance how each group is spread across regions and providers.

For anyone who has opened a peer list and wondered “who are all these connections?”, this is a place to start exploring.

![Peer Map dashboard](https://raw.githubusercontent.com/benunskilled/bitcoin-lab-community-store/main/bitcoinlab-peermap/overview.png)

## Explore your node's neighbourhood

Start with the world view, then zoom in to split country markers into states, provinces and regions. Click a country or region to filter the peer tables, or toggle connection types to focus on one part of your node's network.

The three groups show how each connection got there:

| Group | What it means |
|---|---|
| **Manual** | Connections established through Core's `addnode` interface |
| **Outbound** | Connections Core opened automatically |
| **Inbound** | Connections another peer opened to your node |

Each table includes the address, location, hosting provider, software, network, P2P transport, ping and time connected.

## Look beyond the map

**See which networks your peers use.** Three peers in three countries may still be hosted by the same provider. Sort by provider to bring those connections together and see how widely your peers are distributed across networks.

**Understand the mix of software.** Peer Map recognises Core, Knots, pools, wallets, light clients, indexers, crawlers and research software. Each connection group shows a summary of that mix, with categories not expected to relay blocks marked in the tables.

The software labels come from each peer's reported user agent. Alongside them, Peer Map shows whether the peer advertises services, requests transaction relay and whether Core knows its chain. Together, these details give you a fuller picture of who's connected.

**Keep unplaced peers in view.** Tor, I2P and CJDNS peers have no public-IP location to plot. On Umbrel, Docker hides the real address of inbound IPv6 peers. These connections are counted in a separate panel beside the map.

The map uses approximate IP locations, with country labels generally more reliable than regional positions. It is a useful view of your connections' geographic spread.

![The peer tables, one per connection group](https://raw.githubusercontent.com/benunskilled/bitcoin-lab-community-store/main/bitcoinlab-peermap/2.png)

## Follow a block through your node

With [Bitcoin Lab](https://github.com/benunskilled/bitcoin-lab) installed on the same node, a card above the map follows the newest block: which pool mined it, which of your peers delivered it first, and — if you run Stratum Race — how quickly each pool turned it into fresh work. The peers that delivered it are starred on the map for two minutes, so you can see where your blocks come in from.

In the peer tables, every peer that has ever delivered a block first is tinted green, and the one that delivered the newest block turns orange for two minutes.

With Stratum Race running, the card draws the block's route to scale — from the first job any pool sent, through your peer, Core and the block template, to your own pool's job — and next to it the typical route over the last hundred blocks. You see which stretch takes the time. Without Bitcoin Lab the card does not appear.

## Local data, quiet operation

Peer Map reads your current connections. The map, geolocation and ASN data are bundled with the app, so looking up a peer does not send its address to an external service. Block details come from Bitcoin Lab on the same node, fetched alongside the peer list rather than on a schedule of their own.

The dashboard requests updates only while it is visible. Background tabs stop polling, and a dashboard with no mouse or keyboard activity for 30 minutes pauses until you resume it.

There is no background collection job or stored peer history. The app is a single static Go binary using only the standard library, and on the author's node it uses about 19 MB of RAM.

## Install on Umbrel

Add the [Bitcoin Peer Lab community store](https://github.com/benunskilled/bitcoin-lab-community-store) and install **Peer Map**. It requires the official **Bitcoin Node** app. Open it from Umbrel or at `<your-umbrel>:8791`.

Want to know which of your peers delivers new blocks first? [Bitcoin Lab](https://github.com/benunskilled/bitcoin-lab), available in the same store, measures block delivery and helps you find and keep stronger peers. Both apps work independently and complement each other.

## Data and attribution

- Geolocation: [DB-IP](https://db-ip.com), IP to City Lite, CC BY 4.0 — data from August 2026 (IPv4) and September 2026 (IPv6).
- Network providers: DB-IP, IP to ASN Lite, CC BY 4.0 — data from September 2026.
- World map: Natural Earth, public domain.

See [geo/SOURCE.md](geo/SOURCE.md) and [asn/SOURCE.md](asn/SOURCE.md) for data versions and update instructions.

## Licence

MIT — see [LICENSE](LICENSE).
