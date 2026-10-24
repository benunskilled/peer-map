package main

import (
	"net/netip"
	"strings"
	"time"

	"github.com/benunskilled/peer-map/asn"
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
	MinPing        *float64 `json:"minping"`
	ConnTime       int64    `json:"conntime"`
	Transport      string   `json:"transport_protocol_type"`
	BytesSent      int64    `json:"bytessent"`
	BytesRecv      int64    `json:"bytesrecv"`
	// What the peer offers and whether Core knows its chain. Pointers and a
	// nil slice on purpose: "Core did not report this field" and "the peer
	// offers nothing" are different answers and must not collapse into one.
	ServicesNames []string `json:"servicesnames"`
	RelayTxes     *bool    `json:"relaytxes"`
	// The protocol version from the peer's version message (Core 0.21 and
	// later send 70016). Zero when Core did not report it.
	Version       int    `json:"version"`
	SyncedHeaders *int64 `json:"synced_headers"`
	// Bytes received per message type. Read only to count pings and address
	// requests (see behaviourAbout); nil when Core did not report it.
	BytesRecvPerMsg map[string]int64 `json:"bytesrecv_per_msg"`
	// The address this peer sees us under. Read only to place this node on
	// the map (see homeOf); never passed on.
	AddrLocal string `json:"addrlocal"`

	mockLoc         *geo.Location // demo data only: documentation addresses have no location
	mockASN         *asn.Info     // demo data only, same reason
	mockLinkingLion bool          // demo only: a documentation address standing in for LinkingLion
	mockHome        *geo.Location // demo only: where the demo node itself is
}

// Home is where this node is, as the map draws it: a region, never an address.
type Home struct {
	Country string  `json:"cc"`
	Region  string  `json:"region,omitempty"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
}

// homeOf places this node by the address its peers see it under: getpeerinfo's
// addrlocal. No extra RPC and nothing sent anywhere - the lookup is the
// bundled database, and only the region leaves this function. Nil when no
// peer reports an address that can be placed (a node reachable only over Tor,
// say).
//
// The vote is per region, not per address: IPv6 privacy addresses would
// otherwise split it, and an address that cannot be placed (CGNAT, a
// documentation range) simply does not count. Only connections this node
// opened vote. addrlocal is whatever the peer claims, and inbound peers are
// anyone - a crowd of them could otherwise move this node across the map.
// Inbound ones are asked only when there are no others.
func homeOf(raw []rawPeer) *Home {
	for _, p := range raw {
		if p.mockHome != nil {
			l := *p.mockHome
			return &Home{Country: l.Country, Region: l.Region, Lat: l.Lat, Lon: l.Lon}
		}
	}
	vote := func(inbound bool) *Home {
		count := map[uint16]int{}
		where := map[uint16]geo.Location{}
		var best uint16
		found := false
		for _, p := range raw {
			if p.Inbound != inbound || p.AddrLocal == "" {
				continue
			}
			a, err := netip.ParseAddr(hostOf(p.AddrLocal))
			if err != nil {
				continue
			}
			a = a.Unmap()
			if !a.IsGlobalUnicast() || a.IsPrivate() {
				continue
			}
			l, ok := geo.Lookup(a)
			if !ok {
				continue
			}
			count[l.ID]++
			where[l.ID] = l
			if !found || count[l.ID] > count[best] {
				best, found = l.ID, true
			}
		}
		if !found {
			return nil
		}
		l := where[best]
		return &Home{Country: l.Country, Region: l.Region, Lat: l.Lat, Lon: l.Lon}
	}
	if h := vote(false); h != nil {
		return h
	}
	return vote(true)
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
	// Where a peer is and whose machine it is are two questions. The second is
	// the one that catches three peers in three countries that all sit with the
	// same provider. Empty for an address no network announces.
	ASN      uint32 `json:"asn,omitempty"`
	Operator string `json:"operator,omitempty"`
	// The name of the app when the peer is one of Umbrel's own, such as electrs.
	App     string `json:"app,omitempty"`
	Subver  string `json:"subver"`
	Kind    string `json:"kind"`               // what the user agent claims; see kinds.go
	About   string `json:"about,omitempty"`    // what that software is, when known
	NoRelay bool   `json:"no_relay,omitempty"` // software that passes no blocks on
	// LinkingLion: from the ranges the bnoc banlist names, whatever kind the
	// user agent made it. The header counts these separately.
	LinkingLion bool `json:"linking_lion,omitempty"`
	// Observed rather than claimed, and omitted when false so the common peer
	// carries none of them.
	NoServices   bool     `json:"no_services,omitempty"`   // advertises NODE_NONE
	NoTxRelay    bool     `json:"no_tx_relay,omitempty"`   // version said: send me no transactions
	ChainUnknown bool     `json:"chain_unknown,omitempty"` // Core has never seen a header from it
	PingMs       *float64 `json:"ping_ms"`
	// The lowest round trip Core has ever measured to this peer. Closer to
	// the line itself than the last ping, which also carries whatever was
	// queued on either side at that moment - so it is the one a block's last
	// hop is estimated from.
	MinPingMs *float64 `json:"min_ping_ms,omitempty"`
	ConnTime  int64    `json:"conntime"`
	Transport string   `json:"transport,omitempty"`
	BytesSent int64    `json:"bytes_sent"`
	BytesRecv int64    `json:"bytes_recv"`
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

// operator returns the network announcing this peer's address. Same exclusions
// as locate: an onion or I2P address belongs to no network anybody announces,
// and a Docker-relayed inbound peer carries our own address, not the peer's.
func operator(p rawPeer) (asn.Info, bool) {
	if p.mockASN != nil {
		return *p.mockASN, true
	}
	switch p.Network {
	case "onion", "i2p", "cjdns", "not_publicly_routable":
		return asn.Info{}, false
	}
	a, err := netip.ParseAddr(hostOf(p.Addr))
	if err != nil {
		return asn.Info{}, false
	}
	return asn.Lookup(a)
}

// dockerGateway is where inbound IPv6 comes from on umbrelOS: the Bitcoin
// container has no IPv6, so docker-proxy accepts the connection on the host
// and passes it on from the gateway of Umbrel's network, port and all. The
// peer's own address is gone. On the node this was written for: 9,935 inbound
// sessions in four weeks, against not one outbound IPv6 connection (Core
// knew 13,357 IPv6 addresses and had tried none successfully).
const dockerGateway = "10.21.0.1"

// hiddenIPv6 is the network such a peer is shown under instead of "Private".
const hiddenIPv6 = "ipv6_hidden"

// Tor and I2P peers reach Core through the proxy containers in Umbrel's
// network, which put their own address in place of the peer's.
const (
	torProxy  = "10.21.22.10" // bitcoin_tor_1
	i2pProxy  = "10.21.22.11" // bitcoin_i2pd_daemon_1
	torHidden = "tor_hidden"
	i2pHidden = "i2p_hidden"
)

// isPrivate: an address inside a private network, such as the one umbrelOS
// runs its apps in.
func isPrivate(addr string) bool {
	a, err := netip.ParseAddr(hostOf(addr))
	return err == nil && a.Unmap().IsPrivate()
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
			Subver: r.Subver, Kind: kindOf(r.Subver), About: aboutOf(r.Subver), NoRelay: !relaysBlocks(r.Subver),
			ConnTime: r.ConnTime, Transport: r.Transport,
			BytesSent: r.BytesSent, BytesRecv: r.BytesRecv,
		}
		// An empty list is the peer saying it offers nothing; a missing field
		// is Core saying nothing. Only the first is worth showing.
		p.NoServices = r.ServicesNames != nil && len(r.ServicesNames) == 0
		p.NoTxRelay = r.RelayTxes != nil && !*r.RelayTxes
		p.ChainUnknown = r.SyncedHeaders != nil && *r.SyncedHeaders < 0
		// Offering nothing means no blocks to pass on, whatever the name says.
		// Bitcoin Lab strikes the same peers.
		p.NoRelay = p.NoRelay || p.NoServices
		switch {
		case p.Kind == "Wallet" && offersWhatNoWalletCan(r.ServicesNames):
			p.Kind, p.About = fakeWalletKind, fakeAbout(r.Subver, p.About, r.ServicesNames)
		case p.Kind == "Wallet" && bitcoinjTooNew(r.Subver, r.Version):
			claim := claimOf(r.Subver)
			if p.About != "" {
				claim, _, _ = strings.Cut(p.About, ",")
				claim = "the wallet " + claim
			}
			p.Kind, p.About = fakeWalletKind, "Claims: "+claim+"\nProbably: "+bitcoinjTooNewAbout+stayedLong(r.ConnTime, time.Now())
		case p.Kind == fakeWalletKind:
			p.About = fakeAbout(r.Subver, "", r.ServicesNames)
		case walletBehindCoreName(r.Subver, r.ServicesNames, r.RelayTxes) != "" &&
			behaviourAbout(r.Subver, r.Transport, r.BytesRecvPerMsg, r.ConnTime, time.Now()) == "":
			p.Kind, p.About = "Wallet", walletBehindCoreName(r.Subver, r.ServicesNames, r.RelayTxes)
		default:
			if why := anachronism(r.Subver, r.ServicesNames); why != "" {
				p.Kind, p.About = fakeWalletKind, "Claims: "+claimOf(r.Subver)+"\nProbably: "+why
			} else if why := protocolMismatch(r.Subver, r.Version); why != "" {
				p.Kind, p.About = fakeWalletKind, "Claims: "+claimOf(r.Subver)+"\nProbably: "+why
			} else if offersNothingAsCore(r.Subver, r.ServicesNames) {
				p.Kind, p.About = fakeWalletKind, "Claims: "+claimOf(r.Subver)+"\nProbably: "+offersNothingAbout
			} else if why := behaviourAbout(r.Subver, r.Transport, r.BytesRecvPerMsg, r.ConnTime, time.Now()); why != "" {
				p.Kind, p.About = fakeWalletKind, "Claims: "+claimOf(r.Subver)+"\nProbably: "+why
			}
		}
		if p.Kind == "Other" && p.About == "" {
			p.About = otherAbout(r.ServicesNames)
		}
		// Core reports version 0 until the peer's version message arrives:
		// with a version, the peer did introduce itself, only without a name.
		if p.Kind == noAgentKind && r.Version > 0 {
			p.About = "Introduced itself but left its name empty: software that keeps quiet about what it is"
		} else if p.Kind == noAgentKind {
			p.About = "Has not said what it is: usually a connection that only checks the port, or one still starting"
		}
		// LinkingLion with a borrowed name is Disguised; without any name it
		// claims nothing, so it stays "No user agent" and only the about says
		// who it probably is.
		if isLinkingLion(r.Addr) || r.mockLinkingLion {
			p.LinkingLion = true
			if r.Subver == "" {
				p.About = "Probably: " + linkingLionAbout
			} else {
				p.Kind, p.About = fakeWalletKind, "Claims: "+claimOf(r.Subver)+"\nProbably: "+linkingLionAbout
			}
		}
		if r.Inbound {
			switch hostOf(r.Addr) {
			case dockerGateway:
				p.Network = hiddenIPv6
			case torProxy:
				p.Network = torHidden
			case i2pProxy:
				p.Network = i2pHidden
			}
		}
		// Umbrel's own apps reach Core from inside its network.
		if p.Kind == "Indexer" && isPrivate(r.Addr) {
			p.About = "Probably your own " + claimOf(r.Subver) + " on this Umbrel"
			p.App, _, _ = strings.Cut(strings.Trim(r.Subver, "/"), ":")
		}
		if net, ok := operator(r); ok {
			p.ASN, p.Operator = net.Number, net.Name
		}
		if p.Network == "not_publicly_routable" {
			p.Operator = "Your Umbrel's own network"
		}
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
		if r.MinPing != nil {
			ms := *r.MinPing * 1000
			p.MinPingMs = &ms
		}
		out = append(out, p)
	}
	return out
}
