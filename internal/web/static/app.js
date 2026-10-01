// Progressive enhancement for the server-rendered pages. Everything here is
// optional: without it, links and forms still do the full job.
(function () {
  "use strict";
  document.documentElement.classList.add("js");

  const HEX = 18;
  const HEX_W = Math.sqrt(3) * HEX;
  const main = () => document.getElementById("main");

  // ---------- helpers ----------

  function el(tag, attrs, ...children) {
    const n = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs || {})) {
      if (k === "class") n.className = v;
      else n.setAttribute(k, v);
    }
    for (const c of children) {
      if (c == null || c === false) continue;
      n.append(c instanceof Node ? c : document.createTextNode(String(c)));
    }
    return n;
  }

  function hexId(col, row) {
    return String(col + 1).padStart(2, "0") + String(row + 1).padStart(2, "0");
  }

  function parseHex(id) {
    return { col: parseInt(id.slice(0, 2), 10) - 1, row: parseInt(id.slice(2), 10) - 1 };
  }

  function hexCenter(col, row) {
    return { x: HEX_W * (col + 0.5 * (row & 1)) + HEX_W / 2, y: 1.5 * HEX * row + HEX };
  }

  // Neighbor offsets for odd-row offset hexes; directions are
  // east, north-east, north-west, west, south-west, south-east.
  const NB = [
    [[1, 0], [0, -1], [-1, -1], [-1, 0], [-1, 1], [0, 1]],
    [[1, 0], [1, -1], [0, -1], [-1, 0], [0, 1], [1, 1]],
  ];
  const DIRS = ["east", "north-east", "north-west", "west", "south-west", "south-east"];

  function neighbor(col, row, dir) {
    const d = NB[row & 1][dir];
    return { col: col + d[0], row: row + d[1] };
  }

  // hexAt finds the hex whose center is nearest an SVG point.
  function hexAt(x, y, data) {
    const r0 = Math.round((y - HEX) / (1.5 * HEX));
    let best = null, bd = Infinity;
    for (let r = r0 - 1; r <= r0 + 1; r++) {
      if (r < 0 || r >= data.rows) continue;
      const c0 = Math.round((x - HEX_W / 2) / HEX_W - 0.5 * (r & 1));
      for (let c = c0 - 1; c <= c0 + 1; c++) {
        if (c < 0 || c >= data.cols) continue;
        const p = hexCenter(c, r);
        const d = (p.x - x) ** 2 + (p.y - y) ** 2;
        if (d < bd) { bd = d; best = { col: c, row: r }; }
      }
    }
    return best;
  }

  // sideAt finds the hex side nearest an SVG point, and how far the point is from the hex center.
  function sideAt(x, y, data) {
    const h = hexAt(x, y, data);
    if (!h) return null;
    const c = hexCenter(h.col, h.row);
    const ang = Math.atan2(y - c.y, x - c.x) * 180 / Math.PI;
    const dir = ((Math.round(-ang / 60) % 6) + 6) % 6;
    const n = neighbor(h.col, h.row, dir);
    if (n.col < 0 || n.row < 0 || n.col >= data.cols || n.row >= data.rows) return null;
    return { ...h, dir, dist: Math.hypot(x - c.x, y - c.y) };
  }

  function sideEnds(col, row, dir) {
    const c = hexCenter(col, row);
    const a1 = (30 - 60 * dir) * Math.PI / 180, a2 = (-30 - 60 * dir) * Math.PI / 180;
    return [c.x + HEX * Math.cos(a1), c.y + HEX * Math.sin(a1), c.x + HEX * Math.cos(a2), c.y + HEX * Math.sin(a2)];
  }

  const SVGNS = "http://www.w3.org/2000/svg";
  function svgLine(cls, x1, y1, x2, y2) {
    const l = document.createElementNS(SVGNS, "line");
    l.setAttribute("class", cls);
    [["x1", x1], ["y1", y1], ["x2", x2], ["y2", y2]].forEach(([k, v]) => l.setAttribute(k, v.toFixed(1)));
    return l;
  }

  function sectorOf(id, data) {
    const g = data?.sectors;
    if (!g || !g.cols) return "";
    const { col, row } = parseHex(id);
    return String.fromCharCode(65 + Math.floor(col / g.cols)) + (Math.floor(row / g.rows) + 1);
  }

  function sectorLabel(id, data) {
    const s = sectorOf(id, data);
    const name = s && data.sectors.names?.[s];
    return name ? `${s} · ${name}` : s;
  }

  function mapData() {
    const node = document.getElementById("map-data");
    try { return node ? JSON.parse(node.textContent) : null; } catch { return null; }
  }

  // ---------- map: pan, zoom, select ----------

  const map = {
    svg: null, full: null, vb: null, selected: null,
    pointers: new Map(), dragStart: null, moved: false, pinchDist: 0,

    init() {
      this.svg = document.getElementById("map");
      if (!this.svg) return;
      const [x, y, w, h] = this.svg.dataset.viewbox.split(/\s+/).map(Number);
      this.full = { x, y, w, h };
      this.vb = { ...this.full };

      let stored = null;
      try { stored = JSON.parse(sessionStorage.getItem("aw-map:" + location.pathname)); } catch {}
      if (stored && stored.vb) this.vb = stored.vb;
      else if (this.svg.getBoundingClientRect().width < 700) this.focusStart();
      try {
        if (localStorage.getItem("aw-sectors") === "off") {
          this.svg.classList.add("no-sectors");
          const t = document.getElementById("sectors-toggle");
          if (t) t.checked = false;
        }
        if (localStorage.getItem("aw-labels") === "off") {
          this.svg.classList.add("no-labels");
          document.getElementById("labels-toggle").checked = false;
        }
      } catch {}
      this.clamp();
      this.apply();
      window.addEventListener("resize", () => { this.clamp(); this.apply(); });
      if (stored && stored.selected) this.select(stored.selected, false);

      this.svg.addEventListener("wheel", (e) => {
        e.preventDefault();
        this.zoomAt(this.toSvg(e.clientX, e.clientY), e.deltaY < 0 ? 0.85 : 1 / 0.85);
      }, { passive: false });
      this.svg.addEventListener("pointerdown", (e) => this.down(e));
      this.svg.addEventListener("pointermove", (e) => this.move(e));
      this.svg.addEventListener("pointerup", (e) => this.up(e));
      this.svg.addEventListener("pointercancel", (e) => { this.pointers.delete(e.pointerId); this.editing = false; });
      this.svg.addEventListener("pointerleave", () => editor.hover(null));
      this.svg.addEventListener("keydown", (e) => this.key(e));
      this.svg.addEventListener("focus", () => { if (!this.selected) this.select(hexId(4, 7)); });
    },

    // Small screens open zoomed in on the viewer's own units, or the beachhead.
    focusStart() {
      const anchor = document.querySelector("#units-layer .mine, #features-layer .k-beachhead")?.closest("[data-h]");
      const { col, row } = parseHex(anchor ? anchor.dataset.h : hexId(4, 7));
      const c = hexCenter(col, row);
      this.vb.w = this.full.w / 2.2;
      this.clamp();
      this.vb.x = c.x - this.vb.w / 3;
      this.vb.y = c.y - this.vb.h / 2;
      this.clamp();
    },

    // Height-to-width ratio of the map window; the view box follows it so
    // hexes are never stretched and narrow screens aren't letterboxed.
    aspect() {
      const r = this.svg.getBoundingClientRect();
      return r.width > 0 && r.height > 0 ? r.height / r.width : this.full.h / this.full.w;
    },

    toSvg(cx, cy) {
      const pt = new DOMPoint(cx, cy).matrixTransform(this.svg.getScreenCTM().inverse());
      return { x: pt.x, y: pt.y };
    },

    apply() {
      const v = this.vb;
      this.svg.setAttribute("viewBox", `${v.x} ${v.y} ${v.w} ${v.h}`);
      this.save();
    },

    save() {
      try {
        sessionStorage.setItem("aw-map:" + location.pathname, JSON.stringify({ vb: this.vb, selected: this.selected }));
      } catch {}
    },

    clamp() {
      const f = this.full, v = this.vb, a = this.aspect();
      v.w = Math.min(f.w, f.h / a, Math.max(f.w / 6, v.w));
      v.h = v.w * a;
      v.x = Math.min(f.x + f.w - v.w, Math.max(f.x, v.x));
      v.y = Math.min(f.y + f.h - v.h, Math.max(f.y, v.y));
    },

    zoomAt(p, factor) {
      const v = this.vb;
      const w = v.w * factor, h = v.h * factor;
      v.x = p.x - (p.x - v.x) * (w / v.w);
      v.y = p.y - (p.y - v.y) * (h / v.h);
      v.w = w; v.h = h;
      this.clamp();
      this.apply();
    },

    zoom(dir) {
      if (dir === "reset") { this.vb = { ...this.full }; this.clamp(); this.apply(); return; }
      // Zoom around the selected hex when there is one, so it stays in view.
      const v = this.vb;
      let p = { x: v.x + v.w / 2, y: v.y + v.h / 2 };
      if (this.selected) {
        const { col, row } = parseHex(this.selected);
        p = hexCenter(col, row);
      }
      this.zoomAt(p, dir === "in" ? 0.7 : 1 / 0.7);
    },

    centerOn(col, row) {
      const c = hexCenter(col, row), v = this.vb;
      const margin = HEX * 1.5;
      if (c.x < v.x + margin || c.x > v.x + v.w - margin || c.y < v.y + margin || c.y > v.y + v.h - margin) {
        v.x = c.x - v.w / 2; v.y = c.y - v.h / 2;
        this.clamp();
        this.apply();
      }
    },

    down(e) {
      this.svg.setPointerCapture(e.pointerId);
      if (this.pointers.size === 0 && e.button === 0 && editor.wantsDrag()) {
        this.editing = true;
        editor.dragStart(this.toSvg(e.clientX, e.clientY));
        return;
      }
      this.pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
      if (this.pointers.size === 1) {
        this.dragStart = { x: e.clientX, y: e.clientY, vb: { ...this.vb }, target: e.target };
        this.moved = false;
      } else if (this.pointers.size === 2) {
        const [a, b] = [...this.pointers.values()];
        this.pinchDist = Math.hypot(a.x - b.x, a.y - b.y);
      }
    },

    move(e) {
      if (this.editing) { editor.dragMove(this.toSvg(e.clientX, e.clientY)); return; }
      if (this.pointers.size === 0) { editor.hover(this.toSvg(e.clientX, e.clientY)); return; }
      if (!this.pointers.has(e.pointerId)) return;
      this.pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
      if (this.pointers.size === 2) {
        const [a, b] = [...this.pointers.values()];
        const d = Math.hypot(a.x - b.x, a.y - b.y);
        if (this.pinchDist > 0 && d > 0) {
          this.zoomAt(this.toSvg((a.x + b.x) / 2, (a.y + b.y) / 2), this.pinchDist / d);
          this.pinchDist = d;
        }
        this.moved = true;
        return;
      }
      const s = this.dragStart;
      if (!s) return;
      const dx = e.clientX - s.x, dy = e.clientY - s.y;
      if (!this.moved && Math.hypot(dx, dy) < 6) return;
      this.moved = true;
      this.svg.classList.add("dragging");
      const scale = s.vb.w / this.svg.getBoundingClientRect().width;
      this.vb.x = s.vb.x - dx * scale;
      this.vb.y = s.vb.y - dy * scale;
      this.clamp();
      this.apply();
    },

    up(e) {
      if (this.editing) { this.editing = false; editor.dragEnd(); return; }
      const s = this.dragStart;
      this.pointers.delete(e.pointerId);
      this.svg.classList.remove("dragging");
      if (this.pointers.size > 0) return;
      this.dragStart = null;
      if (s && !this.moved) {
        if (editor.click(this.toSvg(e.clientX, e.clientY))) return;
        if (orders.click(this.toSvg(e.clientX, e.clientY))) return;
        const hit = s.target.closest && s.target.closest("[data-h]");
        if (hit) this.select(hit.dataset.h, false);
      }
    },

    key(e) {
      const data = mapData();
      if (!data) return;
      const cur = this.selected ? parseHex(this.selected) : { col: 4, row: 7 };
      const moves = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] };
      if (moves[e.key]) {
        e.preventDefault();
        const col = Math.min(data.cols - 1, Math.max(0, cur.col + moves[e.key][0]));
        const row = Math.min(data.rows - 1, Math.max(0, cur.row + moves[e.key][1]));
        this.select(hexId(col, row), true);
      } else if (e.key === "+" || e.key === "=") { this.zoom("in"); }
      else if (e.key === "-") { this.zoom("out"); }
      else if (e.key === "0") { this.zoom("reset"); }
    },

    select(id, pan) {
      if (!this.svg) return;
      this.selected = id;
      const { col, row } = parseHex(id);
      const c = hexCenter(col, row);
      const cursor = document.getElementById("cursor");
      cursor.setAttribute("x", c.x.toFixed(1));
      cursor.setAttribute("y", c.y.toFixed(1));
      cursor.setAttribute("visibility", "visible");
      if (pan) this.centerOn(col, row);
      this.save();
      inspect(id);
    },
  };

  function inspect(id) {
    const box = document.getElementById("inspector");
    const data = mapData();
    if (!box || !data) return;
    const terrainId = document.querySelector(`.terrain [data-h="${id}"]`)?.dataset.t;
    const t = data.terrain[terrainId];
    const features = data.features[id] || [];
    const units = data.units[id] || [];

    const body = [el("h3", {}, `Hex ${id}`, t ? ` · ${t.name}` : "")];
    const sec = sectorLabel(id, data);
    if (sec) body.push(el("p", { class: "small sector-tag" }, `Sector ${sec}`));
    const st = data.stack?.[id];
    if (st) body.push(el("p", { class: `small stack-line${st[0] >= st[1] ? " full" : ""}` }, `Stacking: ${st[0]} of ${st[1]} points used${st[0] >= st[1] ? " (full)" : ""}.`));
    if (t) {
      body.push(el("p", { class: "muted small" },
        t.move === 0 ? "Impassable to ground units." : `Move cost ${t.move}, cover ${t.cover}.`,
        t.note ? " " + t.note : ""));
    }
    const sides = data.sides[id] || [];
    const edges = sides.filter((s) => s.edge);
    if (edges.length) {
      const ul = el("ul", { class: "plain small sides" });
      for (const s of edges) {
        ul.append(el("li", {}, el("strong", {}, `${s.dirName[0].toUpperCase()}${s.dirName.slice(1)} side: `),
          s.edgeName, s.crossingName ? ` with a ${s.crossingName.toLowerCase()}` : "",
          s.blocks ? " (no crossing)" : "", s.note ? ` · ${s.note}` : ""));
      }
      body.push(ul);
    }
    const roads = sides.filter((s) => s.road).map((s) => s.dirName);
    if (roads.length) body.push(el("p", { class: "muted small" }, `Road leads ${roads.join(", ")}.`));
    for (const f of features) {
      body.push(el("p", {}, el("strong", {}, f.name), f.hidden ? " (hidden from players)" : "", f.note ? ` · ${f.note}` : ""));
    }
    if (units.length) {
      const ul = el("ul", { class: "plain list" });
      for (const u of units) {
        ul.append(el("li", {},
          el("div", {}, el("span", { class: `abbr side-${u.side}`, "aria-hidden": "true" }, u.abbr),
            el("a", { href: `${main().dataset.base}/units/${u.id}`, class: "unit-link", "data-card": "" }, el("strong", {}, u.name)),
            u.level && u.level !== "Green" ? el("span", { class: "tag tag-xp" }, u.level) : null,
            u.mine ? " (yours)" : "", u.hidden ? " (hidden from players)" : "", u.ghost ? " (out of sight)" : "",
          u.aboard ? ` (aboard ${u.aboard})` : ""),
          el("div", { class: "muted small" }, [u.class, u.strength, u.commander].filter(Boolean).join(" · ")),
          u.stats ? el("div", { class: "muted small" }, u.stats + (u.supply ? " · " + u.supply : "")) : null,
          u.traits ? el("div", { class: "small" }, u.traits) : null,
          data.orders?.open && data.orders.reach?.[u.id]
            ? el("button", { type: "button", class: "link small", "data-order-unit": u.id }, data.orders.orders?.[u.id] ? "Edit order" : "Give order") : null));
      }
      body.push(ul);
    } else {
      body.push(el("p", { class: "muted small" }, "No known units here."));
    }
    box.replaceChildren(...body);
  }

  // ---------- scenario editor ----------
  //
  // Tools turn map clicks and drags into editor ops, which are sent in one
  // batch per gesture. Changes show immediately as a preview; the page then
  // refreshes from the server, which stays the source of truth.

  const editor = {
    active: false, tool: "inspect", queue: [], seen: new Set(), lastHex: null, moving: null,

    init() {
      this.active = !!document.querySelector("[data-editor]");
      if (!this.active) return;
      document.addEventListener("change", (e) => {
        if (e.target.name === "tool") this.setTool(e.target.value);
      });
      document.addEventListener("keydown", (e) => {
        if (e.key === "Escape" && this.moving) { this.moving = null; map.svg.classList.remove("moving"); this.hint(); }
      });
      this.setTool(document.querySelector('input[name="tool"]:checked')?.value || "inspect");
    },

    setTool(tool) {
      this.tool = tool;
      document.querySelectorAll("[data-tool-opts]").forEach((o) => (o.hidden = o.dataset.toolOpts !== tool));
      if (map.svg) map.svg.dataset.tool = tool;
      this.hover(null);
      this.hint();
    },

    hint(text) {
      const h = document.getElementById("tool-hint");
      if (!h) return;
      if (text == null && this.moving) text = `Click the new hex for ${this.moving.name}. Press Escape to cancel.`;
      h.textContent = text || "";
    },

    wantsDrag() {
      return this.active && !this.moving && ["terrain", "edges", "roads"].includes(this.tool);
    },

    data() { return mapData(); },

    queueOp(key, op) {
      if (this.seen.has(key)) return false;
      this.seen.add(key);
      this.queue.push(op);
      return true;
    },

    preview(cls, x1, y1, x2, y2) {
      document.getElementById("edit-preview")?.append(svgLine(cls, x1, y1, x2, y2));
    },

    // --- drag gestures: terrain painting, drawing edges, laying roads ---

    dragStart(p) {
      this.queue = []; this.seen = new Set(); this.lastHex = null;
      this.dragMove(p);
    },

    dragMove(p) {
      this.lastPoint = p;
      const data = this.data();
      if (!data) return;
      if (this.tool === "terrain") {
        const h = hexAt(p.x, p.y, data);
        if (h) this.paint(h, data);
      } else if (this.tool === "edges") {
        const s = sideAt(p.x, p.y, data);
        if (s && s.dist > HEX * 0.45) {
          const type = document.getElementById("edge-type").value;
          if (this.queueOp(`${s.col},${s.row},${s.dir}`, { op: "edge", hex: hexId(s.col, s.row), dir: s.dir, type })) {
            this.preview(`pv-${type || "erase"}`, ...sideEnds(s.col, s.row, s.dir));
          }
        }
      } else if (this.tool === "roads") {
        const h = hexAt(p.x, p.y, data);
        if (!h) return;
        const last = this.lastHex;
        if (last && (last.col !== h.col || last.row !== h.row)) {
          for (let dir = 0; dir < 6; dir++) {
            const n = neighbor(last.col, last.row, dir);
            if (n.col === h.col && n.row === h.row) {
              const a = hexCenter(last.col, last.row), b = hexCenter(h.col, h.row);
              if (this.queueOp(`r${last.col},${last.row},${dir}`, { op: "road", hex: hexId(last.col, last.row), dir, on: true })) {
                this.preview("pv-road", a.x, a.y, b.x, b.y);
              }
            }
          }
        }
        this.lastHex = h;
      }
    },

    paint(h, data) {
      const id = document.querySelector('input[name="paint"]:checked')?.value;
      const i = h.row * data.cols + h.col;
      const base = document.getElementById("terrain-layer").children[i];
      if (!id || !base || base.dataset.t === id) return;
      if (!this.queueOp(`t${i}`, { op: "terrain", hex: hexId(h.col, h.row), terrain: id })) return;
      base.setAttribute("class", `t-${id}`);
      base.dataset.t = id;
      document.getElementById("texture-layer").children[i]?.setAttribute("class", `tx-${id}`);
    },

    dragEnd() {
      const data = this.data();
      // A click with the road tool (no hex change) toggles the road across the nearest side.
      if (this.tool === "roads" && this.queue.length === 0 && this.lastHex && this.lastPoint && data) {
        const s = sideAt(this.lastPoint.x, this.lastPoint.y, data);
        if (s) {
          const has = (data.sides[hexId(s.col, s.row)] || []).some((x) => x.dir === s.dir && x.road);
          this.queue.push({ op: "road", hex: hexId(s.col, s.row), dir: s.dir, on: !has });
        }
      }
      const ops = this.queue;
      this.queue = []; this.seen = new Set(); this.lastHex = null;
      if (ops.length) this.send(ops);
    },

    // --- single clicks: crossings, units, places, moves ---

    click(p) {
      if (!this.active) return false;
      const data = this.data();
      if (!data) return false;
      const h = hexAt(p.x, p.y, data);
      if (!h) return false;
      const hex = hexId(h.col, h.row);
      if (this.moving) {
        const m = this.moving;
        this.moving = null;
        map.svg.classList.remove("moving");
        this.hint();
        this.send([m.kind === "unit" ? { op: "unit-move", unit: m.id, hex } : { op: "feature-move", feature: m.id, hex }]);
        return true;
      }
      switch (this.tool) {
        case "crossings": {
          const s = sideAt(p.x, p.y, data);
          if (s) this.send([{ op: "crossing", hex: hexId(s.col, s.row), dir: s.dir, crossing: document.getElementById("crossing-type").value }]);
          return true;
        }
        case "units": {
          const known = document.getElementById("unit-known").checked;
          const strength = parseInt(document.getElementById("unit-strength").value, 10) || 1;
          this.send([{ op: "unit-add", class: document.getElementById("unit-class").value, hex, strength, known }]);
          return true;
        }
        case "sectors": {
          this.send([{ op: "sector-name", hex, name: document.getElementById("sector-name").value.trim() }]);
          return true;
        }
        case "places": {
          const kind = document.getElementById("place-kind").value;
          const name = document.getElementById("place-name").value.trim() || `New ${kind}`;
          this.send([{ op: "feature-add", kind, hex, name, hidden: document.getElementById("place-hidden").checked }]);
          return true;
        }
      }
      return false;
    },

    // Highlight the side under the pointer for side-based tools.
    hover(p) {
      if (p) this.lastPoint = p;
      const cur = document.getElementById("side-cursor");
      if (!cur) return;
      const data = p && this.active && ["edges", "crossings", "roads"].includes(this.tool) ? this.data() : null;
      const s = data && sideAt(p.x, p.y, data);
      if (!s) { cur.setAttribute("visibility", "hidden"); return; }
      const [x1, y1, x2, y2] = sideEnds(s.col, s.row, s.dir);
      Object.entries({ x1, y1, x2, y2 }).forEach(([k, v]) => cur.setAttribute(k, v.toFixed(1)));
      cur.setAttribute("visibility", "visible");
      this.hint(`Hex ${hexId(s.col, s.row)}, ${DIRS[s.dir]} side`);
    },

    startMove(kind, id, name) {
      this.moving = { kind, id, name };
      map.svg.classList.add("moving");
      map.svg.scrollIntoView({ block: "nearest", behavior: "smooth" });
      this.hint();
    },

    async send(ops) {
      const base = main().dataset.base;
      try {
        const res = await fetch(`${base}/edit/ops`, {
          method: "POST",
          headers: { "Content-Type": "application/json", "X-Requested-With": "fetch" },
          body: JSON.stringify({ ops }),
        });
        if (!res.ok) alert((await res.text()) || "That change was refused.");
      } catch {
        alert("Couldn't reach the server. Your change was not saved.");
      }
      await refresh();
    },
  };

  // Unit and place lists in the editor: edit in place, move, delete.
  document.addEventListener("change", (e) => {
    const f = e.target.dataset.field;
    const row = f && e.target.closest("[data-unit], [data-feature]");
    if (!row) return;
    const value = e.target.type === "checkbox" ? e.target.checked
      : e.target.type === "number" ? parseInt(e.target.value, 10) : e.target.value;
    const op = row.dataset.unit
      ? { op: "unit-update", unit: row.dataset.unit, [f]: value }
      : { op: "feature-update", feature: row.dataset.feature, [f]: value };
    editor.send([op]);
  });

  document.addEventListener("click", (e) => {
    if (e.target.closest("[data-sector-size]")) {
      const cols = parseInt(document.getElementById("sector-cols").value, 10);
      const rows = parseInt(document.getElementById("sector-rows").value, 10);
      if (confirm("Change the sector size? Existing sector names stay with their letters and numbers, so they may cover different ground.")) {
        editor.send([{ op: "sector-size", cols, rows }]);
      }
      return;
    }
    const t = e.target.closest("[data-move-unit], [data-move-feature], [data-delete-unit], [data-delete-feature]");
    if (!t) return;
    const d = t.dataset;
    if (d.moveUnit) editor.startMove("unit", d.moveUnit, d.name);
    else if (d.moveFeature) editor.startMove("feature", d.moveFeature, d.name);
    else if (d.deleteUnit && confirm(`Delete ${d.name}?`)) editor.send([{ op: "unit-delete", unit: d.deleteUnit }]);
    else if (d.deleteFeature && confirm(`Delete ${d.name}?`)) editor.send([{ op: "feature-delete", feature: d.deleteFeature }]);
  });

  // ---------- order entry ----------
  //
  // The server sends each orderable unit's reach: every hex it can get to
  // this turn, with the movement it costs and the previous hex on the
  // cheapest path. The client only draws that; the server re-checks on save.

  const orders = {
    box: null,
    form: null,
    dialog: null,
    pickbar: null,
    mode: "closed", // closed, dialog, or picking (choosing a hex on the map)

    init() {
      this.box = document.getElementById("order-box");
      if (!this.box) return;
      // With script, the form lives in a lightbox; the details element in the
      // info column becomes its launcher.
      this.form = document.getElementById("order-form");
      const close = el("button", { type: "button", class: "secondary small-btn", "aria-label": "Close" }, "Close");
      this.dialog = el("dialog", { id: "order-dialog", class: "order-dialog", "aria-labelledby": "od-title" },
        el("div", { class: "od-head" }, el("h2", { id: "od-title" }, "Write an order"), close), this.form);
      document.body.append(this.dialog);
      close.addEventListener("click", () => this.dialog.close());
      this.dialog.addEventListener("close", () => {
        if (this.mode === "dialog") { this.mode = "closed"; this.sync(); }
      });
      const back = el("button", { type: "button", class: "small-btn" }, "Back to the order");
      const cancel = el("button", { type: "button", class: "secondary small-btn" }, "Cancel");
      this.pickbar = el("div", { class: "pick-bar", role: "status", hidden: "" },
        el("span", { id: "pick-hint" }, "Click a highlighted hex for the destination."), back, cancel);
      document.body.append(this.pickbar);
      back.addEventListener("click", () => this.backToDialog());
      cancel.addEventListener("click", () => { this.pickbar.hidden = true; this.mode = "closed"; this.sync(); });
      document.addEventListener("keydown", (e) => { if (e.key === "Escape" && this.mode === "picking") this.backToDialog(); });
      this.box.addEventListener("click", (e) => {
        if (!e.target.closest("summary")) return;
        e.preventDefault();
        this.open(this.unit());
      });
      document.addEventListener("click", (e) => {
        if (e.target.closest("#of-pick")) this.pick();
      });
      document.addEventListener("change", (e) => {
        if (e.target.id === "of-unit") this.load();
        else if (e.target.id === "of-task" || e.target.id === "of-transport") this.sync();
        else if (e.target.id === "of-dest") this.setDest(e.target.value.trim());
      });
      document.addEventListener("click", (e) => {
        const b = e.target.closest("[data-order-unit]");
        if (!b) return;
        e.preventDefault();
        this.open(b.dataset.orderUnit);
      });
      this.load();
    },

    data() { return mapData()?.orders; },
    $(id) { return document.getElementById(id); },
    unit() { return this.$("of-unit")?.value; },
    active() { return !!this.box && this.mode !== "closed" && !!this.data()?.open; },
    needsDest() { return !!this.data()?.needDest[this.$("of-task").value]; },
    needsTransport() { return !!this.data()?.needTransport?.[this.$("of-task").value]; },

    // transports fills the transport picker for the current unit and draws
    // the way to the chosen one.
    transports() {
      const row = this.$("of-transport-row");
      if (!row) return;
      row.hidden = !this.needsTransport();
      if (row.hidden) return;
      const d = this.data();
      const list = d.transports?.[this.unit()] || [];
      const sel = this.$("of-transport");
      const keep = sel.value || d.orders[this.unit()]?.transport;
      sel.replaceChildren(...list.map((t) => el("option", { value: t.id }, `${t.name} · ${t.hex} · room for ${Math.max(0, t.room)}`)));
      if (list.some((t) => t.id === keep)) sel.value = keep;
      const t = list.find((x) => x.id === sel.value);
      const hint = this.$("of-transport-hint");
      if (!t) {
        hint.textContent = "No transport this unit can reach this turn.";
        hint.classList.add("bad");
        this.route(null);
        return;
      }
      hint.classList.remove("bad");
      hint.textContent = t.cost ? `Moves to ${t.board} (${t.cost} movement) and climbs aboard ${t.name}.` : `Climbs aboard ${t.name} from where it stands.`;
      const reach = d.reach[this.unit()] || {};
      const path = [t.board];
      for (let h = t.board; reach[h] && reach[h][0] > 0; h = reach[h][1]) path.push(reach[h][1]);
      this.route(path.length > 1 ? path.reverse() : null);
    },

    // syncUnits rebuilds the unit list from live data: units can be assigned
    // or taken away while the page is open. It keeps the current choice.
    syncUnits() {
      const d = this.data();
      const sel = this.$("of-unit");
      if (!d || !sel) return;
      const units = d.units || [];
      const keep = sel.value;
      const same = sel.options.length === units.length && units.every((u, i) => sel.options[i].value === u.id && sel.options[i].text === u.label);
      if (!same) {
        sel.replaceChildren(...units.map((u) => el("option", { value: u.id }, u.label)));
        if (units.some((u) => u.id === keep)) sel.value = keep;
      }
      this.box.hidden = !d.open || units.length === 0;
      return keep !== sel.value;
    },

    open(unitId) {
      if (!this.box) return;
      this.syncUnits();
      if (unitId && [...this.$("of-unit").options].some((o) => o.value === unitId)) this.$("of-unit").value = unitId;
      if (!this.unit()) return;
      this.load();
      const start = this.startHex();
      if (start) map.select(start, true);
      this.mode = "dialog";
      this.pickbar.hidden = true;
      if (!this.dialog.open) this.dialog.showModal();
      this.sync();
    },

    // pick tucks the lightbox away so a destination can be clicked on the map.
    pick() {
      this.mode = "picking";
      this.dialog.close();
      this.pickbar.hidden = false;
      this.$("pick-hint").textContent = "Click a highlighted hex for the destination. The number in each is the movement it costs.";
      map.svg?.scrollIntoView({ block: "center", behavior: "smooth" });
      this.sync();
    },

    backToDialog() {
      this.pickbar.hidden = true;
      this.mode = "dialog";
      this.dialog.showModal();
      this.sync();
    },

    // close is called after a commit goes through.
    close() {
      this.mode = "closed";
      this.pickbar.hidden = true;
      if (this.dialog.open) this.dialog.close();
      this.sync();
    },

    // load fills the form from the unit's saved order, or defaults.
    load() {
      const d = this.data();
      if (!d || !this.$("of-unit")) return;
      this.syncUnits();
      const o = d.orders[this.unit()];
      this.$("of-task").value = o?.task || this.$("of-task").options[0].value;
      this.$("of-dest").value = o?.dest || "";
      this.$("of-intent").value = o?.intent || "";
      const risk = o ? this.form.querySelector(`input[name=risk][value="${o.risk}"]`)
        : this.form.querySelector("input[name=risk][data-default]") || this.form.querySelector("input[name=risk]");
      this.$("od-title").textContent = `Orders: ${(d.units.find((u) => u.id === this.unit())?.label || "").split(" · ")[0]}`;
      if (risk) risk.checked = true;
      this.$("of-fallback").value = o?.fallback || this.$("of-fallback").options[0].value;
      if (this.$("of-then")) this.$("of-then").value = o?.then || "";
      if (this.$("of-standing")) this.$("of-standing").checked = !!o?.standing;
      this.status();
      this.sync();
      if (o?.dest) this.setDest(o.dest);
    },

    stackMates() {
      const d = this.data();
      const start = this.startHex();
      const names = [];
      for (const [id, r] of Object.entries(d?.reach || {})) {
        if (id === this.unit()) continue;
        if (r[start] && r[start][0] === 0) names.push(d.units.find((u) => u.id === id)?.label.split(" · ")[0]);
      }
      const row = this.$("of-stack-row");
      if (row) {
        row.hidden = names.length === 0;
        this.$("of-stack-names").textContent = names.length ? `(${names.join(", ")})` : "";
        if (!names.length) this.$("of-stack").checked = false;
      }
    },

    status() {
      this.stackMates();
      const d = this.data();
      const o = d?.orders[this.unit()];
      const w = this.$("of-withdraw");
      this.$("of-status").textContent = !o ? "No order yet for this unit."
        : `${o.status[0].toUpperCase()}${o.status.slice(1)} (version ${o.version}${o.author ? `, by ${o.author}` : ""}): ${o.summary}.`;
      if (w) {
        w.hidden = !(o && o.editable);
        if (o) w.setAttribute("formaction", `${main().dataset.base}/orders/${o.id}/withdraw`);
      }
    },

    startHex() {
      const reach = this.data()?.reach[this.unit()] || {};
      return Object.keys(reach).find((h) => reach[h][0] === 0);
    },

    // sync redraws the reachable hexes and task help for the current form.
    sync() {
      const task = this.$("of-task");
      if (!task) return;
      this.$("of-task-note").textContent = task.selectedOptions[0]?.dataset.note || "";
      this.$("of-dest-row").hidden = !this.needsDest();
      const then = this.$("of-then-row");
      if (then) then.hidden = !this.needsDest();
      this.transports();
      if (this.needsTransport()) { this.$("reach-layer")?.replaceChildren(); return; }
      const layer = this.$("reach-layer");
      layer?.replaceChildren();
      if (!this.active() || !this.needsDest()) { this.route(null); return; }
      const reach = this.data().reach[this.unit()] || {};
      const need = this.data().need?.[this.unit()] || 0;
      for (const [hex, [cost, , free]] of Object.entries(reach)) {
        const { col, row } = parseHex(hex);
        const c = hexCenter(col, row);
        const use = document.createElementNS(SVGNS, "use");
        use.setAttribute("href", "#hx");
        use.setAttribute("x", c.x.toFixed(1));
        use.setAttribute("y", c.y.toFixed(1));
        use.setAttribute("class", cost === 0 ? "reach-start" : free < need ? "reach full" : "reach");
        layer.append(use);
        if (cost > 0) {
          const t = document.createElementNS(SVGNS, "text");
          t.setAttribute("class", "reach-cost");
          t.setAttribute("x", c.x.toFixed(1));
          t.setAttribute("y", (c.y + 12.5).toFixed(1));
          t.textContent = cost;
          layer.append(t);
        }
      }
      const dest = this.$("of-dest").value.trim();
      if (dest) this.setDest(dest); else this.route(null);
    },

    setDest(hex) {
      const d = this.data();
      const hint = this.$("of-dest-hint");
      if (!d || !hint) return;
      const reach = d.reach[this.unit()] || {};
      this.$("of-dest").value = hex;
      if (!hex) { this.route(null); return; }
      const step = reach[hex];
      hint.classList.toggle("bad", !step || step[0] === 0);
      if (!step) {
        hint.textContent = `${hex} is beyond this unit's movement this turn (${d.movement[this.unit()] || 0}), or the way is blocked.`;
        this.route(null);
        return;
      }
      if (step[0] === 0) {
        hint.textContent = "That's where the unit already is. Pick another hex, or choose Hold position.";
        this.route(null);
        return;
      }
      const need = d.need?.[this.unit()] || 0;
      if (step[2] < need) {
        hint.classList.add("bad");
        hint.textContent = `${hex} is full: it has room for ${Math.max(0, step[2])} more stacking points and this unit needs ${need}. Units can pass through but not stop there.`;
        this.route(null);
        return;
      }
      const sector = sectorLabel(hex, mapData());
      hint.textContent = `Destination ${hex}${sector ? ` · sector ${sector}` : ""} · costs ${step[0]} of ${d.movement[this.unit()]} movement.`;
      const path = [hex];
      for (let h = hex; reach[h] && reach[h][0] > 0; h = reach[h][1]) path.push(reach[h][1]);
      this.route(path.reverse());
    },

    route(path) {
      const line = this.$("route-preview");
      if (!line) return;
      line.setAttribute("points", (path || []).map((h) => {
        const { col, row } = parseHex(h);
        const c = hexCenter(col, row);
        return `${c.x.toFixed(1)},${c.y.toFixed(1)}`;
      }).join(" "));
    },

    click(p) {
      if (this.mode !== "picking" || !this.needsDest()) return false;
      const data = mapData();
      const h = data && hexAt(p.x, p.y, data);
      if (!h) return false;
      const hex = hexId(h.col, h.row);
      this.setDest(hex);
      map.select(hex, false);
      const hint = this.$("of-dest-hint");
      if (hint.classList.contains("bad")) {
        this.$("pick-hint").textContent = hint.textContent; // explain, and stay in picking mode
      } else {
        this.backToDialog();
      }
      return true;
    },

    // After a live update: refresh the drawing and status, but keep what
    // the player is typing.
    afterRefresh() {
      if (!this.box) return;
      // If the chosen unit was taken away, start over on the first one.
      if (this.syncUnits()) { this.load(); return; }
      this.status();
      this.sync();
    },
  };

  // ---------- unit cards ----------
  // Links to a unit's card open it in a dialog over the map; without
  // script they're ordinary links to the card page.

  async function openCard(href) {
    let dialog = document.getElementById("card-dialog");
    if (!dialog) {
      dialog = el("dialog", { id: "card-dialog", class: "card-dialog", "aria-label": "Unit card" });
      dialog.addEventListener("click", (e) => { if (e.target === dialog) dialog.close(); });
      document.body.append(dialog);
    }
    try {
      const res = await fetch(href, { headers: { Accept: "text/html" } });
      if (!res.ok) { alert(await res.text()); return; }
      const doc = new DOMParser().parseFromString(await res.text(), "text/html");
      const card = doc.getElementById("unit-card");
      if (!card) { location.href = href; return; }
      const close = el("button", { type: "button", class: "secondary small-btn card-close", "aria-label": "Close" }, "Close");
      close.addEventListener("click", () => dialog.close());
      dialog.replaceChildren(close, document.importNode(card, true));
      dialog.showModal();
    } catch {
      location.href = href;
    }
  }

  document.addEventListener("click", (e) => {
    const a = e.target.closest("a[data-card]");
    if (!a || e.ctrlKey || e.metaKey || e.shiftKey || !window.HTMLDialogElement) return;
    e.preventDefault();
    openCard(a.getAttribute("href"));
  });

  // ---------- live updates ----------

  let refreshing = null;

  async function refresh() {
    if (refreshing) { refreshing.again = true; return; }
    refreshing = { again: false };
    try {
      const res = await fetch(location.pathname, { headers: { Accept: "text/html" }, cache: "no-store" });
      const doc = new DOMParser().parseFromString(await res.text(), "text/html");
      const next = doc.getElementById("main");
      const cur = main();
      // A different kind of page (briefing, removed, declined): navigate.
      if (!next || !cur || next.className !== cur.className || !next.dataset.version) {
        location.reload();
        return;
      }
      for (const node of doc.querySelectorAll("[data-live][id]")) {
        const old = document.getElementById(node.id);
        if (!old) continue;
        // Keep disclosure state of replaced <details>.
        if (old.tagName === "DETAILS" && old.open) node.setAttribute("open", "");
        old.replaceWith(document.importNode(node, true));
      }
      cur.dataset.version = next.dataset.version;
      document.getElementById("edit-preview")?.replaceChildren();
      localizeTimes();
      countdown();
      orders.afterRefresh();
      if (map.selected) inspect(map.selected);
    } catch {
      // Offline or server restarting; the event stream will retry.
    } finally {
      const again = refreshing.again;
      refreshing = null;
      if (again) refresh();
    }
  }

  function listen() {
    const m = main();
    if (!m || !m.dataset.base || !window.EventSource) return;
    const es = new EventSource(`${m.dataset.base}/live`);
    const dot = document.getElementById("sb-live");
    const setLive = (text, cls) => { if (dot) { dot.textContent = text; dot.className = "sb-live " + cls; } };
    es.addEventListener("open", () => setLive("live", "on"));
    es.addEventListener("error", () => setLive("reconnecting…", "off"));
    const onVersion = (e) => { if (String(e.data) !== m.dataset.version) refresh(); };
    es.addEventListener("update", onVersion);
    // On reconnect the server says hello with its current version; catch up if we missed changes.
    es.addEventListener("hello", onVersion);
    const status = document.querySelector("[data-live-status]");
    if (status) es.addEventListener("hello", () => {
      status.textContent = "This page updates by itself when the referee decides. You can also close it and come back later.";
    });
  }

  // Deadlines are printed in the room's time zone; add the viewer's own
  // clock when it differs, since players may live elsewhere.
  function localizeTimes() {
    let here;
    try { here = Intl.DateTimeFormat().resolvedOptions().timeZone; } catch { return; }
    document.querySelectorAll("time[data-local]").forEach((t) => {
      if (t.dataset.done || t.dataset.tz === here) return;
      t.dataset.done = "1";
      const d = new Date(t.getAttribute("datetime"));
      if (isNaN(d)) return;
      const s = d.toLocaleString(undefined, { weekday: "short", hour: "numeric", minute: "2-digit", timeZoneName: "short" });
      t.after(el("span", { class: "local-time" }, ` (${s} your time)`));
    });
  }

  // Status bar countdown to the next lock.
  function countdown() {
    document.querySelectorAll("time[data-countdown]").forEach((t) => {
      const ms = new Date(t.getAttribute("datetime")) - Date.now();
      if (isNaN(ms)) return;
      if (!t.dataset.text) t.dataset.text = t.textContent;
      const m = Math.max(0, Math.round(ms / 60000));
      const h = Math.floor(m / 60), d = Math.floor(h / 24);
      t.textContent = ms <= 0 ? "any moment" : d >= 1 ? `in ${d} d ${h % 24} h` : h >= 1 ? `in ${h} h ${m % 60} m` : `in ${m} m`;
      t.title = t.dataset.text;
    });
  }

  // ---------- forms and controls ----------

  document.addEventListener("submit", async (e) => {
    const form = e.target;
    if (form.dataset.confirm && !confirm(form.dataset.confirm)) { e.preventDefault(); return; }
    if (!form.hasAttribute("data-enhance")) return;
    e.preventDefault();
    const action = e.submitter?.getAttribute("formaction") || form.getAttribute("action");
    // Read the fields before disabling them: disabled controls aren't submitted.
    const body = new URLSearchParams(new FormData(form, e.submitter));
    const buttons = form.querySelectorAll("button, select");
    buttons.forEach((b) => (b.disabled = true));
    try {
      const res = await fetch(action, {
        method: "POST",
        body,
        headers: { "X-Requested-With": "fetch" },
      });
      if (!res.ok) alert((await res.text()) || "That didn't work.");
      await refresh();
      if (res.ok && form.id === "order-form") {
        orders.load();
        if (e.submitter?.value === "commit") orders.close();
      }
    } catch {
      alert("Couldn't reach the server. Check your connection and try again.");
    } finally {
      buttons.forEach((b) => (b.disabled = false));
    }
  });

  document.addEventListener("change", (e) => {
    if (e.target.id === "fog-toggle") {
      map.svg?.classList.toggle("show-fog", e.target.checked);
      return;
    }
    if (e.target.id === "sectors-toggle") {
      map.svg?.classList.toggle("no-sectors", !e.target.checked);
      try { localStorage.setItem("aw-sectors", e.target.checked ? "on" : "off"); } catch {}
      return;
    }
    if (e.target.id === "labels-toggle") {
      map.svg?.classList.toggle("no-labels", !e.target.checked);
      try { localStorage.setItem("aw-labels", e.target.checked ? "on" : "off"); } catch {}
      return;
    }
    const form = e.target.closest("form[data-autosubmit]");
    if (form) form.requestSubmit();
  });

  document.addEventListener("click", async (e) => {
    const t = e.target.closest("[data-copy], [data-zoom], [data-goto]");
    if (t && t.dataset.goto && !document.getElementById("map")) return;
    if (!t) return;
    if (t.dataset.copy) {
      try {
        await navigator.clipboard.writeText(t.dataset.copy);
        t.textContent = "Copied";
        setTimeout(() => (t.textContent = "Copy"), 1500);
      } catch {
        t.textContent = "Select and copy";
      }
    } else if (t.dataset.zoom) {
      map.zoom(t.dataset.zoom);
    } else if (t.dataset.goto) {
      if (!map.svg) return; // another screen: follow the link to the map
      e.preventDefault();
      map.svg?.focus({ preventScroll: true });
      map.svg?.scrollIntoView({ block: "nearest", behavior: "smooth" });
      map.select(t.dataset.goto, true);
    }
  });

  // Replays: a report line's Replay link plays the move or fight back on a
  // close-up of the board in a dialog. Only here, only when asked: the live
  // map never animates.
  const replay = {
    run: 0, // bumped to cancel a replay in progress
    still: matchMedia("(prefers-reduced-motion: reduce)").matches,
    $(id) { return document.getElementById(id); },
    data(key) {
      try { return JSON.parse(this.$("replay-data")?.textContent || "{}")?.[key]; } catch { return null; }
    },
    open(key) {
      const d = this.data(key);
      const dialog = this.$("replay-dialog");
      if (!d || !dialog) return;
      this.current = d;
      if (!dialog.dataset.wired) {
        dialog.dataset.wired = "1";
        dialog.addEventListener("click", (e) => { if (e.target === dialog || e.target.closest("[data-close]")) dialog.close(); });
        dialog.addEventListener("close", () => this.run++);
        this.$("replay-again").addEventListener("click", () => this.play(this.current));
      }
      this.$("replay-title").textContent = d.title;
      dialog.showModal();
      this.play(d);
    },
    frame(d) {
      const pts = d.units.flatMap((u) => [...u.points, ...(u.endAt ? [u.endAt] : [])]);
      pts.push(d.focus);
      const xs = pts.map((p) => p[0]), ys = pts.map((p) => p[1]);
      let x0 = Math.min(...xs) - 34, x1 = Math.max(...xs) + 34, y0 = Math.min(...ys) - 34, y1 = Math.max(...ys) + 34;
      const w = Math.max(x1 - x0, 150), h = Math.max(y1 - y0, 110);
      const cx = (x0 + x1) / 2, cy = (y0 + y1) / 2;
      this.$("replay-svg").setAttribute("viewBox", `${(cx - w / 2).toFixed(1)} ${(cy - h / 2).toFixed(1)} ${w.toFixed(1)} ${h.toFixed(1)}`);
    },
    counter(u) {
      const g = document.createElementNS(SVGNS, "g");
      g.setAttribute("class", `counter side-${u.side}${u.contact ? " contact" : ""}`);
      const add = (tag, attrs, text) => {
        const n = document.createElementNS(SVGNS, tag);
        for (const [k, v] of Object.entries(attrs)) n.setAttribute(k, v);
        if (text != null) n.textContent = text;
        g.append(n);
        return n;
      };
      add("title", {}, u.name);
      add("rect", { class: "ctr-edge", x: -8.4, y: -8.4, width: 18, height: 18, rx: 1 });
      add("rect", { class: "ctr", x: -9, y: -9, width: 18, height: 18, rx: 1 });
      add("rect", { class: "ctr-line", x: -8.2, y: -8.2, width: 16.4, height: 16.4, rx: 0.6 });
      if (u.contact) {
        add("text", { class: "ctr-q", y: 4.2 }, u.abbr || "?");
      } else {
        add("text", { class: "ctr-id", y: -5 }, u.abbr);
        if (u.symbol) add("use", { href: `#s-${u.symbol}`, y: -0.2 });
        u.strengthText = add("text", { class: "ctr-f", y: 7.6 }, u.before ? String(u.before) : "");
      }
      return g;
    },
    place(g, p) { g.setAttribute("transform", `translate(${p[0].toFixed(1)} ${(p[1] + 1.5).toFixed(1)})`); },
    sleep(ms, run) {
      return new Promise((ok) => setTimeout(() => ok(run === this.run), this.still ? 0 : ms));
    },
    slide(g, a, b, ms, run) {
      if (this.still) { this.place(g, b); return Promise.resolve(run === this.run); }
      return new Promise((ok) => {
        const t0 = performance.now();
        const step = (t) => {
          if (run !== this.run) return ok(false);
          const k = Math.min(1, (t - t0) / ms), e = k < 0.5 ? 2 * k * k : 1 - (-2 * k + 2) ** 2 / 2;
          this.place(g, [a[0] + (b[0] - a[0]) * e, a[1] + (b[1] - a[1]) * e]);
          k < 1 ? requestAnimationFrame(step) : ok(true);
        };
        requestAnimationFrame(step);
      });
    },
    caption(text) { this.$("replay-caption").textContent = text; },
    async play(d) {
      const run = ++this.run;
      const svg = this.$("replay-svg");
      const trails = svg.querySelector(".replay-trails"), units = svg.querySelector(".replay-units"), marks = svg.querySelector(".replay-marks");
      trails.replaceChildren(); units.replaceChildren(); marks.replaceChildren();
      this.$("replay-dice").hidden = true;
      this.$("replay-result").textContent = "";
      this.frame(d);
      const list = d.units.map((u) => ({ ...u }));
      for (const u of list) {
        if (u.points.length > 1) {
          const line = document.createElementNS(SVGNS, "polyline");
          line.setAttribute("class", `replay-trail ${u.enemy ? "enemy" : "friend"}`);
          line.setAttribute("points", u.points.map((p) => p.map((n) => n.toFixed(1)).join(",")).join(" "));
          trails.append(line);
        }
        u.g = this.counter(u);
        this.place(u.g, u.points[0]);
        // Contacts appear only at contact.
        if (u.contact && d.kind === "fight") u.g.classList.add("replay-hidden");
        units.append(u.g);
      }
      // 1. Movement, every unit a hex at a time.
      const steps = Math.max(0, ...list.map((u) => u.points.length - 1));
      const movers = list.filter((u) => u.points.length > 1);
      this.caption(movers.length ? (d.kind === "move" ? `${list[0].name} moves out from ${list[0].hexes[0]}.` : "The approach.") : "Positions at the start.");
      if (!await this.sleep(500, run)) return;
      for (let i = 0; i < steps; i++) {
        const legs = list.filter((u) => u.points.length - 1 > i).map((u) => this.slide(u.g, u.points[i], u.points[i + 1], 420, run));
        const ok = (await Promise.all(legs)).every(Boolean);
        if (!ok || run !== this.run) return;
      }
      if (d.kind === "move") {
        const u = list[0];
        this.caption(`${u.name} moved ${u.hexes[0]} → ${u.hexes[u.hexes.length - 1]}.`);
        return;
      }
      // 2. Contact.
      const f = d.fight;
      for (const u of list) u.g.classList.remove("replay-hidden");
      const burst = document.createElementNS(SVGNS, "circle");
      burst.setAttribute("class", "replay-burst");
      burst.setAttribute("cx", d.focus[0]); burst.setAttribute("cy", d.focus[1]); burst.setAttribute("r", 16);
      marks.append(burst);
      this.caption(`Contact at ${list.find((u) => u.enemy)?.hexes.at(-1) || ""}: ${f.sides[0]} against ${f.sides[1]}.`);
      if (!await this.sleep(900, run)) return;
      // 3. Dice.
      this.$("replay-dice").hidden = false;
      this.caption(`Each die showing ${f.success} or better is a hit.`);
      for (const side of [0, 1]) {
        this.$(`replay-side-${side}`).textContent = f.sides[side];
        this.$(`replay-hits-${side}`).textContent = "";
        const tray = this.$(`replay-dice-${side}`);
        tray.replaceChildren();
        const faces = f.faces?.[side] || [];
        for (const face of faces) {
          const die = el("span", { class: "die rolling" }, "·");
          tray.append(die);
          die.dataset.face = face;
        }
        if (!faces.length) tray.append(el("span", { class: "muted small" }, `${f.dice[side]} dice`));
      }
      const all = [...document.querySelectorAll("#replay-dice .die")];
      const spin = this.still ? null : setInterval(() => { for (const die of all) if (die.classList.contains("rolling")) die.textContent = 1 + Math.floor(Math.random() * 6); }, 70);
      for (const die of all) {
        if (!await this.sleep(110, run)) { clearInterval(spin); return; }
        die.classList.remove("rolling");
        die.textContent = die.dataset.face;
        if (+die.dataset.face >= f.success) die.classList.add("hit");
      }
      clearInterval(spin);
      for (const side of [0, 1]) this.$(`replay-hits-${side}`).textContent = `${f.hits[side]} ${f.hits[side] === 1 ? "hit" : "hits"}`;
      if (!await this.sleep(700, run)) return;
      // 4. Results.
      this.caption(f.verdict);
      this.$("replay-result").textContent = f.result || "";
      const after = [];
      for (const u of list) {
        if (u.lost) {
          u.g.classList.add("replay-lost");
          continue;
        }
        if (u.before && u.after < u.before) {
          if (u.strengthText) u.strengthText.textContent = String(u.after);
          const p = u.endAt || u.points.at(-1);
          const t = document.createElementNS(SVGNS, "text");
          t.setAttribute("class", "replay-loss");
          t.setAttribute("x", p[0] + 9); t.setAttribute("y", p[1] - 8);
          t.textContent = `−${u.before - u.after}`;
          marks.append(t);
        }
        if (u.endAt) after.push(this.slide(u.g, u.points.at(-1), u.endAt, 600, run));
      }
      await Promise.all(after);
    },
  };

  document.addEventListener("click", (e) => {
    const a = e.target.closest("[data-replay]");
    if (!a) return;
    e.preventDefault();
    replay.open(a.dataset.replay);
  });

  document.addEventListener("DOMContentLoaded", () => {
    map.init();
    editor.init();
    orders.init();
    localizeTimes();
    countdown();
    setInterval(countdown, 30000);
    // Links from other screens: ?hex=0708 selects a hex, ?order=m1 opens the order form.
    const q = new URLSearchParams(location.search);
    if (q.get("hex") && map.svg) map.select(q.get("hex"), true);
    if (q.get("order") && document.getElementById("order-box")) orders.open(q.get("order"));
    listen();
  });
})();
