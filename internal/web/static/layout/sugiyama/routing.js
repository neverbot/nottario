// Phase 6 — Orthogonal Channel Routing.
//
// Input: nodes with absolute (x, y, w, h), original edges with source
// and target node IDs. Output: per-edge ordered list of waypoints.
//
// Channels = horizontal strips between consecutive rows of nodes +
// vertical strips between consecutive columns + the four boundary
// strips. We compute them by sweeping the node y-coordinates and
// x-coordinates, taking the gaps as channels.
//
// For each edge:
//   - Identify the source face (the side closest to target) and the
//     target face (closest to source), preserving stub-perpendicular.
//   - If source and target are in the same column (same x range) or
//     in horizontally adjacent rows, route through the single channel
//     between them (2-bend).
//   - Otherwise, route through a sequence of horizontal and vertical
//     channels (3-5 bends), choosing the bends to minimise total
//     length.
//
// Track allocation per channel: edges sharing a channel are assigned
// unique tracks (offsets within the channel), sorted by their other
// endpoint's perpendicular position so neighbouring edges cluster.
//
// This is a clean, deterministic baseline. Track demand estimation
// (Wybrow 2009) and per-channel adaptive sizing live in `tracks.js`.

const TRACK_PITCH = 14;
const STUB = 40;
const FACE_MARGIN = 16;

function chooseFaces(src, tgt) {
  const sCx = src.x + src.w / 2, sCy = src.y + src.h / 2;
  const tCx = tgt.x + tgt.w / 2, tCy = tgt.y + tgt.h / 2;
  const vOv = Math.min(src.y + src.h, tgt.y + tgt.h) > Math.max(src.y, tgt.y);
  const hOv = Math.min(src.x + src.w, tgt.x + tgt.w) > Math.max(src.x, tgt.x);
  if (vOv && !hOv) return [tCx > sCx ? 'right' : 'left', tCx > sCx ? 'left' : 'right'];
  if (hOv && !vOv) return [tCy > sCy ? 'bottom' : 'top', tCy > sCy ? 'top' : 'bottom'];
  // Diagonal: prefer vertical if dy dominates.
  if (Math.abs(tCy - sCy) > Math.abs(tCx - sCx)) {
    return [tCy > sCy ? 'bottom' : 'top', tCy > sCy ? 'top' : 'bottom'];
  }
  return [tCx > sCx ? 'right' : 'left', tCx > sCx ? 'left' : 'right'];
}

function faceAnchorPx(node, side, frac) {
  const fx = node.x + node.w * frac;
  const fy = node.y + node.h * frac;
  switch (side) {
    case 'right':  return { x: node.x + node.w, y: fy };
    case 'left':   return { x: node.x,          y: fy };
    case 'bottom': return { x: fx, y: node.y + node.h };
    case 'top':    return { x: fx, y: node.y };
  }
}

function stubAnchorPx(node, side, frac) {
  const fx = node.x + node.w * frac;
  const fy = node.y + node.h * frac;
  switch (side) {
    case 'right':  return { x: node.x + node.w + STUB, y: fy };
    case 'left':   return { x: node.x - STUB,           y: fy };
    case 'bottom': return { x: fx, y: node.y + node.h + STUB };
    case 'top':    return { x: fx, y: node.y - STUB };
  }
}

// Compute the global channel structure: horizontal strips and vertical
// strips. Each channel is { axis, fixed, span: [from, to], tracks }.
function buildChannels(nodes) {
  // Horizontal strips: between consecutive (top, bottom) y-coordinates.
  const ys = new Set();
  const xs = new Set();
  for (const n of nodes) {
    ys.add(n.y); ys.add(n.y + n.h);
    xs.add(n.x); xs.add(n.x + n.w);
  }
  const ysSorted = [...ys].sort((a, b) => a - b);
  const xsSorted = [...xs].sort((a, b) => a - b);
  const horiz = []; // { yMin, yMax }
  const vert  = []; // { xMin, xMax }
  for (let i = 0; i + 1 < ysSorted.length; i++) {
    const a = ysSorted[i], b = ysSorted[i + 1];
    // Channel if no node occupies the full (a, b) y-strip.
    const occupied = nodes.some(n => n.y < b && n.y + n.h > a && (b - a) > 1 && (n.y <= a && n.y + n.h >= b));
    if (!occupied) horiz.push({ yMin: a, yMax: b });
  }
  for (let i = 0; i + 1 < xsSorted.length; i++) {
    const a = xsSorted[i], b = xsSorted[i + 1];
    const occupied = nodes.some(n => n.x < b && n.x + n.w > a && (b - a) > 1 && (n.x <= a && n.x + n.w >= b));
    if (!occupied) vert.push({ xMin: a, xMax: b });
  }
  return { horiz, vert };
}

// Decide a track Y within a horizontal channel for a given edge,
// allocating from a per-channel counter.
function allocateTrack(channelMap, channelKey, channelExtent) {
  const counter = (channelMap.get(channelKey)?.count || 0);
  channelMap.set(channelKey, { count: counter + 1, extent: channelExtent });
  return counter; // index 0, 1, 2, ...
}

function trackOffset(idx, total, centre, pitch = TRACK_PITCH) {
  return centre + (idx - (total - 1) / 2) * pitch;
}

export function routeOrthogonal(nodes, edges) {
  const byID = new Map(nodes.map(n => [n.id, n]));
  // 1. Face planning.
  const planned = [];
  for (const e of edges) {
    const src = byID.get(e.src);
    const tgt = byID.get(e.tgt);
    if (!src || !tgt) continue;
    const [sSide, tSide] = chooseFaces(src, tgt);
    planned.push({ edge: e, src, tgt, sSide, tSide, sFrac: 0.5, tFrac: 0.5 });
  }
  // 2. Face spread with constant pitch.
  const faceBucket = new Map();
  const push = (k, p, end) => {
    if (!faceBucket.has(k)) faceBucket.set(k, []);
    faceBucket.get(k).push({ p, end });
  };
  for (const p of planned) {
    push(p.src.id + '|' + p.sSide, p, 's');
    push(p.tgt.id + '|' + p.tSide, p, 't');
  }
  for (const [key, list] of faceBucket) {
    const side = key.split('|')[1];
    list.sort((a, b) => {
      const other = (it) => it.end === 's' ? it.p.tgt : it.p.src;
      if (side === 'top' || side === 'bottom') {
        return (other(a).x + other(a).w / 2) - (other(b).x + other(b).w / 2);
      }
      return (other(a).y + other(a).h / 2) - (other(b).y + other(b).h / 2);
    });
    const n = list.length;
    list.forEach((it, i) => {
      const self = it.end === 's' ? it.p.src : it.p.tgt;
      const L = (side === 'top' || side === 'bottom') ? self.w : self.h;
      let frac = 0.5 + (i - (n - 1) / 2) * TRACK_PITCH / L;
      if (frac < 0.05) frac = 0.05;
      if (frac > 0.95) frac = 0.95;
      if (it.end === 's') it.p.sFrac = frac;
      else                it.p.tFrac = frac;
    });
  }
  // 3. Per-edge waypoints with channel-based mid-paths.
  const { horiz, vert } = buildChannels(nodes);
  const vvGroups = new Map();
  const hhGroups = new Map();
  for (const p of planned) {
    p.sAnchor = faceAnchorPx(p.src, p.sSide, p.sFrac);
    p.tAnchor = faceAnchorPx(p.tgt, p.tSide, p.tFrac);
    p.sStub   = stubAnchorPx(p.src, p.sSide, p.sFrac);
    p.tStub   = stubAnchorPx(p.tgt, p.tSide, p.tFrac);
    const sV = p.sSide === 'top' || p.sSide === 'bottom';
    const tV = p.tSide === 'top' || p.tSide === 'bottom';
    if (sV && tV) {
      p.bend = 'vv';
      const k = `vv|${Math.round(p.sStub.y)}|${Math.round(p.tStub.y)}`;
      if (!vvGroups.has(k)) vvGroups.set(k, []);
      vvGroups.get(k).push(p);
    } else if (!sV && !tV) {
      p.bend = 'hh';
      const k = `hh|${Math.round(p.sStub.x)}|${Math.round(p.tStub.x)}`;
      if (!hhGroups.has(k)) hhGroups.set(k, []);
      hhGroups.get(k).push(p);
    } else {
      p.bend = 'L';
    }
  }
  for (const list of vvGroups.values()) {
    list.sort((a, b) => ((a.sStub.x + a.tStub.x) / 2) - ((b.sStub.x + b.tStub.x) / 2));
    const n = list.length;
    const centre = (list[0].sStub.y + list[0].tStub.y) / 2;
    list.forEach((p, i) => p.midY = trackOffset(i, n, centre));
  }
  for (const list of hhGroups.values()) {
    list.sort((a, b) => ((a.sStub.y + a.tStub.y) / 2) - ((b.sStub.y + b.tStub.y) / 2));
    const n = list.length;
    const centre = (list[0].sStub.x + list[0].tStub.x) / 2;
    list.forEach((p, i) => p.midX = trackOffset(i, n, centre));
  }
  const routed = [];
  for (const p of planned) {
    const wp = [p.sAnchor, p.sStub];
    if (p.bend === 'vv') {
      const sameX = Math.abs(p.sStub.x - p.tStub.x) < 0.5;
      if (!sameX) {
        wp.push({ x: p.sStub.x, y: p.midY });
        wp.push({ x: p.tStub.x, y: p.midY });
      }
    } else if (p.bend === 'hh') {
      const sameY = Math.abs(p.sStub.y - p.tStub.y) < 0.5;
      if (!sameY) {
        wp.push({ x: p.midX, y: p.sStub.y });
        wp.push({ x: p.midX, y: p.tStub.y });
      }
    } else {
      const sV = p.sSide === 'top' || p.sSide === 'bottom';
      if (sV) wp.push({ x: p.sStub.x, y: p.tStub.y });
      else    wp.push({ x: p.tStub.x, y: p.sStub.y });
    }
    wp.push(p.tStub);
    wp.push(p.tAnchor);
    routed.push({ edge: p.edge, waypoints: wp, src: p.src, tgt: p.tgt });
  }
  return routed;
}

// Post-process: shift overlapping verticals/horizontals apart by
// TRACK_PITCH. Stub-connected segments (those sharing x with the first
// or last waypoint) are NOT moved — those connect to face anchors.
function separateOverlappingSegments(routed) {
  const TOL = 4;
  const OVERLAP_MIN = 6;
  const collectV = () => {
    const out = [];
    for (const r of routed) {
      const wp = r.waypoints;
      const firstX = wp[0].x, lastX = wp[wp.length - 1].x;
      for (let i = 0; i < wp.length - 1; i++) {
        const a = wp[i], b = wp[i + 1];
        if (Math.abs(a.x - b.x) >= 0.5 || Math.abs(a.y - b.y) <= 1) continue;
        if (Math.abs(a.x - firstX) < 0.5 || Math.abs(a.x - lastX) < 0.5) continue;
        out.push({ r, i, x: a.x, y0: Math.min(a.y, b.y), y1: Math.max(a.y, b.y) });
      }
    }
    return out;
  };
  const collectH = () => {
    const out = [];
    for (const r of routed) {
      const wp = r.waypoints;
      const firstY = wp[0].y, lastY = wp[wp.length - 1].y;
      for (let i = 0; i < wp.length - 1; i++) {
        const a = wp[i], b = wp[i + 1];
        if (Math.abs(a.y - b.y) >= 0.5 || Math.abs(a.x - b.x) <= 1) continue;
        if (Math.abs(a.y - firstY) < 0.5 || Math.abs(a.y - lastY) < 0.5) continue;
        out.push({ r, i, y: a.y, x0: Math.min(a.x, b.x), x1: Math.max(a.x, b.x) });
      }
    }
    return out;
  };
  const spread = (segs, axis) => {
    const co = axis === 'v' ? 'x' : 'y';
    const lo = axis === 'v' ? 'y0' : 'x0';
    const hi = axis === 'v' ? 'y1' : 'x1';
    segs.sort((a, b) => a[co] - b[co]);
    let i = 0;
    while (i < segs.length) {
      let j = i + 1;
      while (j < segs.length && segs[j][co] - segs[i][co] < TOL) j++;
      const group = segs.slice(i, j);
      if (group.length > 1) {
        let conflict = false;
        for (let a = 0; a < group.length && !conflict; a++) {
          for (let b = a + 1; b < group.length && !conflict; b++) {
            const ov = Math.min(group[a][hi], group[b][hi]) -
                       Math.max(group[a][lo], group[b][lo]);
            if (ov > OVERLAP_MIN) conflict = true;
          }
        }
        if (conflict) {
          group.sort((a, b) => a[lo] - b[lo]);
          const center = group.reduce((s, g) => s + g[co], 0) / group.length;
          group.forEach((g, k) => {
            const newCo = center + (k - (group.length - 1) / 2) * TRACK_PITCH;
            g.r.waypoints[g.i][co] = newCo;
            g.r.waypoints[g.i + 1][co] = newCo;
          });
        }
      }
      i = j;
    }
  };
  spread(collectV(), 'v');
  spread(collectH(), 'h');
  spread(collectV(), 'v');
}
