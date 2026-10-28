package main

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Every reason a peer is called disguised has its few words for the list:
// the demo data has a peer for each rule, and each one must say why under
// its pill.
func TestEveryDisguiseHasATell(t *testing.T) {
	raw, err := mockPeers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, p := range convertAt(raw, time.Now()) {
		if p.Kind != fakeWalletKind {
			if p.Tell != "" && p.Kind != "Other" && p.Kind != noAgentKind {
				t.Errorf("%s is %s but tells %q", p.Subver, p.Kind, p.Tell)
			}
			continue
		}
		seen++
		if p.Tell == "" {
			t.Errorf("%s: no short reason for %q", p.Subver, p.About)
		}
	}
	if seen == 0 {
		t.Fatal("the demo data has no disguised peer")
	}
}

func TestShortReasons(t *testing.T) {
	for _, c := range []struct{ about, want string }{
		{"Claims: Bitcoin Core 25.1.0\nProbably: " + skippedHandshakeAbout(map[string]int64{"sendaddrv2": 1}), "no wtxidrelay"},
		{"Claims: Bitcoin Core 28.1.0\nProbably: " + offersNothingAbout + "\nAlso: it left out wtxidrelay", "offers no services"},
		{"Claims: Bitcoin Core 31.1.0\nProbably: " + anachronism("/Satoshi:31.1.0/", []string{"NETWORK", "WITNESS"}), "no NETWORK_LIMITED"},
		{"Claims: Bitcoin Core 25.1.0\nProbably: " + protocolMismatch("/Satoshi:25.1.0/", 70015), "protocol 70015 does not fit"},
		{"Claims: x\nProbably: a crawler - it asked for addresses 18 times, Core asks once", "asks for addresses 18×"},
		{"Claims: x\nProbably: not Bitcoin Core - it pings every 2 seconds, Core every 2 minutes", "pings every 2 s"},
		{"Claims: x\nProbably: " + bitcoinjTooNewAbout, "newer protocol than the wallet"},
		{"Claims: x\nProbably: " + anachronism("/Satoshi:0.15.0/Knots:20170914/", []string{"NETWORK", "P2P_V2"}), "offers v2 its version lacked"},
		{fakeAbout("/Sat0shi:25.0.0/", "", nil), "misspelt name"},
		{fakeAbout("Satoshi:25.0.0", "", nil), "misspelt name"},
		{fakeAbout("/Satoshi9:25.0.0/", "", nil), "misspelt name"},
	} {
		if got := tellOf(c.about, false); got != c.want {
			t.Errorf("tellOf(%q) = %q, want %q", c.about, got, c.want)
		}
	}
	lion := "Claims: bitcoinj 0.14.5\nProbably: " + linkingLionAbout + "\nAlso: it offers blocks, which no wallet does"
	if got := tellOf(lion, true); got != "offers blocks" {
		t.Errorf("LinkingLion with blocks tells %q", got)
	}
	if got := tellOf("Claims: Bitcoin Core 0.11.0\nProbably: "+linkingLionAbout, true); got != "listed address range" {
		t.Errorf("bare LinkingLion tells %q", got)
	}
}

// Every sentence otherAbout and noAgentAbout can write has its few words,
// except the two that say nothing was seen. Read from the source, so a new
// sentence without a short form fails here.
func TestEveryBehaviourHasATell(t *testing.T) {
	src, err := os.ReadFile("kinds.go")
	if err != nil {
		t.Fatal(err)
	}
	quiet := map[string]bool{"Software with a name we don't recognise": true, "software that keeps quiet about what it is": true}
	n := 0
	for _, fn := range []string{"func otherAbout(", "func noAgentAbout("} {
		body := string(src[strings.Index(string(src), fn):])
		body = body[:strings.Index(body, "\n}\n")]
		for _, m := range regexp.MustCompile(`return (?:lead \+ )?"([^"]+)"`).FindAllStringSubmatch(body, -1) {
			n++
			got := behaviourTell(m[1])
			if quiet[m[1]] != (got == "") {
				t.Errorf("%q tells %q", m[1], got)
			}
		}
	}
	if n < 10 {
		t.Fatalf("found only %d sentences - did the functions move?", n)
	}
}
