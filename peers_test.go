package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGroup(t *testing.T) {
	cases := []struct {
		typ     string
		inbound bool
		want    string
	}{
		{"manual", false, "manual"},
		{"inbound", true, "inbound"},
		{"outbound-full-relay", false, "outbound"},
		{"block-relay-only", false, "outbound"},
		{"feeler", false, ""},
		{"addr-fetch", false, ""},
		{"", true, "inbound"},
		{"", false, "outbound"},
		{"something-new", true, "inbound"},
	}
	for _, c := range cases {
		if got := group(rawPeer{ConnectionType: c.typ, Inbound: c.inbound}); got != c.want {
			t.Errorf("%q inbound=%v: got %q want %q", c.typ, c.inbound, got, c.want)
		}
	}
}

func TestHostOf(t *testing.T) {
	for in, want := range map[string]string{
		"1.0.0.1:8333":           "1.0.0.1",
		"[2001::1]:8333":         "2001::1",
		"abc.onion:8333":         "abc.onion",
		"2001::1":                "2001::1",
		"10.21.0.1:51234":        "10.21.0.1",
		"[::ffff:1.0.0.1]:40000": "::ffff:1.0.0.1",
	} {
		if got := hostOf(in); got != want {
			t.Errorf("%s: got %q want %q", in, got, want)
		}
	}
}

func TestLocate(t *testing.T) {
	for _, c := range []struct{ addr, network, cc, region string }{
		{"1.0.0.1:8333", "ipv4", "AU", "Queensland"},
		{"[2a01:4f8::1]:8333", "ipv6", "DE", "Bavaria"},
		{"[::ffff:1.0.0.1]:8333", "ipv4", "AU", "Queensland"},
		{"10.21.0.1:51234", "not_publicly_routable", "", ""},
		{"10.21.0.1:51234", "ipv4", "", ""}, // private even if Core labelled it ipv4
		{"127.0.0.1:50122", "onion", "", ""},
		{"x.b32.i2p:0", "i2p", "", ""},
		{"[fc00::1]:8333", "cjdns", "", ""},
	} {
		loc, ok := locate(rawPeer{Addr: c.addr, Network: c.network})
		if ok != (c.cc != "") || loc.Country != c.cc || loc.Region != c.region {
			t.Errorf("%s/%s: got %q/%q ok=%v want %q/%q", c.addr, c.network, loc.Country, loc.Region, ok, c.cc, c.region)
		}
	}
}

// A getpeerinfo reply shaped like Bitcoin Core's (fields trimmed to what a
// real node sends for these connection types).
const coreReply = `{"result":[
 {"id":0,"addr":"1.0.0.1:8333","network":"ipv4","connection_type":"manual","inbound":false,"subver":"/Satoshi:31.1.0/","pingtime":0.0421,"conntime":1757000000,"transport_protocol_type":"v2","bytessent":10,"bytesrecv":20},
 {"id":1,"addr":"[2a01:4f8::1]:48222","network":"ipv6","connection_type":"inbound","inbound":true,"subver":"/Satoshi:30.0.0/","conntime":1757000001,"transport_protocol_type":"v1","bytessent":1,"bytesrecv":2},
 {"id":2,"addr":"1.0.2.200:8333","network":"ipv4","connection_type":"block-relay-only","inbound":false,"subver":"","pingtime":0.1,"conntime":1757000002},
 {"id":3,"addr":"1.0.2.201:8333","network":"ipv4","connection_type":"feeler","inbound":false}
],"error":null,"id":"peermap"}`

func TestRPCAndConvert(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, _ := r.BasicAuth()
		gotAuth = u + ":" + p
		b := make([]byte, 512)
		n, _ := r.Body.Read(b)
		gotBody = string(b[:n])
		if u != "umbrel" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		io := strings.NewReader(coreReply)
		w.Header().Set("Content-Type", "application/json")
		io.WriteTo(w)
	}))
	defer srv.Close()

	c := &rpcClient{url: srv.URL + "/", user: "umbrel", pass: "secret", http: srv.Client()}
	raw, err := c.getPeerInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "umbrel:secret" || !strings.Contains(gotBody, `"method":"getpeerinfo"`) {
		t.Fatalf("auth %q body %q", gotAuth, gotBody)
	}
	peers := convert(raw)
	if len(peers) != 3 {
		t.Fatalf("want 3 peers (feeler dropped), got %d", len(peers))
	}
	want := []struct{ group, cc, region string }{{"manual", "AU", "Queensland"}, {"inbound", "DE", "Bavaria"}, {"outbound", "CN", "Fujian"}}
	for i, w := range want {
		p := peers[i]
		if p.Group != w.group || p.Country != w.cc || p.Region != w.region || p.RegionID == 0 || p.Lat == 0 {
			t.Errorf("peer %d: got %s/%s/%s rid=%d lat=%v want %s/%s/%s", i, p.Group, p.Country, p.Region, p.RegionID, p.Lat, w.group, w.cc, w.region)
		}
	}
	if peers[0].PingMs == nil || *peers[0].PingMs < 42 || *peers[0].PingMs > 42.2 {
		t.Errorf("ping not converted to ms: %v", peers[0].PingMs)
	}
	if peers[1].PingMs != nil {
		t.Errorf("missing pingtime must stay null")
	}

	bad := &rpcClient{url: srv.URL + "/", user: "wrong", pass: "x", http: srv.Client()}
	if _, err := bad.getPeerInfo(context.Background()); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("want 401 error, got %v", err)
	}
}

func TestRPCErrorObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"result":null,"error":{"code":-28,"message":"Loading block index..."},"id":"peermap"}`))
	}))
	defer srv.Close()
	c := &rpcClient{url: srv.URL + "/", http: srv.Client()}
	if _, err := c.getPeerInfo(context.Background()); err == nil || !strings.Contains(err.Error(), "Loading block index") {
		t.Errorf("got %v", err)
	}
}

func TestAtMostOneCallPerInterval(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	calls := 0
	fail := false
	s := &source{
		now: func() time.Time { return now },
		fetch: func(context.Context) ([]rawPeer, error) {
			calls++
			if fail {
				return nil, errors.New("node down")
			}
			return []rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "manual"}}, nil
		},
	}
	ctx := context.Background()
	for i := 0; i < 50; i++ { // many browsers inside one window
		s.get(ctx)
	}
	if calls != 1 {
		t.Fatalf("50 requests in one window made %d RPC calls", calls)
	}
	now = now.Add(interval - time.Second)
	if snap := s.get(ctx); calls != 1 || snap.NextIn != 1 {
		t.Fatalf("calls=%d next_in=%d", calls, snap.NextIn)
	}
	now = now.Add(time.Second)
	fail = true
	snap := s.get(ctx)
	if calls != 2 || snap.Error != "node down" || len(snap.Peers) != 1 {
		t.Fatalf("after failure: calls=%d err=%q peers=%d (last good list must stay)", calls, snap.Error, len(snap.Peers))
	}
	s.get(ctx) // failure does not shorten the window
	if calls != 2 {
		t.Fatalf("retried inside window after failure: calls=%d", calls)
	}
}

func TestHTTP(t *testing.T) {
	s := &source{now: time.Now, fetch: mockPeers}
	h := newHandler(s, offSibling())
	for _, path := range []string{"/", "/world.json", "/app.js", "/style.css", "/api/health"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 {
			t.Errorf("%s: HTTP %d", path, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/peers", nil))
	var snap snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	groups := map[string]int{}
	placed := 0
	for _, p := range snap.Peers {
		groups[p.Group]++
		if p.Type == "feeler" {
			t.Error("feeler leaked into output")
		}
		if p.Country != "" {
			placed++
		}
	}
	if groups["manual"] != 8 || groups["outbound"] != 12 || groups["inbound"] != 26 {
		t.Errorf("groups %v", groups)
	}
	if placed != 8+11+22 {
		t.Errorf("placed %d peers on the map, want 41", placed)
	}
	for _, p := range snap.Peers {
		h := hostOf(p.Addr)
		if p.Network == "ipv4" && !(strings.HasPrefix(h, "192.0.2.") || strings.HasPrefix(h, "198.51.100.") || strings.HasPrefix(h, "203.0.113.")) {
			t.Errorf("demo IPv4 %s is not a documentation address", h)
		}
		if p.Network == "ipv6" && !strings.HasPrefix(h, "2001:db8:") {
			t.Errorf("demo IPv6 %s is not a documentation address", h)
		}
	}
}
