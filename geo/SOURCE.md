# geo.bin

Built from **DB-IP IP to Country Lite**, licensed CC BY 4.0
(<https://db-ip.com>). The dashboard footer carries the attribution link the
licence asks for; keep it.

The CSVs are taken from the npm package `@ip-location-db/dbip-country`, which
republishes that data. Current table: version `2.3.2026060120`
(published 2026-06-16), 355,814 IPv4 and 345,646 IPv6 ranges, 251 codes.

## Refresh

```sh
npm view @ip-location-db/dbip-country version        # newest version
npm pack @ip-location-db/dbip-country                # downloads the .tgz
mkdir -p /tmp/dbip && tar xzf ip-location-db-dbip-country-*.tgz -C /tmp/dbip
go run ./tools/gengeo -v4 /tmp/dbip/package/dbip-country-ipv4.csv \
                      -v6 /tmp/dbip/package/dbip-country-ipv6.csv -o geo/geo.bin
go test ./geo/
```

`geo/testdata/vectors.json` was drawn from the same CSVs; if a refresh moves an
address to another country the test names it, and the vectors can be redrawn
from the new files.

# web/world.json

Built by `tools/genmap` from Natural Earth (public domain),
`ne_110m_admin_0_countries.geojson` for outlines and
`ne_50m_admin_0_countries.geojson` for marker positions, both from
<https://github.com/nvkelso/natural-earth-vector/tree/master/geojson>.
