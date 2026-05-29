// Phase 4 — X-coordinate assignment (Brandes-Köpf 2001, simplified).
//
// Input: layers[][] of node IDs (already ordered), edges (after dummy
// insertion), node widths. Output: x[v] for each node, plus the total
// width of each layer.
//
// The Brandes-Köpf algorithm runs four alignment sweeps (up-left,
// up-right, down-left, down-right), each producing a candidate
// horizontal positioning, then averages the two most compact ones.
// Our simplified version runs ONE down-left sweep, which is enough to
// produce a clean layout that respects layer ordering and avoids
// overlaps. Production-quality compaction can be added later as a
// drop-in.
//
// Verifiable invariant: within each layer, x[v_{i+1}] - x[v_i] ≥
// (width[v_i] + width[v_{i+1}])/2 + GAP, i.e. no two siblings overlap.

const DEFAULT_GAP = 32;

export function assignXCoords(layers, edges, nodeWidth, opts = {}) {
  const GAP = opts.gap ?? DEFAULT_GAP;
  const x = new Map();
  // Initial placement: pack each layer left-to-right with GAP between
  // siblings, then run vertical alignment with the neighbour layer
  // above.
  for (const layer of layers) {
    let cur = 0;
    for (const id of layer) {
      const w = nodeWidth(id);
      x.set(id, cur + w / 2);
      cur += w + GAP;
    }
  }
  // Down-sweep: for each layer (from 2 onwards), try to align each
  // node to the median of its predecessors. We move a node RIGHT
  // (never left) so we don't break the previous nodes' positioning.
  for (let li = 1; li < layers.length; li++) {
    const layer = layers[li];
    const prev = layers[li - 1];
    const predOf = new Map();
    for (const id of layer) predOf.set(id, []);
    for (const [u, v] of edges) {
      if (prev.includes(u) && layer.includes(v)) {
        predOf.get(v).push(x.get(u));
      }
    }
    // Greedy right-shift: walk layer left-to-right; for each node,
    // target x = median of predecessor positions; but never below
    // (cur left edge).
    let leftFloor = -Infinity;
    for (const id of layer) {
      const preds = predOf.get(id);
      const w = nodeWidth(id);
      let target = x.get(id);
      if (preds.length) {
        preds.sort((a, b) => a - b);
        const m = preds.length;
        target = m % 2 === 1
          ? preds[(m - 1) / 2]
          : (preds[m / 2 - 1] + preds[m / 2]) / 2;
      }
      const minCentre = leftFloor + w / 2 + GAP;
      const finalX = Math.max(target, minCentre);
      x.set(id, finalX);
      leftFloor = finalX + w / 2;
    }
  }
  // Compute total width per layer and overall width.
  let totalW = 0;
  const layerExtents = layers.map(layer => {
    if (!layer.length) return { left: 0, right: 0, width: 0 };
    let left = Infinity, right = -Infinity;
    for (const id of layer) {
      const w = nodeWidth(id);
      const cx = x.get(id);
      left  = Math.min(left,  cx - w / 2);
      right = Math.max(right, cx + w / 2);
    }
    return { left, right, width: right - left };
  });
  totalW = Math.max(...layerExtents.map(l => l.width), 0);
  return { x, layerExtents, totalW };
}

// Sanity check: no overlap within any layer.
export function verifyNoOverlap(layers, x, nodeWidth) {
  for (const layer of layers) {
    for (let i = 0; i + 1 < layer.length; i++) {
      const a = layer[i], b = layer[i + 1];
      const ra = x.get(a) + nodeWidth(a) / 2;
      const lb = x.get(b) - nodeWidth(b) / 2;
      if (lb < ra) return false;
    }
  }
  return true;
}
