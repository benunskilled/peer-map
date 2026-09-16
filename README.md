# Peer Map

Where your Bitcoin node's peers are, on a world map - with **manual**, **inbound**
and **outbound** connections kept apart.

![Peer Map](https://raw.githubusercontent.com/benunskilled/bitcoin-lab-community-store/main/bitcoinlab-peermap/1.png)

- One marker per country, split into the three connection types. **Zoom in and
  the markers split into regions** - states, provinces, Bundesländer - so
  Australia becomes New South Wales, Victoria, Queensland and so on.
- Toggle each type on and off; click a country or region to filter the tables
  below to it.
- A table per type: address, location, network, client, P2P transport, ping,
  and how long the connection has been up. Outbound also shows whether a peer
  is full-relay or block-relay-only.
- Peers that cannot be placed are counted in a panel beside the map (collapsed
  by default) instead of being dropped: Tor, I2P and CJDNS have no location,
  and on umbrelOS inbound IPv6 arrives through docker-proxy and shows up as a
  private address.
- Styled like Bitcoin Lab, and always dark.

Regions come from IP geolocation and are less certain than countries: a server
is placed where its provider registers it.

## Built to cost nothing when nobody looks

There is no background job. Bitcoin Core is asked for `getpeerinfo` only while
a dashboard is open and visible, and never more than **once every 10 seconds**
however many browsers are open - every request inside that window gets the
cached answer. A background tab or a minimised window stops asking, and a
dashboard left open with no mouse or keyboard input for 30 minutes pauses
itself until you press Resume.

It is a single static Go binary with no dependencies outside the standard
library. The world map and the region table are embedded; nothing is fetched
from the internet, and no peer address ever leaves your node.

Measured on x86-64 Linux (v0.2.0), against a mock RPC server returning 200
peers with random public addresses:

| | |
|---|---|
| Binary | 41.5 MB (34.9 MB of it is the region table) |
| Memory, idle (20 s, no dashboard open) | 6.3 MB RSS, 0 ms CPU |
| Memory, 70 dashboard requests over 35 s | 21.4 MB RSS, of which 16.7 MB is table pages the kernel can drop again; 50 ms CPU |
| 24 h simulated, 8,640 refreshes | heap 513 KB before, 511 KB after |
| Region lookup | ~39 ns, no allocation |

The table is searched in place, so only the pages a lookup touches are read;
random addresses touch more of it than a real node's peers do.

## Try it without a node

```sh
PEERMAP_MOCK=1 go run .
# then open http://localhost:8789
```

Demo data uses documentation addresses only (RFC 5737 / RFC 3849) and the page
says so in a banner.

## Configuration

| Variable | |
|---|---|
| `APP_BITCOIN_NODE_IP`, `APP_BITCOIN_RPC_PORT`, `APP_BITCOIN_RPC_USER`, `APP_BITCOIN_RPC_PASS` | Bitcoin Core RPC. umbrelOS sets these for apps that depend on the Bitcoin app. |
| `PEERMAP_PORT` | HTTP port, default `8789` |
| `PEERMAP_MOCK=1` | demo data instead of a node |

## Tests

```sh
go test ./...                                          # unit tests
go test -tags integration -run TestAgainstBitcoinCore -v -count=1 .   # needs bitcoind on PATH
```

The integration test starts four regtest nodes, opens one connection of each
type and checks that the app sorts them into the right group. CI runs it on
every release tag.

## Data

- IP geolocation: [IP Geolocation by DB-IP](https://db-ip.com), IP to City Lite, CC BY 4.0
- Map: Natural Earth, public domain

See [geo/SOURCE.md](geo/SOURCE.md) for versions and how to refresh them.

## License

MIT
