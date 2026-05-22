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
    .pex-inline-host text.pex-selectable { cursor: pointer; }
    .pex-inline-host text.pex-text-selected {
      fill: #ef4444 !important;
      font-weight: bold;
    }
    .pex-marquee {
      fill: rgba(37, 99, 235, 0.08);
      stroke: #2563eb;
      stroke-width: 1;
      stroke-dasharray: 4 3;
      pointer-events: none;
    }
    /* Sequence diagram — participant columns are draggable horizontally.
       Lifeline / head / tail share the same x and translate together. */
    .pex-inline-host g.participant-lifeline,
    .pex-inline-host g.participant-head,
    .pex-inline-host g.participant-tail { cursor: grab; }
    .pex-inline-host g.participant-lifeline.pex-dragging,
    .pex-inline-host g.participant-head.pex-dragging,
    .pex-inline-host g.participant-tail.pex-dragging { cursor: grabbing; }
    .pex-inline-host g.participant-lifeline:hover line {
      stroke: #2563eb !important; stroke-width: 1.5 !important;
    }
    .pex-inline-host g.participant-head:hover rect,
    .pex-inline-host g.participant-tail:hover rect {
      stroke: #2563eb !important; stroke-width: 2 !important;
    }
    .pex-inline-host g.participant-head:hover ellipse,
    .pex-inline-host g.participant-tail:hover ellipse {
      stroke: #2563eb !important; stroke-width: 2 !important;
    }
    .pex-inline-host g.participant-head:hover path:not([fill="none"]),
    .pex-inline-host g.participant-tail:hover path:not([fill="none"]) {
      stroke: #2563eb !important; stroke-width: 2 !important;
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
    if (!raw) return { nodes: {}, edges: {}, participants: {} };
    if (raw.nodes !== undefined || raw.edges !== undefined || raw.participants !== undefined) {
      return {
        nodes: raw.nodes || {},
        edges: raw.edges || {},
        participants: raw.participants || {},
      };
    }
    // Legacy plain-object form (pre-edges): treat as nodes-only.
    return { nodes: raw, edges: {}, participants: {} };
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
      // Unified multi-select: selectedSet holds any selectable element —
      // g.entity (node), g.link (edge), or <text> (label/title). `selected`
      // mirrors the most-recently-added member (used as the primary anchor
      // for drags and as the focus for single-target UI like the edge
      // toolbar / curve handles, which only show when the selection is
      // exactly one edge — see singleSelectedEdge()).
      selected: null,
      selectedSet: new Set(),
      dragging: null,
      draggingHandle: null,
      draggingText: null,
      // Sequence-only: in-progress participant column drag.
      draggingParticipant: null,
      // Marquee (rubber-band) drag state. Set on background pointerdown,
      // cleared on pointerup. While non-null, pointermove updates the rect.
      marquee: null,
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
        Object.keys(state.layout.edges || {}).length ||
        Object.keys(state.layout.participants || {}).length
      );
      // Schema 2 added `layout.participants` (sequence diagram column dx).
      // Older readers normalize unknown sub-keys to `{}` and ignore — bumping
      // the marker is a hint, not a hard gate.
      return hasLayout
        ? PexMeta.embedMeta(cleanSource, { schema: 2, layout: state.layout })
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

    // ---- Diagram-type dispatch ---------------------------------------------
    // Sequence/activity diagrams emit completely different SVG markup from
    // the class/component/state family the generic flow below was built for
    // (no g.entity, no g.link, no data-qualified-name on shapes — sequence
    // uses g.participant-* + g.message, activity has bare shapes). Each
    // diagram type gets its own wiring path below. Anything not recognised
    // falls through to the generic g.entity logic.
    const diagramType = svg.getAttribute('data-diagram-type');
    if (diagramType === 'SEQUENCE') {
      // ===== SEQUENCE adapter =================================================
      // Editing model: a "participant" is a vertical column composed of a
      // lifeline (dashed line + transparent click rect), a head (top label /
      // icon), an optional tail (bottom mirror), and zero or more activation
      // rects pinned to that column's x. The user drags the column horizontally;
      // we translate every visible piece by the same dx and recompute every
      // message's line/arrowhead/text x-coords from the participants' deltas.
      // Y is determined by message order in source — not draggable.

      // Build participant inventory keyed by qualified name (uid is volatile —
      // PlantUML auto-generates partN, mirrors the policy we use for ent ids).
      const partsByQname = new Map(); // qname -> { qname, lifelineG, headG, tailG, activationRects[], currentDx, _origCenter }
      const uidToQname = new Map();   // partN -> qname

      function getOrCreatePart(q) {
        let p = partsByQname.get(q);
        if (!p) {
          p = { qname: q, lifelineG: null, headG: null, tailG: null, activationRects: [], currentDx: 0, _origCenter: null };
          partsByQname.set(q, p);
        }
        return p;
      }

      svg.querySelectorAll('g.participant-lifeline').forEach((g) => {
        const q = g.getAttribute('data-qualified-name');
        const uid = g.getAttribute('data-entity-uid');
        if (!q) return;
        if (uid) uidToQname.set(uid, q);
        getOrCreatePart(q).lifelineG = g;
      });
      svg.querySelectorAll('g.participant-head').forEach((g) => {
        const q = g.getAttribute('data-qualified-name');
        if (!q) return;
        getOrCreatePart(q).headG = g;
      });
      svg.querySelectorAll('g.participant-tail').forEach((g) => {
        const q = g.getAttribute('data-qualified-name');
        if (!q) return;
        getOrCreatePart(q).tailG = g;
      });

      // Display-name → qname map. Activation rects identify their owner only
      // by `<title>` text (which is the participant's display name, e.g.
      // "Web App" for qname "Web"). Lifelines carry the same display in their
      // own <title>, so we can build the lookup from the lifelines we already
      // indexed.
      const displayToQname = new Map();
      partsByQname.forEach((p, q) => {
        const t = p.lifelineG && p.lifelineG.querySelector('title');
        const display = (t && t.textContent.trim()) || q;
        displayToQname.set(display, q);
      });

      // Collect activation rects: top-level <g> children of the SVG's main
      // <g> that have NO class and contain just `<title> + <rect>` — that's
      // PlantUML's signature for a per-message activation box.
      const mainG = svg.querySelector('g'); // outermost group inside <svg>
      if (mainG) {
        for (const child of mainG.children) {
          if (child.tagName.toLowerCase() !== 'g') continue;
          if (child.hasAttribute('class')) continue;
          const titleEl = child.firstElementChild;
          if (!titleEl || titleEl.tagName.toLowerCase() !== 'title') continue;
          const display = titleEl.textContent.trim();
          const q = displayToQname.get(display);
          if (!q) continue;
          const p = partsByQname.get(q);
          if (p) p.activationRects.push(child);
        }
      }

      // Snapshot each participant's ORIGINAL center x (used by message-geom
      // to decide which line endpoint belongs to which participant). Must be
      // captured BEFORE any transform is applied.
      function getOrigCenter(p) {
        if (p._origCenter !== null) return p._origCenter;
        // Lifeline's dashed <line> has x1==x2 == column center.
        const line = p.lifelineG && p.lifelineG.querySelector('line');
        if (line) {
          p._origCenter = parseFloat(line.getAttribute('x1'));
        } else {
          // Fallback: use the transparent click rect's mid-x.
          const rect = p.lifelineG && p.lifelineG.querySelector('rect');
          if (rect) {
            const x = parseFloat(rect.getAttribute('x'));
            const w = parseFloat(rect.getAttribute('width'));
            p._origCenter = x + w / 2;
          } else {
            p._origCenter = 0;
          }
        }
        return p._origCenter;
      }
      partsByQname.forEach(getOrigCenter); // prime cache

      // Apply the column translate to every visible piece of one participant.
      function applyParticipantTransform(p) {
        const t = p.currentDx ? `translate(${p.currentDx}, 0)` : null;
        const apply = (g) => {
          if (!g) return;
          if (t) g.setAttribute('transform', t);
          else g.removeAttribute('transform');
        };
        apply(p.lifelineG);
        apply(p.headG);
        apply(p.tailG);
        p.activationRects.forEach(apply);
      }

      // Recompute every message's geometry from current participant deltas.
      // Strategy per message:
      //   - line (single horizontal line between two columns):
      //       For x1 and x2 separately — find which participant's center it
      //       was originally closer to. That end shifts by that participant's
      //       dx. (Handles arrows in either direction without special-casing.)
      //   - polygon (arrowhead at destination):
      //       Compute centroid x; whichever lifeline center is closer wins
      //       the dx; translate every x coord in `points` by that dx.
      //   - text (label, may sit at any t along the line):
      //       Project onto the original line, get tp ∈ [0, 1], shift x by
      //       dx1 + tp * (dx2 - dx1) so labels track the line endpoints
      //       smoothly. (Mirrors how applyEdgeFollow projects edge labels
      //       in the generic class-diagram path.)
      function applyMessageGeometry() {
        svg.querySelectorAll('g.message').forEach((g) => {
          const uid1 = g.getAttribute('data-entity-1');
          const uid2 = g.getAttribute('data-entity-2');
          const q1 = uidToQname.get(uid1);
          const q2 = uidToQname.get(uid2);
          if (!q1 || !q2) return;
          const p1 = partsByQname.get(q1);
          const p2 = partsByQname.get(q2);
          if (!p1 || !p2) return;
          const dx1 = p1.currentDx || 0;
          const dx2 = p2.currentDx || 0;
          const c1 = getOrigCenter(p1);
          const c2 = getOrigCenter(p2);

          // <line> — capture original x1/x2 once; replay on every call.
          const line = g.querySelector('line');
          let ox1 = null, ox2 = null;
          if (line) {
            let s1 = line.getAttribute('data-pex-orig-x1');
            let s2 = line.getAttribute('data-pex-orig-x2');
            if (s1 === null) {
              s1 = line.getAttribute('x1');
              s2 = line.getAttribute('x2');
              line.setAttribute('data-pex-orig-x1', s1);
              line.setAttribute('data-pex-orig-x2', s2);
            }
            ox1 = parseFloat(s1);
            ox2 = parseFloat(s2);
            const x1IsQ1 = Math.abs(ox1 - c1) <= Math.abs(ox1 - c2);
            const x1Dx = x1IsQ1 ? dx1 : dx2;
            const x2Dx = x1IsQ1 ? dx2 : dx1;
            line.setAttribute('x1', String(ox1 + x1Dx));
            line.setAttribute('x2', String(ox2 + x2Dx));
          }

          // <polygon> — arrowhead. Pointed at one end of the line; pick that
          // end's dx by centroid proximity.
          g.querySelectorAll('polygon').forEach((poly) => {
            let opts = poly.getAttribute('data-pex-orig-points');
            if (opts === null) {
              opts = poly.getAttribute('points');
              poly.setAttribute('data-pex-orig-points', opts);
            }
            const nums = opts.trim().split(/[\s,]+/).filter((s) => s.length).map(parseFloat);
            let cx = 0, n = 0;
            for (let i = 0; i + 1 < nums.length; i += 2) { cx += nums[i]; n++; }
            cx = n ? cx / n : 0;
            const arrowDx = Math.abs(cx - c1) <= Math.abs(cx - c2) ? dx1 : dx2;
            const out = [];
            for (let i = 0; i + 1 < nums.length; i += 2) {
              out.push(`${nums[i] + arrowDx},${nums[i + 1]}`);
            }
            poly.setAttribute('points', out.join(' '));
          });

          // <text> — labels. Linear-interpolate the dx by where the label's
          // original x sat along the original line.
          if (line && ox1 !== null) {
            const span = ox2 - ox1;
            g.querySelectorAll('text').forEach((t) => {
              let s = t.getAttribute('data-pex-orig-x');
              if (s === null) {
                s = t.getAttribute('x');
                t.setAttribute('data-pex-orig-x', s);
              }
              const ox = parseFloat(s);
              const tp = (Math.abs(span) > 0.001) ? Math.max(0, Math.min(1, (ox - ox1) / span)) : 0.5;
              const x1IsQ1 = Math.abs(ox1 - c1) <= Math.abs(ox1 - c2);
              const startDx = x1IsQ1 ? dx1 : dx2;
              const endDx   = x1IsQ1 ? dx2 : dx1;
              const labelDx = startDx + tp * (endDx - startDx);
              t.setAttribute('x', String(ox + labelDx));
            });
          }
        });
      }

      // Sequence viewBox adjust: take the live SVG bbox so column drags
      // shrink AND grow the canvas. Identical to the generic path; kept
      // separate only because this branch runs before the generic helpers
      // are defined.
      function adjustViewBoxSequence() {
        let bbox;
        try { bbox = svg.getBBox(); } catch { bbox = null; }
        if (!bbox || bbox.width <= 0 || bbox.height <= 0) return;
        const pad = 10;
        const xMin = bbox.x - pad, yMin = bbox.y - pad;
        const nw = bbox.width + pad * 2, nh = bbox.height + pad * 2;
        svg.setAttribute('viewBox', `${xMin} ${yMin} ${nw} ${nh}`);
        svg.setAttribute('width', `${nw}px`);
        svg.setAttribute('height', `${nh}px`);
        const style = (svg.getAttribute('style') || '')
          .replace(/width\s*:[^;]+;?/g, '')
          .replace(/height\s*:[^;]+;?/g, '');
        if (style.trim()) svg.setAttribute('style', style); else svg.removeAttribute('style');
        const par = svg.getAttribute('preserveAspectRatio');
        if (!par || par === 'none') {
          svg.setAttribute('preserveAspectRatio', 'xMidYMid meet');
        }
      }

      // Apply any saved layout up front so the diagram opens already in its
      // last-edited shape.
      partsByQname.forEach((p) => {
        const meta = state.layout.participants[p.qname];
        if (meta && typeof meta.dx === 'number') {
          p.currentDx = meta.dx;
          applyParticipantTransform(p);
        }
      });
      applyMessageGeometry();
      adjustViewBoxSequence();

      // ---- Drag handlers ---------------------------------------------------
      function startParticipantDrag(e, p) {
        e.preventDefault();
        e.stopPropagation();
        const handle = e.currentTarget;
        const pt = clientToSvg(e, svg);
        state.draggingParticipant = {
          p, handle,
          originDx: p.currentDx || 0,
          startX: pt.x,
        };
        handle.classList.add('pex-dragging');
        try { handle.setPointerCapture(e.pointerId); } catch {}
        handle.addEventListener('pointermove', onParticipantDrag);
        handle.addEventListener('pointerup', endParticipantDrag);
        handle.addEventListener('pointercancel', endParticipantDrag);
      }
      function onParticipantDrag(e) {
        const d = state.draggingParticipant;
        if (!d) return;
        const pt = clientToSvg(e, svg);
        const dx = Math.round(d.originDx + (pt.x - d.startX));
        d.p.currentDx = dx;
        if (dx !== 0) state.layout.participants[d.p.qname] = { dx };
        else delete state.layout.participants[d.p.qname];
        applyParticipantTransform(d.p);
        applyMessageGeometry();
        adjustViewBoxSequence();
      }
      function endParticipantDrag(e) {
        const d = state.draggingParticipant;
        if (!d) return;
        d.handle.removeEventListener('pointermove', onParticipantDrag);
        d.handle.removeEventListener('pointerup', endParticipantDrag);
        d.handle.removeEventListener('pointercancel', endParticipantDrag);
        d.handle.classList.remove('pex-dragging');
        try { d.handle.releasePointerCapture(e.pointerId); } catch {}
        state.draggingParticipant = null;
        fire();
      }

      // Wire pointerdown on lifeline / head / tail for each participant.
      // (Activation rects are NOT drag handles — they translate with the
      // column but the user grabs the lifeline / head instead.)
      const seqHandlers = []; // { g, fnDown, fnClick }
      partsByQname.forEach((p) => {
        [p.lifelineG, p.headG, p.tailG].forEach((g) => {
          if (!g) return;
          const fnDown = (e) => startParticipantDrag(e, p);
          // click bubbles up after pointerup; nothing to do but avoid any
          // future ancestor handler interpreting it as a deselect.
          const fnClick = (e) => { e.stopPropagation(); };
          g.addEventListener('pointerdown', fnDown);
          g.addEventListener('click', fnClick);
          seqHandlers.push({ g, fnDown, fnClick });
        });
      });

      // ---- Draft restoration prompt (sequence variant) --------------------
      let seqDraftPromptEl = null;
      function showSeqDraftPrompt(draft) {
        if (seqDraftPromptEl) return;
        const ageMin = Math.max(1, Math.floor((Date.now() - (draft.ts || 0)) / 60000));
        const el = doc.createElement('div');
        el.className = 'pex-draft-prompt';
        el.innerHTML =
          `<span class="pex-draft-msg">저장되지 않은 레이아웃 변경이 있습니다 (~${ageMin}분 전).</span>`
          + '<button class="primary" data-action="restore">복원</button>'
          + '<button data-action="discard">무시</button>';
        el.addEventListener('click', (ev) => {
          const a = ev.target.closest('button[data-action]');
          if (!a) return;
          if (a.dataset.action === 'restore') {
            const parsed = PexMeta && PexMeta.parseSource(draft.source);
            if (parsed && parsed.meta && parsed.meta.layout) {
              state.layout = normalizeLayout(parsed.meta.layout);
              partsByQname.forEach((p) => {
                const meta = state.layout.participants[p.qname];
                p.currentDx = (meta && typeof meta.dx === 'number') ? meta.dx : 0;
                applyParticipantTransform(p);
              });
              applyMessageGeometry();
              adjustViewBoxSequence();
              fire();
            }
          } else if (a.dataset.action === 'discard') {
            clearDraft();
          }
          el.remove();
          seqDraftPromptEl = null;
        });
        container.insertBefore(el, container.firstChild);
        seqDraftPromptEl = el;
      }
      if (sourceHash) {
        try {
          const raw = win.localStorage.getItem(DRAFT_PREFIX + sourceHash);
          if (raw) {
            const draft = JSON.parse(raw);
            if (draft && typeof draft.source === 'string' && draft.source !== initialSource) {
              showSeqDraftPrompt(draft);
            }
          }
        } catch {}
      }

      // ---- Public API (sequence) ------------------------------------------
      return {
        getLayout() { return state.layout; },
        getSource() { return buildSource(); },
        isDirty() { return state.dirty; },
        setLayout(layout) {
          state.layout = normalizeLayout(layout);
          partsByQname.forEach((p) => {
            const meta = state.layout.participants[p.qname];
            p.currentDx = (meta && typeof meta.dx === 'number') ? meta.dx : 0;
            applyParticipantTransform(p);
          });
          applyMessageGeometry();
          adjustViewBoxSequence();
        },
        markSaved() {
          clearDraft();
          setDirty(false);
        },
        deactivate() {
          seqHandlers.forEach(({ g, fnDown, fnClick }) => {
            g.removeEventListener('pointerdown', fnDown);
            g.removeEventListener('click', fnClick);
          });
          if (dirtyBadgeEl) dirtyBadgeEl.remove();
          if (seqDraftPromptEl) seqDraftPromptEl.remove();
          container.classList.remove('pex-inline-host');
        },
      };
    }

    // ===== Generic adapter (class / component / state / etc.) ===============
    // Everything below here is the original g.entity / g.link wiring used by
    // every diagram type that isn't intercepted above.

    // ---- ViewBox bookkeeping -----------------------------------------------
    // Walk the SVG and compute the union bbox of actual diagram content,
    // skipping our overlay layers. svg.getBBox() would do this in one call
    // but also includes the handle layer + transient marquee rect, which
    // would jitter the viewBox during interactions. Hide those, measure,
    // restore.
    function computeContentBBox() {
      const overlay = svg.querySelector('g.pex-handle-layer');
      const marquee = state.marquee && state.marquee.rect;
      const overlayPrev = overlay ? overlay.getAttribute('display') : null;
      const marqueePrev = marquee ? marquee.getAttribute('display') : null;
      if (overlay) overlay.setAttribute('display', 'none');
      if (marquee) marquee.setAttribute('display', 'none');
      let bbox;
      try { bbox = svg.getBBox(); } catch { bbox = null; }
      if (overlay) {
        if (overlayPrev === null) overlay.removeAttribute('display');
        else overlay.setAttribute('display', overlayPrev);
      }
      if (marquee) {
        if (marqueePrev === null) marquee.removeAttribute('display');
        else marquee.setAttribute('display', marqueePrev);
      }
      if (!bbox || bbox.width <= 0 || bbox.height <= 0) return null;
      return bbox;
    }

    function adjustViewBox() {
      // Recompute the viewBox from the live content bbox so the SVG always
      // tracks what's actually drawn — both grows when a node is dragged
      // outside the original bounds, AND shrinks when edits leave empty
      // space. Excludes our transient overlays (handle layer, marquee) so
      // they never inflate the size.
      const bbox = computeContentBBox();
      if (!bbox) return;
      const pad = 10;
      const xMin = bbox.x - pad, yMin = bbox.y - pad;
      const nw = bbox.width + pad * 2, nh = bbox.height + pad * 2;
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
      if (singleSelectedEdge()) renderHandles();
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
    // Unified selection model: nodes, edges, and text labels live in one
    // selectedSet. Single-target UI (edge toolbar, curve handles) is shown
    // only when the selection is exactly one edge — singleSelectedEdge().
    //
    // select signatures:
    //   select(null)                 clear all
    //   select(el)                   replace selection with [el]
    //   select(el, { toggle: true }) toggle el (Shift+click)
    //   select(el, { add: true })    add without removing others (marquee)
    function selKind(el) {
      if (!el || !el.classList) return null;
      if (el.classList.contains('entity')) return 'node';
      if (el.classList.contains('link')) return 'edge';
      if (el.tagName && el.tagName.toLowerCase() === 'text') return 'text';
      return null;
    }
    function selClass(kind) {
      return kind === 'node' ? 'pex-selected'
           : kind === 'edge' ? 'pex-edge-selected'
           : kind === 'text' ? 'pex-text-selected'
           : null;
    }
    function singleSelectedEdge() {
      if (state.selectedSet.size !== 1) return null;
      const el = state.selected;
      return selKind(el) === 'edge' ? el : null;
    }
    function removeSelectionClass(el) {
      const c = selClass(selKind(el));
      if (c) el.classList.remove(c);
    }
    function addSelectionClass(el) {
      const c = selClass(selKind(el));
      if (c) el.classList.add(c);
    }
    function select(el, opts) {
      opts = opts || {};
      const prevEdge = singleSelectedEdge();
      if (el === null) {
        for (const prev of state.selectedSet) removeSelectionClass(prev);
        state.selectedSet.clear();
        state.selected = null;
        refreshEdgeUI(prevEdge);
        return;
      }
      if (!selKind(el)) return;
      if (opts.toggle) {
        if (state.selectedSet.has(el)) {
          state.selectedSet.delete(el);
          removeSelectionClass(el);
          if (state.selected === el) {
            state.selected = state.selectedSet.values().next().value || null;
          }
        } else {
          state.selectedSet.add(el);
          addSelectionClass(el);
          state.selected = el;
        }
      } else if (opts.add) {
        if (!state.selectedSet.has(el)) {
          state.selectedSet.add(el);
          addSelectionClass(el);
        }
        state.selected = el;
      } else {
        for (const prev of state.selectedSet) {
          if (prev !== el) removeSelectionClass(prev);
        }
        state.selectedSet.clear();
        state.selectedSet.add(el);
        addSelectionClass(el);
        state.selected = el;
      }
      refreshEdgeUI(prevEdge);
    }
    // Update edge-only UI (toolbar, handles, layout.edges auto-create)
    // whenever the single-edge focus changes. `prevEdge` is the edge that
    // had the focus before the mutation (for toolbar-offset reset detection).
    function refreshEdgeUI(prevEdge) {
      const cur = singleSelectedEdge();
      if (cur) {
        if (cur !== prevEdge) state.toolbarOffset = { dx: 0, dy: 0 };
        const eKey = edgeKeyForLink(svg, cur);
        if (eKey && !state.layout.edges[eKey]) {
          const pathEl = cur.querySelector('path:not(.pex-edge-hit)');
          const pathD = pathEl ? (pathEl.getAttribute('data-pex-orig-d') || pathEl.getAttribute('d') || '') : '';
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
      const eg = singleSelectedEdge();
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
      const eg = singleSelectedEdge();
      if (!eg) { hideEdgeToolbar(); return; }
      const eKey = edgeKeyForLink(svg, eg);
      const ov = state.layout.edges[eKey];
      const wasCurved = /[CQS]/i.test(eg.querySelector('path:not(.pex-edge-hit)').getAttribute('data-pex-orig-d') || '');
      const curType = (ov && ov.type) || (wasCurved ? 'curve' : 'straight');
      tb.querySelectorAll('button').forEach((b) => b.classList.toggle('active', b.dataset.type === curType));
      // applyEdgeFollow may have just mutated the path's `d` attribute
      // (select(edge) → refreshEdgeUI → applyEdgeFollow → showEdgeToolbar all
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
      const eg = singleSelectedEdge();
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
        select(g, { toggle: !!e.shiftKey });
      }
      const pt = clientToSvg(e, svg);
      // Snapshot each NODE member's start delta so the group moves rigidly.
      // Edges/text in the unified selection are not draggable here — they
      // remain in selection but stay put while the user drags nodes.
      const members = [];
      for (const el of state.selectedSet) {
        if (selKind(el) !== 'node') continue;
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
      const eg = singleSelectedEdge();
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
      const eg = singleSelectedEdge();
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

    // ---- Per-text drag-or-select (edge labels) ----------------------------
    // Lets the user move auxiliary text on an edge — multiplicity ("0..*"),
    // qualifiers, role names — independently of the line. The offset rides
    // *on top of* the auto-projection in applyEdgeFollow, so the label still
    // follows the line as nodes move while the user's fine-tune is preserved.
    //
    // Pointerdown opens a "maybe drag" state; the first pointermove past a
    // 2px threshold commits to a real drag, otherwise the gesture falls
    // through to the click handler which selects the label. Holding Shift
    // skips drag entirely so Shift+click can extend a multi-selection
    // without any movement.
    const TEXT_DRAG_THRESHOLD = 2;
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
      const startPt = clientToSvg(e, svg);
      try { textEl.setPointerCapture(e.pointerId); } catch {}
      state.draggingText = {
        textEl, idx, linkG,
        startX: startPt.x, startY: startPt.y,
        originDx: 0, originDy: 0,
        committed: false, // true once threshold is exceeded
        eKey: null,
        pointerId: e.pointerId,
      };
      textEl.addEventListener('pointermove', onTextDrag);
      textEl.addEventListener('pointerup', endTextDrag);
      textEl.addEventListener('pointercancel', endTextDrag);
    }
    function commitTextDrag(d) {
      const eKey = edgeKeyForLink(svg, d.linkG);
      if (!eKey) return false;
      const ov = ensureEdgeOverrideFor(d.linkG, eKey);
      ov.texts = ov.texts || {};
      const existing = ov.texts[d.idx] || { dx: 0, dy: 0 };
      d.eKey = eKey;
      d.originDx = existing.dx;
      d.originDy = existing.dy;
      d.committed = true;
      d.textEl.classList.add('pex-text-dragging');
      return true;
    }
    function onTextDrag(e) {
      const d = state.draggingText;
      if (!d) return;
      const pt = clientToSvg(e, svg);
      const offX = pt.x - d.startX, offY = pt.y - d.startY;
      if (!d.committed) {
        if (Math.abs(offX) < TEXT_DRAG_THRESHOLD && Math.abs(offY) < TEXT_DRAG_THRESHOLD) return;
        if (!commitTextDrag(d)) return;
      }
      const ov = state.layout.edges[d.eKey];
      if (!ov) return;  // edge gone (e.g. SVG re-rendered) — abort
      ov.texts = ov.texts || {};
      ov.texts[d.idx] = {
        dx: Math.round(d.originDx + offX),
        dy: Math.round(d.originDy + offY),
      };
      applyEdgeFollow();
    }
    function endTextDrag(e) {
      const d = state.draggingText;
      if (!d) return;
      d.textEl.removeEventListener('pointermove', onTextDrag);
      d.textEl.removeEventListener('pointerup', endTextDrag);
      d.textEl.removeEventListener('pointercancel', endTextDrag);
      try { d.textEl.releasePointerCapture(e.pointerId); } catch {}
      state.draggingText = null;
      if (d.committed) {
        d.textEl.classList.remove('pex-text-dragging');
        adjustViewBox();
        fire();
        // Swallow the trailing click so it doesn't also re-select.
        const swallow = (ev) => {
          ev.stopPropagation();
          d.textEl.removeEventListener('click', swallow, true);
        };
        d.textEl.addEventListener('click', swallow, true);
      }
      // !committed → no movement happened; the upcoming click will select.
    }

    // ---- Wire SVG ----------------------------------------------------------
    const entityHandlers = []; // { g, fn } — for cleanup
    const linkHandlers = [];
    const textHandlers = [];   // { textEl, fnDown, fnClick }
    const onSvgClick = () => { select(null); };

    // ---- Marquee (rubber-band) selection -----------------------------------
    // Drag from empty SVG background to capture every selectable element
    // whose visual bounding box intersects the marquee rect. Holding Shift
    // adds to the existing selection instead of replacing it.
    //
    // The marquee rect itself is rendered in SVG units so it scales with the
    // diagram (keeps a constant 4px-style stroke under PlantUML's units).
    // Hit-testing uses each candidate's getBoundingClientRect() — painted
    // screen-space coordinates — which sidesteps having to compose nested
    // transforms for entities, edges, and lifelines that all live under
    // different coordinate systems.
    const MARQUEE_THRESHOLD_PX = 4; // shorter drags are treated as a click
    function startMarquee(e) {
      e.preventDefault();
      const startSvg = clientToSvg(e, svg);
      const rect = doc.createElementNS(SVG_NS, 'rect');
      rect.setAttribute('class', 'pex-marquee');
      rect.setAttribute('x', startSvg.x);
      rect.setAttribute('y', startSvg.y);
      rect.setAttribute('width', 0);
      rect.setAttribute('height', 0);
      svg.appendChild(rect);
      state.marquee = {
        startSvg,
        startClientX: e.clientX, startClientY: e.clientY,
        curClientX: e.clientX, curClientY: e.clientY,
        rect,
        additive: !!e.shiftKey,
        moved: false,
        pointerId: e.pointerId,
      };
      try { svg.setPointerCapture(e.pointerId); } catch {}
      svg.addEventListener('pointermove', onMarqueeMove);
      svg.addEventListener('pointerup', endMarquee);
      svg.addEventListener('pointercancel', endMarquee);
    }
    function onMarqueeMove(e) {
      const m = state.marquee;
      if (!m) return;
      const pt = clientToSvg(e, svg);
      const x = Math.min(m.startSvg.x, pt.x);
      const y = Math.min(m.startSvg.y, pt.y);
      const w = Math.abs(pt.x - m.startSvg.x);
      const h = Math.abs(pt.y - m.startSvg.y);
      m.rect.setAttribute('x', x);
      m.rect.setAttribute('y', y);
      m.rect.setAttribute('width', w);
      m.rect.setAttribute('height', h);
      m.curClientX = e.clientX;
      m.curClientY = e.clientY;
      if (Math.abs(e.clientX - m.startClientX) > MARQUEE_THRESHOLD_PX
       || Math.abs(e.clientY - m.startClientY) > MARQUEE_THRESHOLD_PX) {
        m.moved = true;
      }
    }
    function endMarquee(e) {
      const m = state.marquee;
      if (!m) return;
      svg.removeEventListener('pointermove', onMarqueeMove);
      svg.removeEventListener('pointerup', endMarquee);
      svg.removeEventListener('pointercancel', endMarquee);
      try { svg.releasePointerCapture(m.pointerId); } catch {}
      m.rect.remove();
      state.marquee = null;
      if (!m.moved) {
        // Below threshold — treat as plain background click; the bubbled
        // 'click' event that follows will fire onSvgClick → clear selection.
        return;
      }
      const left = Math.min(m.startClientX, m.curClientX);
      const top = Math.min(m.startClientY, m.curClientY);
      const right = Math.max(m.startClientX, m.curClientX);
      const bottom = Math.max(m.startClientY, m.curClientY);
      applyMarqueeSelection({ left, top, right, bottom }, m.additive);
      // The pointerup is followed by a synthetic 'click' on the SVG that
      // would clear the just-built selection. Eat it once at capture phase.
      const swallow = (ev) => {
        ev.stopPropagation();
        svg.removeEventListener('click', swallow, true);
      };
      svg.addEventListener('click', swallow, true);
    }
    function applyMarqueeSelection(marquee, additive) {
      if (!additive) select(null);
      const hit = (el) => {
        let r;
        try { r = el.getBoundingClientRect(); } catch { return false; }
        if (!r || (r.width === 0 && r.height === 0)) return false;
        return !(
          r.right < marquee.left || r.left > marquee.right ||
          r.bottom < marquee.top || r.top > marquee.bottom
        );
      };
      svg.querySelectorAll('g.entity').forEach((g) => {
        if (hit(g)) select(g, { add: true });
      });
      svg.querySelectorAll('g.link').forEach((g) => {
        const path = g.querySelector('path:not(.pex-edge-hit)');
        if (path && hit(path)) select(g, { add: true });
      });
      // Edge labels (g.link text) and standalone selectable text both
      // participate. We rely on .pex-selectable as the marker for standalone
      // text; edge labels are detected via the g.link ancestor.
      svg.querySelectorAll('text').forEach((textEl) => {
        const isEdgeLabel = !!textEl.closest('g.link');
        const isStandaloneSelectable = textEl.classList.contains('pex-selectable');
        if (!isEdgeLabel && !isStandaloneSelectable) return;
        if (hit(textEl)) select(textEl, { add: true });
      });
    }
    function onSvgPointerDown(e) {
      if (e.button !== 0) return; // left button only
      if (state.marquee) return;  // already in progress (shouldn't happen)
      const t = e.target;
      // Skip when pressing on anything that has its own pointerdown semantics:
      // node drag, edge click, text label drag/select, curve handles/anchors.
      if (t.closest && t.closest(
        'g.entity, g.link, text, .pex-handle, .pex-anchor, ' +
        'g.participant-head, g.participant-tail, g.participant-lifeline, g.message'
      )) return;
      startMarquee(e);
    }
    svg.addEventListener('pointerdown', onSvgPointerDown);

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
      const fnClick = (e) => {
        e.stopPropagation();
        select(linkG, { toggle: !!e.shiftKey });
      };
      linkG.addEventListener('click', fnClick);
      linkHandlers.push({ g: linkG, fnClick });

      // Edge label gesture: pointerdown opens a "maybe drag" state. If the
      // pointer moves past TEXT_DRAG_THRESHOLD before pointerup, the label
      // moves; otherwise the gesture falls through to a plain click which
      // selects the label. Shift+click skips drag entirely so users can
      // extend a multi-selection without nudging the label.
      linkG.querySelectorAll('text').forEach((textEl, tIdx) => {
        const fnDown = (e) => {
          if (e.button !== 0) return;
          if (e.shiftKey) return; // Shift+click is select-only
          e.preventDefault();
          e.stopPropagation();
          startTextDrag(e, linkG, textEl, tIdx);
        };
        const fnTextClick = (e) => {
          // Always stop the click from bubbling to the link's edge-select
          // handler. If we already swallowed it (after a real drag), this
          // path won't run. Otherwise a plain click reaches here → select.
          e.stopPropagation();
          select(textEl, { toggle: !!e.shiftKey });
        };
        textEl.addEventListener('pointerdown', fnDown);
        textEl.addEventListener('click', fnTextClick);
        textHandlers.push({ textEl, fnDown, fnTextClick });
      });
    });

    // Standalone text — diagram titles, sequence note text, free-floating
    // labels not living inside a draggable container. Make them selectable
    // so users can highlight them with the same gestures as nodes/edges
    // (click / Shift+click / marquee).
    // Skip text inside g.entity / g.participant-* / g.message — those belong
    // to the parent and clicks there should drive the parent's behavior.
    // Text inside g.link is handled in the loop above (edge label).
    const DRAGGABLE_TEXT_PARENTS = 'g.entity, g.link, g.participant-head, g.participant-tail, g.participant-lifeline, g.message';
    svg.querySelectorAll('text').forEach((textEl) => {
      if (textEl.closest(DRAGGABLE_TEXT_PARENTS)) return;
      textEl.classList.add('pex-selectable');
      const fnTextClick = (e) => {
        e.stopPropagation();
        select(textEl, { toggle: !!e.shiftKey });
      };
      textEl.addEventListener('click', fnTextClick);
      textHandlers.push({ textEl, fnTextClick });
    });
    svg.addEventListener('click', onSvgClick);

    // Esc cancels an in-progress marquee, otherwise clears any current
    // selection. Listener is on the document so it works regardless of focus
    // (the SVG itself rarely takes focus).
    const onKeyDown = (e) => {
      if (e.key !== 'Escape') return;
      if (state.marquee) {
        const m = state.marquee;
        m.rect.remove();
        svg.removeEventListener('pointermove', onMarqueeMove);
        svg.removeEventListener('pointerup', endMarquee);
        svg.removeEventListener('pointercancel', endMarquee);
        try { svg.releasePointerCapture(m.pointerId); } catch {}
        state.marquee = null;
        return;
      }
      if (!state.selectedSet.size) return;
      select(null);
    };
    doc.addEventListener('keydown', onKeyDown);

    applyEdgeFollow();
    adjustViewBox();

    // Reposition the floating toolbar on scroll/resize so it tracks the edge.
    // renderHandles runs too so any layout shift that affects SVG element
    // ordering (e.g. async DOM mutations from the markdown preview host)
    // gets the handle layer re-promoted to the top of the SVG.
    const onReposition = () => {
      if (!singleSelectedEdge()) return;
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
          if (fnDown) textEl.removeEventListener('pointerdown', fnDown);
          if (fnTextClick) textEl.removeEventListener('click', fnTextClick);
          if (fnContextMenu) textEl.removeEventListener('contextmenu', fnContextMenu);
          textEl.classList.remove('pex-text-dragging');
          textEl.classList.remove('pex-text-selected');
          textEl.classList.remove('pex-selectable');
        });
        svg.removeEventListener('click', onSvgClick);
        svg.removeEventListener('pointerdown', onSvgPointerDown);
        if (state.marquee) {
          state.marquee.rect.remove();
          svg.removeEventListener('pointermove', onMarqueeMove);
          svg.removeEventListener('pointerup', endMarquee);
          svg.removeEventListener('pointercancel', endMarquee);
          state.marquee = null;
        }
        doc.removeEventListener('keydown', onKeyDown);
        win.removeEventListener('scroll', onReposition, true);
        win.removeEventListener('resize', onReposition);
        clearHandles();
        // Remove edge hit-area paths we injected.
        svg.querySelectorAll('path.pex-edge-hit').forEach((p) => p.remove());
        const layer = svg.querySelector('g.pex-handle-layer');
        if (layer) layer.remove();
        for (const el of state.selectedSet) removeSelectionClass(el);
        tb.remove();
        if (dirtyBadgeEl) dirtyBadgeEl.remove();
        if (draftPromptEl) draftPromptEl.remove();
        container.classList.remove('pex-inline-host');
      },
    };
  }

  return { activate, normalizeLayout };
});
