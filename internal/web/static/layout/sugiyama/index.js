// Faithful Sugiyama compound layout + orthogonal channel routing.
//
// Public API:
//   layout({ nodes, edges, expanded }) → {
//     positions: Map<id, { x, y, w, h }>,
//     routes:    Array<{ edge, waypoints: [{x,y}...] }>,
//     width: number, height: number,
//   }
//
// nodes: [{ id, parentID, w, h, kind, name }]
// edges: [{ id, src, tgt, ... }]
// expanded: Set<id> — container IDs whose children should be laid out
//
// Phase pipeline (each phase has a verifiable invariant):
//   1. Cycle Removal               (Eades-Lin-Smyth 1993)
//   2. Layer Assignment            (longest-path, Gansner candidate)
//   3. Dummy node insertion        (so every edge is 1-layer span)
//   4. Crossing Reduction          (barycenter + median, Eiglsperger)
//   5. X-coordinate Assignment     (Brandes-Köpf simplified)
//   6. Compound recursion          (containers laid out inside-out)
//   7. Orthogonal Routing          (channel + track allocation)
//
// During development each phase exposes a `verify*` function that asserts
// its invariant against the algorithm's input/output. The top-level
// `layout()` runs them in debug mode.

import { cycleRemoval, isDAG } from './cycle-removal.js';
import { assignLayers, verifyLayers, insertDummyNodes } from './layer-assignment.js';
import { reduceCrossings, totalCrossings } from './crossing-reduction.js';
import { assignXCoords, verifyNoOverlap } from './x-coord.js';
import { routeOrthogonal } from './routing.js';

const LEAF_W = 160;
const LEAF_H = 72;
const LABEL_STRIP = 28;
const PAD = 24;
const LAYER_GAP = 96;
const CANVAS_PAD = 24;

export function layout({ nodes, edges, expanded }, opts = {}) {
  const DEBUG = opts.debug === true;
  // Index by id.
  const nodeByID = new Map();
  for (const n of nodes) nodeByID.set(n.id, n);
  // Build wrappers with children pointers.
  const wrappers = new Map();
  for (const n of nodes) {
    wrappers.set(n.id, {
      id: n.id,
      node: n,
      children: [],
      parentID: n.parentID || null,
      w: n.w ?? LEAF_W,
      h: n.h ?? LEAF_H,
      x: 0,
      y: 0,
      _isContainer: false,
      _expanded: false,
    });
  }
  const roots = [];
  for (const w of wrappers.values()) {
    if (w.parentID && wrappers.has(w.parentID)) {
      wrappers.get(w.parentID).children.push(w);
    } else {
      roots.push(w);
    }
  }
  // Recursively lay out each container (bottom-up).
  const layoutContainer = (container, depth) => {
    if (!container.children.length) {
      container.w = LEAF_W;
      container.h = LEAF_H;
      container._isContainer = false;
      return;
    }
    container._isContainer = true;
    container._expanded = expanded?.has(container.id) ?? (depth === 0);
    if (!container._expanded) {
      container.w = LEAF_W;
      container.h = LEAF_H;
      return;
    }
    for (const c of container.children) layoutContainer(c, depth + 1);
    // Sub-layout: cycle removal, layer assignment, etc, on this container's children.
    const ids = container.children.map(c => c.id);
    const internalEdges = [];
    for (const e of edges) {
      if (ids.includes(e.src) && ids.includes(e.tgt)) {
        internalEdges.push([e.src, e.tgt]);
      }
    }
    const { reversed } = cycleRemoval(ids, internalEdges);
    if (DEBUG && !isDAG(ids, internalEdges, reversed)) throw new Error('Cycle removal failed');
    const layer = assignLayers(ids, internalEdges, reversed);
    if (DEBUG && !verifyLayers(ids, internalEdges, reversed, layer)) throw new Error('Layer assignment failed');
    const { nodeIDs: allIDs, edges: layeredEdges } =
      insertDummyNodes(ids, internalEdges, reversed, layer);
    const layerCount = Math.max(0, ...layer.values()) + 1;
    const layers = [];
    for (let i = 0; i < layerCount; i++) layers.push([]);
    for (const id of allIDs) layers[layer.get(id)].push(id);
    const simpleEdges = layeredEdges.map(([u, v]) => [u, v]);
    const { layers: reordered } = reduceCrossings(layers, simpleEdges);
    const nodeWidth = (id) => {
      const w = wrappers.get(id);
      return w ? w.w : 8; // dummy nodes get small width
    };
    const { x } = assignXCoords(reordered, simpleEdges, nodeWidth);
    // Normalise x so the leftmost node's LEFT edge sits at 0; the
    // container's interior padding is added when we place children.
    let minLeft = Infinity;
    for (const id of x.keys()) {
      const wp = wrappers.get(id);
      const w = wp ? wp.w : 8;
      minLeft = Math.min(minLeft, x.get(id) - w / 2);
    }
    if (!Number.isFinite(minLeft)) minLeft = 0;
    // Apply positions. Each layer has its own y based on cumulative
    // height + LAYER_GAP between layers.
    let cy = LABEL_STRIP + PAD;
    for (let li = 0; li < reordered.length; li++) {
      const layer = reordered[li];
      let rowH = 0;
      for (const id of layer) {
        const wp = wrappers.get(id);
        if (!wp) continue; // dummy
        const cx = x.get(id) - minLeft + PAD;
        wp._relX = cx - wp.w / 2;
        wp._relY = cy;
        if (wp.h > rowH) rowH = wp.h;
      }
      cy += rowH + LAYER_GAP;
    }
    // Container w/h from extent.
    let maxRight = 0, maxBottom = LABEL_STRIP + PAD;
    for (const c of container.children) {
      if (c._relX == null) continue;
      maxRight  = Math.max(maxRight,  c._relX + c.w);
      maxBottom = Math.max(maxBottom, c._relY + c.h);
    }
    container.w = maxRight + PAD;
    container.h = maxBottom + PAD;
  };
  for (const r of roots) layoutContainer(r, 0);
  // Now place roots at root level using the same algorithm.
  const rootIDs = roots.map(r => r.id);
  const rootEdges = [];
  for (const e of edges) {
    // Project endpoints to root level.
    const projectToRoot = (id) => {
      let w = wrappers.get(id);
      while (w && w.parentID) w = wrappers.get(w.parentID);
      return w?.id;
    };
    const s = projectToRoot(e.src);
    const t = projectToRoot(e.tgt);
    if (!s || !t || s === t) continue;
    rootEdges.push([s, t]);
  }
  const { reversed: rRev } = cycleRemoval(rootIDs, rootEdges);
  const rLayer = assignLayers(rootIDs, rootEdges, rRev);
  const { nodeIDs: allRoot, edges: rLayered } =
    insertDummyNodes(rootIDs, rootEdges, rRev, rLayer);
  const rLayerCount = Math.max(0, ...rLayer.values()) + 1;
  const rootLayers = [];
  for (let i = 0; i < rLayerCount; i++) rootLayers.push([]);
  for (const id of allRoot) rootLayers[rLayer.get(id)].push(id);
  const rSimple = rLayered.map(([u, v]) => [u, v]);
  const { layers: rootReordered } = reduceCrossings(rootLayers, rSimple);
  const rootNodeW = (id) => wrappers.get(id)?.w ?? 8;
  const { x: rootX } = assignXCoords(rootReordered, rSimple, rootNodeW);
  let rootMinLeft = Infinity;
  for (const id of rootX.keys()) {
    const wp = wrappers.get(id);
    const w = wp ? wp.w : 8;
    rootMinLeft = Math.min(rootMinLeft, rootX.get(id) - w / 2);
  }
  if (!Number.isFinite(rootMinLeft)) rootMinLeft = 0;
  // Apply root positions.
  let rcy = CANVAS_PAD;
  for (let li = 0; li < rootReordered.length; li++) {
    const layer = rootReordered[li];
    let rowH = 0;
    for (const id of layer) {
      const wp = wrappers.get(id);
      if (!wp) continue;
      const cx = rootX.get(id) - rootMinLeft + CANVAS_PAD;
      wp.x = cx - wp.w / 2;
      wp.y = rcy;
      if (wp.h > rowH) rowH = wp.h;
    }
    rcy += rowH + LAYER_GAP;
  }
  // Convert relative positions to absolute (children inside roots).
  const placeChildren = (w) => {
    if (!w.children?.length || !w._expanded) return;
    for (const c of w.children) {
      c.x = w.x + (c._relX ?? 0);
      c.y = w.y + (c._relY ?? 0);
      placeChildren(c);
    }
  };
  for (const r of roots) placeChildren(r);
  // Build positions output.
  const positions = new Map();
  for (const w of wrappers.values()) {
    positions.set(w.id, { x: w.x, y: w.y, w: w.w, h: w.h });
  }
  // Route edges using the orthogonal channel router.
  const flat = [...wrappers.values()].filter(w => w.x != null);
  const nodesForRoute = flat
    .filter(w => !w._isContainer || !w._expanded)
    .map(w => ({ id: w.id, x: w.x, y: w.y, w: w.w, h: w.h }));
  const visibleIDs = new Set(nodesForRoute.map(n => n.id));
  const edgesForRoute = [];
  for (const e of edges) {
    const project = (id) => {
      if (visibleIDs.has(id)) return id;
      let w = wrappers.get(id);
      while (w && w.parentID) {
        if (visibleIDs.has(w.parentID)) return w.parentID;
        w = wrappers.get(w.parentID);
      }
      return null;
    };
    const s = project(e.src);
    const t = project(e.tgt);
    if (!s || !t || s === t) continue;
    edgesForRoute.push({ src: s, tgt: t, _orig: e });
  }
  const routed = routeOrthogonal(nodesForRoute, edgesForRoute);
  const routes = routed.map(r => ({ edge: r.edge._orig, waypoints: r.waypoints }));
  let width = 0, height = 0;
  for (const [, p] of positions) {
    width  = Math.max(width,  p.x + p.w);
    height = Math.max(height, p.y + p.h);
  }
  return {
    positions,
    routes,
    width: width + CANVAS_PAD,
    height: height + CANVAS_PAD,
  };
}
