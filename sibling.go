package main

import (
	"context"
	"net/http"
	"os"
	"sync"
	"time"
)

// The other half of the pair is Bitcoin Lab: a separate app, installed on its
// own, which measures which of these peers actually delivers each block first.
// When it is installed on the same node, this app offers a link to it - both
// for the dashboard as a whole and for a single peer.
//
// Whether it is there is a question for this process, not for the browser. The
// dashboard runs under a Content-Security-Policy with connect-src 'self', which
// is worth keeping, and a browser could only ever find out whether something
// answers on a port. From inside the umbrel network the question is the real
// one: does the container exist. It is reachable by name, exactly like Bitcoin
// Core is.
//
// A miss is the normal case - most people will install one app and not the
// other - so a failure is silent and simply means no link.
const siblingDefault = "http://bitcoinlab-node_dashboard_1:8788/api/health"

type siblingCheck struct {
	url    string
	client *http.Client
	every  time.Duration

	mu        sync.Mutex
	checkedAt time.Time
	present   bool
}

func newSiblingCheck() *siblingCheck {
	url := os.Getenv("PEERMAP_SIBLING_HEALTH_URL")
	if url == "" {
		url = siblingDefault
	}
	return &siblingCheck{
		url:    url,
		client: &http.Client{Timeout: 2 * time.Second},
		// An app is installed or removed by hand, minutes apart at the very
		// fastest. Asking once every five minutes is already generous, and it
		// keeps a missing neighbour from costing a request per dashboard poll.
		every: 5 * time.Minute,
	}
}

// present reports whether Bitcoin Lab answered recently. Never blocks longer
// than the client timeout, and only that rarely.
func (s *siblingCheck) installed(ctx context.Context) bool {
	if s.url == "off" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.checkedAt.IsZero() && time.Since(s.checkedAt) < s.every {
		return s.present
	}
	s.checkedAt = time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		s.present = false
		return false
	}
	resp, err := s.client.Do(req)
	if err != nil {
		s.present = false
		return false
	}
	defer resp.Body.Close()
	s.present = resp.StatusCode == http.StatusOK
	return s.present
}

// offSibling is the check a test wants: no neighbour, no request, no timeout.
func offSibling() *siblingCheck {
	return &siblingCheck{url: "off"}
}
