# geo.bin

Built by `tools/gengeo.py` from **DB-IP IP to City Lite**, licensed CC BY 4.0
(<https://db-ip.com>). The dashboard footer carries the attribution link the
licence asks for; keep it.

Source: the npm package `@ip-location-db/dbip-city-mmdb`, which republishes
that data as `.mmdb`. Current table: version `2.3.2026060513` (database build
2026-06-05), 3,547 regions, 2,071,329 IPv4 and 2,239,460 IPv6 ranges,
34,884,635 bytes.

(The CSV edition of the same package, `@ip-location-db/dbip-city`, was last
published in January 2025, which is why the generator reads the `.mmdb`.)

## Refresh

```sh
pip install maxminddb
npm view @ip-location-db/dbip-city-mmdb version       # newest version
npm pack @ip-location-db/dbip-city-mmdb               # downloads the .tgz
mkdir -p /tmp/dbip && tar xzf ip-location-db-dbip-city-mmdb-*.tgz -C /tmp/dbip
python3 tools/gengeo.py /tmp/dbip/package/dbip-city-ipv4.mmdb \
                        /tmp/dbip/package/dbip-city-ipv6.mmdb geo/geo.bin
go test ./geo/
```

This takes a few minutes. It also rewrites `geo/testdata/vectors.json`, the
sample addresses `go test ./geo/` checks against.

Region names in the source are not consistent ("Berlin" / "State of Berlin",
"ACT" / "Australian Capital Territory"); `region_key` in the generator folds
them, and `TestRegionNamesFolded` checks Germany still has 16 and Australia 8.

# web/world.json

Built by `tools/genmap` from Natural Earth (public domain),
`ne_110m_admin_0_countries.geojson` for outlines and
`ne_50m_admin_0_countries.geojson` for country marker positions, both from
<https://github.com/nvkelso/natural-earth-vector/tree/master/geojson>.

```sh
go run ./tools/genmap -shapes ne_110m_admin_0_countries.geojson \
    -labels ne_50m_admin_0_countries.geojson -o web/world.json
```
