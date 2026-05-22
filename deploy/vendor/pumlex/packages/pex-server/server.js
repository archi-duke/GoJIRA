const express = require('express');
const fs = require('fs');
const path = require('path');
const crypto = require('crypto');
const cheerio = require('cheerio');
const plantumlEncoder = require('plantuml-encoder');
const PexGeom = require('@archi-duke/pex-core/geom');
const PexMeta = require('@archi-duke/pex-core/meta');

// Schema 2 added `layout.participants` (sequence diagram participant column
// dx). Schema 1 readers ignore unknown layout sub-keys gracefully — both
// `inline.js` and `server.js` `normalizeLayout` preserve any of nodes / edges
// / participants present, missing keys default to `{}`.
const META_SCHEMA = 2;

const app = express();
app.use(express.json({ limit: '4mb' }));
// `/render-with-layout` accepts `text/plain` (raw .puml source) bodies so a
// markdown viewer can POST a code block verbatim without JSON wrapping.
app.use(express.text({ type: 'text/plain', limit: '4mb' }));
app.use(express.static(path.join(__dirname, 'public')));
// Serve @archi-duke/pex-core as static assets so edit.html / demo-host.html
// can <script src="/pex-core/{geom,meta,inline}.js"> the same modules the
// server itself requires. Resolves the canonical pex-core dir even via the
// workspace symlink (npm workspaces flat-installs at the repo root).
const pexCoreDir = path.dirname(require.resolve('@archi-duke/pex-core/package.json'));
app.use('/pex-core', express.static(pexCoreDir));

// Open CORS so embeds from a different origin (GoJIRA-App at :3000, a markdown
// viewer page, etc.) can hit the render endpoints. The data we serve is just
// SVG/diagram bytes; nothing privileged.
app.use((req, res, next) => {
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'GET,POST,PUT,OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type');
  if (req.method === 'OPTIONS') return res.sendStatus(204);
  next();
});

const DATA_DIR = path.join(__dirname, 'data');
const DIAGRAMS_DIR = path.join(DATA_DIR, 'diagrams');
fs.mkdirSync(DIAGRAMS_DIR, { recursive: true });

const PLANTUML_URL = (process.env.PLANTUML_URL || 'http://localhost:8080').replace(/\/$/, '');

async function renderSvg(source) {
  if (!source || !source.trim()) throw new Error('empty source');
  const encoded = plantumlEncoder.encode(source);
  const url = `${PLANTUML_URL}/svg/${encoded}`;
  const res = await fetch(url);
  if (!res.ok) {
    const body = await res.text().catch(() => '');
    throw new Error(`render failed (${res.status}) at ${url}: ${body.slice(0, 200)}`);
  }
  return await res.text();
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
  return { nodes: raw, edges: {}, participants: {} };
}

function applyLayout(svg, rawLayout) {
  const layout = normalizeLayout(rawLayout);
  if (Object.keys(layout.nodes).length === 0
      && Object.keys(layout.edges).length === 0
      && Object.keys(layout.participants).length === 0) return svg;
  const $ = cheerio.load(svg, { xmlMode: true });

  // Sequence diagrams have completely different markup (no g.entity / g.link —
  // g.participant-* + g.message instead). Dispatch to a separate helper that
  // mirrors the client-side `activateSequence` adapter in pex-core/inline.js.
  const diagramType = $('svg').first().attr('data-diagram-type');
  if (diagramType === 'SEQUENCE') {
    applyLayoutSequence($, layout);
    return $.xml();
  }

  // Build qname ↔ id maps so PlantUML's auto-generated `ent00XX` ids can shift
  // freely as the source is edited without invalidating saved layouts.
  const qToEl = {};
  const idToQ = {};
  $('g.entity').each((_, el) => {
    const $el = $(el);
    const id = $el.attr('id');
    const q = $el.attr('data-qualified-name') || id;
    if (q) { qToEl[q] = $el; idToQ[id] = q; }
  });

  // Detect simple 1:1 renames (orphan meta key + fresh SVG qname) and
  // migrate the layout's node + edge keys before applying. Lets a meta
  // saved against `Order` keep its dx/dy after the user renames the
  // class to `PurchaseOrder` in source, without manual cleanup.
  PexMeta.migrateRenamedKeys(Object.keys(qToEl), layout);
  const nodes = layout.nodes;
  const edges = layout.edges;

  for (const [qname, delta] of Object.entries(nodes)) {
    const $el = qToEl[qname];
    if (!$el) continue;
    const existing = ($el.attr('transform') || '').trim();
    const add = `translate(${delta.dx}, ${delta.dy})`;
    $el.attr('transform', existing ? `${existing} ${add}` : add);
  }
  $('g.link').each((_, el) => {
    const $el = $(el);
    const e1Id = $el.attr('data-entity-1');
    const e2Id = $el.attr('data-entity-2');
    const q1 = idToQ[e1Id] || e1Id;
    const q2 = idToQ[e2Id] || e2Id;
    const d1 = nodes[q1] || { dx: 0, dy: 0 };
    const d2 = nodes[q2] || { dx: 0, dy: 0 };
    const eKey = `${q1}__${q2}`;
    const edgeOverride = edges[eKey];
    const moved = d1.dx || d1.dy || d2.dx || d2.dy;
    const hasOverride = !!edgeOverride;
    if (!moved && !hasOverride) return;

    const e1G = $(`g[id="${e1Id}"]`).first();
    const e2G = $(`g[id="${e2Id}"]`).first();
    if (!e1G.length || !e2G.length) return;
    const b1 = entityBBox($, e1G);
    const b2 = entityBBox($, e2G);
    if (!b1 || !b2) return;

    const e1Box = { x: b1.x + d1.dx, y: b1.y + d1.dy, w: b1.w, h: b1.h };
    const e2Box = { x: b2.x + d2.dx, y: b2.y + d2.dy, w: b2.w, h: b2.h };
    const c1 = PexGeom.bboxCenter(e1Box);
    const c2 = PexGeom.bboxCenter(e2Box);
    const newStart = (edgeOverride && edgeOverride.startAnchor)
      ? PexGeom.pointOnBboxBorder(e1Box, edgeOverride.startAnchor)
      : PexGeom.rectExit(c1, c2, e1Box);
    const newEnd = (edgeOverride && edgeOverride.endAnchor)
      ? PexGeom.pointOnBboxBorder(e2Box, edgeOverride.endAnchor)
      : PexGeom.rectExit(c2, c1, e2Box);

    const firstPath = $el.find('path').first();
    const origD = firstPath.attr('d');
    const ctx = origD ? PexGeom.getPathContext(origD) : null;
    if (!ctx) return;

    let tipExtStart = 0, tipExtEnd = 0;
    const polyInfo = [];
    $el.find('polygon').each((__, poly) => {
      const $poly = $(poly);
      const op = $poly.attr('points');
      if (!op) return;
      const nums = op.trim().split(/[\s,]+/).filter((s) => s.length).map(parseFloat);
      let cx = 0, cy = 0, n = 0;
      for (let i = 0; i + 1 < nums.length; i += 2) { cx += nums[i]; cy += nums[i + 1]; n++; }
      cx /= n; cy /= n;
      const dStart = (cx - ctx.start.x) ** 2 + (cy - ctx.start.y) ** 2;
      const dEnd = (cx - ctx.end.x) ** 2 + (cy - ctx.end.y) ** 2;
      const atStart = dStart < dEnd;
      const origAnchor = atStart ? ctx.start : ctx.end;
      const outwardAng = atStart ? ctx.startTangent + Math.PI : ctx.endTangent;
      const ext = PexGeom.polygonOutwardExtent(op, origAnchor, outwardAng);
      if (atStart) tipExtStart = Math.max(tipExtStart, ext);
      else tipExtEnd = Math.max(tipExtEnd, ext);
      polyInfo.push({ $poly, atStart, origAnchor });
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
    firstPath.attr('d', built.d);

    const oldStartAngle = ctx.startTangent;
    const oldEndAngle = ctx.endTangent;

    polyInfo.forEach(({ $poly, atStart, origAnchor }) => {
      const op = $poly.attr('data-pex-orig-points') || $poly.attr('points');
      const newAnchor = atStart ? newPathStart : newPathEnd;
      const oldAng = atStart ? oldStartAngle : oldEndAngle;
      const newAng = atStart ? built.startTangent : built.endTangent;
      $poly.attr('points', PexGeom.reorientPolygon(op, origAnchor, newAnchor, newAng - oldAng));
    });

    const odx = ctx.end.x - ctx.start.x, ody = ctx.end.y - ctx.start.y;
    const olen2 = odx * odx + ody * ody;
    const oldLineAngle = Math.atan2(ody, odx);
    const newLineAngle = Math.atan2(ndy, ndx);
    const dAng = newLineAngle - oldLineAngle;
    const cosA = Math.cos(dAng), sinA = Math.sin(dAng);

    const textsOv = (edgeOverride && edgeOverride.texts) || null;
    $el.find('text').each((tIdx, t) => {
      const $t = $(t);
      const x0 = parseFloat($t.attr('x'));
      const y0 = parseFloat($t.attr('y'));
      if (Number.isNaN(x0) || Number.isNaN(y0)) return;
      const tp = olen2 > 0
        ? Math.max(0, Math.min(1, ((x0 - ctx.start.x) * odx + (y0 - ctx.start.y) * ody) / olen2))
        : 0.5;
      const projX = ctx.start.x + tp * odx;
      const projY = ctx.start.y + tp * ody;
      const offX = x0 - projX, offY = y0 - projY;
      const rotX = offX * cosA - offY * sinA;
      const rotY = offX * sinA + offY * cosA;
      // Per-text offset (Ctrl/Cmd-drag of e.g. multiplicity labels). Mirrors
      // pex-core/inline.js applyEdgeFollow so the SAVED layout renders the
      // same way the inline editor previewed it. Without this branch the
      // server strips the user's fine-tune on every /render-with-layout.
      const tov = textsOv && textsOv[tIdx];
      const tdx = (tov && tov.dx) || 0;
      const tdy = (tov && tov.dy) || 0;
      $t.attr('x', String(newStart.x + tp * ndx + rotX + tdx));
      $t.attr('y', String(newStart.y + tp * ndy + rotY + tdy));
    });
  });
  resizeContainers($, nodes);
  expandViewBox($, nodes);
  return $.xml();
}

// Mirrors pex-core/inline.js resizeContainers: PlantUML draws clusters
// (rectangle/package/node) at a fixed size; once a child entity is
// translated by a layout delta the cluster no longer follows. Walk all
// clusters deepest-first and rewrite each cluster's <rect> (and label
// position) to span the union of its direct children's translated bboxes
// plus PlantUML's original padding (snapshotted from the natural layout
// before any delta is applied).
function resizeContainers($, nodes) {
  const clusters = $('g.cluster').toArray();
  if (!clusters.length) return;
  // Deepest-first so a nested cluster's new bbox is in place before its
  // parent reads it.
  clusters.sort((a, b) => {
    const ad = ($(a).attr('data-qualified-name') || '').split('.').length;
    const bd = ($(b).attr('data-qualified-name') || '').split('.').length;
    return bd - ad;
  });
  for (const c of clusters) resizeOneCluster($, $(c), nodes);
}

function resizeOneCluster($, $cluster, nodes) {
  const cqname = $cluster.attr('data-qualified-name');
  if (!cqname) return;
  const prefix = cqname + '.';
  // Direct children = entity OR cluster whose qname is `prefix + <leaf>`
  // (no extra dots in the rest).
  const $children = $('g.entity, g.cluster').filter((_, el) => {
    const $el = $(el);
    if ($el.is($cluster)) return false;
    const q = $el.attr('data-qualified-name');
    if (!q || !q.startsWith(prefix)) return false;
    const rest = q.slice(prefix.length);
    return rest.length > 0 && !rest.includes('.');
  });
  if (!$children.length) return;

  const $rect = $cluster.find('rect').first();
  const $text = $cluster.find('text').first();
  if (!$rect.length) return;

  // Original cluster geometry (PlantUML's natural layout).
  const ox = parseFloat($rect.attr('x'));
  const oy = parseFloat($rect.attr('y'));
  const ow = parseFloat($rect.attr('width'));
  const oh = parseFloat($rect.attr('height'));
  if ([ox, oy, ow, oh].some(Number.isNaN)) return;
  const otOffsetY = $text.length ? (parseFloat($text.attr('y')) - oy) : 0;

  // Original (no-delta) children bbox — for snapshotting padding.
  let omnX = Infinity, omnY = Infinity, omxX = -Infinity, omxY = -Infinity;
  $children.each((_, el) => {
    const b = entityBBox($, $(el));
    if (!b) return;
    if (b.x < omnX) omnX = b.x;
    if (b.y < omnY) omnY = b.y;
    if (b.x + b.w > omxX) omxX = b.x + b.w;
    if (b.y + b.h > omxY) omxY = b.y + b.h;
  });
  if (!isFinite(omnX)) return;
  const padLeft   = omnX - ox;
  const padRight  = (ox + ow) - omxX;
  const padTop    = omnY - oy;
  const padBottom = (oy + oh) - omxY;

  // Translated children bbox (deltas applied for entities; nested clusters
  // already have their rect mutated by a previous deepest-first pass).
  let mnX = Infinity, mnY = Infinity, mxX = -Infinity, mxY = -Infinity;
  $children.each((_, el) => {
    const $el = $(el);
    const b = entityBBox($, $el);
    if (!b) return;
    let dx = 0, dy = 0;
    if ($el.hasClass('entity')) {
      const q = $el.attr('data-qualified-name') || $el.attr('id');
      const d = q ? nodes[q] : null;
      if (d) { dx = d.dx || 0; dy = d.dy || 0; }
    }
    const x = b.x + dx, y = b.y + dy;
    if (x < mnX) mnX = x;
    if (y < mnY) mnY = y;
    if (x + b.w > mxX) mxX = x + b.w;
    if (y + b.h > mxY) mxY = y + b.h;
  });
  if (!isFinite(mnX)) return;

  const newX = mnX - padLeft;
  const newY = mnY - padTop;
  const newW = (mxX - mnX) + padLeft + padRight;
  const newH = (mxY - mnY) + padTop + padBottom;
  $rect.attr('x', String(newX));
  $rect.attr('y', String(newY));
  $rect.attr('width', String(newW));
  $rect.attr('height', String(newH));
  if ($text.length) {
    const tl = parseFloat($text.attr('textLength'));
    if (isFinite(tl)) $text.attr('x', String(newX + (newW - tl) / 2));
    $text.attr('y', String(newY + otOffsetY));
  }
}

function expandViewBox($, layout) {
  const svg = $('svg').first();
  const origVb = (svg.attr('viewBox') || '').split(/\s+/).map(Number);
  // Compute the tight enclosing rect of all post-edit content: entities
  // (translated), clusters (post-resize), edge labels (post-offset), and
  // free text (titles, captions). Mirrors pex-core/inline.js so the SVG
  // returned from /render-with-layout matches what the inline editor
  // showed — and shrinks back to fit when content moves closer together.
  let xMin = Infinity, yMin = Infinity, xMax = -Infinity, yMax = -Infinity;
  let any = false;
  const consider = (x1, y1, x2, y2) => {
    if (!isFinite(x1) || !isFinite(y1) || !isFinite(x2) || !isFinite(y2)) return;
    if (x1 < xMin) xMin = x1;
    if (y1 < yMin) yMin = y1;
    if (x2 > xMax) xMax = x2;
    if (y2 > yMax) yMax = y2;
    any = true;
  };
  // Entities, with their translate offsets
  $('g.entity').each((_, el) => {
    const $el = $(el);
    const q = $el.attr('data-qualified-name') || $el.attr('id');
    const d = (q && layout[q]) || { dx: 0, dy: 0 };
    const b = entityBBox($, $el);
    if (!b) return;
    consider(b.x + d.dx, b.y + d.dy, b.x + d.dx + b.w, b.y + d.dy + b.h);
  });
  // Clusters after resizeContainers updated their <rect>
  $('g.cluster').each((_, el) => {
    const $rect = $(el).find('rect').first();
    if (!$rect.length) return;
    const cx = parseFloat($rect.attr('x'));
    const cy = parseFloat($rect.attr('y'));
    const cw = parseFloat($rect.attr('width'));
    const ch = parseFloat($rect.attr('height'));
    if ([cx, cy, cw, ch].some(Number.isNaN)) return;
    consider(cx, cy, cx + cw, cy + ch);
  });
  // Free text — titles, captions. Skip text inside entity/cluster (already
  // covered by their bboxes) and edge labels (handled below). Use a coarse
  // estimate of the rendered text bbox: width ≈ chars · font · 0.6,
  // height ≈ font, with the y attribute treated as the baseline.
  const TEXT_HEIGHT_DEFAULT = 12;
  $('text').each((_, el) => {
    const $el = $(el);
    if ($el.closest('g.entity, g.cluster, g.link').length) return;
    const x = parseFloat($el.attr('x'));
    const y = parseFloat($el.attr('y'));
    if (Number.isNaN(x) || Number.isNaN(y)) return;
    const fs = parseFloat(($el.attr('font-size') || '').toString().replace('px','')) || TEXT_HEIGHT_DEFAULT;
    const txt = $el.text() || '';
    const w = txt.length * fs * 0.6;
    consider(x, y - fs, x + w, y);
  });
  // Edge labels (post-applyEdgeFollow x/y reflect any user offset)
  $('g.link text').each((_, el) => {
    const $el = $(el);
    const x = parseFloat($el.attr('x'));
    const y = parseFloat($el.attr('y'));
    if (Number.isNaN(x) || Number.isNaN(y)) return;
    const fs = parseFloat(($el.attr('font-size') || '').toString().replace('px','')) || TEXT_HEIGHT_DEFAULT;
    const txt = $el.text() || '';
    const w = txt.length * fs * 0.6;
    consider(x, y - fs, x + w, y);
  });
  if (!any) {
    // Empty / unrecognized markup — keep the original viewBox.
    if (origVb.length !== 4) return;
    return;
  }
  const pad = 10;
  xMin -= pad; yMin -= pad; xMax += pad; yMax += pad;
  const nw = xMax - xMin, nh = yMax - yMin;
  svg.attr('viewBox', `${xMin} ${yMin} ${nw} ${nh}`);
  svg.attr('width', `${nw}px`);
  svg.attr('height', `${nh}px`);
  // Drop inline style width/height so host CSS can scale responsively.
  const style = (svg.attr('style') || '')
    .replace(/width\s*:[^;]+;?/g, '')
    .replace(/height\s*:[^;]+;?/g, '');
  if (style.trim()) svg.attr('style', style); else svg.removeAttr('style');
  // Force uniform scaling so the diagram never squishes when CSS caps width.
  const par = svg.attr('preserveAspectRatio');
  if (!par || par === 'none') svg.attr('preserveAspectRatio', 'xMidYMid meet');
}

// ---- Sequence diagram layout (mirror of pex-core/inline.js activateSequence)
// Translates each participant column by its saved dx (X-only — sequence Y is
// driven by message order in source, not draggable) and recomputes every
// message's <line>/<polygon>/<text> coords from the participants' deltas.
// Kept structurally parallel to the client adapter so any future tweak to
// participant geometry can be ported in one obvious place.
function applyLayoutSequence($, layout) {
  const participants = layout.participants || {};
  if (!Object.keys(participants).length) return;

  const partsByQname = {};
  const uidToQname = {};
  function getOrCreate(q) {
    let p = partsByQname[q];
    if (!p) {
      p = partsByQname[q] = {
        qname: q, lifelineG: null, headG: null, tailG: null,
        activationRects: [], origCenter: null, currentDx: 0,
      };
    }
    return p;
  }
  $('g.participant-lifeline').each((_, el) => {
    const $el = $(el);
    const q = $el.attr('data-qualified-name');
    const uid = $el.attr('data-entity-uid');
    if (!q) return;
    if (uid) uidToQname[uid] = q;
    getOrCreate(q).lifelineG = $el;
  });
  $('g.participant-head').each((_, el) => {
    const $el = $(el);
    const q = $el.attr('data-qualified-name');
    if (q) getOrCreate(q).headG = $el;
  });
  $('g.participant-tail').each((_, el) => {
    const $el = $(el);
    const q = $el.attr('data-qualified-name');
    if (q) getOrCreate(q).tailG = $el;
  });

  // Display name (e.g. "Web App") -> qname (e.g. "Web") so we can attribute
  // activation rects (which only carry the display name in <title>) to their
  // owning column.
  const displayToQname = {};
  for (const q of Object.keys(partsByQname)) {
    const p = partsByQname[q];
    let display = q;
    if (p.lifelineG) {
      const $t = p.lifelineG.find('title').first();
      if ($t.length) display = $t.text().trim() || q;
    }
    displayToQname[display] = q;
  }

  // Activation rects: top-level <g> children of svg's outer <g> with no class
  // and a single <title>+<rect> shape signature.
  const $mainG = $('svg > g').first();
  if ($mainG.length) {
    $mainG.children('g').each((_, el) => {
      const $el = $(el);
      if ($el.attr('class')) return;
      const $first = $el.children().first();
      if (!$first.length || ($first[0].tagName || $first[0].name || '').toLowerCase() !== 'title') return;
      const display = $first.text().trim();
      const q = displayToQname[display];
      if (q && partsByQname[q]) partsByQname[q].activationRects.push($el);
    });
  }

  // Snapshot original column-center x (must be read BEFORE applying transforms).
  function getOrigCenter(p) {
    if (p.origCenter !== null) return p.origCenter;
    let center = 0;
    if (p.lifelineG) {
      const $line = p.lifelineG.find('line').first();
      if ($line.length) {
        center = parseFloat($line.attr('x1'));
      } else {
        const $rect = p.lifelineG.find('rect').first();
        if ($rect.length) {
          center = parseFloat($rect.attr('x')) + parseFloat($rect.attr('width')) / 2;
        }
      }
    }
    p.origCenter = center;
    return center;
  }
  for (const q of Object.keys(partsByQname)) getOrigCenter(partsByQname[q]);

  for (const q of Object.keys(partsByQname)) {
    const meta = participants[q];
    partsByQname[q].currentDx = (meta && typeof meta.dx === 'number') ? meta.dx : 0;
  }

  // Apply translate(dx, 0) to lifeline + head + tail + every activation rect.
  for (const q of Object.keys(partsByQname)) {
    const p = partsByQname[q];
    if (!p.currentDx) continue;
    const t = `translate(${p.currentDx}, 0)`;
    [p.lifelineG, p.headG, p.tailG, ...p.activationRects].forEach(($el) => {
      if ($el && $el.length) $el.attr('transform', t);
    });
  }

  // Recompute message geometry. Strategy mirrors pex-core/inline.js
  // applyMessageGeometry — see that file for the per-rule reasoning.
  $('g.message').each((_, el) => {
    const $g = $(el);
    const q1 = uidToQname[$g.attr('data-entity-1')];
    const q2 = uidToQname[$g.attr('data-entity-2')];
    if (!q1 || !q2) return;
    const p1 = partsByQname[q1];
    const p2 = partsByQname[q2];
    if (!p1 || !p2) return;
    const dx1 = p1.currentDx || 0;
    const dx2 = p2.currentDx || 0;
    const c1 = getOrigCenter(p1);
    const c2 = getOrigCenter(p2);

    const $line = $g.find('line').first();
    let ox1 = null, ox2 = null;
    if ($line.length) {
      let s1 = $line.attr('data-pex-orig-x1');
      let s2 = $line.attr('data-pex-orig-x2');
      if (s1 === undefined || s1 === null) {
        s1 = $line.attr('x1');
        s2 = $line.attr('x2');
        $line.attr('data-pex-orig-x1', s1);
        $line.attr('data-pex-orig-x2', s2);
      }
      ox1 = parseFloat(s1);
      ox2 = parseFloat(s2);
      const x1IsQ1 = Math.abs(ox1 - c1) <= Math.abs(ox1 - c2);
      const x1Dx = x1IsQ1 ? dx1 : dx2;
      const x2Dx = x1IsQ1 ? dx2 : dx1;
      $line.attr('x1', String(ox1 + x1Dx));
      $line.attr('x2', String(ox2 + x2Dx));
    }

    $g.find('polygon').each((__, p) => {
      const $poly = $(p);
      let opts = $poly.attr('data-pex-orig-points');
      if (opts === undefined || opts === null) {
        opts = $poly.attr('points');
        $poly.attr('data-pex-orig-points', opts);
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
      $poly.attr('points', out.join(' '));
    });

    if ($line.length && ox1 !== null) {
      const span = ox2 - ox1;
      $g.find('text').each((__, t) => {
        const $t = $(t);
        let s = $t.attr('data-pex-orig-x');
        if (s === undefined || s === null) {
          s = $t.attr('x');
          $t.attr('data-pex-orig-x', s);
        }
        const ox = parseFloat(s);
        const tp = (Math.abs(span) > 0.001) ? Math.max(0, Math.min(1, (ox - ox1) / span)) : 0.5;
        const x1IsQ1 = Math.abs(ox1 - c1) <= Math.abs(ox1 - c2);
        const startDx = x1IsQ1 ? dx1 : dx2;
        const endDx   = x1IsQ1 ? dx2 : dx1;
        const labelDx = startDx + tp * (endDx - startDx);
        $t.attr('x', String(ox + labelDx));
      });
    }
  });

  expandViewBoxSequence($, partsByQname);
}

function expandViewBoxSequence($, partsByQname) {
  const $svg = $('svg').first();
  // Tight bbox of all sequence content (translated participants, messages,
  // free text). Mirrors the generic expandViewBox so the diagram resizes
  // both up and down with edits.
  let xMin = Infinity, yMin = Infinity, xMax = -Infinity, yMax = -Infinity;
  let any = false;
  const consider = (x1, y1, x2, y2) => {
    if (!isFinite(x1) || !isFinite(y1) || !isFinite(x2) || !isFinite(y2)) return;
    if (x1 < xMin) xMin = x1; if (y1 < yMin) yMin = y1;
    if (x2 > xMax) xMax = x2; if (y2 > yMax) yMax = y2;
    any = true;
  };
  for (const q of Object.keys(partsByQname)) {
    const p = partsByQname[q];
    [p.lifelineG, p.headG, p.tailG, ...p.activationRects].forEach(($el) => {
      if (!$el || !$el.length) return;
      const b = entityBBox($, $el);
      if (!b) return;
      const ex = b.x + (p.currentDx || 0);
      consider(ex, b.y, ex + b.w, b.y + b.h);
    });
  }
  // Free text + message text — coarse estimate, same as expandViewBox.
  const TEXT_HEIGHT_DEFAULT = 12;
  $('text').each((_, el) => {
    const $el = $(el);
    if ($el.closest('g.participant-head, g.participant-tail, g.participant-lifeline').length) return;
    const x = parseFloat($el.attr('x'));
    const y = parseFloat($el.attr('y'));
    if (Number.isNaN(x) || Number.isNaN(y)) return;
    const fs = parseFloat(($el.attr('font-size') || '').toString().replace('px','')) || TEXT_HEIGHT_DEFAULT;
    const txt = $el.text() || '';
    const w = txt.length * fs * 0.6;
    consider(x, y - fs, x + w, y);
  });
  if (!any) return;
  const pad = 10;
  xMin -= pad; yMin -= pad; xMax += pad; yMax += pad;
  const nw = xMax - xMin, nh = yMax - yMin;
  $svg.attr('viewBox', `${xMin} ${yMin} ${nw} ${nh}`);
  $svg.attr('width', `${nw}px`);
  $svg.attr('height', `${nh}px`);
  const style = ($svg.attr('style') || '')
    .replace(/width\s*:[^;]+;?/g, '')
    .replace(/height\s*:[^;]+;?/g, '');
  if (style.trim()) $svg.attr('style', style); else $svg.removeAttr('style');
  const par = $svg.attr('preserveAspectRatio');
  if (!par || par === 'none') $svg.attr('preserveAspectRatio', 'xMidYMid meet');
}

function cssEscape(s) {
  return s.replace(/([!"#$%&'()*+,./:;<=>?@[\\\]^`{|}~])/g, '\\$1');
}

// Compute the bounding box of an entity by scanning its child shapes. Used as
// a server-side substitute for SVGGraphicsElement.getBBox().
function entityBBox($, $entity) {
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
  const consider = (x1, y1, x2, y2) => {
    if (x1 < minX) minX = x1; if (y1 < minY) minY = y1;
    if (x2 > maxX) maxX = x2; if (y2 > maxY) maxY = y2;
  };
  $entity.find('rect').each((_, el) => {
    const $el = $(el);
    const x = parseFloat($el.attr('x') || 0);
    const y = parseFloat($el.attr('y') || 0);
    const w = parseFloat($el.attr('width') || 0);
    const h = parseFloat($el.attr('height') || 0);
    consider(x, y, x + w, y + h);
  });
  $entity.find('ellipse').each((_, el) => {
    const $el = $(el);
    const cx = parseFloat($el.attr('cx') || 0);
    const cy = parseFloat($el.attr('cy') || 0);
    const rx = parseFloat($el.attr('rx') || 0);
    const ry = parseFloat($el.attr('ry') || 0);
    consider(cx - rx, cy - ry, cx + rx, cy + ry);
  });
  $entity.find('circle').each((_, el) => {
    const $el = $(el);
    const cx = parseFloat($el.attr('cx') || 0);
    const cy = parseFloat($el.attr('cy') || 0);
    const r = parseFloat($el.attr('r') || 0);
    consider(cx - r, cy - r, cx + r, cy + r);
  });
  $entity.find('polygon').each((_, el) => {
    const pts = ($(el).attr('points') || '').trim().split(/[\s,]+/).map(parseFloat);
    for (let i = 0; i + 1 < pts.length; i += 2) consider(pts[i], pts[i + 1], pts[i], pts[i + 1]);
  });
  // Actors and other composite entities use <path> (stick figure) and
  // sometimes <line>. Without these, the server-side bbox would be only
  // the actor's head ellipse, and arrow anchors on a moved actor would
  // start from the head instead of the actor's true edge.
  $entity.find('path').each((_, el) => {
    // Coarse but safe: strip command letters, treat every number pair as a
    // coordinate. PlantUML's actor stick-figure path is plain M/L commands
    // so this is exact; for general curves it overestimates slightly but
    // never undershoots, which is what we want for a containing bbox.
    const d = $(el).attr('d') || '';
    const nums = d.replace(/[A-Za-z]/g, ' ')
      .trim().split(/[\s,]+/).map(parseFloat).filter((n) => !Number.isNaN(n));
    for (let i = 0; i + 1 < nums.length; i += 2) consider(nums[i], nums[i + 1], nums[i], nums[i + 1]);
  });
  $entity.find('line').each((_, el) => {
    const $el = $(el);
    const x1 = parseFloat($el.attr('x1') || 0);
    const y1 = parseFloat($el.attr('y1') || 0);
    const x2 = parseFloat($el.attr('x2') || 0);
    const y2 = parseFloat($el.attr('y2') || 0);
    consider(Math.min(x1, x2), Math.min(y1, y2), Math.max(x1, x2), Math.max(y1, y2));
  });
  // Text labels often extend the visual bbox beyond the geometry shapes
  // (most importantly: actor labels sit BELOW the stick figure). Without
  // including them the server's bbox is smaller than the browser's
  // getBBox, and anchor/rectExit calculations land at slightly different
  // points → visible drift between live edit and post-save composite.
  $entity.find('text').each((_, el) => {
    const $el = $(el);
    const x = parseFloat($el.attr('x') || 0);
    const y = parseFloat($el.attr('y') || 0);
    if (Number.isNaN(x) || Number.isNaN(y)) return;
    const fontSize = parseFloat($el.attr('font-size') || 14);
    let width = parseFloat($el.attr('textLength'));
    if (!width || Number.isNaN(width)) {
      // Heuristic: ~0.6em per char for proportional fonts.
      const txt = $el.text() || '';
      width = txt.length * fontSize * 0.6;
    }
    // SVG <text>'s y is the baseline. Approximate ascender ~0.85em /
    // descender ~0.15em above/below it. Slightly conservative on each
    // side keeps the bbox a hair larger than the rendered glyphs.
    consider(x, y - fontSize * 0.85, x + width, y + fontSize * 0.15);
  });
  if (minX === Infinity) return null;
  return { x: minX, y: minY, w: maxX - minX, h: maxY - minY };
}

const pumlPath = (id) => path.join(DIAGRAMS_DIR, `${id}.puml`);
const legacyJsonPath = (id) => path.join(DIAGRAMS_DIR, `${id}.json`);

// Diagram in-memory shape: { id, name, source, layout, updatedAt }.
// On disk: a single .puml file containing the user's clean source plus a
// commented `' @startmeta ... ' @endmeta` block carrying name/layout/etc.
// Legacy `.json` files are still readable; once a diagram is saved through
// the new path it is upgraded to `.puml` (and the legacy file removed).
function loadDiagram(id) {
  const pp = pumlPath(id);
  if (fs.existsSync(pp)) {
    const full = fs.readFileSync(pp, 'utf8');
    const { source, meta } = PexMeta.parseSource(full);
    const m = meta || {};
    return {
      id,
      name: m.name || 'Untitled',
      source,
      layout: normalizeLayout(m.layout),
      updatedAt: m.updatedAt || 0,
    };
  }
  const lp = legacyJsonPath(id);
  if (fs.existsSync(lp)) {
    const d = JSON.parse(fs.readFileSync(lp, 'utf8'));
    return {
      id,
      name: d.name || 'Untitled',
      source: d.source || '',
      layout: normalizeLayout(d.layout),
      updatedAt: d.updatedAt || 0,
    };
  }
  return null;
}

function saveDiagram(id, data) {
  const meta = {
    schema: META_SCHEMA,
    name: data.name || 'Untitled',
    layout: normalizeLayout(data.layout),
    updatedAt: data.updatedAt || Date.now(),
  };
  const full = PexMeta.embedMeta(data.source || '', meta);
  fs.writeFileSync(pumlPath(id), full);
  // Best-effort cleanup of the old JSON sidecar after a successful upgrade.
  const lp = legacyJsonPath(id);
  if (fs.existsSync(lp)) { try { fs.unlinkSync(lp); } catch { /* ignore */ } }
}

app.post('/render', async (req, res) => {
  try {
    // Strip any embedded meta block — PlantUML's /svg/ endpoint rejects
    // content after `@enduml`, even when each line is a `'` comment.
    const clean = PexMeta.stripMeta(req.body.source || '');
    const svg = await renderSvg(clean);
    res.type('image/svg+xml').send(svg);
  } catch (e) {
    res.status(500).json({ error: e.message });
  }
});

// One-shot rendering for stateless hosts (markdown viewers, VS Code preview):
// accepts a full `.puml` body — clean source plus the `' @startmeta ... '
// @endmeta` block — and returns a layout-applied SVG. No diagram is stored.
async function renderWithLayoutFromBody(body) {
  const raw = (typeof body === 'string') ? body
            : (body && typeof body.source === 'string') ? body.source
            : '';
  const { source: clean, meta } = PexMeta.parseSource(raw);
  if (!clean.trim()) throw new Error('empty source');
  const baseSvg = await renderSvg(clean);
  const layout = (meta && meta.layout) ? meta.layout : null;
  return layout ? applyLayout(baseSvg, layout) : baseSvg;
}

app.post('/render-with-layout', async (req, res) => {
  try {
    const svg = await renderWithLayoutFromBody(req.body);
    res.type('image/svg+xml').send(svg);
  } catch (e) {
    res.status(500).type('text/plain').send(`error: ${e.message}`);
  }
});

// GET variant for hosts that can't POST from a CSP-restricted webview but
// CAN load images from localhost (notably VS Code's markdown preview, whose
// CSP omits `connect-src` but allows `img-src http://localhost:* …`).
//
// `?src=…` URL-encoded source. URL length limits (~8KB practical) cap the
// diagram size; long ones should fall back to a different transport.
//
// Errors are returned as a small SVG (not 500) so an `<img>` tag still
// renders something the user can read instead of a broken-image icon.
function renderErrorSvg(msg) {
  const safe = String(msg).replace(/[<>&]/g, (c) => ({ '<':'&lt;','>':'&gt;','&':'&amp;' }[c]));
  return `<?xml version="1.0" encoding="UTF-8"?>` +
    `<svg xmlns="http://www.w3.org/2000/svg" width="560" height="64" viewBox="0 0 560 64">` +
      `<rect width="560" height="64" fill="#fef2f2" stroke="#fecaca" stroke-width="1"/>` +
      `<text x="12" y="28" font-family="-apple-system,system-ui,sans-serif" font-size="13" fill="#b91c1c" font-weight="bold">⚠ pumlex 렌더링 오류</text>` +
      `<text x="12" y="48" font-family="ui-monospace,Menlo,monospace" font-size="11" fill="#7f1d1d">${safe.slice(0, 110)}</text>` +
    `</svg>`;
}
app.get('/render-with-layout', async (req, res) => {
  try {
    const src = req.query.src;
    if (typeof src !== 'string' || !src.trim()) throw new Error('missing ?src');
    const svg = await renderWithLayoutFromBody(src);
    res.type('image/svg+xml').send(svg);
  } catch (e) {
    res.type('image/svg+xml').send(renderErrorSvg(e.message || String(e)));
  }
});

app.post('/diagrams', (req, res) => {
  const id = crypto.randomBytes(6).toString('hex');
  const { name = 'Untitled', source = '' } = req.body || {};
  // If the incoming source already has an embedded meta block, prefer its
  // layout so importing a `.puml` file round-trips its prior edits.
  const parsed = PexMeta.parseSource(source);
  saveDiagram(id, {
    id,
    name: (parsed.meta && parsed.meta.name) || name,
    source: parsed.source,
    layout: normalizeLayout(parsed.meta && parsed.meta.layout),
    updatedAt: Date.now(),
  });
  res.json({ id });
});

// Raw `.puml` (source + embedded meta) for download / sharing.
// MUST be registered before `/diagrams/:id` — otherwise that route's `:id`
// would greedily match `abc.puml`.
app.get('/diagrams/:id.puml', (req, res) => {
  const d = loadDiagram(req.params.id);
  if (!d) return res.status(404).type('text/plain').send('not found');
  const full = PexMeta.embedMeta(d.source || '', {
    schema: META_SCHEMA,
    name: d.name,
    layout: d.layout,
    updatedAt: d.updatedAt,
  });
  res.type('text/plain; charset=utf-8');
  res.setHeader('Content-Disposition', `attachment; filename="${d.id}.puml"`);
  res.send(full);
});

app.get('/diagrams/:id', (req, res) => {
  const d = loadDiagram(req.params.id);
  if (!d) return res.status(404).json({ error: 'not found' });
  res.json(d);
});

app.put('/diagrams/:id', (req, res) => {
  const existing = loadDiagram(req.params.id) || { id: req.params.id, layout: {}, name: 'Untitled', source: '' };
  const { source, name } = req.body || {};
  if (source !== undefined) {
    // Accept either clean source or already-embedded `.puml`. If the latter,
    // adopt its meta as well so a paste-import is lossless.
    const parsed = PexMeta.parseSource(source);
    existing.source = parsed.source;
    if (parsed.meta) {
      if (parsed.meta.name && name === undefined) existing.name = parsed.meta.name;
      if (parsed.meta.layout) existing.layout = normalizeLayout(parsed.meta.layout);
    }
  }
  if (name !== undefined) existing.name = name;
  existing.updatedAt = Date.now();
  saveDiagram(req.params.id, existing);
  res.json({ ok: true });
});

app.put('/layouts/:id', (req, res) => {
  const d = loadDiagram(req.params.id);
  if (!d) return res.status(404).json({ error: 'not found' });
  d.layout = normalizeLayout((req.body && req.body.layout) || {});
  d.updatedAt = Date.now();
  saveDiagram(req.params.id, d);
  res.json({ ok: true });
});

async function composeFor(id) {
  const d = loadDiagram(id);
  if (!d) return null;
  const raw = await renderSvg(d.source || '');
  return { diagram: d, svg: applyLayout(raw, d.layout) };
}

app.get('/embed/:id.svg', async (req, res) => {
  try {
    const out = await composeFor(req.params.id);
    if (!out) return res.status(404).send('not found');
    res.type('image/svg+xml').send(out.svg);
  } catch (e) {
    res.status(500).send(`error: ${e.message}`);
  }
});

app.get('/embed/:id', async (req, res) => {
  try {
    const out = await composeFor(req.params.id);
    if (!out) return res.status(404).send('not found');
    const { diagram, svg } = out;
    res.send(`<!doctype html><html><head><meta charset="utf-8"><title>${escapeHtml(diagram.name)}</title>
<style>
  body { margin:0; padding:12px; font-family: system-ui, sans-serif; }
  .bar { position: fixed; top: 8px; right: 8px; }
  .bar a { padding: 4px 10px; background:#2563eb; color:#fff; text-decoration:none; border-radius:4px; font-size:12px; }
  svg { max-width: 100%; height: auto; }
</style></head><body>
<div class="bar"><a href="/edit/${diagram.id}" target="_top">Edit</a></div>
${svg}
</body></html>`);
  } catch (e) {
    res.status(500).send(`error: ${e.message}`);
  }
});

app.get('/edit/:id', (_req, res) => res.sendFile(path.join(__dirname, 'public', 'edit.html')));
// `/edit` (no id) — used by the embed/popup integration which has no
// pre-existing diagram and gets its source pushed in via `postMessage`.
app.get('/edit', (_req, res) => res.sendFile(path.join(__dirname, 'public', 'edit.html')));
app.get('/', (_req, res) => res.sendFile(path.join(__dirname, 'public', 'edit.html')));

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ '&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;' }[c]));
}

const PORT = process.env.PORT || 3000;
app.listen(PORT, () => {
  console.log(`plantumlEx listening on http://localhost:${PORT}`);
  console.log(`PLANTUML_URL = ${PLANTUML_URL}`);
});
