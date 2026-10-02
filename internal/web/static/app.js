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
        if (mover.unit) { mover.click(this.toSvg(e.clientX, e.clientY)); return; }
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
    if (t && t.note) body.push(el("p", { class: "muted small" }, t.note));
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
            u.mine ? " (yours)" : "", u.hidden ? " (hidden from players)" : "", u.stale ? " (where they last called in)" : ""),
          el("div", { class: "muted small" }, [u.class, u.strength, u.commander].filter(Boolean).join(" · ")),
          u.status ? el("div", { class: "small unit-status" }, u.status) : null,
          el("div", { class: "row tight" },
            data.orders?.open && data.orders.units?.some((x) => x.id === u.id)
              ? el("button", { type: "button", class: "link small", "data-order-unit": u.id }, data.orders.orders?.[u.id] ? "Edit order" : "Write order") : null,
            document.getElementById("main")?.dataset.referee === "1" && !document.querySelector("[data-tool]")
              ? el("button", { type: "button", class: "link small", "data-move-counter": u.id, "data-name": u.name }, "Move") : null)));
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
  // An order is what the player says their marine does, in their own words,
  // plus an optional spot on the map. With script, the form opens in a
  // lightbox, and the spot can be picked by clicking the map.

  const orders = {
    box: null,
    form: null,
    dialog: null,
    pickbar: null,
    mode: "closed", // closed, dialog, or picking (choosing a hex on the map)

    init() {
      this.box = document.getElementById("order-box");
      if (!this.box) return;
      this.form = document.getElementById("order-form");
      const close = el("button", { type: "button", class: "secondary small-btn", "aria-label": "Close" }, "Close");
      this.dialog = el("dialog", { id: "order-dialog", class: "order-dialog", "aria-labelledby": "od-title" },
        el("div", { class: "od-head" }, el("h2", { id: "od-title" }, "Write an order"), close), this.form);
      document.body.append(this.dialog);
      close.addEventListener("click", () => this.dialog.close());
      this.dialog.addEventListener("close", () => {
        if (this.mode === "dialog") { this.mode = "closed"; this.drawMark(); }
      });
      const back = el("button", { type: "button", class: "small-btn" }, "Back to the order");
      this.pickbar = el("div", { class: "pick-bar", role: "status", hidden: "" },
        el("span", { id: "pick-hint" }, "Click the map to mark a spot."), back);
      document.body.append(this.pickbar);
      back.addEventListener("click", () => this.backToDialog());
      document.addEventListener("keydown", (e) => { if (e.key === "Escape" && this.mode === "picking") this.backToDialog(); });
      this.box.addEventListener("click", (e) => {
        if (!e.target.closest("summary")) return;
        e.preventDefault();
        this.open(this.unit());
      });
      document.addEventListener("click", (e) => {
        if (e.target.closest("#of-pick")) this.pick();
        if (e.target.closest("#of-unmark")) { this.$("of-mark").value = ""; this.drawMark(); }
        const b = e.target.closest("[data-order-unit]");
        if (b) { e.preventDefault(); this.open(b.dataset.orderUnit); }
      });
      document.addEventListener("change", (e) => {
        if (e.target.id === "of-unit") this.load();
        else if (e.target.id === "of-mark") this.drawMark();
      });
      this.load();
    },

    data() { return mapData()?.orders; },
    $(id) { return document.getElementById(id); },
    unit() { return this.$("of-unit")?.value; },

    // syncUnits rebuilds the unit list from live data: marines can be
    // assigned or taken away while the page is open. It keeps the choice.
    syncUnits() {
      const d = this.data();
      const sel = this.$("of-unit");
      if (!d || !sel) return false;
      const units = d.units || [];
      const keep = sel.value;
      const same = sel.options.length === units.length && units.every((u, i) => sel.options[i].value === u.id);
      if (!same) {
        sel.replaceChildren(...units.map((u) => el("option", { value: u.id, "data-hex": u.hex }, u.label)));
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
      const hex = this.$("of-unit").selectedOptions[0]?.dataset.hex;
      if (hex && map.svg) map.select(hex, true);
      this.mode = "dialog";
      this.pickbar.hidden = true;
      if (!this.dialog.open) this.dialog.showModal();
      this.$("of-text").focus();
    },

    pick() {
      this.mode = "picking";
      this.dialog.close();
      this.pickbar.hidden = false;
      map.svg?.scrollIntoView({ block: "center", behavior: "smooth" });
    },

    backToDialog() {
      this.pickbar.hidden = true;
      this.mode = "dialog";
      this.dialog.showModal();
      this.drawMark();
    },

    // close is called after a commit goes through.
    close() {
      this.mode = "closed";
      this.pickbar.hidden = true;
      if (this.dialog.open) this.dialog.close();
      this.drawMark();
    },

    // load fills the form from the unit's saved order.
    load() {
      const d = this.data();
      if (!d || !this.$("of-unit")) return;
      this.syncUnits();
      const o = d.orders[this.unit()];
      this.$("of-text").value = o?.text || "";
      this.$("of-mark").value = o?.mark || "";
      this.$("od-title").textContent = `Orders: ${d.units.find((u) => u.id === this.unit())?.label || ""}`;
      this.status();
      this.drawMark();
    },

    status() {
      const o = this.data()?.orders[this.unit()];
      const w = this.$("of-withdraw");
      this.$("of-status").textContent = !o ? "No order yet."
        : `${o.status[0].toUpperCase()}${o.status.slice(1)} (version ${o.version}${o.author ? `, by ${o.author}` : ""}).`;
      if (w) {
        w.hidden = !(o && o.editable);
        if (o) w.setAttribute("formaction", `${main().dataset.base}/orders/${o.id}/withdraw`);
      }
    },

    // drawMark shows the spot being marked while the form is open.
    drawMark() {
      const layer = this.$("reach-layer");
      if (!layer) return;
      layer.replaceChildren();
      const hex = this.$("of-mark")?.value.trim();
      if (this.mode === "closed" || !hex) return;
      const { col, row } = parseHex(hex);
      if (Number.isNaN(col)) return;
      const c = hexCenter(col, row);
      const use = document.createElementNS(SVGNS, "use");
      use.setAttribute("href", "#hx");
      use.setAttribute("x", c.x.toFixed(1));
      use.setAttribute("y", c.y.toFixed(1));
      use.setAttribute("class", "reach-start");
      layer.append(use);
    },

    click(p) {
      if (this.mode !== "picking") return false;
      const data = mapData();
      const h = data && hexAt(p.x, p.y, data);
      if (!h) return false;
      const hex = hexId(h.col, h.row);
      this.$("of-mark").value = hex;
      map.select(hex, false);
      this.backToDialog();
      return true;
    },

    // After a live update: keep what the player is typing.
    afterRefresh() {
      if (!this.box) return;
      if (this.syncUnits()) { this.load(); return; }
      this.status();
    },
  };

  // ---------- referee: moving counters during play ----------
  //
  // From the hex inspector, Move picks a counter up; the next click on the
  // map puts it down. During adjudication players don't see it move until
  // the turn is published.

  const mover = {
    unit: null, name: "",
    start(id, name) {
      this.unit = id;
      this.name = name;
      orders.pickbar && (orders.pickbar.hidden = true);
      let bar = document.getElementById("move-bar");
      if (!bar) {
        const cancel = el("button", { type: "button", class: "secondary small-btn" }, "Cancel");
        cancel.addEventListener("click", () => this.stop());
        bar = el("div", { class: "pick-bar", id: "move-bar", role: "status" }, el("span", { id: "move-hint" }), cancel);
        document.body.append(bar);
      }
      bar.hidden = false;
      document.getElementById("move-hint").textContent = `Click the hex to move ${name} to.`;
    },
    stop() {
      this.unit = null;
      const bar = document.getElementById("move-bar");
      if (bar) bar.hidden = true;
    },
    async click(p) {
      if (!this.unit) return false;
      const data = mapData();
      const h = data && hexAt(p.x, p.y, data);
      if (!h) return false;
      const id = this.unit;
      this.stop();
      const body = new URLSearchParams({ hex: hexId(h.col, h.row) });
      const res = await fetch(`${main().dataset.base}/units/${id}/edit`, { method: "POST", body, headers: { "X-Requested-With": "fetch" } });
      if (!res.ok) alert((await res.text()) || "That didn't work.");
      await refresh();
      map.select(hexId(h.col, h.row), false);
      return true;
    },
  };
  document.addEventListener("click", (e) => {
    const b = e.target.closest("[data-move-counter]");
    if (b) mover.start(b.dataset.moveCounter, b.dataset.name);
  });

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
      const res = await fetch(location.pathname + location.search, { headers: { Accept: "text/html" }, cache: "no-store" });
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
      threadToEnd();
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
      if (res.ok && form.hasAttribute("data-reset")) form.reset();
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


  // Web push: the Notifications panel turns this device on or off for the
  // room. The service worker shows them; the page tells it which seat
  // address to open for this room.
  const notify = {
    box: null,
    key() { return localStorage.getItem("aw-push-" + this.box.dataset.room) === "on"; },
    setKey(on) { try { on ? localStorage.setItem("aw-push-" + this.box.dataset.room, "on") : localStorage.removeItem("aw-push-" + this.box.dataset.room); } catch {} },
    state(text) { document.getElementById("notify-state").textContent = text; },
    show(on) {
      document.getElementById("notify-on").hidden = on;
      document.getElementById("notify-test").hidden = !on;
      document.getElementById("notify-off").hidden = !on;
    },
    async post(path, sub) {
      const res = await fetch(this.box.dataset.base + path, {
        method: "POST", headers: { "Content-Type": "application/json", "X-Requested-With": "fetch" },
        body: sub ? JSON.stringify(sub) : "{}",
      });
      if (!res.ok) throw new Error((await res.text()) || res.statusText);
    },
    async registration() {
      const reg = await navigator.serviceWorker.register("/sw.js", { scope: "/" });
      await navigator.serviceWorker.ready;
      (reg.active || navigator.serviceWorker.controller)?.postMessage({ room: this.box.dataset.room, base: this.box.dataset.base });
      return reg;
    },
    async init() {
      this.box = document.getElementById("notify");
      if (!this.box || !this.box.dataset.key) return;
      if (!window.isSecureContext || !("serviceWorker" in navigator) || !("PushManager" in window) || !("Notification" in window)) {
        this.state(window.isSecureContext ? "This browser can't receive notifications." : "Notifications need the site to be opened over HTTPS.");
        return;
      }
      document.getElementById("notify-on").addEventListener("click", () => this.on());
      document.getElementById("notify-off").addEventListener("click", () => this.off());
      document.getElementById("notify-test").addEventListener("click", async () => {
        try { await this.post("/push/test"); this.state("Test sent. It should arrive in a few seconds."); } catch (e) { this.state(e.message); }
      });
      try {
        const reg = await this.registration();
        const sub = await reg.pushManager.getSubscription();
        const on = !!sub && this.key() && Notification.permission === "granted";
        if (on) await this.post("/push/subscribe", sub.toJSON()); // keep the server in step
        this.show(on);
        this.state(on ? "On for this device." : Notification.permission === "denied"
          ? "This browser is blocking notifications from this site. Allow them in the site settings, then reload."
          : "Off for this device.");
      } catch (e) {
        this.state("Couldn't check notifications: " + e.message);
      }
    },
    async on() {
      try {
        if (await Notification.requestPermission() !== "granted") { this.state("Notifications weren't allowed."); return; }
        const reg = await this.registration();
        const raw = atob(this.box.dataset.key.replace(/-/g, "+").replace(/_/g, "/"));
        const key = Uint8Array.from(raw, (c) => c.charCodeAt(0));
        const sub = await reg.pushManager.getSubscription() || await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key });
        await this.post("/push/subscribe", sub.toJSON());
        this.setKey(true);
        this.show(true);
        this.state("On for this device.");
      } catch (e) {
        this.state("Couldn't turn notifications on: " + e.message);
      }
    },
    async off() {
      try {
        const reg = await navigator.serviceWorker.getRegistration("/");
        const sub = await reg?.pushManager.getSubscription();
        if (sub) await this.post("/push/unsubscribe", sub.toJSON());
        this.setKey(false);
        this.show(false);
        this.state("Off for this device.");
      } catch (e) {
        this.state("Couldn't turn notifications off: " + e.message);
      }
    },
  };

  // The open discussion thread scrolls to its latest message. Ctrl+Enter sends.
  function threadToEnd() {
    const t = document.getElementById("thread");
    if (t) t.scrollTop = t.scrollHeight;
  }
  document.addEventListener("keydown", (e) => {
    if (e.target.id === "post-text" && e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      e.target.form.requestSubmit();
    }
  });

  document.addEventListener("DOMContentLoaded", () => {
    threadToEnd();
    notify.init();
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
