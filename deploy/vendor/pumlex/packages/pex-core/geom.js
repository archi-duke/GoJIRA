// Shared geometry helpers used by both client (edit.html) and server (server.js).
// Loaded as a plain script in browser; required as a module in Node.
(function (root, factory) {
  if (typeof module === 'object' && module.exports) module.exports = factory();
  else root.PexGeom = factory();
}(typeof self !== 'undefined' ? self : this, function () {

  function parsePath(d) {
    const tokens = [];
    const re = /([MLCQZ])([^MLCQZ]*)/gi;
    let m;
    while ((m = re.exec(d)) !== null) {
      const cmd = m[1].toUpperCase();
      const nums = m[2].trim().split(/[\s,]+/).filter((s) => s.length).map(parseFloat);
      const points = [];
      for (let i = 0; i + 1 < nums.length; i += 2) {
        points.push({ x: nums[i], y: nums[i + 1] });
      }
      tokens.push({ cmd, points });
    }
    return tokens;
  }

  function buildPath(tokens) {
    return tokens
      .map((t) => (t.cmd === 'Z' ? 'Z' : t.cmd + t.points.map((p) => `${p.x},${p.y}`).join(' ')))
      .join(' ');
  }

  // Walk path, building a list of endpoints. Each endpoint records its "out"
  // tangent handle (toward the next endpoint) and its "in" tangent handle (from
  // the previous endpoint), when present. M/L produce endpoints with no handle
  // info; C produces an endpoint with cIn=c2 and sets cOut=c1 on the previous
  // endpoint; Q is treated similarly with the same single control on both sides.
  function collectEndpoints(tokens) {
    const endpoints = [];
    for (const tok of tokens) {
      if (tok.cmd === 'M' || tok.cmd === 'L') {
        for (const p of tok.points) endpoints.push({ x: p.x, y: p.y });
      } else if (tok.cmd === 'C') {
        for (let i = 0; i + 2 < tok.points.length + 1; i += 3) {
          const c1 = tok.points[i], c2 = tok.points[i + 1], p = tok.points[i + 2];
          if (!c1 || !c2 || !p) break;
          if (endpoints.length > 0 && !endpoints[endpoints.length - 1].cOut) {
            endpoints[endpoints.length - 1].cOut = { x: c1.x, y: c1.y };
          }
          endpoints.push({ x: p.x, y: p.y, cIn: { x: c2.x, y: c2.y } });
        }
      } else if (tok.cmd === 'Q') {
        for (let i = 0; i + 1 < tok.points.length + 1; i += 2) {
          const c = tok.points[i], p = tok.points[i + 1];
          if (!c || !p) break;
          if (endpoints.length > 0 && !endpoints[endpoints.length - 1].cOut) {
            endpoints[endpoints.length - 1].cOut = { x: c.x, y: c.y };
          }
          endpoints.push({ x: p.x, y: p.y, cIn: { x: c.x, y: c.y } });
        }
      }
    }
    return endpoints;
  }

  // Reroute the path by collapsing it to a single cubic from the original first
  // endpoint to the original last endpoint, preserving the start and end tangent
  // directions (using the first endpoint's outgoing handle and the last
  // endpoint's incoming handle when available). Then apply d1 to the start side
  // and d2 to the end side. This avoids the chaos that arises when intermediate
  // routing waypoints from PlantUML's auto-layout are linearly interpolated.
  function rerouteD(d, d1, d2) {
    const eps = collectEndpoints(parsePath(d));
    if (eps.length < 2) return d;
    const first = eps[0];
    const last = eps[eps.length - 1];
    const dxLine = last.x - first.x, dyLine = last.y - first.y;
    const cOut = first.cOut || { x: first.x + dxLine / 3, y: first.y + dyLine / 3 };
    const cIn  = last.cIn   || { x: last.x  - dxLine / 3, y: last.y  - dyLine / 3 };
    const nFirst = { x: first.x + d1.dx, y: first.y + d1.dy };
    const nCOut  = { x: cOut.x  + d1.dx, y: cOut.y  + d1.dy };
    const nCIn   = { x: cIn.x   + d2.dx, y: cIn.y   + d2.dy };
    const nLast  = { x: last.x  + d2.dx, y: last.y  + d2.dy };
    return `M${nFirst.x},${nFirst.y} C${nCOut.x},${nCOut.y} ${nCIn.x},${nCIn.y} ${nLast.x},${nLast.y}`;
  }

  // Shift a polygon (typically an arrowhead) by d1 or d2, depending on which
  // path endpoint its centroid is closer to. Robust to reversed-direction
  // arrows where the head sits on the M side.
  function shiftPolygonByEnd(pointsStr, d1, d2, line) {
    const nums = pointsStr.trim().split(/[\s,]+/).filter((s) => s.length).map(parseFloat);
    if (nums.length < 2) return pointsStr;
    let cx = 0, cy = 0, n = 0;
    for (let i = 0; i + 1 < nums.length; i += 2) { cx += nums[i]; cy += nums[i + 1]; n++; }
    cx /= n; cy /= n;
    let shift = d2;
    if (line) {
      const ds = (cx - line.x1) ** 2 + (cy - line.y1) ** 2;
      const de = (cx - line.x2) ** 2 + (cy - line.y2) ** 2;
      shift = ds < de ? d1 : d2;
    }
    const out = [];
    for (let i = 0; i + 1 < nums.length; i += 2) {
      out.push(`${nums[i] + shift.dx},${nums[i + 1] + shift.dy}`);
    }
    return out.join(' ');
  }

  function pathEndpoints(d) {
    const tokens = parsePath(d);
    const all = [];
    tokens.forEach((tok) => tok.points.forEach((pt) => all.push(pt)));
    if (all.length < 2) return null;
    return { x1: all[0].x, y1: all[0].y, x2: all[all.length - 1].x, y2: all[all.length - 1].y };
  }

  function shiftPolygonPoints(pointsStr, dx, dy) {
    const nums = pointsStr.trim().split(/[\s,]+/).filter((s) => s.length).map(parseFloat);
    const out = [];
    for (let i = 0; i + 1 < nums.length; i += 2) {
      out.push(`${nums[i] + dx},${nums[i + 1] + dy}`);
    }
    return out.join(' ');
  }

  function tAlongLine(px, py, x1, y1, x2, y2) {
    const dx = x2 - x1, dy = y2 - y1;
    const len2 = dx * dx + dy * dy;
    if (len2 === 0) return 0.5;
    const t = ((px - x1) * dx + (py - y1) * dy) / len2;
    return Math.max(0, Math.min(1, t));
  }

  function lerp(a, b, t) { return a * (1 - t) + b * t; }

  function bboxCenter(b) { return { x: b.x + b.w / 2, y: b.y + b.h / 2 }; }

  // Resolve a {side, t} parameterization to an absolute point on the bbox
  // border. Stable across entity moves: as long as the bbox shifts together,
  // the relative position on the border stays fixed.
  function pointOnBboxBorder(bbox, anchor) {
    if (!anchor) return null;
    const t = Math.max(0, Math.min(1, anchor.t || 0));
    if (anchor.side === 'top')    return { x: bbox.x + t * bbox.w, y: bbox.y };
    if (anchor.side === 'right')  return { x: bbox.x + bbox.w,     y: bbox.y + t * bbox.h };
    if (anchor.side === 'bottom') return { x: bbox.x + t * bbox.w, y: bbox.y + bbox.h };
    if (anchor.side === 'left')   return { x: bbox.x,              y: bbox.y + t * bbox.h };
    return null;
  }

  // Find the closest point on a bbox border to an arbitrary point. Used while
  // dragging an anchor: the user moves freely with the pointer, but the result
  // snaps to the entity boundary.
  function nearestBboxBorder(bbox, p) {
    const tH = Math.max(0, Math.min(1, (p.x - bbox.x) / (bbox.w || 1)));
    const tV = Math.max(0, Math.min(1, (p.y - bbox.y) / (bbox.h || 1)));
    const cands = [
      { side: 'top',    t: tH, point: { x: bbox.x + tH * bbox.w, y: bbox.y } },
      { side: 'bottom', t: tH, point: { x: bbox.x + tH * bbox.w, y: bbox.y + bbox.h } },
      { side: 'left',   t: tV, point: { x: bbox.x,              y: bbox.y + tV * bbox.h } },
      { side: 'right',  t: tV, point: { x: bbox.x + bbox.w,     y: bbox.y + tV * bbox.h } },
    ];
    let best = cands[0], bestD = Infinity;
    for (const c of cands) {
      const d = (c.point.x - p.x) ** 2 + (c.point.y - p.y) ** 2;
      if (d < bestD) { bestD = d; best = c; }
    }
    return best;
  }

  // Find the point at which a ray from `from` toward `target` first crosses the
  // axis-aligned rectangle `rect`. Used to anchor an edge to the boundary of an
  // entity instead of to a fixed coordinate.
  function rectExit(from, target, rect) {
    const dx = target.x - from.x, dy = target.y - from.y;
    if (dx === 0 && dy === 0) return { x: from.x, y: from.y };
    const ts = [];
    if (dx > 0) ts.push((rect.x + rect.w - from.x) / dx);
    else if (dx < 0) ts.push((rect.x - from.x) / dx);
    if (dy > 0) ts.push((rect.y + rect.h - from.y) / dy);
    else if (dy < 0) ts.push((rect.y - from.y) / dy);
    const valid = ts.filter((t) => t > 0);
    const t = valid.length ? Math.min(...valid) : 0;
    return { x: from.x + t * dx, y: from.y + t * dy };
  }

  // Build a path between two boundary points. Returns the d string plus the
  // tangent angles at start and end (used by the caller to orient arrowheads).
  //
  // opts:
  //   type: 'straight' | 'curve' | 'ortho' (default inferred: 'curve' if ctx else 'straight')
  //   u1, u2: explicit handle vectors (start→cOut, cIn←end). Override the
  //           defaults when the user has manually adjusted handles.
  //   ctx:    original-path context from getPathContext, used to derive a
  //           sensible default curve when u1/u2 are not provided.
  function buildEdgePath(newStart, newEnd, opts) {
    opts = opts || {};
    const type = opts.type || (opts.ctx ? 'curve' : 'straight');
    const ndx = newEnd.x - newStart.x, ndy = newEnd.y - newStart.y;
    const nLen = Math.hypot(ndx, ndy) || 1;
    const lineAngle = Math.atan2(ndy, ndx);

    if (type === 'straight') {
      return {
        d: `M${newStart.x},${newStart.y} L${newEnd.x},${newEnd.y}`,
        startTangent: lineAngle,
        endTangent: lineAngle,
      };
    }

    if (type === 'ortho') {
      const horizontalFirst = Math.abs(ndx) >= Math.abs(ndy);
      const corner = horizontalFirst
        ? { x: newEnd.x, y: newStart.y }
        : { x: newStart.x, y: newEnd.y };
      const startTangent = Math.atan2(corner.y - newStart.y, corner.x - newStart.x);
      const endTangent = Math.atan2(newEnd.y - corner.y, newEnd.x - corner.x);
      return {
        d: `M${newStart.x},${newStart.y} L${corner.x},${corner.y} L${newEnd.x},${newEnd.y}`,
        startTangent, endTangent, corner,
      };
    }

    // curve
    let u1, u2;
    if (opts.u1 && opts.u2) {
      u1 = opts.u1;
      u2 = opts.u2;
    } else if (opts.ctx) {
      const ctx = opts.ctx;
      const odx = ctx.end.x - ctx.start.x, ody = ctx.end.y - ctx.start.y;
      const oLen = Math.hypot(odx, ody) || 1;
      const oDir = { x: odx / oLen, y: ody / oLen };
      const oPerp = { x: -oDir.y, y: oDir.x };
      const nDir = { x: ndx / nLen, y: ndy / nLen };
      const nPerp = { x: -nDir.y, y: nDir.x };
      const u1Perp = ctx.u1.x * oPerp.x + ctx.u1.y * oPerp.y;
      const u2Perp = ctx.u2.x * oPerp.x + ctx.u2.y * oPerp.y;
      const MIN_BEND = 0.10;
      const sign1 = u1Perp !== 0 ? Math.sign(u1Perp) : 1;
      const sign2 = u2Perp !== 0 ? Math.sign(u2Perp) : -sign1;
      const scale = nLen / oLen;
      const u1PerpNew = sign1 * Math.max(Math.abs(u1Perp) * scale, nLen * MIN_BEND);
      const u2PerpNew = sign2 * Math.max(Math.abs(u2Perp) * scale, nLen * MIN_BEND);
      const parLen = nLen / 3;
      u1 = { x: parLen * nDir.x + u1PerpNew * nPerp.x, y: parLen * nDir.y + u1PerpNew * nPerp.y };
      u2 = { x: parLen * nDir.x + u2PerpNew * nPerp.x, y: parLen * nDir.y + u2PerpNew * nPerp.y };
    } else {
      u1 = { x: ndx / 3, y: ndy / 3 };
      u2 = { x: ndx / 3, y: ndy / 3 };
    }
    const c1 = { x: newStart.x + u1.x, y: newStart.y + u1.y };
    const c2 = { x: newEnd.x - u2.x, y: newEnd.y - u2.y };
    return {
      d: `M${newStart.x},${newStart.y} C${c1.x},${c1.y} ${c2.x},${c2.y} ${newEnd.x},${newEnd.y}`,
      startTangent: Math.atan2(u1.y, u1.x),
      endTangent: Math.atan2(u2.y, u2.x),
      c1, c2, u1, u2,
    };
  }

  // Extract original path's start/end positions, tangent angles, and the actual
  // handle vectors u1 (start→cOut) and u2 (cIn→end). Handle vectors capture
  // both the tangent direction *and* the handle length, which together describe
  // the local curve character at each endpoint.
  function getPathContext(d) {
    const eps = collectEndpoints(parsePath(d));
    if (eps.length < 2) return null;
    const first = eps[0], last = eps[eps.length - 1];
    const dxLine = last.x - first.x, dyLine = last.y - first.y;
    const startTangent = first.cOut
      ? Math.atan2(first.cOut.y - first.y, first.cOut.x - first.x)
      : Math.atan2(dyLine, dxLine);
    const endTangent = last.cIn
      ? Math.atan2(last.y - last.cIn.y, last.x - last.cIn.x)
      : Math.atan2(dyLine, dxLine);
    const u1 = first.cOut
      ? { x: first.cOut.x - first.x, y: first.cOut.y - first.y }
      : { x: dxLine / 3, y: dyLine / 3 };
    const u2 = last.cIn
      ? { x: last.x - last.cIn.x, y: last.y - last.cIn.y }
      : { x: dxLine / 3, y: dyLine / 3 };
    return {
      start: { x: first.x, y: first.y },
      end: { x: last.x, y: last.y },
      startTangent, endTangent, u1, u2,
    };
  }

  // Geometric extent of a polygon past `anchor` along `outwardAngle`. For an
  // arrowhead, this is essentially the arrow's length: the distance from the
  // path-end anchor to the polygon's farthest point in the direction the
  // arrow points. The path end can then be pulled back by this amount so the
  // arrow tip lands precisely on the entity boundary.
  function polygonOutwardExtent(pointsStr, anchor, outwardAngle) {
    const cos = Math.cos(outwardAngle), sin = Math.sin(outwardAngle);
    const nums = pointsStr.trim().split(/[\s,]+/).filter((s) => s.length).map(parseFloat);
    let maxProj = 0;
    for (let i = 0; i + 1 < nums.length; i += 2) {
      const dx = nums[i] - anchor.x;
      const dy = nums[i + 1] - anchor.y;
      const proj = dx * cos + dy * sin;
      if (proj > maxProj) maxProj = proj;
    }
    return maxProj;
  }

  // Translate + rotate a polygon: move so anchorOld lands at anchorNew, then
  // rotate the whole polygon around anchorNew by dAngle (radians).
  function reorientPolygon(pointsStr, anchorOld, anchorNew, dAngle) {
    const nums = pointsStr.trim().split(/[\s,]+/).filter((s) => s.length).map(parseFloat);
    const cos = Math.cos(dAngle), sin = Math.sin(dAngle);
    const out = [];
    for (let i = 0; i + 1 < nums.length; i += 2) {
      const x = nums[i] - anchorOld.x;
      const y = nums[i + 1] - anchorOld.y;
      const rx = x * cos - y * sin;
      const ry = x * sin + y * cos;
      out.push(`${rx + anchorNew.x},${ry + anchorNew.y}`);
    }
    return out.join(' ');
  }

  return {
    parsePath, buildPath, rerouteD, pathEndpoints,
    shiftPolygonPoints, shiftPolygonByEnd,
    tAlongLine, lerp,
    bboxCenter, rectExit, buildEdgePath, getPathContext, reorientPolygon,
    pointOnBboxBorder, nearestBboxBorder, polygonOutwardExtent,
  };
}));
