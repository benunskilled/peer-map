# asn.bin

Built by `tools/genasn.py` from **ip-location-db's ASN database**, which merges
three sources: RouteViews (address to AS number), the five RIRs' statistics via
the NRO, and DB-IP Lite (most of the operator names). All three are CC BY 4.0
and all three require attribution, which is why the dashboard footer now names
RouteViews, DB-IP and the NRO. Keep that line: it is the licence condition, not
decoration.

Source: the npm package `@ip-location-db/asn-mmdb`. Current table: version
`2.3.2026061719` (database build 2026-06-17), 91,065 networks, 466,960 IPv4 and
177,710 IPv6 ranges, 8,587,356 bytes.

The database carries a number, a name and a range. Only those three are kept,
adjacent ranges of the same network are merged, and the gaps between assigned
ranges get an explicit "unknown" entry so a lookup cannot fall through into the
next range and answer with somebody else's network.

## Refresh

```sh
pip install maxminddb
npm view @ip-location-db/asn-mmdb version           # newest version
npm pack @ip-location-db/asn-mmdb                   # downloads the .tgz
mkdir -p /tmp/asndb && tar xzf ip-location-db-asn-mmdb-*.tgz -C /tmp/asndb
python3 tools/genasn.py /tmp/asndb/package/asn-ipv4.mmdb \
                        /tmp/asndb/package/asn-ipv6.mmdb asn/asn.bin
go test ./asn/
```

This also rewrites `asn/testdata/vectors.json`, the 600 sample addresses
`go test ./asn/` checks the rebuilt table against, each carrying the answer the
source database itself gives.

## Upstream notice (June 2026)

The package README carries a warning from its maintainer: the data published to
npm and to the project's `main` branch contains bugs, npm updates are being
stopped, and corrected data is to be downloaded from the project's GitHub
releases instead. The same warning sits on the package `geo.bin` is built from.
Nothing here is broken by it - the table matches the source it was built from,
which the vectors check - but the next refresh should take the data from the
releases rather than from npm, and the commands above then need adjusting.

## What the table cannot answer

IPv6 is keyed on the upper 64 bits, the same as `geo.bin`. Where two networks
share one `/64`, only one of them survives the build: on the June 2026 database
that is 47 of 166,518 keys, 0.03%. Those keys are left out of the test vectors
rather than covered by a tolerance — a test that allows a few wrong answers
stops noticing when the number grows.

An address nobody announces returns `ok=false`, and so does every address that
is not a public IP: Tor, I2P, CJDNS, and the `10.21.0.1` that docker-proxy puts
in front of inbound IPv6 on umbrelOS. That is a gap in the routing table or a
network without one, not a failure.
