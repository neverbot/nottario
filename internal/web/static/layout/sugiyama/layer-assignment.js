// Phase 2 — Layer Assignment.
//
// Input: DAG (use the reversed-edges set from cycle-removal). Output:
// `layer[v]` = integer ≥ 0 such that for every edge (u,v): layer(u) < layer(v).
//
// We implement the simple longest-path algorithm: layer(v) = the longest
// directed path ending at v. It guarantees the invariant trivially and
// produces compact layerings for trees and DAGs of modest depth.
//
// For very wide graphs the network-simplex algorithm (Gansner et al. 1993)
// minimises the sum of edge lengths (Σ layer(v)-layer(u)) further; we can
// drop it in later without changing the rest of the pipeline since the
// downstream phases only see `layer[v]`.
//
// Complexity: O(|V| + |E|).
//
// Verifiable invariant: ∀ edge (u,v): layer[u] < layer[v].

export function assignLayers(nodeIDs, edges, reversed) {
  const adj = new Map();
  const inDeg = new Map();
  for (const id of nodeIDs) { adj.set(id, []); inDeg.set(id, 0); }
  for (const [u, v] of edges) {
    if (u === v) continue;
    if (!adj.has(u) || !adj.has(v)) continue;
    const isRev = reversed.has(u + '>' + v);
    const a = isRev ? v : u;
    const b = isRev ? u : v;
    adj.get(a).push(b);
    inDeg.set(b, inDeg.get(b) + 1);
  }
  // Topological order via Kahn.
  const order = [];
  const queue = [...nodeIDs].filter(id => inDeg.get(id) === 0);
  const inDegCopy = new Map(inDeg);
  while (queue.length) {
    const u = queue.shift();
    order.push(u);
    for (const w of adj.get(u)) {
      inDegCopy.set(w, inDegCopy.get(w) - 1);
      if (inDegCopy.get(w) === 0) queue.push(w);
    }
  }
  // Compute layer = longest path FROM any source to each node.
  const layer = new Map();
  for (const id of nodeIDs) layer.set(id, 0);
  for (const u of order) {
    const lu = layer.get(u);
    for (const v of adj.get(u)) {
      if (layer.get(v) < lu + 1) layer.set(v, lu + 1);
    }
  }
  return layer;
}

// Sanity check.
export function verifyLayers(nodeIDs, edges, reversed, layer) {
  for (const [u, v] of edges) {
    if (u === v) continue;
    if (!layer.has(u) || !layer.has(v)) continue;
    const isRev = reversed.has(u + '>' + v);
    const a = isRev ? v : u;
    const b = isRev ? u : v;
    if (layer.get(a) >= layer.get(b)) return false;
  }
  return true;
}

// Insert dummy nodes on every edge whose endpoints span more than one
// layer. Necessary for Sugiyama's crossing reduction and x-coord
// assignment: those phases assume each edge connects ADJACENT layers.
// Dummy IDs are synthesised as `__dummy__${edge}__${k}`.
export function insertDummyNodes(nodeIDs, edges, reversed, layer) {
  const newNodes = [...nodeIDs];
  const newEdges = []; // [u, v, origEdge|null, isRev]
  let dummyCounter = 0;
  for (const [u, v] of edges) {
    if (u === v) continue;
    if (!layer.has(u) || !layer.has(v)) continue;
    const isRev = reversed.has(u + '>' + v);
    const a = isRev ? v : u;
    const b = isRev ? u : v;
    const la = layer.get(a);
    const lb = layer.get(b);
    if (lb - la === 1) {
      newEdges.push([a, b, [u, v], isRev]);
      continue;
    }
    // Span > 1: chain a → d_1 → d_2 → ... → b.
    let prev = a;
    for (let k = la + 1; k < lb; k++) {
      const d = `__dummy__${dummyCounter++}`;
      newNodes.push(d);
      layer.set(d, k);
      newEdges.push([prev, d, [u, v], isRev]);
      prev = d;
    }
    newEdges.push([prev, b, [u, v], isRev]);
  }
  return { nodeIDs: newNodes, edges: newEdges };
}
