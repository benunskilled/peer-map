package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
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

// Same exclusions as locate, and one case the two answer differently: geo puts
// 1.0.2.200 in China, no network announces it. Where a peer is and whose
// machine it is really are two lookups.
func TestOperator(t *testing.T) {
	for _, c := range []struct {
		addr, network string
		asn           uint32
	}{
		{"1.0.0.1:8333", "ipv4", 13335},       // Cloudflare
		{"[2a01:4f8::1]:8333", "ipv6", 24940}, // Hetzner
		{"[::ffff:1.0.0.1]:8333", "ipv4", 13335},
		{"1.0.2.200:8333", "ipv4", 0}, // located, but announced by nobody
		{"10.21.0.1:51234", "not_publicly_routable", 0},
		{"10.21.0.1:51234", "ipv4", 0},
		{"127.0.0.1:50122", "onion", 0},
		{"x.b32.i2p:0", "i2p", 0},
		{"[fc00::1]:8333", "cjdns", 0},
	} {
		net, ok := operator(rawPeer{Addr: c.addr, Network: c.network})
		if ok != (c.asn != 0) || net.Number != c.asn {
			t.Errorf("%s/%s: got AS%d (%q, ok=%v) want AS%d", c.addr, c.network, net.Number, net.Name, ok, c.asn)
		}
		if ok && net.Name == "" {
			t.Errorf("%s: operator name missing", c.addr)
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
	// What the failure says is TestRPCFailureStaysOutOfTheSnapshot's business;
	// here it only has to say something and leave the peers alone.
	if calls != 2 || snap.Error == "" || len(snap.Peers) != 1 {
		t.Fatalf("after failure: calls=%d err=%q peers=%d (last good list must stay)", calls, snap.Error, len(snap.Peers))
	}
	s.get(ctx) // failure does not shorten the window
	if calls != 2 {
		t.Fatalf("retried inside window after failure: calls=%d", calls)
	}
}

// A browser that goes away mid-poll must not take the RPC with it: the call
// belongs to the app, and its answer is what every other open dashboard reads
// for the rest of the window.
func TestTheRPCOutlivesTheBrowserRequest(t *testing.T) {
	var gotErr error
	var hadDeadline bool
	s := &source{
		now: time.Now,
		fetch: func(ctx context.Context) ([]rawPeer, error) {
			gotErr = ctx.Err()
			_, hadDeadline = ctx.Deadline()
			return []rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "manual"}}, nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the dashboard is gone before Core is even asked

	snap := s.get(ctx)
	if gotErr != nil {
		t.Fatalf("the RPC was handed the dead browser context: %v", gotErr)
	}
	if !hadDeadline {
		t.Error("detaching the request must not lose the 20s timeout")
	}
	if snap.Error != "" || len(snap.Peers) != 1 {
		t.Fatalf("err=%q peers=%d - the answer must still reach the snapshot", snap.Error, len(snap.Peers))
	}
}

// An RPC that was cut off learned nothing about the node, so it must not hold
// the window: the next poll asks again instead of serving ten seconds of an
// error nobody's node produced.
func TestAnAbortedRPCDoesNotOpenAWindow(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	calls, cut := 0, true
	s := &source{
		now: func() time.Time { return now },
		fetch: func(context.Context) ([]rawPeer, error) {
			calls++
			if cut {
				return nil, context.Canceled
			}
			return []rawPeer{{Addr: "1.0.0.1:8333", ConnectionType: "manual"}}, nil
		},
	}
	if snap := s.get(context.Background()); snap.Error != "" {
		t.Errorf("a cancelled call was served as Core's answer: %q", snap.Error)
	}
	cut = false
	snap := s.get(context.Background()) // same instant: no waiting for the window
	if calls != 2 {
		t.Fatalf("made %d calls, want 2 - the window stayed open", calls)
	}
	if len(snap.Peers) != 1 {
		t.Errorf("peers=%d, want the list from the call that did go through", len(snap.Peers))
	}
}

// Go's *url.Error prints the address it failed to reach, which here is the
// node's host and port. The browser gets a line it can act on; the address
// stays in the log.
func TestRPCFailureStaysOutOfTheSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL + "/"
	srv.Close() // nothing listens there any more
	addr := strings.TrimPrefix(srv.URL, "http://")

	var logged bytes.Buffer
	log.SetOutput(&logged)
	defer log.SetOutput(os.Stderr)

	c := &rpcClient{url: url, http: &http.Client{}}
	s := &source{now: time.Now, fetch: c.getPeerInfo}
	snap := s.get(context.Background())

	if snap.Error == "" {
		t.Fatal("a node that is not there must still say so on the dashboard")
	}
	if strings.Contains(snap.Error, addr) || strings.Contains(snap.Error, "http") {
		t.Errorf("the snapshot names the node: %q", snap.Error)
	}
	if !strings.Contains(logged.String(), addr) {
		t.Errorf("the detail never reached the log: %q", logged.String())
	}
}

// Only lines this app wrote reach the browser; anything else is answered
// vaguely rather than passed through.
func TestShownText(t *testing.T) {
	refused := errors.New("dial tcp 10.21.0.1:8332: connect: connection refused")
	for _, c := range []struct {
		name string
		err  error
		want string
	}{
		{"a line with nothing behind it", &shownError{line: "rejected the RPC credentials (401)"}, "rejected the RPC credentials (401)"},
		{"a line in front of a detail", &shownError{line: "unreachable from this app", err: refused}, "unreachable from this app"},
		{"wrapped further up", fmt.Errorf("getpeerinfo: %w", &shownError{line: "unreachable from this app", err: refused}), "unreachable from this app"},
		{"an error that never passed through here", refused, "could not be asked for its peers"},
	} {
		if got := shownText(c.err); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

// default-src is not a fallback for base-uri or form-action: without them an
// injected <base> could re-point every relative URL on the page, and an
// injected form could post off-site. The page has neither, so both are 'none'.
func TestCSPClosesBaseAndForm(t *testing.T) {
	h := newHandler(&source{now: time.Now, fetch: mockPeers}, offSibling())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 {
		t.Fatalf("the page no longer loads: HTTP %d", rec.Code)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'self'", "script-src 'self'", "base-uri 'none'", "form-action 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP %q is missing %q", csp, want)
		}
	}
	page, err := webFS.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"<base", "<form"} {
		if strings.Contains(string(page), tag) {
			t.Errorf("the page now has a %s, which 'none' forbids", tag)
		}
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
