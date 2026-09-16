// Peer Map shows the peers of a Bitcoin Core node on a world map, split into
// manual, inbound and outbound connections.
//
// It is built to cost nothing while nobody is looking: there is no background
// loop. Core is asked for getpeerinfo only when the dashboard asks for data,
// and never more than once per interval (10s) no matter how many browsers are
// open - every request inside that window gets the cached answer.
package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/benunskilled/peer-map/geo"
)

// Version is set at build time with -ldflags "-X main.Version=...".
var Version = "dev"

const interval = 10 * time.Second

//go:embed web
var webFS embed.FS

type config struct {
	port    string
	mock    bool
	rpcURL  string
	rpcUser string
	rpcPass string
}

func loadConfig() (config, error) {
	c := config{port: env("PEERMAP_PORT", "8789"), mock: os.Getenv("PEERMAP_MOCK") == "1"}
	if p, err := strconv.Atoi(c.port); err != nil || p < 1 || p > 65535 {
		return c, fmt.Errorf("PEERMAP_PORT=%q is not a port number", c.port)
	}
	if c.mock {
		return c, nil
	}
	var missing []string
	get := func(k string) string {
		v := os.Getenv(k)
		if v == "" {
			missing = append(missing, k)
		}
		return v
	}
	host, port := get("APP_BITCOIN_NODE_IP"), get("APP_BITCOIN_RPC_PORT")
	c.rpcUser, c.rpcPass = get("APP_BITCOIN_RPC_USER"), get("APP_BITCOIN_RPC_PASS")
	if len(missing) > 0 {
		return c, fmt.Errorf("missing environment variable(s) %v (set PEERMAP_MOCK=1 to run with demo data)", missing)
	}
	if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
		return c, fmt.Errorf("APP_BITCOIN_RPC_PORT=%q is not a port number", port)
	}
	c.rpcURL = "http://" + net.JoinHostPort(host, port) + "/"
	return c, nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// ---- Bitcoin Core RPC ------------------------------------------------------

type rpcClient struct {
	url, user, pass string
	http            *http.Client
}

func (c *rpcClient) getPeerInfo(ctx context.Context) ([]rawPeer, error) {
	body := []byte(`{"jsonrpc":"1.0","id":"peermap","method":"getpeerinfo","params":[]}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.user, c.pass)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach Bitcoin Core: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("Bitcoin Core rejected the RPC credentials (401)")
	}
	var out struct {
		Result []rawPeer `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	// 200 peers are roughly 250 KB of JSON; 32 MB is a guard, not a budget.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("unreadable RPC reply (HTTP %d): %w", resp.StatusCode, err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("Bitcoin Core RPC error %d: %s", out.Error.Code, out.Error.Message)
	}
	return out.Result, nil
}

// ---- cache: at most one RPC call per interval ------------------------------

type snapshot struct {
	Peers     []Peer `json:"peers"`
	FetchedAt int64  `json:"fetched_at"`
	NextIn    int    `json:"next_in"`
	Interval  int    `json:"interval"`
	Error     string `json:"error,omitempty"`
	Demo      bool   `json:"demo,omitempty"`
	Version   string `json:"version"`
}

type source struct {
	mu    sync.Mutex
	fetch func(context.Context) ([]rawPeer, error)
	now   func() time.Time
	last  time.Time
	snap  snapshot
	calls int // RPC calls made, for tests and the log
}

func (s *source) get(ctx context.Context) snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if s.last.IsZero() || now.Sub(s.last) >= interval {
		// The window starts before the call, and failures count too: a node
		// that is down is not asked again until the interval has passed.
		s.last = now
		s.calls++
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		raw, err := s.fetch(cctx)
		cancel()
		if err != nil {
			if s.snap.Error != err.Error() {
				log.Printf("getpeerinfo failed: %v", err)
			}
			s.snap.Error = err.Error() // keep the last good peer list on screen
		} else {
			s.snap.Peers = convert(raw)
			s.snap.FetchedAt = now.Unix()
			s.snap.Error = ""
		}
	}
	out := s.snap
	out.Interval = int(interval / time.Second)
	out.NextIn = int((interval - now.Sub(s.last) + time.Second - 1) / time.Second)
	if out.NextIn < 1 {
		out.NextIn = 1
	}
	if out.Peers == nil {
		out.Peers = []Peer{}
	}
	return out
}

// ---- HTTP -------------------------------------------------------------------

func newHandler(src *source) http.Handler {
	static, _ := fs.Sub(webFS, "web")
	files := http.FileServer(http.FS(static))
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("GET /api/peers", func(w http.ResponseWriter, r *http.Request) {
		snap := src.get(r.Context())
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(snap)
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		// no-cache for everything, world.json included: it changes between
		// releases, and a day-long cache kept showing the old map after an update.
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'self'")
		mux.ServeHTTP(w, r)
	})
}

func healthcheck() int {
	c := http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get("http://127.0.0.1:" + env("PEERMAP_PORT", "8789") + "/api/health")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			os.Exit(healthcheck())
		case "version", "-version", "--version":
			fmt.Println(Version)
			return
		default:
			fmt.Fprintf(os.Stderr, "usage: %s [healthcheck|version]\n", os.Args[0])
			os.Exit(2)
		}
	}
	log.SetFlags(0)

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("peer-map: %v", err)
	}

	src := &source{now: time.Now}
	if cfg.mock {
		src.fetch = mockPeers
		src.snap.Demo = true
	} else {
		rpc := &rpcClient{url: cfg.rpcURL, user: cfg.rpcUser, pass: cfg.rpcPass, http: &http.Client{}}
		src.fetch = rpc.getPeerInfo
	}
	src.snap.Version = Version

	srv := &http.Server{
		Addr:              ":" + cfg.port,
		Handler:           newHandler(src),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	regions, v4, v6 := geo.Sizes()
	mode := "rpc " + cfg.rpcURL
	if cfg.mock {
		mode = "DEMO DATA (PEERMAP_MOCK=1)"
	}
	log.Printf("peer-map %s listening on :%s, %s, geo table %d regions, %d/%d ranges (v4/v6), Core polled at most every %s and only while the dashboard is open",
		Version, cfg.port, mode, regions, v4, v6, interval)

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("peer-map: %v", err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}
