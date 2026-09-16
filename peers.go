package main

import (
	"net/netip"
	"strings"

	"github.com/benunskilled/peer-map/geo"
)

// rawPeer is the part of Bitcoin Core's getpeerinfo this app reads.
// connection_type and network exist since Core 0.21; transport_protocol_type
// since 26.0 (absent on older nodes, which is fine - it is display only).
type rawPeer struct {
	ID             int64    `json:"id"`
	Addr           string   `json:"addr"`
	Network        string   `json:"network"`
	ConnectionType string   `json:"connection_type"`
	Inbound        bool     `json:"inbound"`
	Subver         string   `json:"subver"`
	PingTime       *float64 `json:"pingtime"`
	ConnTime       int64    `json:"conntime"`
	Transport      string   `json:"transport_protocol_type"`
	BytesSent      int64    `json:"bytessent"`
	BytesRecv      int64    `json:"bytesrecv"`
	// What the peer offers and whether Core knows its chain. Pointers and a
	// nil slice on purpose: "Core did not report this field" and "the peer
	// offers nothing" are different answers and must not collapse into one.
	ServicesNames []string `json:"servicesnames"`
	RelayTxes     *bool    `json:"relaytxes"`
	SyncedHeaders *int64   `json:"synced_headers"`

	mockLoc *geo.Location // demo data only: documentation addresses have no location
}

// Peer is what the dashboard receives.
type Peer struct {
	ID       int64   `json:"id"`
	Addr     string  `json:"addr"`
	Network  string  `json:"network"`
	Group    string  `json:"group"` // manual | inbound | outbound
	Type     string  `json:"type"`  // Core's connection_type, verbatim
	Country  string  `json:"cc"`    // "" when it cannot be placed
	Region   string  `json:"region,omitempty"`
	RegionID uint16  `json:"rid,omitempty"` // groups peers per region; stable within one build
	Lat      float64 `json:"lat,omitempty"`
	Lon      float64 `json:"lon,omitempty"`
	Subver   string  `json:"subver"`
	Kind     string  `json:"kind"` // what the user agent claims; see kinds.go
	// Observed rather than claimed, and omitted when false so the common peer
	// carries none of them.
	NoServices   bool     `json:"no_services,omitempty"`   // advertises NODE_NONE
	NoTxRelay    bool     `json:"no_tx_relay,omitempty"`   // version said: send me no transactions
	ChainUnknown bool     `json:"chain_unknown,omitempty"` // Core has never seen a header from it
	PingMs       *float64 `json:"ping_ms"`
	ConnTime     int64    `json:"conntime"`
	Transport    string   `json:"transport,omitempty"`
	BytesSent    int64    `json:"bytes_sent"`
	BytesRecv    int64    `json:"bytes_recv"`
}

// group sorts a connection into one of the three buckets the map shows.
// Feeler and addr-fetch connections live for seconds and are not peers in any
// useful sense, so they are dropped ("" means skip).
func group(p rawPeer) string {
	switch p.ConnectionType {
	case "manual":
		return "manual"
	case "inbound":
		return "inbound"
	case "outbound-full-relay", "block-relay-only":
		return "outbound"
	case "feeler", "addr-fetch":
		return ""
	case "":
		// Core older than 0.21 has no connection_type; fall back to direction.
		if p.Inbound {
			return "inbound"
		}
		return "outbound"
	default:
		// A type a future Core adds: show it by direction rather than hide it.
		if p.Inbound {
			return "inbound"
		}
		return "outbound"
	}
}

// hostOf strips the port from Core's addr field: "1.2.3.4:8333",
// "[2001:db8::1]:8333", "abc.onion:8333".
func hostOf(addr string) string {
	if strings.HasPrefix(addr, "[") {
		if i := strings.Index(addr, "]"); i > 0 {
			return addr[1:i]
		}
	}
	if strings.Count(addr, ":") == 1 {
		return addr[:strings.LastIndex(addr, ":")]
	}
	return addr // bare IPv6 without port, or no port at all
}

// locate returns where a peer is, or ok=false when the address is not a
// public IP (Tor, I2P, CJDNS, or a private address such as the 10.21.0.1 that
// docker-proxy puts in front of inbound IPv6 on umbrelOS).
func locate(p rawPeer) (geo.Location, bool) {
	if p.mockLoc != nil {
		return *p.mockLoc, true
	}
	switch p.Network {
	case "onion", "i2p", "cjdns", "not_publicly_routable":
		return geo.Location{}, false
	}
	a, err := netip.ParseAddr(hostOf(p.Addr))
	if err != nil {
		return geo.Location{}, false
	}
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() {
		return geo.Location{}, false
	}
	return geo.Lookup(a)
}

func convert(raw []rawPeer) []Peer {
	out := make([]Peer, 0, len(raw))
	for _, r := range raw {
		g := group(r)
		if g == "" {
			continue
		}
		p := Peer{
			ID: r.ID, Addr: r.Addr, Network: r.Network, Group: g, Type: r.ConnectionType,
			Subver: r.Subver, Kind: kindOf(r.Subver), ConnTime: r.ConnTime, Transport: r.Transport,
			BytesSent: r.BytesSent, BytesRecv: r.BytesRecv,
		}
		// An empty list is the peer saying it offers nothing; a missing field
		// is Core saying nothing. Only the first is worth showing.
		p.NoServices = r.ServicesNames != nil && len(r.ServicesNames) == 0
		p.NoTxRelay = r.RelayTxes != nil && !*r.RelayTxes
		p.ChainUnknown = r.SyncedHeaders != nil && *r.SyncedHeaders < 0
		if loc, ok := locate(r); ok {
			p.Country = loc.Country
			if loc.Region != "" {
				p.Region, p.RegionID = loc.Region, loc.ID
				p.Lat, p.Lon = loc.Lat, loc.Lon
			}
		}
		if r.PingTime != nil {
			ms := *r.PingTime * 1000
			p.PingMs = &ms
		}
		out = append(out, p)
	}
	return out
}
