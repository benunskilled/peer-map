# Peer Map

Where your Bitcoin node's peers are, on a world map - with **manual**, **inbound**
and **outbound** connections kept apart.

![Peer Map](https://raw.githubusercontent.com/benunskilled/bitcoin-lab-community-store/main/bitcoinlab-peermap/1.png)

- One marker per country, split into the three connection types. Toggle each
  type on and off; click a country to filter the tables below to it.
- A table per type: address, country, network, client, P2P transport, ping,
  and how long the connection has been up. Outbound also shows whether a peer
  is full-relay or block-relay-only.
- Peers that cannot be placed are counted separately instead of being dropped:
  Tor, I2P and CJDNS have no location, and on umbrelOS inbound IPv6 arrives
  through docker-proxy and shows up as a private address.

## Built to cost nothing when nobody looks

There is no background job. Bitcoin Core is asked for `getpeerinfo` only while
a dashboard is open and visible, and never more than **once every 10 seconds**
however many browsers are open - every request inside that window gets the
cached answer. Close the tab and the app goes quiet.

It is a single static Go binary with no dependencies outside the standard
library. The world map and the country table are embedded; nothing is fetched
from the internet, and no peer address ever leaves your node.

Measured on x86-64 Linux (v0.1.0), against a mock RPC server returning 200 peers:

| | |
|---|---|
| Binary | 11.8 MB (5.2 MB of it is the country table) |
| Memory, idle (20 s, no dashboard open) | 5.8 MB RSS, 0 ms CPU, 0 RPC calls |
| Memory, 70 dashboard requests over 35 s | 13.8 MB RSS, 50 ms CPU, 4 RPC calls |
| Country lookup | ~41 ns |

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

- IP geolocation: [IP Geolocation by DB-IP](https://db-ip.com), CC BY 4.0
- Map: Natural Earth, public domain

See [geo/SOURCE.md](geo/SOURCE.md) for versions and how to refresh them.

## License

MIT
