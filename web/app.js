"use strict";
(() => {
  const GROUPS = ["manual", "inbound", "outbound"];
  const LABEL = { manual: "Manual", inbound: "Inbound", outbound: "Outbound" };
  const NS = "http://www.w3.org/2000/svg";
  const $ = (id) => document.getElementById(id);
  const regionName = (() => {
    try {
      const dn = new Intl.DisplayNames(["en"], { type: "region" });
      return (cc) => { try { return dn.of(cc) || cc; } catch { return cc; } };
    } catch { return (cc) => cc; }
  })();

  const state = {
    data: null,          // last /api/peers reply
    world: null,         // world.json
    show: { manual: true, inbound: true, outbound: true },
    country: "",         // country filter from clicking a marker
    sort: {},            // per group: { key, dir }
    view: { k: 1, x: 0, y: 0 },
  };

  // ---------- polling: only while the page is visible ----------
  let pollTimer = 0;
  let tickTimer = 0;

  async function refresh() {
    clearTimeout(pollTimer);
    if (document.hidden) return;
    let next = 10;
    try {
      const r = await fetch("api/peers", { cache: "no-store" });
      if (!r.ok) throw new Error("HTTP " + r.status);
      state.data = await r.json();
      next = state.data.next_in || state.data.interval || 10;
      render();
    } catch (e) {
      showError("Peer Map backend not reachable: " + e.message);
    }
    if (!document.hidden) pollTimer = setTimeout(refresh, next * 1000 + 250);
  }

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
    if (!tickTimer) tickTimer = setInterval(renderStatus, 1000);
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

  // ---------- render ----------
  function visiblePeers() {
    return state.data ? state.data.peers.filter((p) => state.show[p.group]) : [];
  }

  function render() {
    const d = state.data;
    $("demo").hidden = !d.demo;
    showError(d.error ? "Bitcoin Core: " + d.error + (d.fetched_at ? " - showing the last good peer list." : "") : "");
    $("version").textContent = "v" + (d.version || "dev");

    const counts = { manual: 0, inbound: 0, outbound: 0 };
    for (const p of d.peers) counts[p.group]++;
    for (const g of GROUPS) document.querySelector(`[data-count="${g}"]`).textContent = counts[g];

    renderStatus();
    renderMarkers();
    renderUnplaced();
    renderTables();
  }

  function renderStatus() {
    const d = state.data;
    if (!d) return;
    const total = d.peers.length;
    const since = d.fetched_at ? ago(Date.now() / 1000 - d.fetched_at) + " ago" : "never";
    $("status").textContent = `${total} peers · updated ${since} · refresh every ${d.interval}s while open`;
  }

  // ---------- map ----------
  async function loadWorld() {
    const r = await fetch("world.json");
    state.world = await r.json();
    const svg = $("map");
    svg.setAttribute("viewBox", `0 0 ${state.world.w} ${state.world.h}`);
    const g = $("countries");
    const frag = document.createDocumentFragment();
    for (const [cc, d] of Object.entries(state.world.paths)) {
      const p = document.createElementNS(NS, "path");
      p.setAttribute("d", d);
      p.setAttribute("class", "land");
      p.setAttribute("vector-effect", "non-scaling-stroke");
      p.dataset.cc = cc;
      frag.appendChild(p);
    }
    if (state.world.unnamed) {
      const p = document.createElementNS(NS, "path");
      p.setAttribute("d", state.world.unnamed);
      p.setAttribute("class", "land");
      p.setAttribute("vector-effect", "non-scaling-stroke");
      frag.appendChild(p);
    }
    g.appendChild(frag);
    applyView();
  }

  function countryBuckets() {
    const by = new Map();
    for (const p of visiblePeers()) {
      if (!p.cc) continue;
      let b = by.get(p.cc);
      if (!b) by.set(p.cc, (b = { cc: p.cc, manual: 0, inbound: 0, outbound: 0, total: 0 }));
      b[p.group]++;
      b.total++;
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

  function renderMarkers() {
    if (!state.world || !state.data) return;
    const layer = $("markers");
    layer.textContent = "";
    const buckets = countryBuckets();
    const hot = new Set(buckets.map((b) => b.cc));
    for (const el of $("countries").children) el.classList.toggle("hot", hot.has(el.dataset.cc));

    // Largest first, so small markers are drawn on top and stay clickable.
    for (const b of buckets) {
      const pt = state.world.points[b.cc];
      if (!pt) continue;
      const r = radius(b.total);
      const g = document.createElementNS(NS, "g");
      g.setAttribute("class", "marker" + (state.country === b.cc ? " selected" : ""));
      g.dataset.cc = b.cc;
      g.dataset.x = pt[0];
      g.dataset.y = pt[1];
      g.setAttribute("tabindex", "0");
      g.setAttribute("role", "button");
      g.setAttribute("aria-label", `${regionName(b.cc)}: ${GROUPS.map((k) => b[k] + " " + LABEL[k].toLowerCase()).join(", ")}`);

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
      g.addEventListener("pointerenter", (e) => showTip(b, e));
      g.addEventListener("pointermove", (e) => moveTip(e));
      g.addEventListener("pointerleave", hideTip);
      g.addEventListener("focus", () => showTip(b, null, g));
      g.addEventListener("blur", hideTip);
      g.addEventListener("click", (e) => { e.stopPropagation(); toggleCountry(b.cc); });
      g.addEventListener("keydown", (e) => {
        if (e.key === "Enter" || e.key === " ") { e.preventDefault(); toggleCountry(b.cc); }
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

  function toggleCountry(cc) {
    state.country = state.country === cc ? "" : cc;
    $("countryFilter").hidden = !state.country;
    $("countryFilterName").textContent = state.country ? regionName(state.country) : "";
    renderMarkers();
    renderTables();
  }
  $("countryFilterClear").addEventListener("click", () => toggleCountry(state.country));

  // ---------- tooltip ----------
  const tip = $("tooltip");
  function showTip(b, e, el) {
    tip.textContent = "";
    const h = document.createElement("h3");
    h.textContent = regionName(b.cc);
    tip.appendChild(h);
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
  const MAX_K = 16;
  function clampView() {
    const v = state.view, W = state.world.w, H = state.world.h;
    v.k = Math.min(MAX_K, Math.max(1, v.k));
    v.x = Math.min(0, Math.max(W - W * v.k, v.x));
    v.y = Math.min(0, Math.max(H - H * v.k, v.y));
  }
  function applyView() {
    clampView();
    const { k, x, y } = state.view;
    $("countries").setAttribute("transform", `translate(${x} ${y}) scale(${k})`);
    if (state.data) placeMarkers();
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
    svg.addEventListener("click", () => { if (!moved && state.country) toggleCountry(state.country); });

    const center = () => [state.world.w / 2, state.world.h / 2];
    $("zoomIn").addEventListener("click", () => zoomAt(1.6, ...center()));
    $("zoomOut").addEventListener("click", () => zoomAt(1 / 1.6, ...center()));
    $("zoomReset").addEventListener("click", () => { state.view = { k: 1, x: 0, y: 0 }; applyView(); });
    window.addEventListener("resize", () => { if (state.data) placeMarkers(); });
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

  // ---------- not on the map ----------
  function renderUnplaced() {
    const rows = new Map();
    for (const p of state.data.peers) {
      if (p.cc) continue;
      let key = netLabel(p.network);
      if (p.network === "ipv4" || p.network === "ipv6") key = "No geo data";
      let r = rows.get(key);
      if (!r) rows.set(key, (r = { manual: 0, inbound: 0, outbound: 0 }));
      r[p.group]++;
    }
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
  const COLS = [
    { key: "addr", label: "Address", cls: "addr", val: (p) => p.addr },
    { key: "cc", label: "Country", val: (p) => (p.cc ? regionName(p.cc) : "") , show: (p) => (p.cc ? regionName(p.cc) : "–"), dim: (p) => !p.cc },
    { key: "network", label: "Network", val: (p) => netLabel(p.network) },
    { key: "type", label: "Type", val: (p) => typeLabel(p.type), only: "outbound" },
    { key: "subver", label: "Client", val: (p) => p.subver || "" },
    { key: "transport", label: "P2P", val: (p) => p.transport || "" },
    { key: "ping", label: "Ping", cls: "num", val: (p) => (p.ping_ms == null ? Infinity : p.ping_ms), show: (p) => (p.ping_ms == null ? "–" : Math.round(p.ping_ms) + " ms") },
    { key: "conntime", label: "Connected", cls: "num", val: (p) => p.conntime, show: (p) => ago(Date.now() / 1000 - p.conntime) },
  ];

  function renderTables() {
    const root = $("tables");
    root.textContent = "";
    for (const g of GROUPS) {
      if (!state.show[g]) continue;
      let peers = state.data.peers.filter((p) => p.group === g);
      const all = peers.length;
      if (state.country) peers = peers.filter((p) => p.cc === state.country);

      const card = document.createElement("section");
      card.className = "card group-card";
      const h = document.createElement("h2");
      const sw = document.createElement("span");
      sw.className = "swatch s-" + g;
      const n = document.createElement("span");
      n.className = "n";
      n.textContent = state.country ? `${peers.length} of ${all}` : String(all);
      h.append(sw, LABEL[g] + " ", n);
      card.appendChild(h);

      if (!peers.length) {
        const p = document.createElement("p");
        p.className = "empty";
        p.textContent = state.country ? "None in this country." : "No " + LABEL[g].toLowerCase() + " peers.";
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
        for (const c of cols) {
          const td = document.createElement("td");
          if (c.cls) td.className = c.cls;
          const text = c.show ? c.show(p) : c.val(p);
          td.textContent = text === "" ? "–" : text;
          if ((c.dim && c.dim(p)) || text === "") td.classList.add("dim");
          if (c.key === "addr") td.title = p.addr;
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
