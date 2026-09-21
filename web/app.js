"use strict";
(() => {
  const GROUPS = ["manual", "inbound", "outbound"];
  const LABEL = { manual: "Manual", inbound: "Inbound", outbound: "Outbound" };
  const NS = "http://www.w3.org/2000/svg";
  const REGION_ZOOM = 2.5;                 // from this zoom factor on, markers split into regions
  const IDLE_PAUSE_MS = 30 * 60 * 1000;    // stop asking Core after 30 min without input
  const SIDE_KEY = "peermap.sideOpen";
  const $ = (id) => document.getElementById(id);

  const regionName = (() => {
    try {
      const dn = new Intl.DisplayNames(["en"], { type: "region" });
      return (cc) => { try { return dn.of(cc) || cc; } catch { return cc; } };
    } catch { return (cc) => cc; }
  })();

  const state = {
    data: null,           // last /api/peers reply
    world: null,          // world.json
    show: { manual: true, inbound: true, outbound: true },
    place: null,          // location filter from clicking a marker: { key, label }
    kind: null,           // kind filter from clicking an entry in a group's kind line
    sort: {},             // per group: { key, dir }
    view: { k: 1, x: 0, y: 0 },
    paused: false,
    // A peer another app pointed at, via ?peer=<address>. Highlighted once and
    // scrolled to, then left alone - it is a starting point, not a filter.
    focus: new URLSearchParams(location.search).get("peer") || null,
    focusDone: false,
    lastInput: Date.now(),
    // When the current snapshot arrived here. The block's age comes from the
    // node's clock; how long it has been sitting in this browser is added on
    // top, so the two-minute mark never depends on the browser's own clock
    // agreeing with the node's.
    receivedAt: 0,
    markShown: false,
    open: { manual: true, inbound: true, outbound: true }, // table sections
  };

  const store = {
    get(k) { try { return localStorage.getItem(k); } catch { return null; } },
    set(k, v) { try { localStorage.setItem(k, v); } catch { /* private mode etc. */ } },
  };

  // ---------- projection: same Natural Earth polynomial as tools/genmap ----------
  function nePoly(lon, lat) {
    const l = lon * Math.PI / 180, p = lat * Math.PI / 180;
    const p2 = p * p, p4 = p2 * p2;
    return [
      l * (0.8707 - 0.131979 * p2 + p4 * (-0.013791 + p4 * (0.003971 * p2 - 0.001529 * p4))),
      p * (1.007226 + p2 * (0.015085 + p4 * (-0.044475 + 0.028874 * p2 - 0.005916 * p4))),
    ];
  }
  const X_MAX = nePoly(180, 0)[0];
  const Y_MAX = nePoly(0, 84)[1];
  function project(lon, lat) {
    const [x, y] = nePoly(lon, lat);
    const s = 1000 / (2 * X_MAX);
    return [(x + X_MAX) * s, (Y_MAX - y) * s];
  }

  // ---------- polling: only while visible, and not after 30 min idle ----------
  let pollTimer = 0;
  let tickTimer = 0;
  let inFlight = false;

  // Exactly one polling chain at a time. Without the inFlight guard, a tab
  // that became visible again while a request was still running started a
  // second chain next to the first, and every such moment added another.
  async function refresh() {
    clearTimeout(pollTimer);
    if (inFlight || document.hidden || state.paused) return;
    if (Date.now() - state.lastInput > IDLE_PAUSE_MS) {
      setPaused(true);
      return;
    }
    let next = 10;
    inFlight = true;
    try {
      const r = await fetch("api/peers", { cache: "no-store" });
      if (!r.ok) throw new Error("HTTP " + r.status);
      state.data = await r.json();
      state.receivedAt = Date.now();
      next = state.data.next_in || state.data.interval || 10;
      render();
    } catch (e) {
      showError("Peer Map backend not reachable: " + e.message);
    } finally {
      inFlight = false;
    }
    clearTimeout(pollTimer);
    if (!document.hidden && !state.paused) pollTimer = setTimeout(refresh, next * 1000 + 250);
  }

  function setPaused(p) {
    state.paused = p;
    $("paused").hidden = !p;
    if (p) clearTimeout(pollTimer);
    renderStatus();
  }

  let inputStamp = 0;
  const noteInput = () => {
    const now = Date.now();
    if (now - inputStamp < 1000) return; // cheap throttle
    inputStamp = now;
    state.lastInput = now;
  };
  for (const ev of ["pointerdown", "pointermove", "keydown", "wheel", "touchstart", "scroll"]) {
    window.addEventListener(ev, noteInput, { passive: true, capture: true });
  }
  $("resume").addEventListener("click", () => {
    state.lastInput = Date.now();
    setPaused(false);
    refresh();
  });

  document.addEventListener("visibilitychange", () => {
    if (document.hidden) {
      clearTimeout(pollTimer);
      clearInterval(tickTimer);
      tickTimer = 0;
    } else {
      startTicker();
      refresh();
    }
  });

  function startTicker() {
    if (!tickTimer) {
      tickTimer = setInterval(() => {
        renderStatus();
        // Two minutes are up between two polls, and a paused dashboard never
        // polls again at all - so the mark takes itself off rather than
        // waiting for the next answer.
        if (state.markShown && !liveBlock()) render();
      }, 1000);
    }
  }

  function showError(msg) {
    const el = $("error");
    el.textContent = msg;
    el.hidden = !msg;
  }

  // ---------- formatting ----------
  function ago(sec) {
    if (sec < 60) return Math.max(0, Math.floor(sec)) + "s";
    const m = Math.floor(sec / 60), h = Math.floor(m / 60), d = Math.floor(h / 24);
    if (d > 0) return d + "d " + (h % 24) + "h";
    if (h > 0) return h + "h " + (m % 60) + "m";
    return m + "m";
  }
  const typeLabel = (t) => ({
    "outbound-full-relay": "full-relay",
    "block-relay-only": "block-relay",
  }[t] || t || "-");
  const netLabel = (n) => ({
    ipv4: "IPv4", ipv6: "IPv6", onion: "Tor", i2p: "I2P", cjdns: "CJDNS",
    not_publicly_routable: "Private",
  }[n] || n || "-");
  const locLabel = (p) => (p.cc ? (p.region ? `${p.region}, ${regionName(p.cc)}` : regionName(p.cc)) : "");

  // ---------- the block that just landed ----------

  // Bitcoin Lab next door says which block arrived last, who mined it, and
  // which peer here delivered it. The mark lasts two minutes - long enough to
  // see on a dashboard somebody just opened, short enough that a star stays a
  // piece of news instead of turning into decoration.
  function liveBlock() {
    const b = state.data && state.data.last_block;
    if (!b || !b.mark_for_ms) return null;
    const age = b.age_ms + Math.max(0, Date.now() - state.receivedAt);
    return age < b.mark_for_ms ? b : null;
  }

  // Refreshed once per render and read by the table and the map, so both
  // always show the same peers starred.
  let deliveredNow = new Set();

  function deliveredSet() {
    const b = liveBlock();
    return new Set(b && b.first_peers ? b.first_peers : []);
  }

  // The card under the filters: one block, and the three roles it has here.
  // Mined by (from Bitcoin Lab's coinbase reading), delivered by (the peers
  // Core credited), raced by (the pools that turned it into work). Each of the
  // three can be missing, and then it says so rather than leaving a gap.
  function renderBlockCard() {
    const card = $("blockCard");
    if (!card) return;
    const b = state.data && state.data.last_block;
    card.hidden = !b;
    if (!b) return;

    // With a route the card stays open: the summary and the route are the
    // point, and only the full pool list folds away. Without one - Stratum
    // Race off, or no race recorded for this block - it is today's card, a
    // single line that opens onto who delivered it.
    const stops = routeStops(b);
    const more = $("blockMore");
    const line = stops ? $("blockSummary") : $("blockMoreLabel");
    $("blockSummary").hidden = !stops;
    $("blockRoute").hidden = !stops;
    more.classList.toggle("minor", Boolean(stops));
    fillSummary(line, b);
    if (stops) {
      const n = b.stratum.entries.length;
      $("blockMoreLabel").textContent = `All ${n} ${n === 1 ? "pool" : "pools"}`;
      renderRoute($("blockRoute"), b, stops);
    }
    renderBlockBody(b, Boolean(stops));
  }

  function fillSummary(sum, b) {
    sum.textContent = "";
    const add = (cls, text, title) => {
      const el = document.createElement("span");
      if (cls) el.className = cls;
      el.textContent = text;
      if (title) el.title = title;
      sum.appendChild(el);
      return el;
    };
    const sep = () => add("b-sep", "·");

    add("b-height", b.height ? "Block " + b.height.toLocaleString() : "Last block", b.hash || "");
    if (b.pool || b.pool_tag) {
      sep();
      add("b-label", "mined by");
      add("b-pool" + (b.pool ? "" : " raw"), b.pool || `"${b.pool_tag}"`,
        b.pool ? b.pool_name : "No pool we know of - this is the text in its coinbase");
    } else {
      sep();
      add("miss", "miner unknown", "Neither the payout address nor the coinbase text matched a known pool");
    }
    const n = (b.first_peers || []).length;
    sep();
    add(null, n === 0 ? "nobody credited" : n === 1 ? "delivered by 1 peer" : `delivered by ${n} peers`,
      b.eligible ? `${b.eligible} peers were connected when it arrived` : "");
    sep();
    add("miss", ago(blockAgeMs(b) / 1000) + " ago", "How long ago your node saw this block");
  }

  const blockAgeMs = (b) => b.age_ms + Math.max(0, Date.now() - state.receivedAt);

  // ---------- the route of one block ----------
  //
  // Where the time between "this block exists" and "your pool has a job for
  // it" goes, drawn to scale. The zero is the first new job any pool sent
  // here: when the block was found cannot be known - the header's time is the
  // miner's own, in whole seconds - and the first job is the earliest moment
  // it is visible from this node. After that everything is measured by one
  // clock on one machine, except the last hop into Core, which is estimated
  // as the delivering peer's ping.
  //
  // A whole ping rather than half: half is only the floor. A compact block
  // that arrives complete costs half a round trip; one that is missing a few
  // transactions costs another full one to ask for them. Which happened is not
  // visible from here, and a whole ping sits in the middle. It only moves the
  // peer's dot - Core and the pools stand where they were measured.
  const ROUTE_STUB = 140;  // the miner's dashed lead-in, not to scale
  const ROUTE_GAP = 150;   // the closest two stops may sit, so labels never touch

  const hostOf = (a) => (a.startsWith("[") ? a.slice(1, a.indexOf("]")) : a.replace(/:\d+$/, ""));
  const signedMs = (t) => (Math.round(t) === 0 ? "0" : (t > 0 ? "+" : "−") + Math.abs(Math.round(t)) + " ms");

  function routeStops(b) {
    const s = b.stratum;
    if (!s || s.core_ms == null || !(s.entries || []).length) return null;
    const winner = s.entries.find((e) => e.rank === 1 && e.latency_ms != null);
    if (!winner) return null;
    const own = s.entries.find((e) => e.own && e.latency_ms != null) || null;

    const stops = [];
    // The winner is the zero. When it is the owner's own pool the two are one
    // stop, and it is drawn as his.
    if (own !== winner) stops.push({ kind: "win", t: 0, name: winner.label, sub: "first job" });

    const known = new Map(((state.data && state.data.peers) || []).map((p) => [p.addr, p]));
    const credited = (b.first_peers || []).map((addr) => ({ addr, peer: known.get(addr) }));
    const pingOf = (c) => (c.peer ? (c.peer.min_ping_ms != null ? c.peer.min_ping_ms : c.peer.ping_ms) : null);
    // Two credited is the rare case; the one with the shorter line sets the dot.
    const lead = credited.filter((c) => pingOf(c) != null).sort((x, y) => pingOf(x) - pingOf(y))[0];
    if (lead) {
      const p = lead.peer;
      const where = p.region || (p.cc ? regionName(p.cc) : "");
      const more = credited.length > 1 ? ` +${credited.length - 1}` : "";
      stops.push({
        kind: "peer", group: p.group, t: s.core_ms - pingOf(lead),
        name: hostOf(lead.addr) + more,
        sub: [where, p.operator].filter(Boolean),
        title: credited.map((c) => c.addr).join("\n"),
      });
    }
    stops.push({
      kind: "core", t: s.core_ms, name: "Core",
      // Credited, but gone before this was drawn: there is no ping to place
      // it by, so it is named rather than drawn.
      sub: [!lead && credited.length ? "via " + hostOf(credited[0].addr) : "your node"],
    });
    if (own) stops.push({ kind: "own", t: own.latency_ms, name: own.label, sub: ["your job"] });
    for (const st of stops) if (typeof st.sub === "string") st.sub = [st.sub];
    return stops.sort((x, y) => x.t - y.t);
  }

  // To scale, but never so close that two labels touch: a stop that would
  // sit nearer than ROUTE_GAP to the one before is pushed along, and the
  // whole row pulled back in from the right. Too narrow for that at all - a
  // phone - and it is a list instead, which says the same thing.
  function placeStops(stops, left, right) {
    const t0 = stops[0].t, span = Math.max(1, stops[stops.length - 1].t - t0);
    const x = stops.map((s) => left + ((s.t - t0) / span) * (right - left));
    for (let i = 1; i < x.length; i++) x[i] = Math.max(x[i], x[i - 1] + ROUTE_GAP);
    x[x.length - 1] = Math.min(x[x.length - 1], right);
    for (let i = x.length - 2; i >= 0; i--) x[i] = Math.min(x[i], x[i + 1] - ROUTE_GAP);
    return x;
  }

  function segLabel(a, z) {
    const d = Math.round(z.t - a.t) + " ms";
    if (a.kind === "peer" && z.kind === "core") return "~" + d + " · ping";
    if (a.kind === "win" && z.kind === "peer") return "reaches your peer · " + d;
    return d;
  }

  function renderRoute(box, b, stops) {
    box.textContent = "";
    const miner = b.pool || (b.pool_tag ? `"${b.pool_tag}"` : "unknown miner");
    const W = box.clientWidth - 12;
    // The last label is right-aligned on its dot, so the dot keeps a margin.
    const left = ROUTE_STUB, right = W - 14;

    if ((stops.length - 1) * ROUTE_GAP > right - left) {
      // Narrow: the same stops as rows.
      const t = document.createElement("table");
      t.className = "rt-list";
      const row = (cls, name, time, sub) => {
        const tr = document.createElement("tr");
        tr.className = cls;
        for (const [c, v] of [["n", name], ["num", time], ["miss", sub]]) {
          const td = document.createElement("td");
          td.className = c;
          td.textContent = v;
          tr.appendChild(td);
        }
        t.appendChild(tr);
      };
      row("pool", miner, "", "mined it");
      for (const s of stops) row(s.kind, s.name, signedMs(s.t), s.sub.join(" · "));
      box.appendChild(t);
    } else {
      const rt = document.createElement("div");
      rt.className = "rt";
      box.appendChild(rt);
      const el = (cls, x, html) => {
        const e = document.createElement("div");
        e.className = cls;
        if (x != null) e.style.left = x + "px"; // CSSOM, which the CSP allows
        if (html) e.append(...html);
        rt.appendChild(e);
        return e;
      };
      const txt = (tag, text, cls) => {
        const e = document.createElement(tag);
        e.textContent = text;
        if (cls) e.className = cls;
        return e;
      };
      const xs = placeStops(stops, left, right);

      const stub = el("rt-stub", 6);
      stub.style.width = (xs[0] - 6) + "px";
      const track = el("rt-track", xs[0]);
      track.style.width = (xs[xs.length - 1] - xs[0]) + "px";
      el("rt-dot pool", 6);
      el("rt-lab pool first", 0, [txt("b", miner), txt("small", "mined it")]);

      stops.forEach((s, i) => {
        el("rt-dot " + s.kind + (s.group ? " " + s.group : ""), xs[i]);
        const last = i === stops.length - 1;
        const lab = el("rt-lab " + s.kind + (last ? " last" : ""), xs[i],
          [txt("b", s.name), txt("span", signedMs(s.t), "t"), ...s.sub.map((line) => txt("small", line))]);
        if (s.title) lab.title = s.title;
        if (i > 0) el("rt-seg", (xs[i - 1] + xs[i]) / 2, [txt("span", segLabel(stops[i - 1], s))]);
      });
    }
    const note = document.createElement("p");
    note.className = "note";
    note.textContent = "0 is the first new job any pool sent here. When the block was found cannot be measured; "
      + "the hop into your node is estimated from the peer's ping.";
    box.appendChild(note);
  }

  function renderBlockBody(b, routed) {
    const body = $("blockBody");
    body.textContent = "";

    // --- who brought it here - on the route when there is one ------------
    if (!routed) {
      const peers = document.createElement("div");
      const h1 = document.createElement("h3");
      h1.textContent = "Delivered first";
      peers.appendChild(h1);
      if (!(b.first_peers || []).length) {
        const p = document.createElement("p");
        p.className = "note";
        p.textContent = "No peer was credited for this block.";
        peers.appendChild(p);
      } else {
        const known = new Map((state.data.peers || []).map((p) => [p.addr, p]));
        const t = document.createElement("table");
        for (const addr of b.first_peers) {
          const peer = known.get(addr);
          const tr = document.createElement("tr");
          const a = document.createElement("td");
          a.className = "mono";
          a.textContent = addr;
          const where = document.createElement("td");
          where.textContent = peer ? locLabel(peer) || "–" : "";
          const who = document.createElement("td");
          who.textContent = peer ? peer.operator || "–" : "no longer connected";
          if (!peer) who.className = "miss";
          if (peer && peer.asn) who.title = "AS" + peer.asn;
          tr.append(a, where, who);
          t.appendChild(tr);
        }
        peers.appendChild(t);
      }
      body.appendChild(peers);
    }

    // --- who turned it into work ---------------------------------------
    const race = document.createElement("div");
    const h2 = document.createElement("h3");
    h2.textContent = "Stratum race";
    race.appendChild(h2);
    if (!b.stratum || !(b.stratum.entries || []).length) {
      const p = document.createElement("p");
      p.className = "note";
      p.textContent = "No race recorded for this block - Bitcoin Lab's Stratum Race is off, or it was not running.";
      race.appendChild(p);
    } else {
      const t = document.createElement("table");
      for (const e of b.stratum.entries) {
        const tr = document.createElement("tr");
        const rank = document.createElement("td");
        rank.className = "num miss";
        rank.textContent = e.rank ? e.rank + "." : "\u2013";
        const label = document.createElement("td");
        label.textContent = e.label;
        if (e.own) {
          label.className = "own";
          label.title = "Your own pool";
        }
        const delta = document.createElement("td");
        delta.className = "num" + (e.miss ? " miss" : "");
        delta.textContent = e.miss
          ? "no job"
          : e.latency_ms === 0 ? "first" : "+" + Math.round(e.latency_ms) + " ms";
        tr.append(rank, label, delta);
        t.appendChild(tr);
      }
      race.appendChild(t);
      const note = document.createElement("p");
      note.className = "note";
      note.textContent = "Times are relative to the first job seen here, not to the moment the block was found.";
      race.appendChild(note);
    }
    body.appendChild(race);
  }

  function renderBlockMark() {
    const el = $("block-mark");
    if (!el) return;
    const b = liveBlock();
    const name = b ? b.pool || b.pool_tag : null;
    state.markShown = Boolean(name);
    el.hidden = !name;
    el.textContent = "";
    el.classList.toggle("raw", Boolean(b && !b.pool && b.pool_tag));
    if (!name) return;
    const star = document.createElement("span");
    star.className = "star";
    star.textContent = "\u2605";
    const label = document.createElement("span");
    label.className = "name";
    label.textContent = b.pool ? name : `"${name}"`;
    el.append(star, label);
    el.title = b.pool
      ? `Block ${b.height ? b.height.toLocaleString() : ""} was mined by ${b.pool_name || name}. The starred peers delivered it to your node.`
      : `Block ${b.height ? b.height.toLocaleString() : ""} is from a miner we cannot name; this is the text in its coinbase. The starred peers delivered it to your node.`;
  }

  // ---------- render ----------
  const matchesKind = (p) => !state.kind || p.kind === state.kind;
  const visiblePeers = () =>
    (state.data ? state.data.peers.filter((p) => state.show[p.group] && matchesKind(p)) : []);
  const regionMode = () => state.view.k >= REGION_ZOOM;

  function render() {
    const d = state.data;
    $("demo").hidden = !d.demo;
    showError(d.error ? "Bitcoin Core: " + d.error + (d.fetched_at ? " - showing the last good peer list." : "") : "");
    $("version").textContent = "v" + (d.version || "dev");

    const counts = { manual: 0, inbound: 0, outbound: 0 };
    for (const p of d.peers) counts[p.group]++;
    for (const g of GROUPS) document.querySelector(`[data-count="${g}"]`).textContent = counts[g];

    renderStatus();
    deliveredNow = deliveredSet();
    renderSiblingLink();
    renderBlockMark();
    renderBlockCard();
    renderMarkers();
    renderUnplaced();
    renderTables();
    focusOnce();
  }

  // Somebody arrived from Bitcoin Lab asking about one peer. Put it in front of
  // them once. Not a filter and not sticky: the rest of the node is the reason
  // they are looking at a map, and a row that keeps jumping under the cursor on
  // every ten-second refresh would be its own kind of rude.
  function focusOnce() {
    if (!state.focus || state.focusDone) return;
    const row = document.querySelector("tr.focus");
    if (!row) return;
    state.focusDone = true;
    row.scrollIntoView({ block: "center", behavior: "smooth" });
  }

  // Bitcoin Lab measures which of these peers actually delivers each block
  // first. The server says whether it is installed; the port is the one its
  // store manifest publishes, and the host is this one.
  const SIBLING_PORT = 8790;
  function siblingURL(peerAddress) {
    const base = `${location.protocol}//${location.hostname}:${SIBLING_PORT}/`;
    return peerAddress ? base + "?peer=" + encodeURIComponent(peerAddress) : base;
  }
  function renderSiblingLink() {
    const a = $("sibling-link");
    if (!a || !state.data) return;
    a.hidden = !state.data.sibling;
    if (state.data.sibling) a.href = siblingURL(null);
  }

  function renderStatus() {
    const d = state.data;
    if (!d) return;
    const since = d.fetched_at ? ago(Date.now() / 1000 - d.fetched_at) + " ago" : "never";
    const mode = state.paused ? "paused" : `every ${d.interval}s while open`;
    $("status").textContent = `${d.peers.length} peers · updated ${since} · ${mode}`;
  }

  // ---------- map ----------
  async function loadWorld() {
    const r = await fetch("world.json");
    state.world = await r.json();
    $("map").setAttribute("viewBox", `0 0 ${state.world.w} ${state.world.h}`);
    const frag = document.createDocumentFragment();
    const addPath = (d, cc) => {
      const p = document.createElementNS(NS, "path");
      p.setAttribute("d", d);
      p.setAttribute("class", "land");
      p.setAttribute("vector-effect", "non-scaling-stroke");
      if (cc) p.dataset.cc = cc;
      frag.appendChild(p);
    };
    for (const [cc, d] of Object.entries(state.world.paths)) addPath(d, cc);
    if (state.world.unnamed) addPath(state.world.unnamed, "");
    $("countries").appendChild(frag);
    applyView();
  }

  // A bucket is one marker: a country (world view) or a region (zoomed in).
  // Peers whose region is unknown stay on their country's point.
  function placeKey(p, byRegion) {
    return byRegion && p.rid ? "r" + p.rid : "c" + p.cc;
  }
  function peerMatchesPlace(p) {
    if (!state.place) return true;
    const k = state.place.key;
    return k[0] === "r" ? "r" + p.rid === k : p.cc === k.slice(1);
  }

  function buckets() {
    const byRegion = regionMode();
    const delivered = deliveredNow;
    const by = new Map();
    for (const p of visiblePeers()) {
      if (!p.cc) continue;
      const key = placeKey(p, byRegion);
      let b = by.get(key);
      if (!b) {
        let pt = state.world.points[p.cc];
        let title = regionName(p.cc), sub = "";
        if (key[0] === "r") {
          pt = project(p.lon, p.lat);
          title = p.region;
          sub = regionName(p.cc);
        } else if (byRegion) {
          sub = "region unknown";
        }
        if (!pt) continue;
        by.set(key, (b = { key, title, sub, pt, cc: p.cc, manual: 0, inbound: 0, outbound: 0, total: 0 }));
      }
      b[p.group]++;
      b.total++;
      if (delivered.has(p.addr)) b.delivered = true;
    }
    return [...by.values()].sort((a, b) => b.total - a.total);
  }

  const radius = (n) => Math.min(26, 5 + 3.2 * Math.sqrt(n));

  function wedge(r, a0, a1) {
    if (a1 - a0 >= Math.PI * 2 - 1e-6) {
      return `M${-r} 0A${r} ${r} 0 1 1 ${r} 0A${r} ${r} 0 1 1 ${-r} 0Z`;
    }
    const x0 = r * Math.sin(a0), y0 = -r * Math.cos(a0);
    const x1 = r * Math.sin(a1), y1 = -r * Math.cos(a1);
    const large = a1 - a0 > Math.PI ? 1 : 0;
    return `M0 0L${x0.toFixed(2)} ${y0.toFixed(2)}A${r} ${r} 0 ${large} 1 ${x1.toFixed(2)} ${y1.toFixed(2)}Z`;
  }

  let renderedMode = null;
  function renderMarkers() {
    if (!state.world || !state.data) return;
    renderedMode = regionMode();
    $("mapLevel").textContent = renderedMode ? "Regions" : "Countries";
    $("mapLevelHint").hidden = renderedMode;
    const layer = $("markers");
    layer.textContent = "";
    const bs = buckets();
    const hot = new Set(bs.map((b) => b.cc));
    for (const el of $("countries").children) el.classList.toggle("hot", hot.has(el.dataset.cc));

    // Largest first, so small markers are drawn on top and stay clickable -
    // and the one that delivered the block last of all, so a star is never
    // hidden under a neighbouring country.
    for (const b of [...bs].sort((x, y) => Number(Boolean(x.delivered)) - Number(Boolean(y.delivered)))) {
      const r = radius(b.total);
      const g = document.createElementNS(NS, "g");
      const selected = state.place && state.place.key === b.key;
      g.setAttribute("class", "marker" + (selected ? " selected" : "") + (b.delivered ? " delivered" : ""));
      g.dataset.x = b.pt[0];
      g.dataset.y = b.pt[1];
      g.setAttribute("tabindex", "0");
      g.setAttribute("role", "button");
      g.setAttribute("aria-label", `${b.title}${b.sub ? ", " + b.sub : ""}: ${GROUPS.map((k) => b[k] + " " + LABEL[k].toLowerCase()).join(", ")}`);

      const ring = document.createElementNS(NS, "circle");
      ring.setAttribute("class", "ring");
      ring.setAttribute("r", r + 2);
      g.appendChild(ring);

      let a = 0;
      for (const k of GROUPS) {
        if (!b[k]) continue;
        const span = (b[k] / b.total) * Math.PI * 2;
        const s = document.createElementNS(NS, "path");
        s.setAttribute("class", "seg seg-" + k);
        s.setAttribute("d", wedge(r, a, a + span));
        g.appendChild(s);
        a += span;
      }
      if (b.total > 1) {
        const t = document.createElementNS(NS, "text");
        t.setAttribute("x", r + 4);
        t.setAttribute("y", 4);
        t.textContent = b.total;
        g.appendChild(t);
      }
      if (b.delivered) {
        const star = document.createElementNS(NS, "text");
        star.setAttribute("class", "delivered-star");
        star.setAttribute("x", 0);
        star.setAttribute("y", -(r + 5));
        star.setAttribute("text-anchor", "middle");
        star.textContent = "\u2605";
        g.appendChild(star);
      }
      g.addEventListener("pointerenter", (e) => showTip(b, e));
      g.addEventListener("pointermove", (e) => moveTip(e));
      g.addEventListener("pointerleave", hideTip);
      g.addEventListener("focus", () => showTip(b, null, g));
      g.addEventListener("blur", hideTip);
      g.addEventListener("click", (e) => { e.stopPropagation(); togglePlace(b); });
      g.addEventListener("keydown", (e) => {
        if (e.key === "Enter" || e.key === " ") { e.preventDefault(); togglePlace(b); }
      });
      layer.appendChild(g);
    }
    placeMarkers();
  }

  function placeMarkers() {
    const { k, x, y } = state.view;
    // Markers keep their on-screen size while the map zooms: they are
    // positioned in viewBox units but scaled by the inverse of the fit ratio.
    const svg = $("map");
    const fit = svg.clientWidth ? state.world.w / svg.clientWidth : 1;
    // On a phone the whole map is ~360px wide; full-size markers would bury Europe.
    const shrink = Math.min(1, Math.max(0.55, (svg.clientWidth || 800) / 800));
    for (const g of $("markers").children) {
      const px = +g.dataset.x * k + x, py = +g.dataset.y * k + y;
      g.setAttribute("transform", `translate(${px.toFixed(1)} ${py.toFixed(1)}) scale(${(fit * shrink).toFixed(3)})`);
    }
  }

  function togglePlace(b) {
    state.place = state.place && state.place.key === b.key ? null : { key: b.key, label: b.sub && b.key[0] === "r" ? `${b.title}, ${b.sub}` : b.title };
    renderPlaceFilter();
    renderMarkers();
    renderTables();
  }
  function renderPlaceFilter() {
    $("placeFilter").hidden = !state.place;
    $("placeFilterName").textContent = state.place ? state.place.label : "";
    $("kindFilter").hidden = !state.kind;
    $("kindFilterName").textContent = state.kind || "";
  }

  // Both filters redraw the same three things, and the map is in that list on
  // purpose: after clicking "5 Pool node" the next question is where those
  // five are, and the map answers it without a second click.
  function applyFilters() {
    renderPlaceFilter();
    renderUnplaced();
    renderMarkers();
    renderTables();
  }

  function setKind(kind) {
    state.kind = state.kind === kind ? null : kind;
    applyFilters();
  }

  $("placeFilterClear").addEventListener("click", () => {
    state.place = null;
    applyFilters();
  });
  $("kindFilterClear").addEventListener("click", () => {
    state.kind = null;
    applyFilters();
  });

  // ---------- tooltip ----------
  const tip = $("tooltip");
  function showTip(b, e, el) {
    tip.textContent = "";
    const h = document.createElement("h3");
    h.textContent = b.title;
    tip.appendChild(h);
    if (b.sub) {
      const s = document.createElement("div");
      s.className = "sub";
      s.textContent = b.sub;
      tip.appendChild(s);
    }
    for (const k of GROUPS) {
      if (!state.show[k]) continue;
      const row = document.createElement("div");
      row.className = "row";
      const sw = document.createElement("span");
      sw.className = "swatch s-" + k;
      const label = document.createElement("span");
      label.textContent = LABEL[k];
      const n = document.createElement("b");
      n.textContent = b[k];
      row.append(sw, label, n);
      tip.appendChild(row);
    }
    tip.hidden = false;
    if (e) moveTip(e);
    else if (el) {
      const wr = $("mapWrap").getBoundingClientRect(), r = el.getBoundingClientRect();
      positionTip(r.right - wr.left, r.top - wr.top);
    }
  }
  function moveTip(e) {
    const wr = $("mapWrap").getBoundingClientRect();
    positionTip(e.clientX - wr.left, e.clientY - wr.top);
  }
  function positionTip(x, y) {
    const wrap = $("mapWrap");
    const w = tip.offsetWidth, h = tip.offsetHeight;
    let left = x + 14, top = y + 14;
    if (left + w > wrap.clientWidth - 8) left = x - w - 14;
    if (top + h > wrap.clientHeight - 8) top = y - h - 14;
    tip.style.left = Math.max(8, left) + "px";
    tip.style.top = Math.max(8, top) + "px";
  }
  function hideTip() { tip.hidden = true; }

  // ---------- pan & zoom ----------
  const MAX_K = 24;
  function clampView() {
    const v = state.view, W = state.world.w, H = state.world.h;
    v.k = Math.min(MAX_K, Math.max(1, v.k));
    v.x = Math.min(0, Math.max(W - W * v.k, v.x));
    v.y = Math.min(0, Math.max(H - H * v.k, v.y));
  }
  function applyView() {
    if (!state.world) return;
    clampView();
    const { k, x, y } = state.view;
    $("countries").setAttribute("transform", `translate(${x} ${y}) scale(${k})`);
    if (!state.data) return;
    if (renderedMode !== regionMode()) {
      hideTip();
      renderMarkers(); // crossing the region threshold regroups the markers
    } else {
      placeMarkers();
    }
  }
  function toSvg(clientX, clientY) {
    const svg = $("map");
    const pt = svg.createSVGPoint();
    pt.x = clientX; pt.y = clientY;
    return pt.matrixTransform(svg.getScreenCTM().inverse());
  }
  function zoomAt(factor, sx, sy) {
    const v = state.view;
    const nk = Math.min(MAX_K, Math.max(1, v.k * factor));
    const f = nk / v.k;
    v.x = sx - (sx - v.x) * f;
    v.y = sy - (sy - v.y) * f;
    v.k = nk;
    applyView();
  }
  function setupPanZoom() {
    const svg = $("map");
    svg.addEventListener("wheel", (e) => {
      e.preventDefault();
      const p = toSvg(e.clientX, e.clientY);
      zoomAt(Math.exp(-e.deltaY * 0.0015), p.x, p.y);
    }, { passive: false });

    const pointers = new Map();
    let last = null, pinch = null, moved = false;
    svg.addEventListener("pointerdown", (e) => {
      if (e.target.closest(".marker")) return;
      svg.setPointerCapture(e.pointerId);
      pointers.set(e.pointerId, toSvg(e.clientX, e.clientY));
      moved = false;
      if (pointers.size === 1) { last = pointers.get(e.pointerId); svg.classList.add("dragging"); }
      if (pointers.size === 2) {
        const [a, b] = [...pointers.values()];
        pinch = { d: Math.hypot(a.x - b.x, a.y - b.y) };
      }
    });
    svg.addEventListener("pointermove", (e) => {
      if (!pointers.has(e.pointerId)) return;
      const p = toSvg(e.clientX, e.clientY);
      pointers.set(e.pointerId, p);
      if (pointers.size === 2 && pinch) {
        const [a, b] = [...pointers.values()];
        const d = Math.hypot(a.x - b.x, a.y - b.y);
        if (pinch.d > 0) zoomAt(d / pinch.d, (a.x + b.x) / 2, (a.y + b.y) / 2);
        pinch.d = d;
        moved = true;
      } else if (pointers.size === 1 && last) {
        state.view.x += p.x - last.x;
        state.view.y += p.y - last.y;
        if (Math.abs(p.x - last.x) + Math.abs(p.y - last.y) > 0.5) moved = true;
        applyView();
        last = toSvg(e.clientX, e.clientY);
      }
    });
    const end = (e) => {
      pointers.delete(e.pointerId);
      if (pointers.size < 2) pinch = null;
      if (pointers.size === 0) { last = null; svg.classList.remove("dragging"); }
      else last = [...pointers.values()][0];
    };
    svg.addEventListener("pointerup", end);
    svg.addEventListener("pointercancel", end);
    svg.addEventListener("dblclick", (e) => {
      const p = toSvg(e.clientX, e.clientY);
      zoomAt(2, p.x, p.y);
    });
    svg.addEventListener("click", () => {
      if (!moved && state.place) {
        state.place = null;
        applyFilters();
      }
    });

    const center = () => [state.world.w / 2, state.world.h / 2];
    $("zoomIn").addEventListener("click", () => zoomAt(1.6, ...center()));
    $("zoomOut").addEventListener("click", () => zoomAt(1 / 1.6, ...center()));
    $("zoomReset").addEventListener("click", () => { state.view = { k: 1, x: 0, y: 0 }; applyView(); });
    window.addEventListener("resize", () => { if (state.data) placeMarkers(); });
    // The route is laid out in pixels from the card's width, so a new width
    // needs a new layout - and a narrow one may need the list instead.
    let routeResize = null;
    window.addEventListener("resize", () => {
      clearTimeout(routeResize);
      routeResize = setTimeout(() => { if (state.data) renderBlockCard(); }, 150);
    });
  }

  // ---------- filters ----------
  for (const btn of document.querySelectorAll(".chip[data-group]")) {
    btn.addEventListener("click", () => {
      const g = btn.dataset.group;
      state.show[g] = !state.show[g];
      btn.setAttribute("aria-pressed", String(state.show[g]));
      if (state.data) render();
    });
  }

  // ---------- not on the map (collapsible side panel) ----------
  function setSide(open) {
    $("unplaced").hidden = !open;
    $("sideToggle").setAttribute("aria-expanded", String(open));
    store.set(SIDE_KEY, open ? "1" : "0");
    if (state.world && state.data) placeMarkers(); // map width changed
  }
  $("sideToggle").addEventListener("click", () => setSide($("unplaced").hidden));
  setSide(store.get(SIDE_KEY) === "1"); // collapsed unless opened before

  function renderUnplaced() {
    const rows = new Map();
    let total = 0;
    for (const p of state.data.peers) {
      if (p.cc || !matchesKind(p)) continue;
      let key = netLabel(p.network);
      if (p.network === "ipv4" || p.network === "ipv6") key = "No geo data";
      let r = rows.get(key);
      if (!r) rows.set(key, (r = { manual: 0, inbound: 0, outbound: 0 }));
      r[p.group]++;
      if (state.show[p.group]) total++;
    }
    $("unplacedCount").textContent = total;
    const body = $("unplacedBody");
    body.textContent = "";
    if (!rows.size) {
      const tr = document.createElement("tr");
      const td = document.createElement("td");
      td.colSpan = 4;
      td.textContent = "Every peer is on the map.";
      tr.appendChild(td);
      body.appendChild(tr);
      return;
    }
    for (const [name, r] of [...rows.entries()].sort()) {
      const tr = document.createElement("tr");
      const th = document.createElement("td");
      th.textContent = name;
      tr.appendChild(th);
      for (const g of GROUPS) {
        const td = document.createElement("td");
        td.textContent = state.show[g] ? r[g] : "–";
        tr.appendChild(td);
      }
      body.appendChild(tr);
    }
  }

  // ---------- tables ----------

  // The kind is what the peer calls itself, the flags are what Core observed.
  // They are shown in one cell on purpose: a peer claiming to be Bitcoin Core
  // while offering nothing and having no known chain is only obvious when the
  // claim and the observation sit next to each other.
  function kindLabel(p) {
    const flags = [];
    if (p.no_services) flags.push("no services");
    if (p.no_tx_relay) flags.push("no tx");
    if (p.chain_unknown) flags.push("chain unknown");
    return flags.length ? p.kind + " \u00b7 " + flags.join(", ") : p.kind;
  }

  // The mix under a heading counts what is in the table - and each count is
  // the way to those peers. Clicking "5 Pool node" shows exactly those five,
  // here and on the map; clicking it again puts everyone back.
  function renderKindMix(el, peers) {
    const n = new Map();
    for (const p of peers) n.set(p.kind, (n.get(p.kind) || 0) + 1);
    const entries = [...n].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
    el.textContent = "";
    entries.forEach(([kind, count], i) => {
      if (i) el.appendChild(document.createTextNode(" \u00b7 "));
      const b = document.createElement("button");
      b.type = "button";
      b.className = "kind-pick" + (state.kind === kind ? " on" : "");
      b.textContent = count + " " + kind;
      b.title = state.kind === kind ? "Show every kind again" : `Show only ${kind}`;
      b.setAttribute("aria-pressed", String(state.kind === kind));
      b.addEventListener("click", () => setKind(kind));
      el.appendChild(b);
    });
  }

  const COLS = [
    {
      key: "addr", label: "Address", cls: "addr", val: (p) => p.addr,
      // With Bitcoin Lab installed the address becomes the way over: same peer,
      // the other question. Without it, a plain cell - no dead links.
      cell: (td, p) => {
        // A long onion or I2P address is cut off by the column, so the full one
        // lives in the title either way.
        td.title = p.addr;
        if (deliveredNow.has(p.addr)) {
          const star = document.createElement("span");
          star.className = "star";
          star.textContent = "\u2605";
          star.title = "Delivered the block that just landed";
          td.appendChild(star);
        }
        if (!state.data || !state.data.sibling) { td.textContent = p.addr; return; }
        const a = document.createElement("a");
        a.className = "peer-jump";
        a.href = siblingURL(p.addr);
        a.target = "_blank";
        a.rel = "noopener";
        a.textContent = p.addr;
        a.title = p.addr + "\nOpen this peer in Bitcoin Lab";
        td.appendChild(a);
      },
    },
    {
      // Shown as "Region, Country", sorted by country first - otherwise
      // Bavaria and Hesse end up on either side of England and the column
      // looks like it cannot do the one thing it obviously should. Peers
      // without a place sort last rather than first, where an empty string
      // would have put them.
      key: "loc", label: "Location", cls: "loc", show: locLabel,
      val: (p) => (p.cc ? regionName(p.cc) + "\u0000" + (p.region || "") : "\uffff"),
    },
    {
      // Where a peer sits and whose machine it is are two questions, and the
      // second is the one that catches three peers in three countries sitting
      // with the same company. Sorting by this column puts them next to each
      // other. Empty for Tor, I2P and anything without a public address.
      key: "operator", label: "Operator", cls: "op", val: (p) => p.operator || "",
      cell: (td, p) => {
        td.textContent = p.operator || "";
        if (p.asn) td.title = "AS" + p.asn;
      },
    },
    { key: "network", label: "Network", val: (p) => netLabel(p.network) },
    { key: "type", label: "Type", val: (p) => typeLabel(p.type), only: "outbound" },
    {
      key: "kind", label: "Kind", val: (p) => p.kind || "", show: kindLabel,
      // Struck through for software that passes no blocks on. It is not a
      // fault: a wallet is doing exactly what a wallet does. It only means this
      // connection was never going to hand your node a block.
      //
      // The user agent itself lives in this cell's title rather than in a
      // column of its own. It is the raw material this class is derived from -
      // worth having, not worth the width, and it was the widest column in the
      // table by a distance.
      cell: (td, p) => {
        td.textContent = kindLabel(p);
        const note = p.no_relay
          ? "Software that does not pass blocks on - this peer can never deliver one first."
          : "";
        td.title = [p.subver, note].filter(Boolean).join("\n");
        if (p.no_relay) td.className = (td.className ? td.className + " " : "") + "norelay";
      },
    },
    { key: "transport", label: "P2P", val: (p) => p.transport || "" },
    { key: "ping", label: "Ping", cls: "num", val: (p) => (p.ping_ms == null ? Infinity : p.ping_ms), show: (p) => (p.ping_ms == null ? "" : Math.round(p.ping_ms) + " ms") },
    { key: "conntime", label: "Connected", cls: "num", val: (p) => p.conntime, show: (p) => ago(Date.now() / 1000 - p.conntime) },
  ];

  function renderTables() {
    const root = $("tables");
    root.textContent = "";
    for (const g of GROUPS) {
      if (!state.show[g]) continue;
      let peers = state.data.peers.filter((p) => p.group === g);
      const all = peers.length;
      if (state.place) peers = peers.filter(peerMatchesPlace);
      const kindPool = peers;
      if (state.kind) peers = peers.filter(matchesKind);

      const card = document.createElement("details");
      card.className = "card group-card";
      card.open = state.open[g];
      card.addEventListener("toggle", () => { state.open[g] = card.open; });
      const summary = document.createElement("summary");
      const h = document.createElement("h2");
      const sw = document.createElement("span");
      sw.className = "swatch s-" + g;
      const n = document.createElement("span");
      n.className = "n";
      n.textContent = state.place || state.kind ? `${peers.length} of ${all}` : String(all);
      h.append(sw, LABEL[g] + " ", n);
      summary.appendChild(h);
      card.appendChild(summary);

      if (kindPool.length) {
        const mix = document.createElement("p");
        mix.className = "kinds";
        // Counted before this group's own kind filter and drawn before the
        // empty check, so a group the filter emptied still offers the way
        // back - and switching from one kind to another is one click.
        renderKindMix(mix, kindPool);
        card.appendChild(mix);
      }

      if (!peers.length) {
        const p = document.createElement("p");
        p.className = "empty";
        p.textContent = state.place || state.kind
          ? "None here."
          : "No " + LABEL[g].toLowerCase() + " peers.";
        card.appendChild(p);
        root.appendChild(card);
        continue;
      }

      const cols = COLS.filter((c) => !c.only || c.only === g);
      const s = state.sort[g] || { key: "conntime", dir: 1 };
      const col = cols.find((c) => c.key === s.key) || cols[cols.length - 1];
      peers.sort((a, b) => {
        const x = col.val(a), y = col.val(b);
        return (x < y ? -1 : x > y ? 1 : 0) * s.dir;
      });

      const wrap = document.createElement("div");
      wrap.className = "scroll";
      const table = document.createElement("table");
      table.className = "peers";
      const thead = document.createElement("thead");
      const hr = document.createElement("tr");
      for (const c of cols) {
        const th = document.createElement("th");
        th.scope = "col";
        if (c.cls) th.className = c.cls;
        if (c.key === col.key) th.setAttribute("aria-sort", s.dir > 0 ? "ascending" : "descending");
        const b = document.createElement("button");
        b.textContent = c.label;
        b.addEventListener("click", () => {
          state.sort[g] = { key: c.key, dir: c.key === col.key ? -s.dir : 1 };
          renderTables();
        });
        th.appendChild(b);
        hr.appendChild(th);
      }
      thead.appendChild(hr);
      table.appendChild(thead);
      const tbody = document.createElement("tbody");
      for (const p of peers) {
        const tr = document.createElement("tr");
        if (state.focus && p.addr === state.focus) tr.className = "focus";
        for (const c of cols) {
          const td = document.createElement("td");
          if (c.cls) td.className = c.cls;
          const text = c.show ? c.show(p) : c.val(p);
          if (c.cell) {
            c.cell(td, p);
          } else {
            td.textContent = text === "" ? "–" : text;
            if (text === "") td.classList.add("dim");
          }
          if (c.key === "addr" || c.key === "loc") td.title = text;
          tr.appendChild(td);
        }
        tbody.appendChild(tr);
      }
      table.appendChild(tbody);
      wrap.appendChild(table);
      card.appendChild(wrap);
      root.appendChild(card);
    }
  }


  // ---------- start ----------
  setupPanZoom();
  loadWorld().then(() => { if (state.data) renderMarkers(); });
  startTicker();
  refresh();
})();
