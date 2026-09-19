package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
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

// How long a delivered block is worth pointing at. Long enough to notice on a
// dashboard somebody just opened, short enough that a star never turns into
// decoration: after two minutes the block is history and the map goes back to
// showing connections.
const blockMarkFor = 2 * time.Minute

type siblingCheck struct {
	url      string
	blockURL string
	client   *http.Client
	every    time.Duration
	now      func() time.Time

	mu        sync.Mutex
	checkedAt time.Time
	present   bool

	blockAt         time.Time
	blockDetectedAt int64
	block           *lastBlock
}

// What the neighbour says about the newest block. Its own fields are the
// short pool name, the full one, and the addresses Bitcoin Lab credited with
// delivering it - the same strings Core reports here, ports and all, so they
// match this app's peers without any guessing.
type lastBlock struct {
	Height     int64    `json:"height,omitempty"`
	Pool       string   `json:"pool,omitempty"`
	PoolName   string   `json:"pool_name,omitempty"`
	PoolTag    string   `json:"pool_tag,omitempty"`
	PoolSource string   `json:"pool_source,omitempty"`
	FirstPeers []string `json:"first_peers,omitempty"`
	// Age at the moment this snapshot was built, from the clock both apps
	// share - the browser adds however long its copy has been sitting there
	// rather than comparing two clocks that may disagree.
	AgeMs int64 `json:"age_ms"`
	// How long a browser should keep the mark, so the rule lives in one place.
	MarkForMs int64 `json:"mark_for_ms"`
}

func newSiblingCheck() *siblingCheck {
	url := os.Getenv("PEERMAP_SIBLING_HEALTH_URL")
	if url == "" {
		url = siblingDefault
	}
	return &siblingCheck{
		url:      url,
		blockURL: strings.TrimSuffix(url, "/api/health") + "/api/blocks/latest",
		now:      time.Now,
		client:   &http.Client{Timeout: 2 * time.Second},
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

// latest returns the newest block the neighbour knows about, or nil when
// there is no neighbour, it did not answer, or it has not seen a block yet.
//
// Asked at most once per poll interval and never on the page's critical path:
// a slow or missing Bitcoin Lab costs this app nothing but a missing star.
func (s *siblingCheck) latest(ctx context.Context) *lastBlock {
	if s.url == "off" || !s.installed(ctx) {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if !s.blockAt.IsZero() && now.Sub(s.blockAt) < interval {
		return s.withAge(s.block, now)
	}
	s.blockAt = now

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.blockURL, nil)
	if err != nil {
		s.block = nil
		return nil
	}
	resp, err := s.client.Do(req)
	if err != nil {
		s.block = nil
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		s.block = nil
		return nil
	}
	// Bitcoin Lab's own shape, which is not this app's: decoded here and
	// handed on in this app's vocabulary.
	var got struct {
		Height     int64    `json:"height"`
		DetectedAt int64    `json:"detectedAt"`
		Pool       string   `json:"pool"`
		PoolName   string   `json:"poolName"`
		PoolTag    string   `json:"poolTag"`
		PoolSource string   `json:"poolSource"`
		FirstPeers []string `json:"firstPeers"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&got); err != nil || got.DetectedAt == 0 {
		s.block = nil
		return nil
	}
	age := now.UnixMilli() - got.DetectedAt
	if age < 0 {
		age = 0
	}
	s.block = &lastBlock{
		Height:     got.Height,
		Pool:       got.Pool,
		PoolName:   got.PoolName,
		PoolTag:    got.PoolTag,
		PoolSource: got.PoolSource,
		FirstPeers: got.FirstPeers,
		AgeMs:      age,
		MarkForMs:  blockMarkFor.Milliseconds(),
	}
	// Keep the arrival instant rather than the age, so a cached copy handed
	// out nine seconds later does not claim to be nine seconds younger.
	s.blockDetectedAt = got.DetectedAt
	return s.block
}

func (s *siblingCheck) withAge(b *lastBlock, now time.Time) *lastBlock {
	if b == nil {
		return nil
	}
	out := *b
	out.AgeMs = now.UnixMilli() - s.blockDetectedAt
	if out.AgeMs < 0 {
		out.AgeMs = 0
	}
	return &out
}

// offSibling is the check a test wants: no neighbour, no request, no timeout.
func offSibling() *siblingCheck {
	return &siblingCheck{url: "off", now: time.Now}
}
