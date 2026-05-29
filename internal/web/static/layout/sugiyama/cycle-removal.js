// Phase 1 — Cycle Removal (Eades-Lin-Smyth 1993).
//
// Input: directed graph (nodes, edges).
// Output: { edgesForLayering, reversed } where reversed is the subset
// of input edges that had to be flipped to break cycles. Downstream
// phases treat them as forward; only the final rendering remembers
// they were reversed so the arrowhead lands on the original target.
//
// Algorithm: produce a vertex ordering π such that |{(u,v): π(u) > π(v)}|
// (the count of "backward" edges) is small. The classical heuristic
// builds π greedily from both ends:
//   sources go to π's front, sinks to π's back, then the highest
//   (out_degree − in_degree) vertex from the rest goes to the front.
// Edges (u,v) with π(u) > π(v) are then reversed.
//
// Complexity: O(|V| + |E|).
//
// Verifiable invariant: the graph with `reversed` edges flipped is a
// DAG (no cycles).

export function cycleRemoval(nodeIDs, edges) {
  const inDeg  = new Map();
  const outDeg = new Map();
  const adjOut = new Map();
  const adjIn  = new Map();
  for (const id of nodeIDs) {
    inDeg.set(id, 0); outDeg.set(id, 0);
    adjOut.set(id, []); adjIn.set(id, []);
  }
  for (const [u, v] of edges) {
    if (u === v) continue; // self-loops carry no cycle info
    if (!inDeg.has(u) || !inDeg.has(v)) continue;
    adjOut.get(u).push(v); adjIn.get(v).push(u);
    outDeg.set(u, outDeg.get(u) + 1);
    inDeg.set(v, inDeg.get(v) + 1);
  }
  const remaining = new Set(nodeIDs);
  const sL = [];
  const sR = [];
  // Track current in/out degrees as we peel vertices off.
  const curIn  = new Map(inDeg);
  const curOut = new Map(outDeg);
  const removeVertex = (v) => {
    remaining.delete(v);
    for (const w of adjOut.get(v) || []) {
      if (remaining.has(w)) curIn.set(w, curIn.get(w) - 1);
    }
    for (const u of adjIn.get(v) || []) {
      if (remaining.has(u)) curOut.set(u, curOut.get(u) - 1);
    }
  };
  while (remaining.size) {
    // Peel sinks (curOut === 0) onto sR.
    let progress = true;
    while (progress) {
      progress = false;
      for (const v of remaining) {
        if (curOut.get(v) === 0) {
          sR.unshift(v); removeVertex(v); progress = true; break;
        }
      }
    }
    if (!remaining.size) break;
    // Peel sources (curIn === 0) onto sL.
    progress = true;
    while (progress) {
      progress = false;
      for (const v of remaining) {
        if (curIn.get(v) === 0) {
          sL.push(v); removeVertex(v); progress = true; break;
        }
      }
    }
    if (!remaining.size) break;
    // Pick the vertex with max (curOut − curIn) and put it in sL.
    let bestV = null, bestScore = -Infinity;
    for (const v of remaining) {
      const score = curOut.get(v) - curIn.get(v);
      if (score > bestScore) { bestScore = score; bestV = v; }
    }
    if (bestV == null) break;
    sL.push(bestV); removeVertex(bestV);
  }
  const order = [...sL, ...sR];
  const pos = new Map();
  order.forEach((id, i) => pos.set(id, i));
  const reversed = new Set();
  for (const [u, v] of edges) {
    if (u === v) continue;
    if (!pos.has(u) || !pos.has(v)) continue;
    if (pos.get(u) > pos.get(v)) reversed.add(u + '>' + v);
  }
  return { order, reversed, position: pos };
}

// Sanity check used in development/tests: verify that with `reversed`
// applied, the graph is a DAG.
export function isDAG(nodeIDs, edges, reversed) {
  const inDeg = new Map();
  const adj   = new Map();
  for (const id of nodeIDs) { inDeg.set(id, 0); adj.set(id, []); }
  for (const [u, v] of edges) {
    if (u === v) continue;
    const isRev = reversed.has(u + '>' + v);
    const a = isRev ? v : u;
    const b = isRev ? u : v;
    if (!inDeg.has(a) || !inDeg.has(b)) continue;
    adj.get(a).push(b);
    inDeg.set(b, inDeg.get(b) + 1);
  }
  const queue = [...nodeIDs].filter(id => inDeg.get(id) === 0);
  let visited = 0;
  while (queue.length) {
    const u = queue.shift(); visited++;
    for (const w of adj.get(u)) {
      inDeg.set(w, inDeg.get(w) - 1);
      if (inDeg.get(w) === 0) queue.push(w);
    }
  }
  return visited === nodeIDs.length;
}
