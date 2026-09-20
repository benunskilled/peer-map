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
				{"label":"Quiet one","own":false,"latencyMs":null,"rank":null,"miss":true}]}}`, detected, detected)
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
