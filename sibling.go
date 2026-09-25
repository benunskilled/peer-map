package main

import (
	"context"
	"encoding/json"
	"errors"
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

type stratumRace struct {
	CreatedAt int64          `json:"created_at,omitempty"`
	Entries   []stratumEntry `json:"entries,omitempty"`
	// When this node's Core had the block, counted from the first new job any
	// pool sent here - the race's zero. Both instants come from Bitcoin Lab,
	// taken on one machine by one clock: the ZMQ announcement and the first
	// notify. Negative when Core had the block before any pool sent a job,
	// which does happen. A pointer because zero is a real answer.
	//
	// When the block was found cannot be known - the header's time is set by
	// the miner, in whole seconds - which is exactly why the first job is the
	// zero: it is the earliest moment the block is visible from here.
	CoreMs *int64 `json:"core_ms,omitempty"`
}

// One stop of the typical route: its median and over how many blocks.
type medianStop struct {
	Ms float64 `json:"ms"`
	N  int     `json:"n"`
}

type routeMedian struct {
	Blocks   int         `json:"blocks"`
	Core     *medianStop `json:"core,omitempty"`
	Peer     *medianStop `json:"peer,omitempty"`
	Template *medianStop `json:"template,omitempty"`
	Own      *medianStop `json:"own,omitempty"`
	OwnLabel string      `json:"own_label,omitempty"`
	// Transactions per template, the median over the same blocks.
	Tx *medianStop `json:"tx,omitempty"`
}

type stratumEntry struct {
	Label     string   `json:"label"`
	Own       bool     `json:"own,omitempty"` // a pool the owner added, not one of the public ones
	LatencyMs *float64 `json:"latency_ms"`    // null means it sent nothing in time
	Rank      *int     `json:"rank"`
	Miss      bool     `json:"miss,omitempty"`
}

// What the neighbour says about the newest block. Its own fields are the
// short pool name, the full one, and the addresses Bitcoin Lab credited with
// delivering it - the same strings Core reports here, ports and all, so they
// match this app's peers without any guessing.
type lastBlock struct {
	Hash       string   `json:"hash,omitempty"`
	Height     int64    `json:"height,omitempty"`
	Pool       string   `json:"pool,omitempty"`
	PoolName   string   `json:"pool_name,omitempty"`
	PoolTag    string   `json:"pool_tag,omitempty"`
	PoolSource string   `json:"pool_source,omitempty"`
	FirstPeers []string `json:"first_peers,omitempty"`
	// How many peers were connected when it arrived - the number the one that
	// delivered it was up against.
	Eligible int `json:"eligible,omitempty"`
	// The same block from the mining side: which pool turned it into fresh
	// work first, and how far behind the others were. Absent when Bitcoin Lab
	// recorded no race for it, which is the normal case with Stratum Race off.
	Stratum *stratumRace `json:"stratum,omitempty"`
	// The route's inner stops, measured by Bitcoin Lab at the moment the
	// block arrived: how long after the announcement Core had a new block
	// template ready, and the delivering peer's lowest ping from the snapshot
	// that credited it. Absent on blocks recorded before the Lab measured them.
	TemplateMs  *float64 `json:"template_ms,omitempty"`
	FirstPingMs *float64 `json:"first_ping_ms,omitempty"`
	// How many transactions that template carried: a pool spends time on
	// every one, so this is why one block's job is slower than the next.
	TemplateTx *int `json:"template_tx,omitempty"`
	// The same route for the typical block, the median over the last hundred.
	RouteMedian *routeMedian `json:"route_median,omitempty"`
	// Every address Bitcoin Lab has ever credited with delivering a block
	// first. The tables tint those rows.
	DeliveredEver []string `json:"delivered_ever,omitempty"`
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
	// The probe is this app's, not the browser's: tied to the incoming request
	// it was cancelled whenever a dashboard went away mid-poll, and that
	// cancellation was then remembered as "no neighbour" for the next five
	// minutes - the link vanished for everyone. The lock is held across the
	// call, so pollers arriving meanwhile still share this one probe.
	req, err := http.NewRequestWithContext(context.WithoutCancel(ctx), http.MethodGet, s.url, nil)
	if err != nil {
		s.checkedAt, s.present = time.Now(), false
		return false
	}
	resp, err := s.client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// Nothing was learned about the neighbour, so nothing is
			// remembered: the next poll probes again.
			return false
		}
		// A neighbour that is not there is the normal case and is an answer:
		// remembered, or a missing app costs a request per dashboard poll.
		s.checkedAt, s.present = time.Now(), false
		return false
	}
	defer resp.Body.Close()
	s.checkedAt, s.present = time.Now(), resp.StatusCode == http.StatusOK
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
	// A neighbour that answers badly has answered: no block for this window,
	// so a broken Bitcoin Lab costs one request per interval and no more.
	miss := func() *lastBlock {
		s.blockAt, s.block = now, nil
		return nil
	}
	// Detached from the browser request for the same reason as the probe
	// above: a dashboard that goes away must not blank the block card for
	// every other dashboard until the window runs out.
	req, err := http.NewRequestWithContext(context.WithoutCancel(ctx), http.MethodGet, s.blockURL, nil)
	if err != nil {
		return miss()
	}
	resp, err := s.client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil // nothing learned; the next poll asks again
		}
		return miss()
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return miss()
	}
	// Bitcoin Lab's own shape, which is not this app's: decoded here and
	// handed on in this app's vocabulary.
	var got struct {
		Hash       string   `json:"hash"`
		Height     int64    `json:"height"`
		DetectedAt int64    `json:"detectedAt"`
		Pool       string   `json:"pool"`
		PoolName   string   `json:"poolName"`
		PoolTag    string   `json:"poolTag"`
		PoolSource string   `json:"poolSource"`
		FirstPeers []string `json:"firstPeers"`
		Eligible   int      `json:"eligible"`
		Stratum    *struct {
			CreatedAt int64 `json:"createdAt"`
			Entries   []struct {
				Label     string   `json:"label"`
				Own       bool     `json:"own"`
				LatencyMs *float64 `json:"latencyMs"`
				Rank      *int     `json:"rank"`
				Miss      bool     `json:"miss"`
			} `json:"entries"`
		} `json:"stratum"`
		TemplateMs    *float64 `json:"templateMs"`
		TemplateTx    *int     `json:"templateTx"`
		FirstPingMs   *float64 `json:"firstPingMs"`
		DeliveredEver []string `json:"deliveredEver"`
		RouteMedian   *struct {
			Blocks   int         `json:"blocks"`
			Core     *medianStop `json:"core"`
			Peer     *medianStop `json:"peer"`
			Template *medianStop `json:"template"`
			Own      *medianStop `json:"own"`
			OwnLabel string      `json:"ownLabel"`
			Tx       *medianStop `json:"tx"`
		} `json:"routeMedian"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&got); err != nil || got.DetectedAt == 0 {
		return miss()
	}
	age := now.UnixMilli() - got.DetectedAt
	if age < 0 {
		age = 0
	}
	var race *stratumRace
	if got.Stratum != nil {
		race = &stratumRace{CreatedAt: got.Stratum.CreatedAt}
		if got.Stratum.CreatedAt > 0 {
			ms := got.DetectedAt - got.Stratum.CreatedAt
			race.CoreMs = &ms
		}
		for _, e := range got.Stratum.Entries {
			race.Entries = append(race.Entries, stratumEntry{
				Label: e.Label, Own: e.Own, LatencyMs: e.LatencyMs, Rank: e.Rank, Miss: e.Miss,
			})
		}
	}
	var med *routeMedian
	if m := got.RouteMedian; m != nil && m.Blocks > 0 {
		med = &routeMedian{Blocks: m.Blocks, Core: m.Core, Peer: m.Peer, Template: m.Template, Own: m.Own, OwnLabel: m.OwnLabel, Tx: m.Tx}
	}
	s.blockAt = now
	s.block = &lastBlock{
		Hash:          got.Hash,
		Height:        got.Height,
		Pool:          got.Pool,
		PoolName:      got.PoolName,
		PoolTag:       got.PoolTag,
		PoolSource:    got.PoolSource,
		FirstPeers:    got.FirstPeers,
		Eligible:      got.Eligible,
		Stratum:       race,
		TemplateMs:    got.TemplateMs,
		TemplateTx:    got.TemplateTx,
		FirstPingMs:   got.FirstPingMs,
		RouteMedian:   med,
		DeliveredEver: got.DeliveredEver,
		AgeMs:         age,
		MarkForMs:     blockMarkFor.Milliseconds(),
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
