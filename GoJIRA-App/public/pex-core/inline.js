// pex-inline.js
// Reusable in-place layout-editing overlay for a PlantUML SVG. Hosts (markdown
// viewers, the full editor UI shell, VS Code webviews) call `PexInline.activate`
// on a container that already has a rendered <svg>; the module wires entity
// drag, edge selection, edge-type toggle, and curve handles directly on the SVG
// and reports layout changes via callback.
//
// Dependencies: PexGeom (geometry math), PexMeta (only used by callers, not by
// this module — but we expect callers to embed/extract meta around us).
//
// Multi-instance safe: each activate() call has its own closure state.

(function (root, factory) {
  if (typeof module === 'object' && module.exports) {
    module.exports = factory(require('./pex-geom'), require('./pex-meta'));
  } else {
    root.PexInline = factory(root.PexGeom, root.PexMeta);
  }
})(typeof self !== 'undefined' ? self : this, function (PexGeom, PexMeta) {
  const SVG_NS = 'http://www.w3.org/2000/svg';
  const DRAFT_PREFIX = 'pex:draft:';

  // Cheap, stable string hash for draft keys. Source-content based so a draft
  // follows the diagram regardless of where it sits in the document.
  function hashString(str) {
    let h = 5381;
    for (let i = 0; i < str.length; i++) h = ((h * 33) ^ str.charCodeAt(i)) >>> 0;
    return h.toString(36);
  }

  // ---- Stylesheet (injected once per document) -----------------------------
  const STYLE_ID = 'pex-inline-styles';
  const STYLES = `
    .pex-inline-host { position: relative; }
    .pex-inline-host g.entity { cursor: grab; }
    .pex-inline-host g.entity:hover > rect,
    .pex-inline-host g.entity:hover > polygon,
    .pex-inline-host g.entity:hover > ellipse,
    .pex-inline-host g.entity:hover > path {
      stroke: #2563eb !important; stroke-width: 2 !important;
    }
    .pex-inline-host g.pex-selected > rect,
    .pex-inline-host g.pex-selected > polygon,
    .pex-inline-host g.pex-selected > ellipse,
    .pex-inline-host g.pex-selected > path {
      stroke: #ef4444 !important; stroke-width: 2.5 !important;
    }
    .pex-inline-host g.link { cursor: pointer; }
    .pex-inline-host g.link.pex-edge-selected > path {
      stroke: #2563eb !important; stroke-width: 2.5 !important;
    }
    .pex-inline-host g.link text { cursor: grab; }
    .pex-inline-host g.link text.pex-text-dragging {
      cursor: grabbing;
      fill: #2563eb !important;
      font-weight: bold;
    }
    .pex-handle { fill: #2563eb; stroke: white; stroke-width: 1.5; cursor: grab; }
    .pex-handle:active { cursor: grabbing; }
    .pex-anchor { fill: white; stroke: #2563eb; stroke-width: 1.5; cursor: grab; }
    .pex-anchor:active { cursor: grabbing; }
    .pex-handle-guide { stroke: #2563eb; stroke-width: 1; stroke-dasharray: 4 3; pointer-events: none; opacity: 0.7; }
    .pex-edge-toolbar {
      position: absolute; z-index: 10;
      display: none; gap: 4px; align-items: center;
      padding: 4px 6px; background: white; border: 1px solid #d0d7de; border-radius: 6px;
      box-shadow: 0 2px 8px rgba(0,0,0,0.08);
      font: 12px system-ui, sans-serif;
      transform: translate(-50%, -100%);
    }
    .pex-edge-toolbar.shown { display: inline-flex; }
    .pex-edge-toolbar .drag-handle {
      color: #888; padding: 0 6px 0 2px;
      cursor: grab; user-select: none;
      font-size: 14px; line-height: 1;
    }
    .pex-edge-toolbar .drag-handle:active { cursor: grabbing; }
    .pex-edge-toolbar .label { color: #666; padding: 0 4px 0 0; }
    .pex-edge-toolbar button {
      padding: 3px 9px; font-size: 12px;
      background: white; border: 1px solid #d0d7de; border-radius: 4px; cursor: pointer;
    }
    .pex-edge-toolbar button:hover { background: #f6f8fa; }
    .pex-edge-toolbar button.active { background: #2563eb; color: white; border-color: #2563eb; }
    .pex-draft-prompt {
      margin: 0 0 8px; padding: 6px 10px;
      background: #fff7ed; border: 1px solid #fed7aa; border-radius: 4px;
      font: 13px system-ui, sans-serif; color: #9a3412;
      display: flex; gap: 8px; align-items: center;
    }
    .pex-draft-prompt .pex-draft-msg { flex: 1; }
    .pex-draft-prompt button {
      padding: 2px 10px; font: 12px system-ui, sans-serif;
      background: white; border: 1px solid #d0d7de; border-radius: 4px; cursor: pointer;
    }
    .pex-draft-prompt button:hover { background: #f6f8fa; }
    .pex-draft-prompt button.primary { background: #2563eb; color: white; border-color: #2563eb; }
    .pex-draft-prompt button.primary:hover { filter: brightness(1.05); }
    .pex-dirty-badge {
      position: absolute; bottom: 8px; right: 8px;
      font: 11px system-ui, sans-serif; color: #b45309;
      background: #fef3c7; border: 1px solid #fde68a; border-radius: 12px;
      padding: 2px 8px; pointer-events: none; z-index: 6;
    }
  `;
  function ensureStylesInjected(doc) {
    if (doc.getElementById(STYLE_ID)) return;
    const s = doc.createElement('style');
    s.id = STYLE_ID;
    s.textContent = STYLES;
    doc.head.appendChild(s);
  }

  // ---- Helpers (pure / self-contained) -------------------------------------
  function entityQname(g) { return g.getAttribute('data-qualified-name') || g.id; }

  function edgeKeyForLink(svg, linkG) {
    const e1G = svg.querySelector(`g[id="${linkG.getAttribute('data-entity-1')}"]`);
    const e2G = svg.querySelector(`g[id="${linkG.getAttribute('data-entity-2')}"]`);
    if (!e1G || !e2G) return null;
    return `${entityQname(e1G)}__${entityQname(e2G)}`;
  }

  function normalizeLayout(raw) {
    if (!raw) return { nodes: {}, edges: {} };
    if (raw.nodes !== undefined || raw.edges !== undefined) {
      return { nodes: raw.nodes || {}, edges: raw.edges || {} };
    }
    return { nodes: raw, edges: {} };
  }

  function clientToSvg(e, svg) {
    const pt = svg.createSVGPoint();
    pt.x = e.clientX; pt.y = e.clientY;
    return pt.matrixTransform(svg.getScreenCTM().inverse());
  }

  // ---- The actual activation logic ----------------------------------------
  function activate(container, opts) {
    if (!container) throw new Error('PexInline.activate: container required');
    const svg = container.querySelector('svg');
    if (!svg) throw new Error('PexInline.activate: container has no <svg>');
    const doc = container.ownerDocument || document;
    const win = doc.defaultView || window;
    ensureStylesInjected(doc);
    container.classList.add('pex-inline-host');

    // ---- Source-aware initialization (preferred over raw `layout`) ---------
    // Hosts that pass `source` (full text including any embedded `' @startmeta`)
    // get higher-level conveniences: onSourceChange callback, localStorage
    // drafts keyed by clean-source hash, restoration prompts.
    opts = opts || {};
    const initialSource = typeof opts.source === 'string' ? opts.source : null;
    // Hosts that already provide their own dirty-tracking + recovery
    // (notably VS Code: ● tab indicator + Hot Exit) can opt out of L2/L3 by
    // passing `useDrafts: false` and `useDirtyBadge: false`. Default true to
    // protect plain browser hosts.
    const useDrafts = opts.useDrafts !== false;
    const useDirtyBadge = opts.useDirtyBadge !== false;
    let cleanSource = '';
    let metaLayout = null;
    if (initialSource && PexMeta) {
      const parsed = PexMeta.parseSource(initialSource);
      cleanSource = parsed.source;
      metaLayout = parsed.meta && parsed.meta.layout;
    }
    const sourceHash = (cleanSource && useDrafts) ? hashString(cleanSource) : null;

    const state = {
      layout: normalizeLayout(opts.layout || metaLayout || null),
      dirty: false,
      // Multi-select: selectedSet holds all currently-selected entities.
      // `selected` mirrors the most-recently-added member (used as the
      // primary anchor for drags and the legacy single-select callsites).
      selected: null,
      selectedSet: new Set(),
      selectedEdge: null,
      dragging: null,
      draggingHandle: null,
      draggingText: null,
      // user-applied offset for the floating edge toolbar (per selection,
      // resets when a different edge is selected so each selection starts
      // from the auto-positioned spot near the path midpoint)
      toolbarOffset: { dx: 0, dy: 0 },
    };
    const onLayoutChange = opts.onLayoutChange || (() => {});
    const onSourceChange = opts.onSourceChange || null;

    function buildSource() {
      if (!cleanSource || !PexMeta) return null;
      const hasLayout = state.layout && (
        Object.keys(state.layout.nodes || {}).length ||
        Object.keys(state.layout.edges || {}).length
      );
      return hasLayout
        ? PexMeta.embedMeta(cleanSource, { schema: 1, layout: state.layout })
        : cleanSource;
    }

    function saveDraft() {
      if (!sourceHash) return;
      try {
        win.localStorage.setItem(DRAFT_PREFIX + sourceHash, JSON.stringify({
          source: buildSource(),
          ts: Date.now(),
        }));
      } catch { /* quota / private mode — ignore */ }
    }
    function clearDraft() {
      if (!sourceHash) return;
      try { win.localStorage.removeItem(DRAFT_PREFIX + sourceHash); } catch {}
    }

    // Visual "● 저장 필요" badge (L3). Created lazily, removed when clean.
    let dirtyBadgeEl = null;
    function updateDirtyBadge() {
      if (!useDirtyBadge) return; // host opted out
      if (!state.dirty) {
        if (dirtyBadgeEl) { dirtyBadgeEl.remove(); dirtyBadgeEl = null; }
        return;
      }
      if (!dirtyBadgeEl) {
        dirtyBadgeEl = doc.createElement('div');
        dirtyBadgeEl.className = 'pex-dirty-badge';
        dirtyBadgeEl.textContent = '● 저장 필요';
        container.appendChild(dirtyBadgeEl);
      }
    }
    function setDirty(d) {
      const changed = state.dirty !== d;
      state.dirty = d;
      if (changed) updateDirtyBadge();
    }

    const fire = () => {
      try { onLayoutChange(state.layout); } catch {}
      if (onSourceChange) {
        const s = buildSource();
        if (s !== null) { try { onSourceChange(s); } catch {} }
      }
      setDirty(true);
      saveDraft();
    };

    // ---- ViewBox bookkeeping -----------------------------------------------
    function adjustViewBox() {
      let orig = svg.getAttribute('data-pex-original-viewbox');
      if (!orig) {
        orig = svg.getAttribute('viewBox') || '';
        svg.setAttribute('data-pex-original-viewbox', orig);
      }
      const vb = orig.split(/\s+/).map(Number);
      if (vb.length !== 4) return;
      const [x, y, w, h] = vb;
      // Compute the *exact* enclosing rect of the original viewBox plus
      // every entity's post-translate bbox. The previous heuristic naively
      // grew the viewBox by max|dx|/max|dy| which over-extended whenever
      // the moved entity wasn't originally at the diagram's edge — leaving
      // visible empty padding to one side after a layout edit.
      let xMin = x, yMin = y, xMax = x + w, yMax = y + h;
      svg.querySelectorAll('g.entity').forEach((g) => {
        const q = entityQname(g);
        const d = state.layout.nodes[q];
        if (!d || (d.dx === 0 && d.dy === 0)) return;
        let b;
        try { b = g.getBBox(); } catch { return; }
        if (!b) return;
        const ex = b.x + d.dx, ey = b.y + d.dy;
        if (ex < xMin) xMin = ex;
        if (ey < yMin) yMin = ey;
        if (ex + b.width > xMax) xMax = ex + b.width;
        if (ey + b.height > yMax) yMax = ey + b.height;
      });
      const pad = 8;
      xMin -= pad; yMin -= pad; xMax += pad; yMax += pad;
      const nw = xMax - xMin, nh = yMax - yMin;
      svg.setAttribute('viewBox', `${xMin} ${yMin} ${nw} ${nh}`);
      // Width/height attributes act as the SVG's intrinsic size (used as the
      // fallback when the host doesn't constrain via CSS). We set them so the
      // diagram gets a sensible default, but DROP any matching inline style
      // we (or PlantUML) injected so the host's CSS — typically
      // `max-width: 100%; height: auto` — can scale the SVG responsively.
      svg.setAttribute('width', `${nw}px`);
      svg.setAttribute('height', `${nh}px`);
      const style = (svg.getAttribute('style') || '')
        .replace(/width\s*:[^;]+;?/g, '')
        .replace(/height\s*:[^;]+;?/g, '');
      if (style.trim()) svg.setAttribute('style', style); else svg.removeAttribute('style');
      // PlantUML emits `preserveAspectRatio="none"` for some diagram types
      // (notably use-case / description). Combined with a host CSS that
      // caps width but keeps the SVG height free, "none" stretches content
      // horizontally to fit the cap → diagram squish. Force uniform scaling
      // so the content always preserves its aspect ratio.
      const par = svg.getAttribute('preserveAspectRatio');
      if (!par || par === 'none') {
        svg.setAttribute('preserveAspectRatio', 'xMidYMid meet');
      }
    }

    // ---- Edge follow geometry ----------------------------------------------
    function applyEdgeFollow() {
      svg.querySelectorAll('g.link').forEach((g) => {
        g.removeAttribute('transform');
        const e1Id = g.getAttribute('data-entity-1');
        const e2Id = g.getAttribute('data-entity-2');
        const e1G = svg.querySelector(`g[id="${e1Id}"]`);
        const e2G = svg.querySelector(`g[id="${e2Id}"]`);
        const q1 = e1G ? entityQname(e1G) : e1Id;
        const q2 = e2G ? entityQname(e2G) : e2Id;
        const d1 = state.layout.nodes[q1] || { dx: 0, dy: 0 };
        const d2 = state.layout.nodes[q2] || { dx: 0, dy: 0 };
        const eKey = `${q1}__${q2}`;
        const edgeOverride = state.layout.edges[eKey];
        const moved = d1.dx || d1.dy || d2.dx || d2.dy;
        const hasOverride = !!edgeOverride;

        const firstPath = g.querySelector('path:not(.pex-edge-hit)');
        if (!firstPath) return;
        let origD = firstPath.getAttribute('data-pex-orig-d');
        if (!origD) { origD = firstPath.getAttribute('d'); firstPath.setAttribute('data-pex-orig-d', origD); }
        g.querySelectorAll('polygon').forEach((poly) => {
          if (!poly.getAttribute('data-pex-orig-points')) {
            poly.setAttribute('data-pex-orig-points', poly.getAttribute('points'));
          }
        });
        g.querySelectorAll('text').forEach((t, tIdx) => {
          if (t.getAttribute('data-pex-orig-x') === null) {
            // The server-rendered text x/y already includes any saved per-text
            // offset (server.js applyLayout mirrors our +tdx/+tdy step). Subtract
            // it back out so the snapshot captures the *natural* projected
            // position; otherwise applyEdgeFollow re-adds the offset on every
            // call and the label drifts on re-edit.
            const tov = (edgeOverride && edgeOverride.texts) ? edgeOverride.texts[tIdx] : null;
            const tdx = (tov && tov.dx) || 0;
            const tdy = (tov && tov.dy) || 0;
            t.setAttribute('data-pex-orig-x', String(parseFloat(t.getAttribute('x')) - tdx));
            t.setAttribute('data-pex-orig-y', String(parseFloat(t.getAttribute('y')) - tdy));
          }
        });

        if ((!moved && !hasOverride) || !e1G || !e2G) {
          g.querySelectorAll('path:not(.pex-edge-hit)').forEach((p) => {
            const o = p.getAttribute('data-pex-orig-d'); if (o) p.setAttribute('d', o);
          });
          g.querySelectorAll('polygon').forEach((poly) => poly.setAttribute('points', poly.getAttribute('data-pex-orig-points')));
          g.querySelectorAll('text').forEach((t) => {
            t.setAttribute('x', t.getAttribute('data-pex-orig-x'));
            t.setAttribute('y', t.getAttribute('data-pex-orig-y'));
          });
          return;
        }

        const b1 = e1G.getBBox();
        const b2 = e2G.getBBox();
        const e1Box = { x: b1.x + d1.dx, y: b1.y + d1.dy, w: b1.width, h: b1.height };
        const e2Box = { x: b2.x + d2.dx, y: b2.y + d2.dy, w: b2.width, h: b2.height };
        const c1c = PexGeom.bboxCenter(e1Box);
        const c2c = PexGeom.bboxCenter(e2Box);
        const newStart = (edgeOverride && edgeOverride.startAnchor)
          ? PexGeom.pointOnBboxBorder(e1Box, edgeOverride.startAnchor)
          : PexGeom.rectExit(c1c, c2c, e1Box);
        const newEnd = (edgeOverride && edgeOverride.endAnchor)
          ? PexGeom.pointOnBboxBorder(e2Box, edgeOverride.endAnchor)
          : PexGeom.rectExit(c2c, c1c, e2Box);

        const ctx = PexGeom.getPathContext(origD);
        if (!ctx) return;

        let tipExtStart = 0, tipExtEnd = 0;
        const polyInfo = [];
        g.querySelectorAll('polygon').forEach((poly) => {
          const o = poly.getAttribute('data-pex-orig-points');
          const nums = o.trim().split(/[\s,]+/).filter((s) => s.length).map(parseFloat);
          let cx = 0, cy = 0, n = 0;
          for (let i = 0; i + 1 < nums.length; i += 2) { cx += nums[i]; cy += nums[i + 1]; n++; }
          cx /= n; cy /= n;
          const dStart = (cx - ctx.start.x) ** 2 + (cy - ctx.start.y) ** 2;
          const dEnd = (cx - ctx.end.x) ** 2 + (cy - ctx.end.y) ** 2;
          const atStart = dStart < dEnd;
          const origAnchor = atStart ? ctx.start : ctx.end;
          const outwardAng = atStart ? ctx.startTangent + Math.PI : ctx.endTangent;
          const ext = PexGeom.polygonOutwardExtent(o, origAnchor, outwardAng);
          if (atStart) tipExtStart = Math.max(tipExtStart, ext);
          else tipExtEnd = Math.max(tipExtEnd, ext);
          polyInfo.push({ poly, atStart, origAnchor });
        });

        const ndx = newEnd.x - newStart.x, ndy = newEnd.y - newStart.y;
        const nLen = Math.hypot(ndx, ndy) || 1;
        const fwd = { x: ndx / nLen, y: ndy / nLen };
        const newPathStart = { x: newStart.x + tipExtStart * fwd.x, y: newStart.y + tipExtStart * fwd.y };
        const newPathEnd   = { x: newEnd.x   - tipExtEnd   * fwd.x, y: newEnd.y   - tipExtEnd   * fwd.y };

        const wasCurved = /[CQS]/i.test(origD);
        const edgeType = (edgeOverride && edgeOverride.type) || (wasCurved ? 'curve' : 'straight');
        const built = PexGeom.buildEdgePath(newPathStart, newPathEnd, {
          type: edgeType,
          ctx: edgeType === 'curve' ? ctx : null,
          u1: edgeOverride && edgeOverride.u1,
          u2: edgeOverride && edgeOverride.u2,
        });
        g._pexBuilt = { ...built, newStart, newEnd, pathStart: newPathStart, pathEnd: newPathEnd, type: edgeType };

        g.querySelectorAll('path:not(.pex-edge-hit)').forEach((p, idx) => {
          const o = p.getAttribute('data-pex-orig-d');
          p.setAttribute('d', idx === 0 ? built.d : (o || ''));
        });

        const oldStartAngle = ctx.startTangent, oldEndAngle = ctx.endTangent;
        polyInfo.forEach(({ poly, atStart, origAnchor }) => {
          const o = poly.getAttribute('data-pex-orig-points');
          const newAnchor = atStart ? newPathStart : newPathEnd;
          const oldAng = atStart ? oldStartAngle : oldEndAngle;
          const newAng = atStart ? built.startTangent : built.endTangent;
          poly.setAttribute('points', PexGeom.reorientPolygon(o, origAnchor, newAnchor, newAng - oldAng));
        });

        const odx = ctx.end.x - ctx.start.x, ody = ctx.end.y - ctx.start.y;
        const olen2 = odx * odx + ody * ody;
        const oldLineAngle = Math.atan2(ody, odx);
        const newLineAngle = Math.atan2(ndy, ndx);
        const dAng = newLineAngle - oldLineAngle;
        const cosA = Math.cos(dAng), sinA = Math.sin(dAng);
        const textsOv = (edgeOverride && edgeOverride.texts) || null;
        g.querySelectorAll('text').forEach((t, tIdx) => {
          const x0 = parseFloat(t.getAttribute('data-pex-orig-x'));
          const y0 = parseFloat(t.getAttribute('data-pex-orig-y'));
          const tp = olen2 > 0
            ? Math.max(0, Math.min(1, ((x0 - ctx.start.x) * odx + (y0 - ctx.start.y) * ody) / olen2))
            : 0.5;
          const projX = ctx.start.x + tp * odx;
          const projY = ctx.start.y + tp * ody;
          const offX = x0 - projX, offY = y0 - projY;
          const rotX = offX * cosA - offY * sinA;
          const rotY = offX * sinA + offY * cosA;
          // Per-text override (Ctrl/Cmd-drag of e.g. multiplicity labels) is
          // applied on TOP of the auto-projected position, in screen-aligned
          // SVG units. So the label keeps following the line as nodes move,
          // and the user's fine-tune offset rides along with it.
          const tov = textsOv && textsOv[tIdx];
          const tdx = (tov && tov.dx) || 0;
          const tdy = (tov && tov.dy) || 0;
          t.setAttribute('x', newStart.x + tp * ndx + rotX + tdx);
          t.setAttribute('y', newStart.y + tp * ndy + rotY + tdy);
        });
      });
      resizeContainers();
      syncEdgeHitboxes();
      if (state.selectedEdge) renderHandles();
    }

    // PlantUML clusters (rectangle/package/node) wrap their children but
    // don't auto-resize when a child moves. We derive the cluster's rect
    // from the union of its children's current bboxes (with deltas), plus
    // padding for the label band. Deepest cluster first so nested
    // containers settle correctly.
    function resizeContainers() {
      const clusters = [...svg.querySelectorAll('g.cluster')];
      if (!clusters.length) return;
      clusters.sort((a, b) => {
        const ad = (a.getAttribute('data-qualified-name') || '').split('.').length;
        const bd = (b.getAttribute('data-qualified-name') || '').split('.').length;
        return bd - ad; // deepest first
      });
      for (const cluster of clusters) resizeOneCluster(cluster);
    }

    function getEntityBBoxWithTransform(g) {
      const b = g.getBBox();
      const m = (g.getAttribute('transform') || '').match(/translate\(\s*([-\d.]+)[,\s]+([-\d.]+)\s*\)/);
      const dx = m ? parseFloat(m[1]) : 0;
      const dy = m ? parseFloat(m[2]) : 0;
      return { x: b.x + dx, y: b.y + dy, w: b.width, h: b.height };
    }

    function resizeOneCluster(cluster) {
      const cqname = cluster.getAttribute('data-qualified-name');
      if (!cqname) return;
      const prefix = cqname + '.';
      // Direct children only (one extra dot in qname). Both g.entity and
      // nested g.cluster qualify so we recursively support nesting.
      const children = [];
      svg.querySelectorAll('g.entity, g.cluster').forEach((g) => {
        if (g === cluster) return;
        const q = g.getAttribute('data-qualified-name');
        if (!q || !q.startsWith(prefix)) return;
        const rest = q.slice(prefix.length);
        if (rest.includes('.')) return;
        children.push(g);
      });
      if (!children.length) return;

      let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
      for (const child of children) {
        const b = getEntityBBoxWithTransform(child);
        if (!isFinite(b.x) || !isFinite(b.y) || b.w <= 0 || b.h <= 0) continue;
        if (b.x < minX) minX = b.x;
        if (b.y < minY) minY = b.y;
        if (b.x + b.w > maxX) maxX = b.x + b.w;
        if (b.y + b.h > maxY) maxY = b.y + b.h;
      }
      if (!isFinite(minX)) return;

      const rect = cluster.querySelector('rect');
      const text = cluster.querySelector('text');
      // Snapshot the cluster's padding from the SVG as we first received
      // it. Use bboxes WITH transforms applied so re-entry into edit mode
      // works correctly: when the server has already pre-resized the
      // cluster and pre-translated children based on the saved meta, the
      // rect attrs and the children's translated bboxes are in the same
      // coordinate frame — padding = (childMinX_translated) - rect.x.
      // (If we used naked getBBox() we'd compute padLeft against the
      // already-shifted rect.x and end up with negative or inflated
      // padding, then expand the cluster again on each re-entry.)
      let orig = cluster._pexClusterOrig;
      if (!orig && rect) {
        const ox = parseFloat(rect.getAttribute('x'));
        const oy = parseFloat(rect.getAttribute('y'));
        const ow = parseFloat(rect.getAttribute('width'));
        const oh = parseFloat(rect.getAttribute('height'));
        let imnX = Infinity, imnY = Infinity, imxX = -Infinity, imxY = -Infinity;
        children.forEach((c) => {
          const b = getEntityBBoxWithTransform(c);
          if (b.x < imnX) imnX = b.x;
          if (b.y < imnY) imnY = b.y;
          if (b.x + b.w > imxX) imxX = b.x + b.w;
          if (b.y + b.h > imxY) imxY = b.y + b.h;
        });
        orig = {
          padLeft:   imnX - ox,
          padRight:  (ox + ow) - imxX,
          padTop:    imnY - oy,
          padBottom: (oy + oh) - imxY,
          textOffsetY: text ? parseFloat(text.getAttribute('y')) - oy : 0,
        };
        cluster._pexClusterOrig = orig;
      }
      if (!rect || !orig) return;

      const newX = minX - orig.padLeft;
      const newY = minY - orig.padTop;
      const newW = (maxX - minX) + orig.padLeft + orig.padRight;
      const newH = (maxY - minY) + orig.padTop + orig.padBottom;
      rect.setAttribute('x', newX);
      rect.setAttribute('y', newY);
      rect.setAttribute('width', newW);
      rect.setAttribute('height', newH);

      // Keep the label centered horizontally at the same vertical offset
      // PlantUML originally used relative to the rect's top edge.
      if (text) {
        const tl = parseFloat(text.getAttribute('textLength'));
        if (isFinite(tl)) text.setAttribute('x', newX + (newW - tl) / 2);
        text.setAttribute('y', newY + orig.textOffsetY);
      }
    }

    // Each g.link's first <path> has fill="none" so only the visible stroke
    // (~1px) is hit-testable — the user has to click the arrowhead instead
    // of the line. We mirror the visible path with a transparent companion
    // path that has a fat stroke + pointer-events="stroke" so clicking
    // anywhere along the line selects the edge.
    function syncEdgeHitboxes() {
      svg.querySelectorAll('g.link').forEach((g) => {
        const visible = g.querySelector('path:not(.pex-edge-hit)');
        if (!visible) return;
        const d = visible.getAttribute('d');
        let hit = g.querySelector('path.pex-edge-hit');
        if (!hit) {
          hit = doc.createElementNS(SVG_NS, 'path');
          hit.setAttribute('class', 'pex-edge-hit');
          hit.setAttribute('fill', 'none');
          hit.setAttribute('stroke', 'transparent');
          hit.setAttribute('stroke-width', '14');
          hit.setAttribute('pointer-events', 'stroke');
          hit.style.cursor = 'pointer';
          // Insert as first child so it sits behind the visible stroke (z-order
          // doesn't matter for hit-testing but keeps the DOM order intuitive).
          g.insertBefore(hit, g.firstChild);
        }
        if (d && hit.getAttribute('d') !== d) hit.setAttribute('d', d);
      });
    }

    // ---- Selection + handle layer ------------------------------------------
    // selectNode / selectEdge are mutually exclusive: selecting a node clears
    // any selected edge (and vice versa).
    //
    // selectNode signatures:
    //   selectNode(null)              clear all node selection
    //   selectNode(g)                 replace selection with [g]
    //   selectNode(g, { toggle })     toggle g in selection (Shift+click)
    function selectNode(g, opts) {
      opts = opts || {};
      if (g === null) {
        clearNodeSelection();
        return;
      }
      if (opts.toggle) {
        if (state.selectedSet.has(g)) {
          state.selectedSet.delete(g);
          g.classList.remove('pex-selected');
          if (state.selected === g) {
            // pick another member as new primary, or null if empty
            const next = state.selectedSet.values().next().value || null;
            state.selected = next || null;
          }
        } else {
          state.selectedSet.add(g);
          g.classList.add('pex-selected');
          state.selected = g;
          if (state.selectedEdge) clearEdgeSelection();
        }
        return;
      }
      // Plain select: replace current selection with just g
      for (const prev of state.selectedSet) {
        if (prev !== g) prev.classList.remove('pex-selected');
      }
      state.selectedSet.clear();
      state.selectedSet.add(g);
      state.selected = g;
      g.classList.add('pex-selected');
      if (state.selectedEdge) clearEdgeSelection();
    }
    function clearNodeSelection() {
      for (const el of state.selectedSet) el.classList.remove('pex-selected');
      state.selectedSet.clear();
      state.selected = null;
    }
    function clearEdgeSelection() {
      if (state.selectedEdge) state.selectedEdge.classList.remove('pex-edge-selected');
      state.selectedEdge = null;
      hideEdgeToolbar();
      clearHandles();
    }
    function selectEdge(g) {
      const sameEdge = state.selectedEdge === g;
      if (state.selectedEdge && !sameEdge) state.selectedEdge.classList.remove('pex-edge-selected');
      state.selectedEdge = g;
      if (g) {
        if (!sameEdge) state.toolbarOffset = { dx: 0, dy: 0 };
        g.classList.add('pex-edge-selected');
        if (state.selectedSet.size) clearNodeSelection();
        const eKey = edgeKeyForLink(svg, g);
        if (eKey && !state.layout.edges[eKey]) {
          const pathD = g.querySelector('path:not(.pex-edge-hit)').getAttribute('data-pex-orig-d') || g.querySelector('path:not(.pex-edge-hit)').getAttribute('d') || '';
          const wasCurved = /[CQS]/i.test(pathD);
          state.layout.edges[eKey] = { type: wasCurved ? 'curve' : 'straight' };
        }
        applyEdgeFollow();
        showEdgeToolbar();
      } else {
        hideEdgeToolbar();
        clearHandles();
      }
    }
    function ensureHandleLayer() {
      let layer = svg.querySelector('g.pex-handle-layer');
      if (!layer) {
        layer = doc.createElementNS(SVG_NS, 'g');
        layer.setAttribute('class', 'pex-handle-layer');
        svg.appendChild(layer);
        // After dragging a curve handle, the browser synthesizes a `click`
        // event on the handle that bubbles up to the SVG and triggers
        // onSvgClick → selection cleared. Swallow click + pointerup at the
        // layer so the selection survives any handle interaction.
        const swallow = (e) => e.stopPropagation();
        layer.addEventListener('click', swallow);
        layer.addEventListener('pointerup', swallow);
      } else if (svg.lastElementChild !== layer) {
        // SVG has no z-index — paint order is document order. If anything
        // (a re-render, a defensively-added child) shifted the handle layer
        // away from the end, handles would paint underneath entities/links.
        // Re-append puts it back on top.
        svg.appendChild(layer);
      }
      return layer;
    }
    function clearHandles() {
      const layer = svg.querySelector('g.pex-handle-layer');
      if (layer) layer.innerHTML = '';
    }
    function renderHandles() {
      const layer = ensureHandleLayer();
      const eg = state.selectedEdge;
      if (!eg || !eg._pexBuilt) { layer.innerHTML = ''; return; }
      const built = eg._pexBuilt;
      const ensure = (selector, factory) => {
        let el = layer.querySelector(selector);
        if (!el) { el = factory(); layer.appendChild(el); }
        return el;
      };
      const setAttrs = (el, attrs) => { for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v); };

      const a1 = ensure('.pex-anchor[data-role="start"]', () => {
        const e = doc.createElementNS(SVG_NS, 'circle');
        e.setAttribute('class', 'pex-anchor'); e.setAttribute('data-role', 'start'); e.setAttribute('r', 5);
        e.addEventListener('pointerdown', (ev) => startHandleDrag(ev, 'start'));
        return e;
      });
      setAttrs(a1, { cx: built.newStart.x, cy: built.newStart.y });

      const a2 = ensure('.pex-anchor[data-role="end"]', () => {
        const e = doc.createElementNS(SVG_NS, 'circle');
        e.setAttribute('class', 'pex-anchor'); e.setAttribute('data-role', 'end'); e.setAttribute('r', 5);
        e.addEventListener('pointerdown', (ev) => startHandleDrag(ev, 'end'));
        return e;
      });
      setAttrs(a2, { cx: built.newEnd.x, cy: built.newEnd.y });

      if (built.type !== 'curve' || !built.c1 || !built.c2) {
        layer.querySelectorAll('.pex-handle, .pex-handle-guide').forEach((el) => el.remove());
        return;
      }
      const ps = built.pathStart || built.newStart;
      const pe = built.pathEnd || built.newEnd;
      const g1 = ensure('.pex-handle-guide[data-role="g1"]', () => {
        const e = doc.createElementNS(SVG_NS, 'line');
        e.setAttribute('class', 'pex-handle-guide'); e.setAttribute('data-role', 'g1');
        return e;
      });
      setAttrs(g1, { x1: ps.x, y1: ps.y, x2: built.c1.x, y2: built.c1.y });
      const g2 = ensure('.pex-handle-guide[data-role="g2"]', () => {
        const e = doc.createElementNS(SVG_NS, 'line');
        e.setAttribute('class', 'pex-handle-guide'); e.setAttribute('data-role', 'g2');
        return e;
      });
      setAttrs(g2, { x1: pe.x, y1: pe.y, x2: built.c2.x, y2: built.c2.y });
      const h1 = ensure('.pex-handle[data-which="c1"]', () => {
        const e = doc.createElementNS(SVG_NS, 'circle');
        e.setAttribute('class', 'pex-handle'); e.setAttribute('data-which', 'c1'); e.setAttribute('r', 6);
        e.addEventListener('pointerdown', (ev) => startHandleDrag(ev, 'c1'));
        return e;
      });
      setAttrs(h1, { cx: built.c1.x, cy: built.c1.y });
      const h2 = ensure('.pex-handle[data-which="c2"]', () => {
        const e = doc.createElementNS(SVG_NS, 'circle');
        e.setAttribute('class', 'pex-handle'); e.setAttribute('data-which', 'c2'); e.setAttribute('r', 6);
        e.addEventListener('pointerdown', (ev) => startHandleDrag(ev, 'c2'));
        return e;
      });
      setAttrs(h2, { cx: built.c2.x, cy: built.c2.y });
    }

    // ---- Floating edge-style toolbar ---------------------------------------
    // Sits above the selected edge's midpoint by default. The leading "⋮⋮"
    // is a drag handle so the user can move the toolbar off the line when it
    // covers what they're trying to edit (curve handles, anchor markers).
    // Per-edge: offset resets when a different edge is selected.
    const TOOLBAR_GAP = 16; // px above midpoint, breathing room for the line
    const tb = doc.createElement('div');
    tb.className = 'pex-edge-toolbar';
    tb.innerHTML = '<span class="drag-handle" title="드래그해서 위치 이동">⋮⋮</span>'
      + '<span class="label">선:</span>'
      + '<button data-type="straight">직선</button>'
      + '<button data-type="curve">곡선</button>'
      + '<button data-type="ortho">꺾은선</button>';
    container.appendChild(tb);
    tb.addEventListener('click', (e) => {
      const b = e.target.closest('button[data-type]');
      if (!b) return;
      e.stopPropagation();
      setEdgeType(b.dataset.type);
    });

    // Drag the toolbar by its handle. Updates state.toolbarOffset and
    // re-positions on every move; offset persists until the user selects a
    // different edge.
    const dragHandle = tb.querySelector('.drag-handle');
    dragHandle.addEventListener('pointerdown', (e) => {
      e.preventDefault();
      e.stopPropagation();
      const startX = e.clientX, startY = e.clientY;
      const startOff = { dx: state.toolbarOffset.dx, dy: state.toolbarOffset.dy };
      dragHandle.setPointerCapture(e.pointerId);
      const onMove = (ev) => {
        state.toolbarOffset = {
          dx: startOff.dx + (ev.clientX - startX),
          dy: startOff.dy + (ev.clientY - startY),
        };
        showEdgeToolbar();
      };
      const onUp = (ev) => {
        dragHandle.removeEventListener('pointermove', onMove);
        dragHandle.removeEventListener('pointerup', onUp);
        dragHandle.removeEventListener('pointercancel', onUp);
        try { dragHandle.releasePointerCapture(ev.pointerId); } catch {}
      };
      dragHandle.addEventListener('pointermove', onMove);
      dragHandle.addEventListener('pointerup', onUp);
      dragHandle.addEventListener('pointercancel', onUp);
    });

    function showEdgeToolbar() {
      const eg = state.selectedEdge;
      if (!eg) { hideEdgeToolbar(); return; }
      const eKey = edgeKeyForLink(svg, eg);
      const ov = state.layout.edges[eKey];
      const wasCurved = /[CQS]/i.test(eg.querySelector('path:not(.pex-edge-hit)').getAttribute('data-pex-orig-d') || '');
      const curType = (ov && ov.type) || (wasCurved ? 'curve' : 'straight');
      tb.querySelectorAll('button').forEach((b) => b.classList.toggle('active', b.dataset.type === curType));
      // applyEdgeFollow may have just mutated the path's `d` attribute
      // (selectEdge → applyEdgeFollow → setAttribute → showEdgeToolbar all
      // run synchronously in one click handler). In some webview
      // environments getBBox / getScreenCTM / getBoundingClientRect can
      // still report the *previous* layout for one frame, which positions
      // the toolbar against the old path — invisible until a later scroll
      // or resize event triggers another showEdgeToolbar with fresh values.
      // Force a synchronous layout flush before reading.
      void container.offsetHeight;
      // Position the toolbar above the path's full visual bounding box (so
      // for curves we clear the apex, not just the line midpoint). Anchored
      // at horizontal center, vertical above the bbox top.
      const path = eg.querySelector('path:not(.pex-edge-hit)');
      const bbox = path.getBBox();
      const anchorX = bbox.x + bbox.width / 2;
      const anchorY = bbox.y;
      const ctm = svg.getScreenCTM();
      const screen = svg.createSVGPoint();
      screen.x = anchorX; screen.y = anchorY;
      const sp = screen.matrixTransform(ctm);
      const cRect = container.getBoundingClientRect();
      // CSS `transform: translate(-50%, -100%)` positions the toolbar's
      // bottom-center at (left, top), so we put (left, top) at the bbox
      // top edge minus a gap → toolbar bottom is TOOLBAR_GAP px above the
      // line's top.
      tb.style.left = (sp.x - cRect.left + state.toolbarOffset.dx) + 'px';
      tb.style.top  = (sp.y - cRect.top - TOOLBAR_GAP + state.toolbarOffset.dy) + 'px';
      tb.classList.add('shown');
    }
    function hideEdgeToolbar() { tb.classList.remove('shown'); }
    function setEdgeType(type) {
      const eg = state.selectedEdge;
      if (!eg) return;
      const eKey = edgeKeyForLink(svg, eg);
      if (!eKey) return;
      const cur = state.layout.edges[eKey] || {};
      const next = { ...cur, type };
      if (type !== 'curve') { delete next.u1; delete next.u2; }
      state.layout.edges[eKey] = next;
      applyEdgeFollow();
      adjustViewBox();
      showEdgeToolbar();
      fire();
    }

    // ---- Drag handlers (entity + handles) ----------------------------------
    function startDrag(e, g) {
      e.preventDefault();
      // Decide what's being dragged based on current selection state:
      //  - g already in multi-selection → drag whole group
      //  - g not in selection + Shift held → add g, then group drag
      //  - g not in selection + no Shift → replace selection with g, single drag
      if (!state.selectedSet.has(g)) {
        selectNode(g, { toggle: !!e.shiftKey });
      }
      const pt = clientToSvg(e, svg);
      // Snapshot each member's start delta so the group moves rigidly.
      const members = [];
      for (const el of state.selectedSet) {
        const q = entityQname(el);
        const existing = state.layout.nodes[q] || { dx: 0, dy: 0 };
        members.push({ g: el, q, originDx: existing.dx, originDy: existing.dy });
      }
      state.dragging = {
        primary: g,
        members,
        startX: pt.x, startY: pt.y,
      };
      g.setPointerCapture(e.pointerId);
      g.addEventListener('pointermove', onDrag);
      g.addEventListener('pointerup', endDrag);
      g.addEventListener('pointercancel', endDrag);
    }
    function onDrag(e) {
      const d = state.dragging;
      if (!d) return;
      const pt = clientToSvg(e, svg);
      const ox = pt.x - d.startX, oy = pt.y - d.startY;
      for (const m of d.members) {
        const dx = Math.round(m.originDx + ox);
        const dy = Math.round(m.originDy + oy);
        m.g.setAttribute('transform', `translate(${dx}, ${dy})`);
        state.layout.nodes[m.q] = { dx, dy };
      }
      applyEdgeFollow();
      adjustViewBox();
    }
    function endDrag(e) {
      const d = state.dragging;
      if (!d) return;
      d.primary.removeEventListener('pointermove', onDrag);
      d.primary.removeEventListener('pointerup', endDrag);
      d.primary.removeEventListener('pointercancel', endDrag);
      try { d.primary.releasePointerCapture(e.pointerId); } catch {}
      state.dragging = null;
      fire();
    }
    function startHandleDrag(e, which) {
      e.stopPropagation(); e.preventDefault();
      const eg = state.selectedEdge;
      if (!eg) return;
      const eKey = edgeKeyForLink(svg, eg);
      if (!eKey) return;
      const target = e.currentTarget;
      state.draggingHandle = { which, eKey, target };
      target.setPointerCapture(e.pointerId);
      target.addEventListener('pointermove', onHandleDrag);
      target.addEventListener('pointerup', endHandleDrag);
      target.addEventListener('pointercancel', endHandleDrag);
    }
    function onHandleDrag(e) {
      const h = state.draggingHandle;
      if (!h) return;
      const eg = state.selectedEdge;
      if (!eg || !eg._pexBuilt) return;
      const pt = clientToSvg(e, svg);
      const built = eg._pexBuilt;
      const cur = state.layout.edges[h.eKey] || { type: 'curve' };
      if (h.which === 'c1' || h.which === 'c2') {
        if (h.which === 'c1') {
          cur.u1 = { x: pt.x - built.newStart.x, y: pt.y - built.newStart.y };
          if (!cur.u2) cur.u2 = built.u2 || { x: 0, y: 0 };
        } else {
          cur.u2 = { x: built.newEnd.x - pt.x, y: built.newEnd.y - pt.y };
          if (!cur.u1) cur.u1 = built.u1 || { x: 0, y: 0 };
        }
        cur.type = 'curve';
      } else if (h.which === 'start' || h.which === 'end') {
        const entityId = eg.getAttribute(h.which === 'start' ? 'data-entity-1' : 'data-entity-2');
        const entityG = svg.querySelector(`g[id="${entityId}"]`);
        if (!entityG) return;
        const q = entityQname(entityG);
        const delta = state.layout.nodes[q] || { dx: 0, dy: 0 };
        const b = entityG.getBBox();
        const bbox = { x: b.x + delta.dx, y: b.y + delta.dy, w: b.width, h: b.height };
        const snapped = PexGeom.nearestBboxBorder(bbox, pt);
        cur[h.which === 'start' ? 'startAnchor' : 'endAnchor'] = { side: snapped.side, t: snapped.t };
      }
      state.layout.edges[h.eKey] = cur;
      applyEdgeFollow();
    }
    function endHandleDrag(e) {
      const h = state.draggingHandle;
      if (!h) return;
      h.target.removeEventListener('pointermove', onHandleDrag);
      h.target.removeEventListener('pointerup', endHandleDrag);
      h.target.removeEventListener('pointercancel', endHandleDrag);
      try { h.target.releasePointerCapture(e.pointerId); } catch {}
      state.draggingHandle = null;
      fire();
    }

    // ---- Per-text drag (Ctrl/Cmd + drag on edge labels) -------------------
    // Lets the user move auxiliary text on an edge — multiplicity ("0..*"),
    // qualifiers, role names — independently of the line. The offset rides
    // *on top of* the auto-projection in applyEdgeFollow, so the label still
    // follows the line as nodes move while the user's fine-tune is preserved.
    function ensureEdgeOverrideFor(linkG, eKey) {
      let ov = state.layout.edges[eKey];
      if (ov) return ov;
      const pathEl = linkG.querySelector('path:not(.pex-edge-hit)');
      const pathD = pathEl ? (pathEl.getAttribute('data-pex-orig-d') || pathEl.getAttribute('d') || '') : '';
      const wasCurved = /[CQS]/i.test(pathD);
      ov = state.layout.edges[eKey] = { type: wasCurved ? 'curve' : 'straight' };
      return ov;
    }
    function startTextDrag(e, linkG, textEl, idx) {
      const eKey = edgeKeyForLink(svg, linkG);
      if (!eKey) return;
      const ov = ensureEdgeOverrideFor(linkG, eKey);
      ov.texts = ov.texts || {};
      const existing = ov.texts[idx] || { dx: 0, dy: 0 };
      const pt = clientToSvg(e, svg);
      textEl.classList.add('pex-text-dragging');
      try { textEl.setPointerCapture(e.pointerId); } catch {}
      state.draggingText = {
        textEl, idx, eKey,
        startX: pt.x, startY: pt.y,
        originDx: existing.dx, originDy: existing.dy,
      };
      textEl.addEventListener('pointermove', onTextDrag);
      textEl.addEventListener('pointerup', endTextDrag);
      textEl.addEventListener('pointercancel', endTextDrag);
    }
    function onTextDrag(e) {
      const d = state.draggingText;
      if (!d) return;
      const pt = clientToSvg(e, svg);
      const dx = Math.round(d.originDx + (pt.x - d.startX));
      const dy = Math.round(d.originDy + (pt.y - d.startY));
      const ov = state.layout.edges[d.eKey];
      if (!ov) return;  // edge gone (e.g. SVG re-rendered) — abort
      ov.texts = ov.texts || {};
      ov.texts[d.idx] = { dx, dy };
      applyEdgeFollow();
    }
    function endTextDrag(e) {
      const d = state.draggingText;
      if (!d) return;
      d.textEl.classList.remove('pex-text-dragging');
      d.textEl.removeEventListener('pointermove', onTextDrag);
      d.textEl.removeEventListener('pointerup', endTextDrag);
      d.textEl.removeEventListener('pointercancel', endTextDrag);
      try { d.textEl.releasePointerCapture(e.pointerId); } catch {}
      state.draggingText = null;
      fire();
    }

    // ---- Wire SVG ----------------------------------------------------------
    const entityHandlers = []; // { g, fn } — for cleanup
    const linkHandlers = [];
    const textHandlers = [];   // { textEl, fnDown, fnClick }
    const onSvgClick = () => { selectNode(null); selectEdge(null); };

    // Reconcile saved meta keys with the current SVG's entity qnames before
    // wiring transforms. If the user renamed an entity in source, our saved
    // delta lives under the OLD name; without migration the rename would
    // silently drop the layout. After 1:1 detection state.layout is in sync
    // and we mark dirty so the next commit writes the cleaned meta back.
    if (PexMeta && PexMeta.migrateRenamedKeys) {
      const liveQnames = [];
      svg.querySelectorAll('g.entity').forEach((g) => {
        const q = entityQname(g);
        if (q) liveQnames.push(q);
      });
      const renames = PexMeta.migrateRenamedKeys(liveQnames, state.layout);
      if (renames && Object.keys(renames).length > 0) {
        // Fire so the host (markdown extension, demo-host page) gets the
        // updated source with the migrated meta keys and can write it back.
        try { onLayoutChange(state.layout); } catch {}
        if (onSourceChange) {
          const s = buildSource();
          if (s !== null) { try { onSourceChange(s); } catch {} }
        }
        setDirty(true);
      }
    }

    svg.querySelectorAll('g.entity').forEach((g) => {
      const d = state.layout.nodes[entityQname(g)];
      if (d) g.setAttribute('transform', `translate(${d.dx}, ${d.dy})`);
      const fnDown = (e) => startDrag(e, g);
      // pointerdown handles selection (in startDrag); click only stops the
      // event from reaching svg's onSvgClick which would clear selection.
      // (Doing selection on click too would double-toggle for Shift+click.)
      const fnClick = (e) => { e.stopPropagation(); };
      g.addEventListener('pointerdown', fnDown);
      g.addEventListener('click', fnClick);
      entityHandlers.push({ g, fnDown, fnClick });
    });
    svg.querySelectorAll('g.link').forEach((linkG) => {
      const fnClick = (e) => { e.stopPropagation(); selectEdge(linkG); };
      linkG.addEventListener('click', fnClick);
      linkHandlers.push({ g: linkG, fnClick });

      // Per-text drag — only engages with Ctrl/Cmd held, otherwise the event
      // bubbles up to the link's click handler and selects the whole edge.
      linkG.querySelectorAll('text').forEach((textEl, tIdx) => {
        const fnDown = (e) => {
          if (!(e.ctrlKey || e.metaKey)) return;
          e.preventDefault();
          e.stopPropagation();
          startTextDrag(e, linkG, textEl, tIdx);
        };
        // After Ctrl-drag, the resulting click would still bubble to linkG
        // and trigger edge selection. Swallow it so the gesture is purely
        // "move this label" with no selection side-effect.
        const fnTextClick = (e) => {
          if (e.ctrlKey || e.metaKey) e.stopPropagation();
        };
        // On macOS, Ctrl+click is the secondary-click gesture and fires the
        // OS context menu, which would preempt our drag. Suppress it on edge
        // labels — there's no useful default action for right-clicking a
        // label in this editor anyway. Mac users can also use Cmd+drag,
        // which doesn't trigger contextmenu at all.
        const fnContextMenu = (e) => {
          if (e.ctrlKey || e.metaKey) e.preventDefault();
        };
        textEl.addEventListener('pointerdown', fnDown);
        textEl.addEventListener('click', fnTextClick);
        textEl.addEventListener('contextmenu', fnContextMenu);
        textHandlers.push({ textEl, fnDown, fnTextClick, fnContextMenu });
      });
    });
    svg.addEventListener('click', onSvgClick);

    // Esc clears any current selection. Listener is on the document so it
    // works regardless of focus (the SVG itself rarely takes focus).
    const onKeyDown = (e) => {
      if (e.key !== 'Escape') return;
      if (!state.selectedSet.size && !state.selectedEdge) return;
      selectNode(null);
      selectEdge(null);
    };
    doc.addEventListener('keydown', onKeyDown);

    applyEdgeFollow();
    adjustViewBox();

    // Reposition the floating toolbar on scroll/resize so it tracks the edge.
    // renderHandles runs too so any layout shift that affects SVG element
    // ordering (e.g. async DOM mutations from the markdown preview host)
    // gets the handle layer re-promoted to the top of the SVG.
    const onReposition = () => {
      if (!state.selectedEdge) return;
      showEdgeToolbar();
      renderHandles();
    };
    win.addEventListener('scroll', onReposition, true);
    win.addEventListener('resize', onReposition);

    // ---- L2: draft restoration ---------------------------------------------
    // If a localStorage draft exists for this source-hash and differs from the
    // current source, surface a non-blocking prompt above the SVG.
    let draftPromptEl = null;
    function showDraftPrompt(draft) {
      if (draftPromptEl) return;
      const ageMin = Math.max(1, Math.floor((Date.now() - (draft.ts || 0)) / 60000));
      const el = doc.createElement('div');
      el.className = 'pex-draft-prompt';
      el.innerHTML =
        `<span class="pex-draft-msg">저장되지 않은 레이아웃 변경이 있습니다 (~${ageMin}분 전).</span>`
        + '<button class="primary" data-action="restore">복원</button>'
        + '<button data-action="discard">무시</button>';
      el.addEventListener('click', (e) => {
        const a = e.target.closest('button[data-action]');
        if (!a) return;
        if (a.dataset.action === 'restore') {
          const parsed = PexMeta && PexMeta.parseSource(draft.source);
          if (parsed && parsed.meta && parsed.meta.layout) {
            state.layout = normalizeLayout(parsed.meta.layout);
            svg.querySelectorAll('g.entity').forEach((g) => {
              const d = state.layout.nodes[entityQname(g)];
              if (d) g.setAttribute('transform', `translate(${d.dx}, ${d.dy})`);
              else g.removeAttribute('transform');
            });
            applyEdgeFollow();
            adjustViewBox();
            fire(); // restored layout is dirty until host saves
          }
        } else if (a.dataset.action === 'discard') {
          clearDraft();
        }
        el.remove();
        draftPromptEl = null;
      });
      container.insertBefore(el, container.firstChild);
      draftPromptEl = el;
    }
    if (sourceHash) {
      try {
        const raw = win.localStorage.getItem(DRAFT_PREFIX + sourceHash);
        if (raw) {
          const draft = JSON.parse(raw);
          if (draft && typeof draft.source === 'string' && draft.source !== initialSource) {
            showDraftPrompt(draft);
          }
        }
      } catch {}
    }

    // ---- Public API --------------------------------------------------------
    return {
      getLayout() { return state.layout; },
      getSource() { return buildSource(); },
      isDirty() { return state.dirty; },
      setLayout(layout) {
        state.layout = normalizeLayout(layout);
        applyEdgeFollow();
        adjustViewBox();
      },
      // Host calls this after its own persist succeeds. Clears the dirty
      // badge AND removes the localStorage draft for this diagram.
      markSaved() {
        clearDraft();
        setDirty(false);
      },
      deactivate() {
        entityHandlers.forEach(({ g, fnDown, fnClick }) => {
          g.removeEventListener('pointerdown', fnDown);
          g.removeEventListener('click', fnClick);
        });
        linkHandlers.forEach(({ g, fnClick }) => g.removeEventListener('click', fnClick));
        textHandlers.forEach(({ textEl, fnDown, fnTextClick, fnContextMenu }) => {
          textEl.removeEventListener('pointerdown', fnDown);
          textEl.removeEventListener('click', fnTextClick);
          textEl.removeEventListener('contextmenu', fnContextMenu);
          textEl.classList.remove('pex-text-dragging');
        });
        svg.removeEventListener('click', onSvgClick);
        doc.removeEventListener('keydown', onKeyDown);
        win.removeEventListener('scroll', onReposition, true);
        win.removeEventListener('resize', onReposition);
        clearHandles();
        // Remove edge hit-area paths we injected.
        svg.querySelectorAll('path.pex-edge-hit').forEach((p) => p.remove());
        const layer = svg.querySelector('g.pex-handle-layer');
        if (layer) layer.remove();
        for (const el of state.selectedSet) el.classList.remove('pex-selected');
        if (state.selectedEdge) state.selectedEdge.classList.remove('pex-edge-selected');
        tb.remove();
        if (dirtyBadgeEl) dirtyBadgeEl.remove();
        if (draftPromptEl) draftPromptEl.remove();
        container.classList.remove('pex-inline-host');
      },
    };
  }

  return { activate, normalizeLayout };
});
