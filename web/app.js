"use strict";
(() => {
  // This window answers to a name, and links to Bitcoin Lab aim at Bitcoin Lab's name: clicking back and forth between
  // the two apps then reuses one window each instead of opening a new one per click. That needs the opener kept, so
  // these links carry no rel="noopener" - both windows are this Umbrel's own apps.
  window.name = "peer-map";
  // On a phone or tablet Safari loads a named tab in the background and stays where it is, so a tap seemed to do
  // nothing. There the jump stays in the same tab, and Back returns.
  const LAB_TARGET = window.matchMedia("(hover: none) and (pointer: coarse)").matches ? "_self" : "bitcoin-lab";
  const GROUPS = ["inbound", "outbound", "manual"];
  const INBOUND_LIMIT = 10;
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
    lastHash: null,       // newest block hash seen, to pulse the map when it changes
    lastInput: Date.now(),
    // When the current snapshot arrived here. The block's age comes from the
    // node's clock; how long it has been sitting in this browser is added on
    // top, so the two-minute mark never depends on the browser's own clock
    // agreeing with the node's.
    receivedAt: 0,
    liveShown: false,
    showAllInbound: false, // the inbound table shows its top INBOUND_LIMIT until asked for all
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
        // polls again at all - so the star and the row colour take themselves
        // off rather than waiting for the next answer.
        if (state.liveShown && !liveBlock()) render();
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
    "outbound-full-relay": "Full relay",
    "block-relay-only": "Block relay",
  }[t] || t || "-");
  // As Bitcoin Lab writes it: milliseconds below a second, seconds above.
  const fmtPing = (ms) => (ms < 1000 ? Math.round(ms) + " ms" : (ms / 1000).toFixed(2) + " s");
  const netLabel = (n) => ({
    ipv4: "IPv4", ipv6: "IPv6", onion: "Tor", i2p: "I2P", cjdns: "CJDNS",
    not_publicly_routable: "Private", ipv6_hidden: "IPv6 (address hidden)",
    tor_hidden: "Tor (address hidden)", i2p_hidden: "I2P (address hidden)",
  }[n] || n || "-");
  const HIDDEN = {
    ipv6_hidden: { short: "IPv6", tip: "Inbound IPv6 passes through Docker, which puts its own address in place of the peer's." },
    tor_hidden: { short: "Tor", tip: "Reached through the Tor proxy, which hides the peer's address." },
    i2p_hidden: { short: "I2P", tip: "Reached through the I2P router, which hides the peer's address." },
  };
  // A flag from the two-letter country code: regional-indicator letters, no
  // image files. Unknown or missing codes get none.
  const HIDDEN_FLAG = {
    not_publicly_routable: "\u{1F3E0}",
    onion: "\u{1F9C5}", tor_hidden: "\u{1F9C5}",
    i2p: "\u{1F310}", i2p_hidden: "\u{1F310}", ipv6_hidden: "\u{1F310}",
  };
  const flagOf = (cc) => (/^[A-Z]{2}$/.test(cc || "") ? String.fromCodePoint(...[...cc].map((c) => 0x1f1e6 + c.charCodeAt(0) - 65)) : "");
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
  let deliveredEver = new Set();

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
    const own = routeStops(b);
    const typical = medianStops(b);
    const view = state.routeView === "median" && typical ? "median" : "block";
    // The card is open whenever there is any route to show - this block's,
    // or the typical one if this block happens to have no race.
    const stops = own || typical ? (view === "median" ? typical : own || typical) : null;
    const more = $("blockMore");
    const line = stops ? $("blockSummary") : $("blockMoreLabel");
    $("blockSummary").hidden = !stops;
    $("blockRoute").hidden = !stops;
    more.classList.toggle("minor", Boolean(stops));
    fillSummary(line, b);
    if (stops) {
      // This block may have no race of its own while the typical route
      // exists - Stratum Race was off for a moment, say. Then the fold holds
      // what this block does have: who delivered it.
      const entries = (b.stratum && b.stratum.entries) || [];
      $("blockMoreLabel").textContent = entries.length
        ? `All ${entries.length} ${entries.length === 1 ? "pool" : "pools"}`
        : "Delivered first";
      renderRoute($("blockRoute"), b, stops, {
        view: stops === typical ? "median" : "block",
        canSwitch: Boolean(own && typical),
        noRaceHere: !own,
      });
    }
    // "Delivered first" is on the route only when this block's own route is.
    renderBlockBody(b, Boolean(own) && stops === own);
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
    // One peer and a known place: say where it came from, with its flag.
    const one = n === 1 && state.data.peers.find((p) => p.addr === b.first_peers[0]);
    add(null, n === 0 ? "nobody credited" : one && one.cc ? `delivered from ${flagOf(one.cc)} ${regionName(one.cc)}`
      : n === 1 ? "delivered by 1 peer" : `delivered by ${n} peers`,
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
  // Text cut off with an ellipsis is readable on hover - and only then, a
  // tooltip that repeats what is on screen says nothing. Whether it is cut
  // depends on the screen, so it is decided when the pointer gets there.
  // `tip` is what the element says on hover anyway, cut or not.
  function fit(e, full, tip) {
    e.classList.add("fit");
    e.dataset.full = full;
    if (tip) e.dataset.tip = tip;
    return e;
  }
  document.addEventListener("mouseover", (ev) => {
    const e = ev.target.closest && ev.target.closest(".fit");
    if (!e) return;
    const cut = e.scrollWidth > e.clientWidth;
    const link = e.querySelector("a[data-tip]");
    const lines = [cut ? e.dataset.full : "", e.dataset.tip || ""];
    if (link) link.title = [...lines, link.dataset.tip].filter(Boolean).join("\n");
    else e.title = lines.filter(Boolean).join("\n");
  });

  const ROUTE_GAP = 150;   // the closest two stops may sit, so labels never touch

  const hostOf = (a) => (a.startsWith("[") ? a.slice(1, a.indexOf("]")) : a.replace(/:\d+$/, ""));
  const signedMs = (t) => (Math.round(t) === 0 ? "0" : (t > 0 ? "+" : "−") + Math.abs(Math.round(t)) + " ms");

  // Which route the card shows: this block, or the typical one. Remembered
  // per browser, because somebody tuning a setup wants the median every time.
  const ROUTE_VIEW_KEY = "peermap.routeView";
  state.routeView = (() => { try { return localStorage.getItem(ROUTE_VIEW_KEY) || "block"; } catch { return "block"; } })();
  function setRouteView(v) {
    state.routeView = v;
    try { localStorage.setItem(ROUTE_VIEW_KEY, v); } catch { /* private window: fine */ }
    renderBlockCard();
  }

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
    // The ping Bitcoin Lab took from the snapshot that credited the peer is
    // the right one - it is from the moment the block arrived. Only a block
    // from before the Lab stored it falls back to what Core says now.
    const pingOf = (c) => (c.peer ? (c.peer.min_ping_ms != null ? c.peer.min_ping_ms : c.peer.ping_ms) : null);
    const lead = credited.filter((c) => pingOf(c) != null).sort((x, y) => pingOf(x) - pingOf(y))[0]
      || (credited.length && b.first_ping_ms != null ? credited[0] : null);
    const ping = b.first_ping_ms != null ? b.first_ping_ms : (lead ? pingOf(lead) : null);
    if (lead && ping != null) {
      const p = lead.peer;
      const where = p ? p.region || (p.cc ? regionName(p.cc) : "") : "";
      const more = credited.length > 1 ? ` +${credited.length - 1}` : "";
      stops.push({
        kind: "peer", group: p ? p.group : "", t: s.core_ms - ping,
        name: hostOf(lead.addr) + more,
        sub: p ? [where, p.operator].filter(Boolean) : ["no longer connected"],
        title: credited.map((c) => c.addr).join("\n"),
      });
    }
    stops.push({
      kind: "core", t: s.core_ms, name: "Core",
      // Credited, but with no ping to place it by: named rather than drawn.
      sub: [!(lead && ping != null) && credited.length ? "via " + hostOf(credited[0].addr) : "your node"],
    });
    if (b.template_ms != null) {
      stops.push({ kind: "tpl", t: s.core_ms + b.template_ms, name: "Template", sub: ["ready in Core"], tx: b.template_tx });
    }
    if (own) stops.push({ kind: "own", t: own.latency_ms, name: own.label, sub: ["your job"] });
    for (const st of stops) if (typeof st.sub === "string") st.sub = [st.sub];
    return byTime(stops);
  }

  // Core's template time is kept in whole milliseconds, the pool's job to the
  // fraction, and with small templates the job is ready the same moment:
  // rounding alone can put the job before the template it was built from.
  // It cannot be, so the template waits for it, and the job stretch - the
  // one that says how many transactions the template carried - stays.
  function byTime(stops) {
    const tpl = stops.find((s) => s.kind === "tpl");
    const own = stops.find((s) => s.kind === "own");
    if (tpl && own && own.t < tpl.t) tpl.t = own.t;
    return stops.sort((x, y) => x.t - y.t);
  }

  // The typical block: every stop at its median over the last hundred, each
  // counted from its own first job. Stops are medians of their own, so they
  // are not one real block - they are where each stop usually is, which is
  // the thing that moves when something is changed.
  function medianStops(b) {
    const m = b.route_median;
    if (!m || !m.core) return null;
    const n = (st) => `median of ${st.n} ${st.n === 1 ? "block" : "blocks"}`;
    const stops = [{ kind: "win", t: 0, name: "First pool", sub: ["first job"] }];
    if (m.peer) stops.push({ kind: "peer", t: m.peer.ms, name: "Your peer", sub: ["has the block"], title: n(m.peer), plain: true });
    stops.push({ kind: "core", t: m.core.ms, name: "Core", sub: ["your node"], title: n(m.core) });
    if (m.template) stops.push({ kind: "tpl", t: m.template.ms, name: "Template", sub: ["ready in Core"], title: n(m.template), tx: m.tx && m.tx.ms });
    if (m.own) stops.push({ kind: "own", t: m.own.ms, name: m.own_label || "Your pool", sub: ["your job"], title: n(m.own) });
    return byTime(stops);
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
    if (a.kind === "peer" && z.kind === "core") return ["~" + d + " · ping"];
    if (a.kind === "win" && z.kind === "peer") return ["reaches your peer · " + d];
    if (a.kind === "core" && z.kind === "tpl") {
      return [d + " · template", "Core builds a new block template (getblocktemplate)"];
    }
    if (a.kind === "tpl" && z.kind === "own") {
      // The template's size is most of the reason this stretch varies: the
      // pool handles every transaction in it before it can send the job.
      const tx = a.tx != null ? " · " + Math.round(a.tx).toLocaleString("en-US") + " tx" : "";
      return [d + " · job" + tx,
        "Your pool notices the block and turns the template into a job" +
        (tx ? " - the more transactions the template carries, the longer that takes" : "")];
    }
    // No template timing for this block: the two are one stretch.
    if (a.kind === "core" && z.kind === "own") {
      return [d + " · template + job",
        "Core builds a new block template (getblocktemplate), your pool notices the block and turns it into a job"];
    }
    return [d];
  }

  function renderRoute(box, b, stops, { view, canSwitch, noRaceHere }) {
    box.textContent = "";
    const median = view === "median";
    const n = (b.route_median && b.route_median.blocks) || 0;
    if (canSwitch) {
      const sw = document.createElement("div");
      sw.className = "rt-switch";
      for (const [v, label] of [["block", "This block"], ["median", `Typical \u00b7 last ${n}`]]) {
        const btn = document.createElement("button");
        btn.type = "button";
        btn.textContent = label;
        btn.className = v === view ? "on" : "";
        btn.addEventListener("click", () => setRouteView(v));
        sw.appendChild(btn);
      }
      box.appendChild(sw);
    }
    const miner = median ? "Miner" : b.pool || (b.pool_tag ? `"${b.pool_tag}"` : "unknown miner");
    const W = box.clientWidth - 12;
    // The last label is centred on its dot like the others, so the dot keeps
    // room for half of it - at the edge it looked squeezed.
    const left = ROUTE_STUB, right = W - 60;

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
          if (c === "miss" && v) td.title = v;
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
      // A raw coinbase text can be long; it stops short of the first stop's
      // label instead of running underneath it.
      const minerLab = el("rt-lab pool first", 0, [fit(txt("b", miner), miner), txt("small", "mined it")]);
      minerLab.style.maxWidth = (xs[0] - 50) + "px";

      stops.forEach((s, i) => {
        el("rt-dot " + s.kind + (s.group ? " " + s.group : ""), xs[i]);
        const name = fit(txt("b", s.name), s.name, s.title);
        const lab = el("rt-lab " + s.kind + (s.plain ? " plain" : ""), xs[i],
          [name, txt("span", signedMs(s.t), "t"), ...s.sub.map((line) => txt("small", line))]);
        if (s.title) lab.title = s.title;
      });

      // Stretches run between the stations the block actually passes -
      // peer, Core, template, your pool. The first job is the zero the times
      // are counted from, not a station: when the peer had the block before
      // any pool sent a job, the zero falls inside the ping, and splitting the
      // ping there would only produce two pieces nobody can read. So the zero
      // keeps its dot and its label below, and the stretch above it stays
      // whole. Only when the zero comes before the peer is there a stretch
      // from it to the peer.
      const at = (st) => xs[stops.indexOf(st)];
      const seg = (a, z) => {
        const [label, why] = segLabel(a, z);
        const e = el("rt-seg", (at(a) + at(z)) / 2, [txt("span", label)]);
        if (why) e.title = why;
      };
      const journey = stops.filter((st) => st.kind !== "win");
      const win = stops.find((st) => st.kind === "win");
      if (win && journey.length && win.t <= journey[0].t) seg(win, journey[0]);
      for (let i = 1; i < journey.length; i++) seg(journey[i - 1], journey[i]);
    }
    const note = document.createElement("p");
    note.className = "note";
    note.textContent = ((noRaceHere ? "No race was recorded for this block, so this is the typical route. " : "")
      + (median
      ? `Each stop is its median over the last ${n} blocks, counted from that block's first job. `
      : "0 is the first new job any pool sent here. When the block was found cannot be measured; "
        + "the hop into your node is estimated from the peer's ping.")
      ).trim();
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

  // ---------- render ----------
  const matchesKind = (p) => !state.kind || p.kind === state.kind;
  const visiblePeers = () =>
    (state.data ? state.data.peers.filter((p) => state.show[p.group] && matchesKind(p)) : []);
  const regionMode = () => state.view.k >= REGION_ZOOM;

  function render() {
    // Read before anything changes: rebuilding the tables makes the page
    // shorter for a moment, and Safari clamps the scroll position to it.
    const y = window.scrollY;
    const d = state.data;
    $("demo").hidden = !d.demo;
    showError(d.error ? "Bitcoin Core: " + d.error + (d.fetched_at ? " - showing the last good peer list." : "") : "");
    $("version").textContent = "v" + (d.version || "dev");

    const counts = { manual: 0, inbound: 0, outbound: 0 };
    for (const p of d.peers) counts[p.group]++;
    for (const g of GROUPS) document.querySelector(`[data-count="${g}"]`).textContent = counts[g];

    renderStatus();
    deliveredNow = deliveredSet();
    deliveredEver = new Set((d.last_block && d.last_block.delivered_ever) || []);
    renderSiblingLink();
    state.liveShown = Boolean(liveBlock()) || deliveredNow.size > 0;
    renderBlockCard();
    renderMarkers();
    renderUnplaced();
    renderTables();
    if (window.scrollY !== y) window.scrollTo(0, y);
    focusOnce();
    const hash = d.last_block && d.last_block.hash;
    if (hash && state.lastHash && hash !== state.lastHash) pulseBlock();
    if (hash) state.lastHash = hash;
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
  // store manifest publishes (or PEERMAP_SIBLING_LINK_PORT, passed on as
  // sibling_port, for a test store), and the host is this one.
  const SIBLING_PORT = 8790;
  function siblingURL(peerAddress) {
    const port = (state.data && state.data.sibling_port) || SIBLING_PORT;
    const base = `${location.protocol}//${location.hostname}:${port}/`;
    return peerAddress ? base + "?peer=" + encodeURIComponent(peerAddress) : base;
  }
  function renderSiblingLink() {
    const a = $("sibling-link");
    if (a) a.target = LAB_TARGET;
    if (!a || !state.data) return;
    a.hidden = !state.data.sibling;
    if (state.data.sibling) a.href = siblingURL(null);
  }

  // Nodes are what passes blocks on (every "Node (...)" kind and pool nodes),
  // disguised is everything pretending, LinkingLion included; the rest -
  // wallets, crawlers, indexers, unknown - are "others", so the three add up.
  // Named once, in the tooltip and the Disguised explanation, not on every card.
  const LION_CREDIT = "Tracked by 0xB10C, b10c.me";
  const isNode = (p) => p.kind.startsWith("Node (") || p.kind === "Pool node";
  function renderStatus() {
    const d = state.data;
    if (!d) return;
    const since = d.fetched_at ? ago(Date.now() / 1000 - d.fetched_at) + " ago" : "never";
    const mode = state.paused ? "paused" : `every ${d.interval}s while open`;
    const disguised = d.peers.filter((p) => p.kind === "Disguised" || p.linking_lion).length;
    const nodes = d.peers.filter((p) => !(p.kind === "Disguised" || p.linking_lion) && isNode(p)).length;
    const others = d.peers.length - nodes - disguised;
    const el = $("status");
    el.textContent = `${d.peers.length} peers: ${nodes} nodes, ${disguised} disguised, ${others} others`;
    el.append(` · updated ${since} · ${mode}`);
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
    buildDots();
    applyView();
  }

  // The world view is drawn as a grid of dots rather than filled countries:
  // calmer, and the markers stand out against it. The grid is worked out once,
  // in the browser, by painting every country into an offscreen canvas in its
  // own colour and reading back which country each grid point falls on - so a
  // country with peers can light up its own dots. Zoomed in, the dots fade out
  // and the country shapes fade in (see applyView): a dot grid has nothing to
  // say about regions.
  const DOT_STEP = 4.4;
  let dotsByCC = null;
  function buildDots() {
    const W = state.world.w, H = state.world.h, S = 2;
    const off = document.createElement("canvas");
    off.width = W * S; off.height = H * S;
    const c = off.getContext("2d", { willReadFrequently: true });
    if (!c || typeof Path2D !== "function") return;
    c.scale(S, S);
    const ccs = Object.keys(state.world.paths);
    ccs.forEach((cc, i) => {
      c.fillStyle = `rgb(${(i + 1) & 255},${(i + 1) >> 8},7)`;
      c.fill(new Path2D(state.world.paths[cc]));
    });
    if (state.world.unnamed) { c.fillStyle = "rgb(0,0,9)"; c.fill(new Path2D(state.world.unnamed)); }
    const img = c.getImageData(0, 0, off.width, off.height).data;
    dotsByCC = new Map();
    let all = "";
    for (let y = DOT_STEP / 2, row = 0; y < H; y += DOT_STEP, row++) {
      for (let x = (row % 2) * DOT_STEP / 2; x < W; x += DOT_STEP) {
        const o = (Math.round(y * S) * off.width + Math.round(x * S)) * 4;
        if (img[o + 3] < 200) continue;
        const seg = `M${x.toFixed(1)} ${y.toFixed(1)}h0`;
        all += seg;
        const i = img[o] + (img[o + 1] << 8) - 1;
        if (img[o + 2] === 7 && i >= 0 && ccs[i]) dotsByCC.set(ccs[i], (dotsByCC.get(ccs[i]) || "") + seg);
      }
    }
    const grid = document.createElementNS(NS, "g");
    grid.setAttribute("class", "dots");
    for (const [id, d] of [["dotsAll", all], ["dotsHot", ""]]) {
      const p = document.createElementNS(NS, "path");
      p.id = id;
      p.setAttribute("d", d);
      grid.appendChild(p);
    }
    $("countries").appendChild(grid);
    $("map").classList.add("dotted");
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

  function arcStroke(r, a0, a1) {
    if (a1 <= a0) a1 = a0 + 0.01;
    const x0 = r * Math.sin(a0), y0 = -r * Math.cos(a0);
    const x1 = r * Math.sin(a1), y1 = -r * Math.cos(a1);
    return `M${x0.toFixed(2)} ${y0.toFixed(2)}A${r} ${r} 0 ${a1 - a0 > Math.PI ? 1 : 0} 1 ${x1.toFixed(2)} ${y1.toFixed(2)}`;
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
      g.dataset.key = b.key;
      g.setAttribute("tabindex", "0");
      g.setAttribute("role", "button");
      g.setAttribute("aria-label", `${b.title}${b.sub ? ", " + b.sub : ""}: ${GROUPS.map((k) => b[k] + " " + LABEL[k].toLowerCase()).join(", ")}`);

      // A light, not a pie: the colour is the place's leading group - manual
      // whenever it holds one of yours - and the split is in the tooltip.
      const lead = leadGroup(b);
      const glow = document.createElementNS(NS, "circle");
      glow.setAttribute("class", "glow");
      glow.setAttribute("r", r * 1.9);
      glow.setAttribute("fill", `url(#glow-${lead})`);
      g.appendChild(glow);
      const core = r * 0.38 + 1.4;
      const ring = document.createElementNS(NS, "circle");
      ring.setAttribute("class", "ring");
      ring.setAttribute("r", core + 4.2);
      g.appendChild(ring);
      // Selection gets its own outline, so a selected place that also
      // delivered the block keeps its orange ring.
      const sel = document.createElementNS(NS, "circle");
      sel.setAttribute("class", "sel");
      sel.setAttribute("r", core + 6.6);
      g.appendChild(sel);
      const dot = document.createElementNS(NS, "circle");
      dot.setAttribute("class", "core c-" + lead);
      dot.setAttribute("r", core);
      g.appendChild(dot);
      // The split between the three groups, as the pie used to show it: a
      // thin ring around the light, one arc per group, a small gap between.
      // The gap never eats a small group: at most half its own arc.
      const minSpan = Math.min(...GROUPS.filter((k) => b[k]).map((k) => (b[k] / b.total) * Math.PI * 2));
      const rr = core + 1.8, gap = b.total > 1 ? Math.min(0.32, minSpan / 2) : 0;
      let a = 0;
      for (const k of GROUPS) {
        if (!b[k]) continue;
        const span = (b[k] / b.total) * Math.PI * 2;
        const s = document.createElementNS(NS, "path");
        s.setAttribute("class", "seg seg-" + k);
        s.setAttribute("d", span >= Math.PI * 2 - 1e-6
          ? `M${-rr} 0A${rr} ${rr} 0 1 1 ${rr} 0A${rr} ${rr} 0 1 1 ${-rr} 0`
          : arcStroke(rr, a + gap / 2, a + span - gap / 2));
        g.appendChild(s);
        a += span;
      }
      if (b.total > 1) {
        const t = document.createElementNS(NS, "text");
        t.setAttribute("x", core + 5);
        t.setAttribute("y", -core);
        t.textContent = b.total;
        g.appendChild(t);
      }
      if (b.delivered) {
        const star = document.createElementNS(NS, "text");
        star.setAttribute("class", "delivered-star");
        star.setAttribute("x", 0);
        star.setAttribute("y", -(core + 6));
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
    const hotDots = $("dotsHot");
    if (hotDots && dotsByCC) hotDots.setAttribute("d", [...hot].map((cc) => dotsByCC.get(cc) || "").join(""));
    renderLinks(bs);
    placeMarkers();
  }

  const leadGroup = (b) => (b.manual ? "manual" : b.outbound > b.inbound ? "outbound" : "inbound");

  // This node itself, from the region its peers see it in (the server reads
  // getpeerinfo's addrlocal and sends only the region, never the address).
  function homePoint() {
    const h = state.data && state.data.home;
    return h ? project(h.lon, h.lat) : null;
  }

  // A thin arc from every place to this node. They make the map read as what
  // it is - your node's connections - and they are the track a new block
  // travels along (pulseBlock).
  let linkByKey = new Map();
  function arcPath(a, b) {
    const dx = b[0] - a[0], dy = b[1] - a[1];
    const d = Math.hypot(dx, dy);
    const lift = Math.min(90, d * 0.28);
    const cx = (a[0] + b[0]) / 2, cy = (a[1] + b[1]) / 2 - lift;
    return `M${a[0].toFixed(1)} ${a[1].toFixed(1)}Q${cx.toFixed(1)} ${cy.toFixed(1)} ${b[0].toFixed(1)} ${b[1].toFixed(1)}`;
  }
  function renderLinks(bs) {
    const layer = $("links");
    layer.textContent = "";
    linkByKey = new Map();
    const home = homePoint();
    if (!home) return;
    for (const b of bs) {
      if (Math.hypot(b.pt[0] - home[0], b.pt[1] - home[1]) < 2) continue;
      const p = document.createElementNS(NS, "path");
      p.setAttribute("class", "link l-" + leadGroup(b));
      p.setAttribute("d", arcPath(b.pt, home));
      p.setAttribute("vector-effect", "non-scaling-stroke");
      layer.appendChild(p);
      linkByKey.set(b.key, p);
    }
    const g = document.createElementNS(NS, "g");
    g.setAttribute("class", "home-mark");
    g.dataset.x = home[0];
    g.dataset.y = home[1];
    const halo = document.createElementNS(NS, "circle");
    halo.setAttribute("r", 11);
    halo.setAttribute("fill", "url(#glow-home)");
    const dot = document.createElementNS(NS, "circle");
    dot.setAttribute("r", 3.2);
    dot.setAttribute("class", "home-dot");
    const t = document.createElementNS(NS, "title");
    t.textContent = "Your node";
    g.append(halo, dot, t);
    $("markers").appendChild(g);
  }

  // A new block: a light runs from the place that delivered it along its arc
  // to this node, rings spread where it lands, and the wave rolls across the
  // page. Only for a block that arrives while the page is open - not on the
  // first load - and not at all for someone who asked for less motion.
  const motionQuery = window.matchMedia ? window.matchMedia("(prefers-reduced-motion: reduce)") : null;
  function pulseBlock() {
    if ((motionQuery && motionQuery.matches) || document.hidden || !state.world) return;
    const wave = $("blockWave");
    wave.classList.remove("run"); void wave.offsetWidth; wave.classList.add("run");
    const home = homePoint();
    const fx = $("fx");
    const svg = $("map");
    // Sizes in screen pixels: the fx layer is scaled with the map.
    const px = (n) => (n * state.world.w) / (svg.clientWidth || 1000) / state.view.k;
    const from = [...$("markers").querySelectorAll(".marker.delivered")].map((g) => linkByKey.get(g.dataset.key)).filter(Boolean);
    const land = () => {
      if (!home) return;
      for (let i = 0; i < 3; i++) ring(home, i * 220);
    };
    const ring = (pt, delay) => {
      const c = document.createElementNS(NS, "circle");
      c.setAttribute("class", "fx-ring");
      c.setAttribute("cx", pt[0]);
      c.setAttribute("cy", pt[1]);
      c.setAttribute("vector-effect", "non-scaling-stroke");
      fx.appendChild(c);
      const t0 = performance.now() + delay;
      const step = (now) => {
        const k = Math.max(0, (now - t0) / 1500);
        c.setAttribute("r", px(3 + 70 * (1 - Math.pow(1 - Math.min(1, k), 3))));
        c.style.opacity = String(Math.max(0, 1 - k));
        if (k < 1) requestAnimationFrame(step); else c.remove();
      };
      requestAnimationFrame(step);
    };
    if (!from.length) { land(); return; }
    let left = from.length;
    for (const path of from) {
      const len = path.getTotalLength();
      const head = document.createElementNS(NS, "circle");
      head.setAttribute("class", "fx-head");
      fx.appendChild(head);
      // Without non-scaling-stroke: the dash lengths must be in the same units
      // as getTotalLength(), so the width is scaled by hand instead.
      const trail = path.cloneNode();
      trail.removeAttribute("vector-effect");
      trail.setAttribute("class", "fx-trail");
      trail.style.strokeWidth = String(px(2.2));
      fx.appendChild(trail);
      const t0 = performance.now();
      const step = (now) => {
        const k = Math.min(1, (now - t0) / 950), e = 1 - Math.pow(1 - k, 2.2), d = e * len;
        const pt = path.getPointAtLength(d);
        head.setAttribute("cx", pt.x); head.setAttribute("cy", pt.y); head.setAttribute("r", px(2.6));
        trail.setAttribute("stroke-dasharray", `${len * 0.22} ${len}`);
        trail.setAttribute("stroke-dashoffset", String(-(d - len * 0.22)));
        if (k < 1) { requestAnimationFrame(step); return; }
        head.remove(); trail.remove();
        if (--left === 0) land();
      };
      requestAnimationFrame(step);
    }
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
    $("kindFilterWhy").textContent = state.kind && KIND_WHY[state.kind] ? "- " + KIND_WHY[state.kind] : "";
  }

  // Why a peer lands in a kind, in words - the rule from kindRules in
  // kinds.go. Shown beside the kind filter, because "Only Pool node" says
  // what is left but not what put it there. kinds_test.go checks that every
  // kind there has a line here.
  const KIND_WHY = {
    "Pool node": 'its user agent names ckpool software (ckpool, ckp2p, ckcoind), or has "pool" in brackets (not "mempool")',
    "Other chain": "its user agent names another chain (Bitcoin Cash clients such as ABC or Classic, Bitcoin SV, XT, and other forks)",
    "Research scanner": "its user agent names a university or research network",
    "Crawler": "its user agent says crawl, scan, seeder, census, monitor, watch, listener, bitnodes or similar",
    "Indexer": "its user agent says electrs, electrum or esplora, or contains /mempool",
    "Wallet": "its user agent names wallet software (bitcoinj, Wasabi, ...)",
    "Light client": "its user agent says neutrino or Kyoto (Rust BIP-157)",
    "Node (Floresta)": "its user agent says Floresta",
    "Disguised": "it claims software it is not: Core misspelt (Sat0shi, Satoshi2, no slashes), a version offering what that version never had (v2 before Core 26), lacking what it must always offer, or speaking a protocol it never spoke, Core offering nothing, a wallet offering blocks or speaking a newer protocol than it can, or LinkingLion under a borrowed name (tracked by 0xB10C, b10c.me)",
    "Node (Knots)": "its user agent says Knots",
    "Node (Core)": 'its user agent starts with "/Satoshi:"',
    "Other": "its user agent matches none of the known kinds",
    "No user agent": "it has sent no user agent, or an empty one - usually a port check, a connection still starting, or a tool keeping quiet",
    "Node (btcd)": "its user agent says btcd",
    "Node (libbitcoin)": "its user agent says libbitcoin",
    "Node (bcoin)": "its user agent says bcoin",
    "Node (Gocoin)": "its user agent says Gocoin",
  };

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
    for (const id of ["countries", "links", "fx"]) $(id).setAttribute("transform", `translate(${x} ${y}) scale(${k})`);
    // Dots at the world view, country shapes once zoomed in towards regions.
    $("map").style.setProperty("--dots", String(Math.max(0, Math.min(1, (REGION_ZOOM + 0.5 - k) / 1.5))));
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
    let routeResize = null, routeWidth = window.innerWidth;
    window.addEventListener("resize", () => {
      if (window.innerWidth === routeWidth) return;
      routeWidth = window.innerWidth;
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
    entries.forEach(([kind, count]) => {
      const b = document.createElement("button");
      b.type = "button";
      b.className = "chip kind-pick" + (state.kind === kind ? " on" : "");
      const i = document.createElement("i");
      i.textContent = count;
      b.append(i, " " + kind);
      b.title = state.kind === kind ? "Show every kind again" : `Show only ${kind}`;
      b.setAttribute("aria-pressed", String(state.kind === kind));
      b.addEventListener("click", () => setKind(kind));
      el.appendChild(b);
    });
  }

  // How spread out a group is: distinct operators and countries. Operators by
  // the name in the Operator column, not by ASN: a large company runs several
  // ASNs (Amazon has AS16509 and AS14618) and should still count once. Peers
  // that cannot be placed - Tor, I2P, private addresses - have no country, and
  // are named instead of being silently left out. Your Umbrel's own network
  // has an operator line but is no provider, so it is not counted as one.
  function spreadText(peers) {
    const ops = new Set(), ccs = new Set();
    let unplaced = 0;
    for (const p of peers) {
      if ((p.operator || p.asn) && p.network !== "not_publicly_routable") ops.add(p.operator || "AS" + p.asn);
      if (p.cc) ccs.add(p.cc);
      else unplaced++;
    }
    const parts = [];
    if (ops.size) parts.push(ops.size + (ops.size === 1 ? " provider" : " providers"));
    if (ccs.size) parts.push(ccs.size + (ccs.size === 1 ? " country" : " countries"));
    if (unplaced) parts.push(unplaced + " not placed");
    return parts.join(" \u00b7 ");
  }

  const TRANSPORT_TITLE = {
    v1: "Unencrypted - the original P2P protocol",
    v2: "Encrypted (BIP324) - an observer on the line cannot read what passes between the nodes",
  };

  // Two lines per peer, as in the approved design: what a row is about on
  // top, the detail under it - the software, the AS number, the address -
  // so nothing needs a pointer to be read.
  const placeTitle = (p) => p.network === "not_publicly_routable" ? (p.app || "This Umbrel")
    : p.region || (p.cc ? regionName(p.cc) : netLabel(p.network));
  const neverNote = (p) => !p.no_relay ? ""
    : p.no_services ? "Offers no services, so it has no blocks to pass on - it can never deliver one first."
    : "Software that does not pass blocks on - this peer can never deliver one first.";
  const line = (cls, text) => {
    const e = document.createElement("small");
    e.className = cls;
    e.textContent = text;
    return e;
  };

  const COLS = [
    {
      // Where it is on top, the country and the address under it. Sorted by
      // country first - otherwise Bavaria and Hesse end up on either side of
      // England. Peers without a place sort last.
      key: "peer", label: "Peer", cls: "peer",
      val: (p) => (p.cc ? regionName(p.cc) + "\u0000" + (p.region || "") : "\uffff" + p.addr),
      cell: (td, p) => {
        const who = document.createElement("div");
        who.className = "who";
        const f = document.createElement("span");
        f.className = "flag";
        f.textContent = HIDDEN_FLAG[p.network] || flagOf(p.cc);
        const text = document.createElement("div");
        text.className = "who-text";
        const title = placeTitle(p);
        const b = fit(document.createElement("b"), title);
        b.textContent = title;
        if (HIDDEN[p.network]) b.dataset.tip = HIDDEN[p.network].tip;
        const sub = document.createElement("small");
        if (p.region && p.cc) sub.append(regionName(p.cc) + " ");
        // What the row colour means, said where the peer is named rather
        // than over every cell of the row.
        const got = deliveredNow.has(p.addr) ? "Delivered the newest block first"
          : deliveredEver.has(p.addr) ? "Has delivered a block first (Bitcoin Lab)" : "";
        fit(sub, p.addr, got);
        if (deliveredNow.has(p.addr)) {
          const star = document.createElement("span");
          star.className = "star";
          star.textContent = "\u2605";
          star.title = "Delivered the block that just landed";
          sub.appendChild(star);
        }
        // With Bitcoin Lab installed the address is the way over: same peer,
        // the other question. Without it, plain text - no dead links.
        if (state.data && state.data.sibling) {
          const a = document.createElement("a");
          a.className = "peer-jump addr-txt";
          a.href = siblingURL(p.addr);
          a.target = LAB_TARGET;
          a.textContent = p.addr;
          a.dataset.tip = "Open this peer in Bitcoin Lab";
          a.title = [got, a.dataset.tip].filter(Boolean).join("\n");
          sub.appendChild(a);
        } else {
          const a = document.createElement("span");
          a.className = "addr-txt";
          a.textContent = p.addr;
          sub.appendChild(a);
        }
        text.append(b, sub);
        who.append(f, text);
        td.appendChild(who);
      },
    },
    {
      // Whose machine it is, with the AS number under it. Sorting by this
      // column catches three peers in three countries with the same company.
      key: "operator", label: "Operator", cls: "op", val: (p) => p.operator || "\uffff",
      cell: (td, p) => {
        if (!p.operator && !p.asn) { td.textContent = "–"; td.classList.add("dim"); return; }
        const name = fit(document.createElement("span"), p.operator || "");
        name.className += " op-name";
        name.textContent = p.operator || "";
        td.appendChild(name);
        if (p.asn) td.appendChild(line("sub", "AS" + p.asn));
      },
    },
    {
      // What it calls itself and what Core saw, as a pill; the software it
      // announced under it. A peer that can never deliver a block first is
      // dimmed and says so in the pill - not a fault, a wallet is doing what
      // a wallet does.
      key: "kind", label: "Kind", val: (p) => p.kind || "",
      cell: (td, p) => {
        const pill = document.createElement("span");
        // Green for every peer that can deliver a block first, grey for those
        // that never can, red for a disguise or LinkingLion even when it could.
        // LinkingLion has its own pill whatever software it claims; the
        // software stays in the line under it.
        pill.className = "kind " + (p.linking_lion ? "lion" : p.kind === "Disguised" ? "disg" : p.no_relay ? "never" : "first");
        // The pill says the kind and nothing else, for every peer: what Core
        // saw (no services, no tx, chain unknown) goes to the line under it.
        // One layout in every row, and the pill is never cut.
        const flags = kindLabel(p).slice(p.kind.length).replace(/^ \u00b7 /, "");
        const never = p.no_relay ? " \u00b7 never first" : "";
        const label = (p.linking_lion ? "LinkingLion" : p.kind) + never;
        if (p.no_relay) {
          const ico = document.createElement("span");
          ico.className = "ico";
          ico.textContent = "⊘";
          pill.append(ico, " ");
        }
        pill.append(label);
        fit(pill, label, [p.about, p.linking_lion ? LION_CREDIT : "", neverNote(p)].filter(Boolean).join("\n"));
        td.appendChild(pill);
        // What Core saw stays grey; the name the peer sends is bright, it is
        // what tells one "Other" from the next. The user's own apps on this
        // Umbrel need no flags: nothing about them is a surprise.
        const seen = p.app ? "" : flags;
        const under = [seen, p.subver].filter(Boolean).join(" \u00b7 ");
        if (under) {
          const e = line("sub ua", seen && p.subver ? seen + " \u00b7 " : seen);
          if (p.subver) {
            const name = document.createElement("span");
            name.className = "ua-name";
            name.textContent = p.subver;
            e.append(name);
          }
          td.appendChild(fit(e, under));
        }
        // With a kind filter on, the question is why these peers are in it:
        // the description is shown under the kind, only then.
        if (state.kind && p.about) {
          const about = document.createElement("div");
          about.className = "about";
          about.textContent = p.about;
          td.appendChild(about);
        }
      },
    },
    {
      // Network and P2P version as two small tags; outbound also its type.
      key: "net", label: "Net", val: (p) => netLabel(p.network) + (p.transport || ""),
      cell: (td, p) => {
        const tag = (text, title, cls) => {
          const e = document.createElement("span");
          e.className = cls ? "net " + cls : "net";
          e.textContent = text;
          if (title) e.title = title;
          td.appendChild(e);
        };
        tag(HIDDEN[p.network] ? HIDDEN[p.network].short : netLabel(p.network) || "–",
          HIDDEN[p.network] ? HIDDEN[p.network].tip : "");
        if (p.transport) tag(p.transport, TRANSPORT_TITLE[p.transport] || "");
        if (p.group === "outbound" && p.type) tag(typeLabel(p.type), "", "type");
      },
    },
    {
      key: "ping", label: "Ping", cls: "num", val: (p) => (p.ping_ms == null ? Infinity : p.ping_ms),
      cell: (td, p) => {
        if (p.ping_ms == null) { td.textContent = "–"; td.classList.add("dim"); return; }
        td.textContent = fmtPing(p.ping_ms);
      },
    },
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

      const card = document.createElement("section");
      card.className = "card group-card";
      card.id = "group-" + g;
      const summary = document.createElement("div");
      summary.className = "group-head";
      const h = document.createElement("h2");
      const sw = document.createElement("span");
      sw.className = "swatch s-" + g;
      h.append(sw, LABEL[g]);
      // The count and the mix of kinds as chips beside the name. Counted
      // before this group's own kind filter, so a group the filter emptied
      // still offers the way back - and switching kinds is one click.
      const mix = document.createElement("span");
      mix.className = "kinds chips";
      const n = document.createElement("span");
      n.className = "chip count";
      const ni = document.createElement("i");
      ni.textContent = state.place || state.kind ? `${peers.length} of ${all}` : String(all);
      n.append(ni, " peers");
      mix.appendChild(n);
      if (kindPool.length) renderKindMix(mix, kindPool);
      summary.append(h, mix);
      card.appendChild(summary);

      if (peers.length) {
        const spread = document.createElement("p");
        spread.className = "kinds spread";
        spread.textContent = spreadText(peers);
        card.appendChild(spread);
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

      const cols = COLS;
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
        th.className = "k-" + c.key + (c.cls ? " " + c.cls : "");
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
      // Inbound runs to a couple of hundred rows and pushed Outbound and Manual
      // far down the page, so it shows its top ten - in the order of the
      // current sort - with a toggle for the rest, as Bitcoin Lab's ranking
      // does. The others never get long. A peer Bitcoin Lab pointed at opens
      // the full list once, or its row would not exist to land on.
      let shown = peers;
      const limited = g === "inbound" && peers.length > INBOUND_LIMIT;
      if (limited) {
        if (state.focus && !state.focusDone && !state.showAllInbound
            && peers.slice(INBOUND_LIMIT).some((p) => p.addr === state.focus)) state.showAllInbound = true;
        if (!state.showAllInbound) shown = peers.slice(0, INBOUND_LIMIT);
      }
      const tbody = document.createElement("tbody");
      for (const p of shown) {
        const tr = document.createElement("tr");
        if (state.focus && p.addr === state.focus) tr.className = "focus";
        // The peer that brought the newest block, for as long as the star
        // is up - one rule for "just delivered", in the row and on the map.
        else if (deliveredNow.has(p.addr)) {
          tr.className = "delivered-now";
        }
        // A connection that has brought this node a block, at least once: its address in green, also on an orange or
        // focused row (as in Bitcoin Lab).
        if (deliveredEver.has(p.addr) || deliveredNow.has(p.addr)) tr.classList.add("delivered-ever");
        if (p.no_relay) tr.classList.add("weak");
        for (const c of cols) {
          const td = document.createElement("td");
          if (c.cls) td.className = c.cls;
          td.classList.add("k-" + c.key);
          td.dataset.label = c.label;
          if (c.cell) {
            c.cell(td, p);
          } else {
            const text = c.show ? c.show(p) : c.val(p);
            td.textContent = text === "" ? "–" : text;
            if (text === "") td.classList.add("dim");
          }
          tr.appendChild(td);
        }
        tbody.appendChild(tr);
      }
      table.appendChild(tbody);
      wrap.appendChild(table);
      card.appendChild(wrap);
      if (limited) {
        const more = document.createElement("button");
        more.type = "button";
        more.className = "more";
        more.textContent = state.showAllInbound ? `Show top ${INBOUND_LIMIT} only` : `Show all ${peers.length}`;
        more.addEventListener("click", () => {
          const collapsing = state.showAllInbound;
          state.showAllInbound = !state.showAllInbound;
          renderTables();
          // Collapsing takes away most of the page under the reader; bring
          // the card back into view if it went off the top.
          const c = $("group-inbound");
          if (collapsing && c && c.getBoundingClientRect().top < 0) c.scrollIntoView({ block: "start", behavior: "smooth" });
        });
        card.appendChild(more);
      }
      root.appendChild(card);
    }
    fitTables();
  }

  // A table that does not fit its card turns each row into a small card of
  // its own (see .scroll.cards in style.css) instead of scrolling sideways:
  // every value stays, below each other rather than beside. Decided by
  // measuring, not by a fixed screen width - the columns are as wide as
  // their content, and a closed group has no width to measure until it opens.
  // All groups switch together: one group as cards and the next as a table
  // looked like two different pages.
  // Measuring takes the cards away for a moment, and on a phone the page is
  // then far shorter: Safari clamps the scroll position to it and keeps it
  // there, so a reader far down was thrown up on every refresh. The position
  // is put back before anything is painted.
  //
  // Before that, a tight table gives up its least important columns a step at
  // a time - Net, then Operator, then Connected (see [data-drops] in
  // style.css). Cards came far too early otherwise: a laptop window a little
  // under 1200px already stacked every peer into five or six lines.
  const DROP_LEVELS = 3;
  function fitTables() {
    const y = window.scrollY;
    const wraps = [...document.querySelectorAll("#tables .scroll")];
    for (const w of wraps) w.classList.remove("cards");
    const tight = () => wraps.some((w) => w.clientWidth && w.scrollWidth > w.clientWidth + 1);
    let level = 0;
    for (; ; level++) {
      for (const w of wraps) w.dataset.drops = String(level);
      if (level === DROP_LEVELS || !tight()) break;
    }
    const cards = level === DROP_LEVELS && tight();
    for (const w of wraps) w.classList.toggle("cards", cards);
    if (window.scrollY !== y) window.scrollTo(0, y);
  }
  let fitTimer = null;
  // Only a change of width can change whether the tables fit; on an iPhone the
  // height changes all the time while Safari's toolbar shrinks and grows.
  let fitWidth = window.innerWidth;
  window.addEventListener("resize", () => {
    if (window.innerWidth === fitWidth) return;
    fitWidth = window.innerWidth;
    clearTimeout(fitTimer); fitTimer = setTimeout(fitTables, 100);
  });
  document.addEventListener("toggle", fitTables, true);


  // ---------- start ----------
  setupPanZoom();
  loadWorld().then(() => { if (state.data) renderMarkers(); });
  startTicker();
  refresh();
})();
