package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSiblingInstalled(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := &siblingCheck{url: srv.URL, client: srv.Client(), every: time.Minute}
	if !s.installed(context.Background()) {
		t.Fatal("a neighbour that answers 200 must count as installed")
	}
	// Second call inside the window must not ask again: a dashboard polls every
	// ten seconds and an app is not installed twice a minute.
	s.installed(context.Background())
	if calls != 1 {
		t.Errorf("asked %d times, want 1 - the answer is cached", calls)
	}
}

func TestSiblingMissingIsSilent(t *testing.T) {
	// A port nobody listens on: the normal case for someone who installed one
	// app and not the other.
	s := &siblingCheck{url: "http://127.0.0.1:1/api/health", client: &http.Client{Timeout: time.Second}, every: time.Minute}
	if s.installed(context.Background()) {
		t.Error("an unreachable neighbour must not produce a link")
	}
}

func TestSiblingOff(t *testing.T) {
	if offSibling().installed(context.Background()) {
		t.Error(`url "off" must switch the check off entirely`)
	}
}

func TestSiblingLatestBlock(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	detected := now.Add(-30 * time.Second).UnixMilli()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/api/blocks/latest" {
			t.Errorf("asked for %q", r.URL.Path)
		}
		calls++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"hash":"00beef","height":967724,"detectedAt":%d,"pool":"Foundry",
			"poolName":"Foundry USA","poolTag":"Foundry USA Pool","poolSource":"address",
			"firstPeers":["1.2.3.4:8333"],"eligible":204,
			"stratum":{"createdAt":%d,"entries":[
				{"label":"Public A","own":false,"latencyMs":0,"rank":1,"miss":false},
				{"label":"My pool","own":true,"latencyMs":412.5,"rank":2,"miss":false},
				{"label":"Quiet one","own":false,"latencyMs":null,"rank":null,"miss":true}]},
			"templateMs":84.5,"firstPingMs":98,"deliveredEver":["1.2.3.4:8333","5.6.7.8:8333"],
			"routeMedian":{"blocks":100,"core":{"ms":210,"n":100},"peer":{"ms":120,"n":97},
				"template":{"ms":295,"n":60},"own":{"ms":420,"n":99},"ownLabel":"My pool"}}`, detected, detected-225)
	}))
	defer srv.Close()

	s := &siblingCheck{
		url:      srv.URL + "/api/health",
		blockURL: srv.URL + "/api/blocks/latest",
		client:   srv.Client(),
		every:    time.Minute,
		now:      func() time.Time { return now },
	}

	got := s.latest(context.Background())
	if got == nil {
		t.Fatal("no block")
	}
	if got.Height != 967724 || got.Pool != "Foundry" || got.PoolName != "Foundry USA" {
		t.Errorf("got %+v", got)
	}
	if len(got.FirstPeers) != 1 || got.FirstPeers[0] != "1.2.3.4:8333" {
		t.Errorf("first peers: %v", got.FirstPeers)
	}
	// The age is what the browser marks by, so it has to be the block's real
	// age and not the age of this reply.
	if got.AgeMs != 30_000 {
		t.Errorf("age %d ms, want 30000", got.AgeMs)
	}
	if got.MarkForMs != blockMarkFor.Milliseconds() {
		t.Errorf("mark window %d ms", got.MarkForMs)
	}
	// The three roles arrive separately and stay separate: mined, delivered,
	// and turned into work.
	if got.Eligible != 204 {
		t.Errorf("eligible %d, want 204", got.Eligible)
	}
	if got.Stratum == nil || len(got.Stratum.Entries) != 3 {
		t.Fatalf("stratum race not carried through: %+v", got.Stratum)
	}
	if !got.Stratum.Entries[1].Own {
		t.Error("the owner's own pool must stay marked as his")
	}
	if got.Stratum.Entries[2].LatencyMs != nil || !got.Stratum.Entries[2].Miss {
		t.Error("a pool that reported nothing must stay in the list as a miss")
	}
	// The first job came 225 ms before Core announced the block, so on the
	// route Core stands 225 ms after the zero.
	if got.Stratum.CoreMs == nil || *got.Stratum.CoreMs != 225 {
		t.Errorf("core_ms = %v, want 225", got.Stratum.CoreMs)
	}
	// The route's inner stops and the typical route arrive intact.
	if got.TemplateMs == nil || *got.TemplateMs != 84.5 || got.FirstPingMs == nil || *got.FirstPingMs != 98 {
		t.Errorf("template %v, ping %v", got.TemplateMs, got.FirstPingMs)
	}
	if len(got.DeliveredEver) != 2 || got.DeliveredEver[1] != "5.6.7.8:8333" {
		t.Errorf("delivered ever: %v", got.DeliveredEver)
	}
	m := got.RouteMedian
	if m == nil || m.Blocks != 100 || m.Template == nil || m.Template.Ms != 295 || m.Template.N != 60 || m.OwnLabel != "My pool" {
		t.Errorf("route median %+v", m)
	}

	// Inside the poll interval the answer is cached - but it must age while it
	// sits there, or a cached copy would claim the block is younger than it is.
	now = now.Add(5 * time.Second)
	again := s.latest(context.Background())
	if calls != 1 {
		t.Errorf("asked the neighbour %d times, want 1", calls)
	}
	if again.AgeMs != 35_000 {
		t.Errorf("cached age %d ms, want 35000", again.AgeMs)
	}
}

func TestSiblingLatestWithoutNeighbour(t *testing.T) {
	if offSibling().latest(context.Background()) != nil {
		t.Error(`"off" must not produce a block`)
	}
	s := &siblingCheck{
		url:      "http://127.0.0.1:1/api/health",
		blockURL: "http://127.0.0.1:1/api/blocks/latest",
		client:   &http.Client{Timeout: time.Second},
		every:    time.Minute,
		now:      time.Now,
	}
	if s.latest(context.Background()) != nil {
		t.Error("an unreachable neighbour must not produce a block")
	}
}

// Where Core stands on the route, counted from the first job. Negative is a
// real answer - Core can have the block before any pool sends a job - and a
// race with no start instant has no zero to count from at all.
func TestSiblingCoreOffset(t *testing.T) {
	for _, tc := range []struct {
		name      string
		createdAt string
		want      *int64
	}{
		{"core after the first job", "1789900000000", ptr(int64(300))},
		{"core before any job", "1789900000500", ptr(int64(-200))},
		{"no start instant", "0", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/health" {
					w.WriteHeader(http.StatusOK)
					return
				}
				fmt.Fprintf(w, `{"hash":"00beef","height":1,"detectedAt":1789900000300,
					"stratum":{"createdAt":%s,"entries":[{"label":"A","latencyMs":0,"rank":1}]}}`, tc.createdAt)
			}))
			defer srv.Close()
			s := &siblingCheck{
				url: srv.URL + "/api/health", blockURL: srv.URL + "/api/blocks/latest",
				client: srv.Client(), every: time.Minute,
				now: func() time.Time { return time.UnixMilli(1789900010000) },
			}
			got := s.latest(context.Background())
			if got == nil || got.Stratum == nil {
				t.Fatal("no block or no race")
			}
			switch {
			case tc.want == nil && got.Stratum.CoreMs != nil:
				t.Errorf("core_ms = %d, want none", *got.Stratum.CoreMs)
			case tc.want != nil && (got.Stratum.CoreMs == nil || *got.Stratum.CoreMs != *tc.want):
				t.Errorf("core_ms = %v, want %d", got.Stratum.CoreMs, *tc.want)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }
