package main

import (
	"context"
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
