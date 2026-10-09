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

  function hexCenter(col, row, w = HEX_W) {
    const size = w / Math.sqrt(3);
    return { x: w * (col + 0.5 * (row & 1)) + w / 2, y: 1.5 * size * row + size };
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

  // hexAt finds the hex (w wide) whose center is nearest a board point.
  // With limits, it stays on the drawn map.
  function hexAt(x, y, data, w = HEX_W) {
    const size = w / Math.sqrt(3);
    const r0 = Math.round((y - size) / (1.5 * size));
    let best = null, bd = Infinity;
    for (let r = r0 - 1; r <= r0 + 1; r++) {
      if (r < 0 || (data && r >= data.rows)) continue;
      const c0 = Math.round((x - w / 2) / w - 0.5 * (r & 1));
      for (let c = c0 - 1; c <= c0 + 1; c++) {
        if (c < 0 || (data && c >= data.cols)) continue;
        const p = hexCenter(c, r, w);
        const d = (p.x - x) ** 2 + (p.y - y) ** 2;
        if (d < bd) { bd = d; best = { col: c, row: r }; }
      }
    }
    return best;
  }

  // sideAt finds the hex side nearest a point, and how far the point is from the hex center.
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
  function svgEl(tag, attrs) {
    const n = document.createElementNS(SVGNS, tag);
    for (const [k, v] of Object.entries(attrs)) n.setAttribute(k, typeof v === "number" ? v.toFixed(1) : v);
    return n;
  }
  function svgLine(cls, x1, y1, x2, y2) { return svgEl("line", { class: cls, x1, y1, x2, y2 }); }

  function mapData() {
    const node = document.getElementById("map-data");
    try { return node ? JSON.parse(node.textContent) : null; } catch { return null; }
  }

  // ---------- the board: where things are, how far ----------

  function colLetters(n) {
    let s = "";
    for (; n >= 0; n = Math.floor(n / 26) - 1) s = String.fromCharCode(65 + (n % 26)) + s;
    return s;
  }

  function gridRef(p, d) {
    if (d.grid === "square") return colLetters(Math.floor(p.x / d.cell)) + (Math.floor(p.y / d.cell) + 1);
    if (d.grid === "hex") { const h = hexAt(p.x, p.y, null, d.cell); return h ? hexId(h.col, h.row) : ""; }
    return "";
  }

  function terrainAt(p, d) {
    if (!d.hexes) return null;
    const h = hexAt(p.x, p.y, d);
    if (!h) return null;
    const cell = document.getElementById("terrain-layer")?.children[h.row * d.cols + h.col];
    return cell ? d.terrain[cell.dataset.t] : null;
  }

  function nearestPlace(p, d) {
    let best = null, bd = Infinity;
    for (const f of d.places || []) {
      const c = Math.hypot(f.x - p.x, f.y - p.y) / d.cell;
      if (c < bd && c <= 1.5) { bd = c; best = (c <= 0.6 ? "at " : "near ") + f.name; }
    }
    return best;
  }

  function fmtNum(n) {
    return n >= 100 ? String(Math.round(n)) : String(Math.round(n * 10) / 10);
  }

  function distance(a, b, d) {
    const cells = Math.hypot(a.x - b.x, a.y - b.y) / d.cell;
    if (d.distance && d.distance.amount) return `about ${fmtNum(cells * d.distance.amount)} ${d.distance.unit}`;
    const n = fmtNum(cells);
    return `about ${n} ${n === "1" ? d.cellName : d.cellName === "hex" ? "hexes" : d.cellName + "s"}`;
  }

  // measureText is a distance in the world and in grid cells: "about 105 m · 7 hexes".
  function measureText(a, b, d) {
    const cells = Math.hypot(a.x - b.x, a.y - b.y) / d.cell;
    const n = fmtNum(cells), unit = n === "1" ? d.cellName : d.cellName === "hex" ? "hexes" : d.cellName + "s";
    if (!d.distance || !d.distance.amount) return `about ${n} ${unit}`;
    return `${distance(a, b, d)} · ${n} ${unit}`;
  }

  function whereText(p, d) {
    return [gridRef(p, d), nearestPlace(p, d)].filter(Boolean).join(", ");
  }

  // ---------- map: pan, zoom, select, drag ----------

  const map = {
    svg: null, full: null, vb: null, selected: null,
    group: new Set(), multi: false, // the referee's selection of counters, to move together
    pointers: new Map(), dragStart: null, moved: false, pinchDist: 0, dragging: null,

    init() {
      this.svg = document.getElementById("map");
      if (!this.svg) return;
      const [x, y, w, h] = this.svg.dataset.viewbox.split(/\s+/).map(Number);
      this.full = { x, y, w, h };
      this.vb = { ...this.full };

      let stored = null;
      try { stored = JSON.parse(sessionStorage.getItem("aw-map:" + location.pathname)); } catch {}
      if (stored && stored.vb && stored.full === this.svg.dataset.viewbox) this.vb = stored.vb;
      else if (this.svg.getBoundingClientRect().width < 480) this.focusStart();
      try {
        if (localStorage.getItem("aw-grid") === "off") {
          this.svg.classList.add("no-grid");
          const t = document.getElementById("grid-toggle");
          if (t) t.checked = false;
        }
        if (localStorage.getItem("aw-labels") === "off") {
          this.svg.classList.add("no-labels");
          const t = document.getElementById("labels-toggle");
          if (t) t.checked = false;
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
      this.svg.addEventListener("pointercancel", (e) => { this.pointers.delete(e.pointerId); this.editing = false; this.cancelDrag(); });
      this.svg.addEventListener("pointerleave", () => editor.hover(null));
      this.svg.addEventListener("keydown", (e) => this.key(e));
    },

    marker(id) { return id ? this.svg?.querySelector(`[data-unit="${CSS.escape(id)}"], [data-members~="${CSS.escape(id)}"]`) : null; },
    markerPos(m) { return { x: parseFloat(m.dataset.x), y: parseFloat(m.dataset.y) }; },

    // Small screens open zoomed in on the viewer's own counters, or the middle.
    focusStart() {
      const m = this.svg.querySelector("#units-layer .mine")?.closest("[data-unit]");
      const c = m ? this.markerPos(m) : { x: this.full.w / 2, y: this.full.h / 2 };
      this.vb.w = this.full.w / 2.2;
      this.clamp();
      this.vb.x = c.x - this.vb.w / 2;
      this.vb.y = c.y - this.vb.h / 2;
      this.clamp();
    },

    // Height-to-width ratio of the map window; the view box follows it so
    // the map is never stretched and narrow screens aren't letterboxed.
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
        sessionStorage.setItem("aw-map:" + location.pathname, JSON.stringify({ vb: this.vb, selected: this.selected, full: this.svg.dataset.viewbox }));
      } catch {}
    },

    clamp() {
      const f = this.full, v = this.vb, a = this.aspect();
      v.w = Math.min(f.w, f.h / a, Math.max(f.w / 8, v.w));
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
      const v = this.vb;
      const m = this.marker(this.selected);
      const p = m ? this.markerPos(m) : { x: v.x + v.w / 2, y: v.y + v.h / 2 };
      this.zoomAt(p, dir === "in" ? 0.7 : 1 / 0.7);
    },

    centerOn(c) {
      const v = this.vb, margin = v.w * 0.08;
      if (c.x < v.x + margin || c.x > v.x + v.w - margin || c.y < v.y + margin || c.y > v.y + v.h - margin) {
        v.x = c.x - v.w / 2; v.y = c.y - v.h / 2;
        this.clamp();
        this.apply();
      }
    },

    // The referee can drag counters (and, in the editor, places).
    draggable(target) {
      if (ruler.on || orders.mode === "picking" || refereeMove.unit) return null;
      if (planning.on) {
        const m = target.closest?.("[data-unit]");
        const u = mapData()?.units?.find((u) => u.id === m?.dataset.unit);
        return m && (mapData()?.referee || (u?.mine && mapData()?.orders?.open)) ? m : null;
      }
      if (main()?.dataset.phase === "complete" || (main()?.dataset.referee !== "1" && !editor.active)) return null;
      if (editor.active && editor.tool !== "inspect") return null;
      const m = target.closest?.("[data-unit], [data-place]");
      if (!m || (m.dataset.place != null && !editor.active)) return null;
      return m;
    },

    down(e) {
      this.svg.setPointerCapture(e.pointerId);
      if (this.pointers.size === 0 && e.button === 0 && editor.wantsDrag() && !planning.on) {
        this.editing = true;
        editor.dragStart(this.toSvg(e.clientX, e.clientY));
        return;
      }
      this.pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
      if (this.pointers.size === 1) {
        const piece = this.draggable(e.target);
        const add = this.canGroup() && (e.shiftKey || e.ctrlKey || e.metaKey || this.multi);
        this.dragStart = { x: e.clientX, y: e.clientY, vb: { ...this.vb }, target: e.target, piece, add,
          box: add && !piece && !e.target.closest?.("[data-unit], [data-place]") && !ruler.on };
        this.moved = false;
      } else if (this.pointers.size === 2) {
        this.cancelDrag();
        const [a, b] = [...this.pointers.values()];
        this.pinchDist = Math.hypot(a.x - b.x, a.y - b.y);
      }
    },

    move(e) {
      if (this.editing) { editor.dragMove(this.toSvg(e.clientX, e.clientY)); return; }
      if (this.pointers.size === 0) {
        const p = this.toSvg(e.clientX, e.clientY);
        editor.hover(p);
        ruler.move(p);
        return;
      }
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
      if (s.box) { this.drawBox(this.toSvg(s.x, s.y), this.toSvg(e.clientX, e.clientY)); return; }
      if (s.piece) { this.dragPiece(s.piece, this.toSvg(e.clientX, e.clientY)); return; }
      this.svg.classList.add("dragging");
      const scale = s.vb.w / this.svg.getBoundingClientRect().width;
      this.vb.x = s.vb.x - dx * scale;
      this.vb.y = s.vb.y - dy * scale;
      this.clamp();
      this.apply();
    },

    // dragPiece moves a counter or place with the pointer, or the whole
    // group when the counter is part of a selection of several.
    dragPiece(piece, p) {
      if (!this.dragging) {
        const id = piece.dataset.unit;
        const els = id && this.group.size > 1 && this.group.has(id)
          ? [...this.group].map((g) => this.marker(g)).filter(Boolean) : [piece];
        const start = this.toSvg(this.dragStart.x, this.dragStart.y);
        this.dragging = {
          start,
          pieces: els.map((el) => {
            const t = el.getAttribute("transform") || "";
            const at = (t.match(/translate\(([-\d.]+)[ ,]+([-\d.]+)\)/) || []).slice(1).map(Number);
            return { el, from: t, scale: (t.match(/scale\(([^)]+)\)/) || [])[1] || "1", x0: at[0] || 0, y0: at[1] || 0 };
          }),
        };
        els.forEach((el) => el.classList.add("lifted"));
      }
      const g = this.dragging;
      const dx = p.x - g.start.x, dy = p.y - g.start.y;
      for (const m of g.pieces) {
        m.to = { x: Math.min(this.full.w, Math.max(0, m.x0 + dx)), y: Math.min(this.full.h, Math.max(0, m.y0 + dy)) };
        m.el.setAttribute("transform", `translate(${m.to.x.toFixed(1)} ${m.to.y.toFixed(1)}) scale(${m.scale})`);
      }
      const d = mapData();
      if (g.pieces.length > 1) ruler.hint(`Moving ${g.pieces.length} counters together`);
      else if (d) ruler.hint(`${piece.querySelector("title")?.textContent.split(",")[0] || ""} → ${whereText(g.pieces[0].to, d) || "here"}`);
    },

    cancelDrag() {
      if (!this.dragging) return;
      ruler.hint("");
      for (const m of this.dragging.pieces) {
        m.el.setAttribute("transform", m.from);
        m.el.classList.remove("lifted");
      }
      this.dragging = null;
    },

    async dropPiece() {
      const g = this.dragging;
      this.dragging = null;
      ruler.hint("");
      g.pieces.forEach((m) => m.el.classList.remove("lifted"));
      const moved = g.pieces.filter((m) => m.to);
      if (!moved.length) return;
      const units = moved.filter((m) => m.el.dataset.unit);
      if (editor.active && !planning.on) {
        const places = mapData()?.places || [];
        editor.send(moved.flatMap((m) => m.el.dataset.unit
          ? (m.el.dataset.members || m.el.dataset.unit).split(" ").map((id) => ({ op: "unit-move", unit: id, x: +m.to.x.toFixed(1), y: +m.to.y.toFixed(1) }))
          : [{ op: "feature-move", feature: places[+m.el.dataset.place]?.id, x: +m.to.x.toFixed(1), y: +m.to.y.toFixed(1) }])
          .filter((op) => op.unit || op.feature));
        return;
      }
      if (!units.length) return;
      const body = new URLSearchParams();
      for (const m of units) {
        for (const id of (m.el.dataset.members || m.el.dataset.unit).split(" ")) {
          if (planning.on && !mapData()?.referee && !mapData()?.units?.find((u) => u.id === id)?.mine) continue;
          body.append("unit", id);
          body.append("x", m.to.x.toFixed(1));
          body.append("y", m.to.y.toFixed(1));
        }
      }
      try {
        const res = await fetch(`${main().dataset.base}/${planning.on ? "plans" : "units/move"}`, { method: "POST", body, headers: { "X-Requested-With": "fetch" } });
        if (!res.ok) throw new Error((await res.text()) || "That didn't work.");
        if (planning.on && body.getAll("unit").includes(orders.unit())) orders.setMark({ x: +body.get("x"), y: +body.get("y") });
      } catch (err) { alert(err.message || "Couldn't reach the server."); }
      units.forEach((m) => m.el.setAttribute("transform", m.from));
      await refresh();
    },

    // --- selecting several counters (the referee) ---

    canGroup() { return main()?.dataset.phase !== "complete" && !planning.on && (main()?.dataset.referee === "1" || editor.active); },

    paintGroup() {
      this.svg?.querySelectorAll("[data-unit]").forEach((m) => m.classList.toggle("picked", this.group.has(m.dataset.unit)));
    },

    toggleInGroup(id) {
      if (this.selected && !this.group.size) this.group.add(this.selected);
      this.group.has(id) ? this.group.delete(id) : this.group.add(id);
      this.afterGroupChange();
    },

    afterGroupChange() {
      this.paintGroup();
      if (this.group.size === 1) { this.select([...this.group][0], false); return; }
      this.selected = null;
      this.cursorAt(null);
      this.save();
      if (this.group.size > 1) showGroup(); else this.clearPanel();
    },

    clearGroup() {
      this.group.clear();
      this.paintGroup();
    },

    clearPanel() {
      const box = document.getElementById("inspector");
      if (box) box.replaceChildren(el("p", { class: "muted small" }, "Nothing selected."));
    },

    setMulti(on) {
      this.multi = on;
      document.querySelectorAll("[data-multi]").forEach((b) => b.setAttribute("aria-pressed", on ? "true" : "false"));
      ruler.hint(on ? "Selecting several: click counters to add or remove them, or drag a box around them." : "");
    },

    drawBox(a, b) {
      const r = document.getElementById("select-box");
      if (!r) return;
      r.setAttribute("x", Math.min(a.x, b.x).toFixed(1));
      r.setAttribute("y", Math.min(a.y, b.y).toFixed(1));
      r.setAttribute("width", Math.abs(a.x - b.x).toFixed(1));
      r.setAttribute("height", Math.abs(a.y - b.y).toFixed(1));
      r.setAttribute("visibility", "visible");
      this.box = { x1: Math.min(a.x, b.x), y1: Math.min(a.y, b.y), x2: Math.max(a.x, b.x), y2: Math.max(a.y, b.y) };
    },

    endBox() {
      document.getElementById("select-box")?.setAttribute("visibility", "hidden");
      const b = this.box;
      this.box = null;
      if (!b) return;
      if (this.selected && !this.group.size) this.group.add(this.selected);
      this.svg.querySelectorAll("[data-unit]").forEach((m) => {
        const p = this.markerPos(m);
        if (p.x >= b.x1 && p.x <= b.x2 && p.y >= b.y1 && p.y <= b.y2) this.group.add(m.dataset.unit);
      });
      this.afterGroupChange();
    },

    // After a live update the counters are new elements: mark the selection again.
    afterRefresh() {
      for (const id of [...this.group]) if (!this.marker(id)) this.group.delete(id);
      this.paintGroup();
      if (this.group.size > 1) showGroup();
      else if (this.selected) this.select(this.selected, false);
    },

    up(e) {
      if (this.editing) { this.editing = false; editor.dragEnd(); return; }
      const s = this.dragStart;
      this.pointers.delete(e.pointerId);
      this.svg.classList.remove("dragging");
      if (this.pointers.size > 0) return;
      this.dragStart = null;
      if (this.dragging) { this.dropPiece(); return; }
      if (s && s.box && this.moved) { this.endBox(); return; }
      if (s && !this.moved) {
        const p = this.toSvg(e.clientX, e.clientY);
        if (refereeMove.unit) { refereeMove.send(refereeMove.unit, p); return; }
        if (ruler.click(p, s.target)) return;
        if (!planning.on && editor.click(p)) return;
        if (orders.click(p)) return;
        const hit = s.target.closest && s.target.closest("[data-unit], [data-place]");
        if (hit && hit.dataset.unit && s.add) this.toggleInGroup(hit.dataset.unit);
        else if (hit && hit.dataset.unit) this.select(hit.dataset.unit, false);
        else if (hit) { this.clearGroup(); this.showPlace(+hit.dataset.place); }
        else if (s.add) { /* a click on open ground while adding: keep the selection */ }
        else { this.clearGroup(); this.point(p); }
      }
    },

    key(e) {
      const marker = e.target.closest?.(".marker[data-unit]");
      if (marker && (e.key === "Enter" || e.key === " ")) {
        e.preventDefault(); this.select(marker.dataset.unit, false); return;
      }
      const step = this.vb.w / 10;
      const moves = { ArrowLeft: [-step, 0], ArrowRight: [step, 0], ArrowUp: [0, -step], ArrowDown: [0, step] };
      if (moves[e.key]) {
        e.preventDefault();
        this.vb.x += moves[e.key][0]; this.vb.y += moves[e.key][1];
        this.clamp(); this.apply();
      } else if (e.key === "+" || e.key === "=") { this.zoom("in"); }
      else if (e.key === "-") { this.zoom("out"); }
      else if (e.key === "0") { this.zoom("reset"); }
      else if (e.key === "m" || e.key === "M") { ruler.toggle(); }
      else if (e.key === "Escape") { refereeMove.stop(); this.cancelDrag(); ruler.stop(); this.setMulti(false); this.clear(); this.clearPanel(); }
    },

    cursorAt(p) {
      const c = document.getElementById("cursor");
      if (!c) return;
      if (!p) { c.setAttribute("visibility", "hidden"); return; }
      c.setAttribute("cx", p.x.toFixed(1));
      c.setAttribute("cy", p.y.toFixed(1));
      c.setAttribute("visibility", "visible");
    },

    clear() {
      this.selected = null;
      this.clearGroup();
      this.cursorAt(null);
      this.save();
    },

    select(id, pan) {
      if (!this.svg) return;
      const m = this.marker(id);
      if (!m) { this.clear(); return; }
      this.selected = id;
      if (this.canGroup() && !(this.group.size === 1 && this.group.has(id))) {
        this.group.clear();
        this.group.add(id);
        this.paintGroup();
      }
      const p = this.markerPos(m);
      this.cursorAt(p);
      if (pan) this.centerOn(p);
      this.save();
      inspect(id);
      judge.selectUnit(id);
    },

    showPlace(i) {
      const d = mapData(), f = d?.places?.[i];
      const box = document.getElementById("inspector");
      if (!f || !box) return;
      box.replaceChildren(...[el("h3", {}, f.name), el("p", { class: "muted small" }, [f.where, f.kind].filter(Boolean).join(" · ")),
        f.note ? el("p", { class: "small" }, f.note) : null, f.hidden ? el("p", { class: "small" }, "Hidden from players.") : null,
        el("div", { class: "row tight" }, el("button", { type: "button", class: "link small", "data-measure-place": String(i) }, "Measure from here"))].filter(Boolean));
    },


    // A click on open ground: where it is.
    point(p) {
      const d = mapData();
      const box = document.getElementById("inspector");
      if (!d || !box) return;
      const t = terrainAt(p, d);
      this.clear();
      box.replaceChildren(...[el("h3", {}, whereText(p, d) || "This spot", t ? ` · ${t.name}` : ""),
        t && t.note ? el("p", { class: "muted small" }, t.note) : null,
        el("p", { class: "muted small" }, "To find a distance, use Measure (or press M): click where to measure from, then where to.")].filter(Boolean));
    },
  };

  // The ruler. Measure (or M) turns it on: click where to measure from,
  // then where to. Clicking a counter or a place snaps to it. The line
  // follows the pointer only between the two clicks; a third click starts
  // over. Escape, or Measure again, puts it away.
  const ruler = {
    on: false, a: null, b: null,
    layer() { return document.getElementById("ruler-layer"); },
    buttons() { return document.querySelectorAll("[data-ruler]"); },

    toggle() { this.on ? this.stop() : this.start(); },

    start(from) {
      if (!map.svg) return;
      if (orders.mode === "picking") orders.stopPicking();
      this.on = true;
      this.a = from || null;
      this.b = null;
      map.svg.classList.add("measuring");
      this.buttons().forEach((b) => b.setAttribute("aria-pressed", "true"));
      this.layer()?.replaceChildren();
      if (from) this.mark(from);
      this.hint(from ? `Measuring from ${from.name}: click where to.` : "Click where to measure from: a counter, a place, or anywhere.");
    },

    stop() {
      if (!this.on) return;
      this.on = false;
      this.a = this.b = null;
      map.svg?.classList.remove("measuring");
      this.buttons().forEach((b) => b.setAttribute("aria-pressed", "false"));
      this.layer()?.replaceChildren();
      this.hint("");
    },

    // snap turns a click into a named point: a counter, a place, or the spot.
    snap(p, target) {
      const d = mapData();
      const m = target?.closest?.("[data-unit]");
      if (m) {
        const u = d?.units?.find((x) => x.id === m.dataset.unit);
        return { ...map.markerPos(m), name: u?.name || "the counter" };
      }
      const f = target?.closest?.("[data-place]");
      const place = f && d?.places?.[+f.dataset.place];
      if (place) return { x: place.x, y: place.y, name: place.name };
      return { x: p.x, y: p.y, name: (d && whereText(p, d)) || "there" };
    },

    click(p, target) {
      if (!this.on) return false;
      const pt = this.snap(p, target);
      if (!this.a || this.b) {
        this.a = pt;
        this.b = null;
        this.layer()?.replaceChildren();
        this.mark(pt);
        this.hint(`Measuring from ${pt.name}: click where to.`);
        return true;
      }
      this.b = pt;
      const d = mapData();
      this.draw(this.a, this.b);
      const text = `${this.a.name} to ${this.b.name}: ${measureText(this.a, this.b, d)}.`;
      this.hint(`${text} Click to measure again; Esc to stop.`);
      const box = document.getElementById("inspector");
      if (box) box.replaceChildren(el("h3", {}, "Measured"), el("p", {}, text));
      return true;
    },

    move(p) {
      if (this.on && this.a && !this.b) this.draw(this.a, p);
    },

    mark(p) {
      const k = mapData()?.k || 1;
      this.layer()?.append(svgEl("circle", { class: "ruler-end", cx: p.x, cy: p.y, r: 3.2 * k }));
    },

    draw(a, b) {
      const d = mapData(), layer = this.layer();
      if (!d || !layer) return;
      const k = d.k || 1;
      const text = svgEl("text", { class: "ruler-text", x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 - 6 * k, "font-size": 7.5 * k });
      text.textContent = measureText(a, b, d);
      const back = svgEl("rect", { class: "ruler-label", rx: 2 * k });
      layer.replaceChildren(svgEl("line", { class: "ruler", x1: a.x, y1: a.y, x2: b.x, y2: b.y }), back, text);
      this.mark(a);
      this.mark(b);
      const box = text.getBBox();
      const pad = 2 * k;
      [["x", box.x - pad], ["y", box.y - pad], ["width", box.width + 2 * pad], ["height", box.height + 2 * pad]]
        .forEach(([n, v]) => back.setAttribute(n, v.toFixed(1)));
    },

    hint(text) {
      let h = document.getElementById("map-hint");
      if (!h) {
        const wrap = document.querySelector(".map-wrap");
        if (!wrap) return;
        h = el("p", { id: "map-hint", class: "map-hint", role: "status" });
        wrap.append(h);
      }
      h.textContent = text || "";
      h.hidden = !text;
    },
  };

  document.addEventListener("click", (e) => {
    if (e.target.closest("[data-ruler]")) { ruler.toggle(); return; }
    const u = e.target.closest("[data-measure-unit]");
    if (u) {
      const m = map.marker(u.dataset.measureUnit);
      if (m) ruler.start({ ...map.markerPos(m), name: u.dataset.name });
      return;
    }
    const pl = e.target.closest("[data-measure-place]");
    const place = pl && mapData()?.places?.[+pl.dataset.measurePlace];
    if (place) ruler.start({ x: place.x, y: place.y, name: place.name });
  });

  function showGroup() {
    const box = document.getElementById("inspector");
    const d = mapData();
    if (!box || !d) return;
    const names = [...map.group].map((id) => d.units?.find((u) => u.id === id)?.name).filter(Boolean);
    box.replaceChildren(el("h3", {}, `${names.length} counters selected`),
      el("p", { class: "small" }, names.join(", ")),
      el("p", { class: "muted small" }, "Drag any of them to move them all together. Shift-click (or Select several) adds or removes one."),
      el("div", { class: "row tight" }, el("button", { type: "button", class: "link small", "data-clear-group": "" }, "Clear selection")));
  }

  document.addEventListener("click", (e) => {
    if (e.target.closest("[data-multi]")) { map.setMulti(!map.multi); return; }
    if (e.target.closest("[data-clear-group]")) { map.clear(); map.clearPanel(); }
  });

  function inspect(id) {
    const box = document.getElementById("inspector");
    const data = mapData();
    if (!box || !data) return;
    const u = data.units?.find((x) => x.id === id);
    if (!u) { box.replaceChildren(el("p", { class: "muted small" }, "Not on the map any more.")); return; }
    const orderID = data.orders?.units?.find((x) => x.id === u.id || u.members?.includes(x.id))?.id;
    const canOrder = !u.destroyed && data.orders?.open && orderID;
    const ref = data.referee && main()?.dataset.phase !== "complete";
    const staged = data.plans?.find((p) => p.unit === (u.card || u.id) || u.members?.includes(p.unit));
    const chooser = document.getElementById("referee-unit");
    if (chooser) chooser.value = u.card || u.id;
    box.replaceChildren(...[
      el("div", {}, el("span", { class: `abbr side-${u.side}`, "aria-hidden": "true" }, u.abbr),
        el("a", { href: `${main().dataset.base}/units/${u.card || u.id}${u.members?.length ? "?squad=1" : ""}`, class: "unit-link", "data-card": "" }, el("strong", {}, u.name)),
        u.mine ? " (yours)" : "", u.hidden ? " (hidden from players)" : "", u.stale ? " (where they last called in)" : ""),
      el("div", { class: "muted small" }, [u.class, u.strength, u.where, u.commander].filter(Boolean).join(" · ")),
      u.destroyed ? el("div", { class: "small destroyed-status" }, "Destroyed · out of action") : null,
      u.status ? el("div", { class: "small unit-status" }, u.status) : null,
      ref && staged ? el("p", { class: "staged-destination small" }, `Staged destination: ${whereText(staged.to, data) || `(${staged.to.x}, ${staged.to.y})`}. Applies when you publish.`) : null,
      ref ? el("p", { class: "muted small" }, main()?.dataset.phase === "resolving" ? "Drag to stage its final position. Publishing applies the move." : "Drag it to move it.") : null,
      el("div", { class: "row tight" },
        ref && !u.destroyed ? el("button", { type: "button", class: "secondary small-btn", "data-ref-move": u.id }, "Move chit · click destination") : null,
        el("button", { type: "button", class: "link small", "data-measure-unit": u.id, "data-name": u.name }, "Measure from here"),
        canOrder ? el("button", { type: "button", class: "link small", "data-order-unit": orderID }, data.orders.orders?.[orderID] ? "Edit order" : "Write order") : null,
        el("a", { href: `${main().dataset.base}/units/${u.card || u.id}${u.members?.length ? "?squad=1" : ""}`, class: "small", "data-card": "" }, ref ? "Edit strength, status & visibility" : "Open card"))].filter(Boolean));
    if (ref && !u.destroyed) box.append(refereeMove.form(u, data));
    const ms = u.members ? data.units.filter((m) => !m.members && m.formation === u.formation) : u.formation ? [u] : [];
    for (const m of ms) {
      const row = el("p", { class: "small" }, [m.name, m.class, m.commander, m.destroyed ? "destroyed" : m.status, m.destroyed ? "out of action" : m.detached ? "split off" : m.stale ? "last known position" : "together"].filter(Boolean).join(" · "));
      box.append(row);
      if (ref && !m.destroyed) {
        const form = el("form", { method: "post", action: `${main().dataset.base}/units/${m.id}/${m.detached ? "rejoin" : "split"}`, "data-enhance": "" },
          el("button", { type: "submit", class: "secondary small-btn" }, m.detached ? `Rejoin ${m.name}` : `Split off ${m.name}`));
        box.append(form);
      }
    }
  }

  const planning = {
    on: false,
    toggle() {
      this.on = !this.on;
      map.cancelDrag(); map.clearGroup(); map.setMulti(false); ruler.stop();
      if (orders.mode === "picking") orders.stopPicking();
      document.querySelectorAll("[data-plan]").forEach((b) => b.setAttribute("aria-pressed", String(this.on)));
      ruler.hint(this.on ? (mapData()?.referee ? "Drag counters to stage final positions. Publishing applies these moves." : "Drag your counter to propose a destination for your order.") : "");
      this.draw();
    },
    draw() {
      const d = mapData();
      if (!map.svg || !d) return;
      let layer = document.getElementById("plans-layer");
      if (!layer) { layer = svgEl("g", { id: "plans-layer", "aria-hidden": "true", "pointer-events": "none" }); map.svg.append(layer); }
      layer.replaceChildren();
      const seen = new Set();
      for (const p of d.plans || []) {
        const m = map.marker(p.unit);
        if (!m) continue;
        const key = `${m.dataset.unit}:${p.to.x}:${p.to.y}`;
        if (seen.has(key)) continue;
        seen.add(key);
        const from = map.markerPos(m);
        layer.append(svgEl("line", { class: "plan-arrow", x1: from.x, y1: from.y, x2: p.to.x, y2: p.to.y, "marker-end": "url(#arrow)" }));
        const ghost = m.cloneNode(true);
        ghost.removeAttribute("data-unit"); ghost.removeAttribute("data-members");
        ghost.removeAttribute("tabindex"); ghost.removeAttribute("role");
        ghost.setAttribute("class", "plan-ghost");
        ghost.setAttribute("transform", `translate(${p.to.x} ${p.to.y}) scale(${d.k})`);
        layer.append(ghost);
      }
      document.querySelectorAll("[data-plan]").forEach((b) => b.setAttribute("aria-pressed", String(this.on)));
      const clear = document.getElementById("clear-plans");
      if (clear) clear.disabled = !(d.plans?.length);
    },
  };
  document.addEventListener("click", (e) => { if (e.target.closest("[data-plan]")) planning.toggle(); });

  // Referees can pick a destination without dragging, including on a phone.
  // The same server endpoint stages resolving moves and validates formations.
  const refereeMove = {
    unit: null,
    start(id) {
      if (!mapData()?.referee || main()?.dataset.phase === "complete" || !map.marker(id)) return;
      this.stop(); ruler.stop(); map.cancelDrag(); map.setMulti(false);
      if (orders.mode === "picking") orders.stopPicking();
      map.select(id, true);
      this.unit = id;
      map.svg.classList.add("picking");
      ruler.hint("Click the destination for this chit. Press Escape to cancel.");
      map.svg.scrollIntoView({ block: "nearest", behavior: "smooth" });
      map.svg.focus({ preventScroll: true });
    },
    stop() { this.unit = null; map.svg?.classList.remove("picking"); ruler.hint(""); },
    form(u, d) {
      const form = el("form", { class: "row tight chit-destination", "data-chit-destination": u.id });
      if (d.grid === "hex" || d.grid === "square") {
        form.append(el("label", { class: "inline small" }, d.grid === "hex" ? "Destination hex " : "Destination cell ",
          el("input", { name: "cell", required: "", maxlength: "12", class: "compact-input", placeholder: d.grid === "hex" ? "0804" : "B4" })));
      } else {
        for (const [name, value, max] of [["x", u.x, d.width], ["y", u.y, d.height]]) {
          form.append(el("label", { class: "inline small" }, `${name.toUpperCase()} `,
            el("input", { name, type: "number", required: "", min: "0", max, step: "any", value, class: "num-s" })));
        }
      }
      form.append(el("button", { type: "submit", class: "secondary small-btn" }, main()?.dataset.phase === "resolving" ? "Stage move" : "Move chit"));
      return form;
    },
    submit(form) {
      const d = mapData(), cell = form.elements.cell;
      let at;
      if (cell) {
        const value = cell.value.trim().toUpperCase();
        cell.setCustomValidity("");
        if (d.grid === "hex" && /^\d{4}$/.test(value)) {
          const col = +value.slice(0, 2) - 1, row = +value.slice(2) - 1;
          if (col >= 0 && row >= 0) at = hexCenter(col, row, d.cell);
        } else if (d.grid === "square" && /^[A-Z]+[1-9]\d*$/.test(value)) {
          const [, letters, number] = value.match(/^([A-Z]+)(\d+)$/);
          let col = 0;
          for (const letter of letters) col = col * 26 + letter.charCodeAt(0) - 64;
          at = { x: (col - 0.5) * d.cell, y: (+number - 0.5) * d.cell };
        }
        if (!at || at.x > d.width || at.y > d.height) {
          cell.setCustomValidity(d.grid === "hex" ? "Enter a hex on this map, such as 0804." : "Enter a cell on this map, such as B4.");
          cell.reportValidity();
          return;
        }
      } else at = { x: +form.elements.x.value, y: +form.elements.y.value };
      this.send(form.dataset.chitDestination, at);
    },
    async send(id, at) {
      if (!mapData()?.referee || main()?.dataset.phase === "complete") return;
      const m = map.marker(id);
      if (!m) return;
      const body = new URLSearchParams();
      for (const unit of (m.dataset.members || m.dataset.unit).split(" ")) {
        body.append("unit", unit); body.append("x", at.x.toFixed(1)); body.append("y", at.y.toFixed(1));
      }
      this.stop();
      try {
        const res = await fetch(`${main().dataset.base}/units/move`, { method: "POST", body, headers: { "X-Requested-With": "fetch" } });
        if (!res.ok) throw new Error((await res.text()) || "Couldn't move the chit.");
        await refresh();
      } catch (err) { alert(err.message || "Couldn't reach the server."); }
    },
  };

  const judge = {
    init() {
      const items = [...document.querySelectorAll("details.desk-item")];
      const first = items.find((item) => item.querySelector(".outcome-form textarea")?.value === "") || items[0];
      items.forEach((item) => { item.open = item === first; });
      if (first) map.select(first.dataset.rulingUnit, false);
    },
    open(id, select = true) {
      const item = document.getElementById(`item-${id}`);
      if (!item) return;
      document.querySelectorAll("details.desk-item").forEach((other) => { other.open = other === item; });
      if (select) map.select(item.dataset.rulingUnit, true);
      return item;
    },
    selectUnit(id) {
      if (!main()?.classList.contains("screen-desk")) return;
      const marker = map.marker(id);
      const item = [...document.querySelectorAll("details.desk-item")].find((x) => map.marker(x.dataset.rulingUnit) === marker);
      if (item) this.open(item.dataset.order, false);
    },
    destinations() {
      const d = mapData();
      if (!d) return;
      document.querySelectorAll("[data-destination-x]").forEach((node) => {
        const at = { x: +node.dataset.destinationX, y: +node.dataset.destinationY };
        node.textContent = whereText(at, d) || `(${at.x}, ${at.y})`;
      });
    },
  };

  document.addEventListener("click", (e) => {
    const move = e.target.closest("[data-ref-move]");
    if (move) { refereeMove.start(move.dataset.refMove); return; }
    if (e.target.closest("[data-select-chit]")) {
      const id = document.getElementById("referee-unit")?.value;
      if (id) map.select(id, true);
      return;
    }
    const ruling = e.target.closest("[data-ruling]");
    if (ruling) {
      e.preventDefault();
      judge.open(ruling.dataset.ruling)?.scrollIntoView({ block: "start", behavior: "smooth" });
      return;
    }
    const section = e.target.closest("[data-judge-section]");
    if (section) {
      const target = document.getElementById(section.dataset.judgeSection);
      if (target?.classList.contains("is-collapsed")) folds.show(target, target.querySelector(".fold-btn"), true);
      if (target?.tagName === "DETAILS") target.open = true;
    }
  });
  document.addEventListener("toggle", (e) => {
    if (e.target.matches("details.desk-item") && e.target.open) judge.open(e.target.dataset.order);
  }, true);

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
      if (text == null && this.moving) text = `Click where ${this.moving.name} goes. Press Escape to cancel.`;
      h.textContent = text || "";
    },

    wantsDrag() {
      return this.active && !this.moving && ["terrain", "edges", "roads"].includes(this.tool) && !!this.data()?.hexes;
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
      const x = +p.x.toFixed(1), y = +p.y.toFixed(1);
      if (this.moving) {
        const m = this.moving;
        this.moving = null;
        map.svg.classList.remove("moving");
        this.hint();
        this.send([m.kind === "unit" ? { op: "unit-move", unit: m.id, x, y } : { op: "feature-move", feature: m.id, x, y }]);
        return true;
      }
      switch (this.tool) {
        case "crossings": {
          if (!data.hexes) return true;
          const s = sideAt(p.x, p.y, data);
          if (s) this.send([{ op: "crossing", hex: hexId(s.col, s.row), dir: s.dir, crossing: document.getElementById("crossing-type").value }]);
          return true;
        }
        case "units": {
          const known = document.getElementById("unit-known").checked;
          const strength = parseInt(document.getElementById("unit-strength").value, 10) || 1;
          this.send([{ op: "unit-add", class: document.getElementById("unit-class").value, x, y, strength, known }]);
          return true;
        }
        case "places": {
          const kind = document.getElementById("place-kind").value;
          const name = document.getElementById("place-name").value.trim() || `New ${kind}`;
          this.send([{ op: "feature-add", kind, x, y, name, hidden: document.getElementById("place-hidden").checked }]);
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
      const d0 = p && this.active && ["edges", "crossings", "roads"].includes(this.tool) ? this.data() : null;
      const data = d0 && d0.hexes ? d0 : null;
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
  // plus an optional pin on the map. "Drop a pin" turns the next click on
  // the map into the pin.

  const orders = {
    box: null, pickbar: null,
    mode: "idle", // idle, or picking (choosing a spot on the map)

    init() {
      this.box = document.getElementById("order-box");
      if (!this.box) return;
      if (window.HTMLDialogElement) {
        this.box.classList.remove("is-collapsed");
        this.box.querySelector(".fold-btn")?.remove();
        this.launcher = el("button", { type: "button", class: "order-launcher", "data-order-unit": "" }, "Write an order");
        this.box.before(this.launcher);
        this.dialog = el("dialog", { class: "card-dialog order-dialog", "aria-labelledby": "of-h" });
        const close = el("button", { type: "button", class: "secondary small-btn card-close" }, "Close");
        close.addEventListener("click", () => this.dialog.close());
        this.dialog.append(close, this.box); document.body.append(this.dialog);
      }
      const cancel = el("button", { type: "button", class: "secondary small-btn" }, "Cancel");
      this.pickbar = el("div", { class: "pick-bar", role: "status", hidden: "" },
        el("span", {}, "Tap the map where you want the pin."), cancel);
      document.body.append(this.pickbar);
      cancel.addEventListener("click", () => this.stopPicking());
      document.addEventListener("keydown", (e) => { if (e.key === "Escape" && this.mode === "picking") this.stopPicking(); });
      document.addEventListener("click", (e) => {
        if (e.target.closest("#of-pick")) this.pick();
        if (e.target.closest("#of-unmark")) this.setMark(null);
        const b = e.target.closest("[data-order-unit]");
        if (b) { e.preventDefault(); this.open(b.dataset.orderUnit); }
      });
      document.addEventListener("change", (e) => { if (e.target.id === "of-unit") this.load(); });
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
        sel.replaceChildren(...units.map((u) => el("option", { value: u.id }, u.label)));
        if (units.some((u) => u.id === keep)) sel.value = keep;
      }
      sel.closest("label").hidden = units.length <= 1;
      this.box.hidden = !d.open || units.length === 0;
      if (this.launcher) this.launcher.hidden = this.box.hidden;
      if (this.box.hidden) this.dialog?.close();
      return keep !== sel.value;
    },

    open(unitId) {
      if (!this.box) return;
      const before = this.unit();
      this.syncUnits();
      if (unitId && [...this.$("of-unit").options].some((o) => o.value === unitId)) this.$("of-unit").value = unitId;
      if (before !== this.unit()) this.load();
      if (this.dialog && !this.box.hidden) this.dialog.showModal();
      else this.box.scrollIntoView({ block: "nearest", behavior: "smooth" });
      this.$("of-text").focus({ preventScroll: true });
    },

    pick() {
      ruler.stop();
      if (planning.on) planning.toggle();
      this.dialog?.close();
      this.mode = "picking";
      this.pickbar.hidden = false;
      map.svg?.classList.add("picking");
      map.svg?.scrollIntoView({ block: "nearest", behavior: "smooth" });
    },

    stopPicking() {
      if (this.mode === "picking" && this.dialog && !this.box.hidden) this.dialog.showModal();
      this.mode = "idle";
      this.pickbar.hidden = true;
      map.svg?.classList.remove("picking");
    },

    setMark(p) {
      this.$("of-mark-x").value = p ? p.x.toFixed(1) : "";
      this.$("of-mark-y").value = p ? p.y.toFixed(1) : "";
      this.drawMark();
    },

    mark() {
      const x = parseFloat(this.$("of-mark-x")?.value), y = parseFloat(this.$("of-mark-y")?.value);
      return Number.isNaN(x) || Number.isNaN(y) ? null : { x, y };
    },

    // load fills the form from the unit's saved order.
    load() {
      const d = this.data();
      if (!d || !this.$("of-unit")) return;
      this.syncUnits();
      const o = d.orders[this.unit()];
      this.$("of-h").textContent = this.unit()?.startsWith("f-") ? "Global squad order" : "Individual order";
      const inherited = o?.group && !this.unit().startsWith("f-");
      this.$("of-text").value = inherited ? "" : o?.text || "";
      const p = o ? o.mark : mapData()?.plans?.find((p) => p.unit === this.unit())?.to;
      this.$("of-mark-x").value = p ? p.x : "";
      this.$("of-mark-y").value = p ? p.y : "";
      this.status();
      this.drawMark();
    },

    status() {
      const o = this.data()?.orders[this.unit()];
      const w = this.$("of-withdraw");
      this.$("of-status").textContent = o?.group && !this.unit().startsWith("f-") ? "Following the squad order. Writing here gives this member a separate order." : !o ? "No order yet."
        : o.status === "committed" ? `Sent${o.author ? ` by ${o.author}` : ""}. You can change it until the turn locks.`
        : `${o.status[0].toUpperCase()}${o.status.slice(1)}${o.author ? `, by ${o.author}` : ""}.`;
      if (w) {
        w.hidden = !(o && o.editable);
        if (o) w.setAttribute("formaction", `${main().dataset.base}/orders/${o.id}/withdraw`);
      }
    },

    // drawMark shows the pin being placed, and says where it is and how far.
    drawMark() {
      const layer = this.$("pin-layer");
      const p = this.mark();
      const d = mapData();
      this.$("of-unmark").hidden = !p;
      this.$("of-pick").textContent = p ? "Move the pin" : "Drop a pin";
      const u = d?.units?.find((x) => x.id === this.unit());
      this.$("of-mark-label").textContent = p && d ? `Pin: ${whereText(p, d) || "on the map"}${u ? `, ${distance(u, p, d)} from ${u.name}` : ""}` : "";
      if (!layer) return;
      layer.replaceChildren();
      if (!p) return;
      const g = svgEl("g", { class: "order-mark editing", transform: `translate(${p.x.toFixed(1)} ${p.y.toFixed(1)}) scale(${d?.k || 1})` });
      g.append(svgEl("path", { class: "pin", d: "M0 0C-1.5-3.5-5-5.5-5-9.5a5 5 0 0 1 10 0C5-5.5 1.5-3.5 0 0z" }), svgEl("circle", { class: "pin-dot", cy: -9.5, r: 1.8 }));
      layer.append(g);
    },

    click(p) {
      if (this.mode !== "picking") return false;
      this.stopPicking();
      this.setMark(p);
      this.$("of-text")?.focus({ preventScroll: true });
      return true;
    },

    // After a live update: keep what the player is typing.
    afterRefresh() {
      if (!this.box) return;
      if (this.syncUnits()) { this.load(); return; }
      if (this.box.hidden && this.mode === "picking") this.stopPicking();
      this.status();
      this.drawMark();
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

  function announceTurn() {
    const m = main();
    if (!m?.dataset.seat || !m.dataset.turn) return;
    const turn = +m.dataset.turn;
    const key = `aw-turn:${m.dataset.base.split("/k/")[0]}:${m.dataset.seat}:${m.dataset.operation}`;
    let previous = 0;
    try { previous = +(localStorage.getItem(key) || 0); localStorage.setItem(key, String(Math.max(turn, previous))); } catch { return; }
    if (turn <= previous || m.dataset.phase !== "planning") return;
    document.getElementById("turn-splash")?.remove();
    const splash = el("div", { id: "turn-splash", class: "turn-splash", role: "status" }, `Turn ${turn} · orders are open`);
    document.body.append(splash);
    setTimeout(() => splash.remove(), 4200);
  }

  let refreshing = null;
  let liveTimer = null, liveDirty = false;

  // A burst of 100 orders should not make every browser fetch 100 pages.
  // Coalesce updates, stagger clients, and catch hidden tabs up when reopened.
  function scheduleRefresh() {
    liveDirty = true;
    if (document.hidden || liveTimer !== null) return;
    liveTimer = setTimeout(() => {
      liveTimer = null;
      if (document.hidden) return;
      liveDirty = false;
      refresh();
    }, 700 + Math.random() * 600);
  }
  document.addEventListener("visibilitychange", () => {
    if (!document.hidden && liveDirty) scheduleRefresh();
  });

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
      if (cur.classList.contains("screen-desk") && cur.dataset.phase !== next.dataset.phase) {
        deskDrafts.clear();
        location.reload();
        return;
      }
      const disclosures = [...cur.querySelectorAll("details[id]")].map((node) => [node.id, node.open]);
      const focused = document.activeElement?.closest("form[data-draft]");
      const focus = focused ? { key: focused.dataset.draft, name: document.activeElement.name, start: document.activeElement.selectionStart, end: document.activeElement.selectionEnd } : null;
      for (const node of doc.querySelectorAll("[data-live][id]")) {
        const old = document.getElementById(node.id);
        if (!old) continue;
        // Keep disclosure state of replaced <details>.
        if (old.tagName === "DETAILS" && old.open) node.setAttribute("open", "");
        old.replaceWith(document.importNode(node, true));
      }
      const turnChanged = cur.dataset.turn !== next.dataset.turn || cur.dataset.operation !== next.dataset.operation;
      for (const key of ["version", "turn", "phase", "operation", "seat", "automatic"]) cur.dataset[key] = next.dataset[key];
      if (turnChanged) { deskDrafts.clear(); planning.on = false; orders.dialog?.close(); orders.load(); }
      for (const [id, open] of disclosures) {
        const node = document.getElementById(id);
        if (node?.tagName === "DETAILS") node.open = open;
      }
      deskDrafts.restore(focus);
      announceTurn();
      document.getElementById("edit-preview")?.replaceChildren();
      localizeTimes();
      countdown();
      orders.afterRefresh();
      threadToEnd();
      map.afterRefresh();
      planning.draw();
      judge.destinations();
      folds.apply();
    } catch {
      // Offline or server restarting; the event stream will retry.
    } finally {
      const again = refreshing.again;
      refreshing = null;
      if (again) scheduleRefresh();
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
    const onVersion = (e) => { if (String(e.data) !== m.dataset.version) scheduleRefresh(); };
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

  // A map move or another saved ruling refreshes the desk. Keep only edited
  // fields, so server updates (including AI drafts) still reach untouched forms.
  const deskDrafts = {
    values: new Map(), revision: 0,
    record(form, field) {
      const key = form.dataset.draft;
      const draft = this.values.get(key) || { fields: new Map() };
      draft.fields.set(field.name, field.type === "checkbox" || field.type === "radio" ? field.checked : field.value);
      draft.revision = ++this.revision;
      this.values.set(key, draft);
      this.status();
    },
    restore(focus) {
      for (const form of document.querySelectorAll("form[data-draft]")) {
        const draft = this.values.get(form.dataset.draft);
        if (!draft) continue;
        for (const [name, value] of draft.fields) {
          const field = form.elements.namedItem(name);
          if (!field) continue;
          if (typeof value === "boolean") field.checked = value;
          else field.value = value;
        }
        if (focus?.key === form.dataset.draft) {
          const field = form.elements.namedItem(focus.name);
          field?.focus({ preventScroll: true });
          if (field?.setSelectionRange && focus.start != null) field.setSelectionRange(focus.start, focus.end);
        }
      }
      this.status();
    },
    status() {
      document.querySelectorAll("form[data-draft] .draft-state").forEach((node) => {
        if (this.values.has(node.closest("form").dataset.draft)) node.textContent = "Unsaved changes";
      });
      const review = document.getElementById("publish-review");
      if (!review) return;
      let note = document.getElementById("unsaved-rulings");
      if (!note) { note = el("p", { id: "unsaved-rulings", class: "alert small", role: "status" }); review.append(note); }
      note.hidden = !this.values.size;
      note.textContent = `${this.values.size} unsaved form(s). Save your changes before publishing.`;
    },
    clear() { this.values.clear(); },
  };

  document.addEventListener("input", (e) => {
    const field = e.target;
    if (field.closest("[data-chit-destination]")) field.setCustomValidity("");
    const form = field.closest("form[data-draft]");
    if (form && field.name) deskDrafts.record(form, field);
  });
  document.addEventListener("change", (e) => {
    const form = e.target.closest("form[data-draft]");
    if (form && e.target.name) deskDrafts.record(form, e.target);
  });

  document.addEventListener("submit", async (e) => {
    const form = e.target;
    if (form.hasAttribute("data-chit-destination")) { e.preventDefault(); refereeMove.submit(form); return; }
    if (form.getAttribute("action")?.endsWith("/turn/next") && deskDrafts.values.size) {
      e.preventDefault(); alert("Save your unsaved rulings and other edits before publishing."); return;
    }
    if (form.dataset.confirm && !confirm(form.dataset.confirm)) { e.preventDefault(); return; }
    if (!form.hasAttribute("data-enhance")) return;
    e.preventDefault();
    const action = e.submitter?.getAttribute("formaction") || form.getAttribute("action");
    const draftRevision = deskDrafts.values.get(form.dataset.draft)?.revision;
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
      if (res.ok && action === form.getAttribute("action") && deskDrafts.values.get(form.dataset.draft)?.revision === draftRevision) deskDrafts.values.delete(form.dataset.draft);
      if (res.ok && form.hasAttribute("data-reset")) form.reset();
      await refresh();
      if (res.ok && form.id === "order-form") {
        orders.load();
        if (e.submitter?.value === "commit") orders.stopPicking();
        orders.dialog?.close();
      }
    } catch {
      alert("Couldn't reach the server. Check your connection and try again.");
    } finally {
      buttons.forEach((b) => (b.disabled = false));
      planning.draw();
    }
  });

  document.addEventListener("change", (e) => {
    if (e.target.id === "fog-toggle") {
      map.svg?.classList.toggle("show-fog", e.target.checked);
      return;
    }
    if (e.target.id === "grid-toggle") {
      map.svg?.classList.toggle("no-grid", !e.target.checked);
      try { localStorage.setItem("aw-grid", e.target.checked ? "on" : "off"); } catch {}
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

  // ---------- collapsible sections ----------
  //
  // Every panel gets a button in its header to fold it away. The choice is
  // remembered in this browser, per room seat and screen, so a player who
  // never needs a section can keep it out of the way. Folding panels
  // (<details>) remember whether they're open the same way.

  const folds = {
    key(panel) {
      const name = panel.id || panel.querySelector("h2")?.textContent.trim().toLowerCase().replace(/\s+/g, "-") || "";
      return name ? `aw-fold:${main()?.dataset.base || ""}:${main()?.className || ""}:${name}` : "";
    },
    get(k) { try { return localStorage.getItem(k); } catch { return null; } },
    set(k, v) { try { localStorage.setItem(k, v); } catch {} },

    header(panel) {
      const h = panel.querySelector(":scope > h2, :scope > .panel-head, :scope > .desk-head > h2");
      return h && !h.classList.contains("sr-only") ? h : null;
    },

    apply() {
      document.querySelectorAll(".screen section.panel").forEach((panel) => {
        if (panel.hasAttribute("data-no-fold") || main()?.classList.contains("screen-desk") && (panel.classList.contains("map-panel") || panel.classList.contains("selected-panel") || panel.id === "referee-chits" || panel.id === "publish-review")) return;
        const head = this.header(panel);
        const k = this.key(panel);
        if (!head || !k) return;
        let btn = head.querySelector(":scope > .fold-btn");
        if (!btn) {
          btn = el("button", { type: "button", class: "fold-btn", "aria-expanded": "true" });
          head.append(btn);
        }
        const stored = this.get(k);
        this.show(panel, btn, stored ? stored === "open" : !panel.hasAttribute("data-collapsed"));
      });
      document.querySelectorAll(".screen details.panel").forEach((d) => {
        const k = this.key(d);
        const stored = k && this.get(k);
        if (stored) d.open = stored === "open";
        if (!d.dataset.foldWatch) {
          d.dataset.foldWatch = "1";
          d.addEventListener("toggle", () => { const key = this.key(d); if (key) this.set(key, d.open ? "open" : "closed"); });
        }
      });
    },

    show(panel, btn, open) {
      panel.classList.toggle("is-collapsed", !open);
      btn.setAttribute("aria-expanded", open ? "true" : "false");
      const name = panel.querySelector("h2")?.textContent.trim() || "section";
      btn.setAttribute("aria-label", `${open ? "Collapse" : "Expand"} ${name}`);
      btn.title = open ? "Collapse" : "Expand";
    },
  };

  document.addEventListener("click", (e) => {
    const btn = e.target.closest(".fold-btn");
    if (!btn) return;
    e.preventDefault();
    const panel = btn.closest(".panel");
    const open = panel.classList.contains("is-collapsed");
    folds.show(panel, btn, open);
    const k = folds.key(panel);
    if (k) folds.set(k, open ? "open" : "closed");
  });

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
    folds.apply();
    threadToEnd();
    notify.init();
    map.init();
    editor.init();
    orders.init();
    judge.init();
    judge.destinations();
    planning.draw();
    announceTurn();
    localizeTimes();
    countdown();
    setInterval(countdown, 30000);
    // Links from other screens: ?hex=0708 selects a hex, ?order=m1 opens the order form.
    const q = new URLSearchParams(location.search);
    if (q.get("order") && document.getElementById("order-box")) orders.open(q.get("order"));
    listen();
  });
})();
