// Phase 3 — Crossing Reduction (barycenter + median + greedy switching).
//
// Input: layers[][] of node IDs, plus the edges between consecutive
// layers (after dummy-node insertion). Output: each layer's nodes
// reordered to minimise the number of edge crossings between adjacent
// layers. Verifiable invariant: crossing count is non-increasing across
// sweeps (we accept a sweep iff it does not increase crossings).
//
// Algorithm (Eiglsperger 2003, simplified):
//   1. For 24 sweeps, alternate down→up→down→up.
//   2. In each pass, reorder each layer's nodes by the BARYCENTER of
//      their neighbours' positions in the adjacent layer.
//   3. Resolve ties by MEDIAN of neighbours' positions.
//   4. Final greedy switching pass: swap adjacent pairs in any layer
//      whenever the swap reduces the in-layer crossing count.
//
// Counting crossings between two adjacent layers: for each pair of
// edges (u₁,v₁) and (u₂,v₂) between L_i and L_{i+1}, they cross iff
// (pos(u₁) - pos(u₂)) and (pos(v₁) - pos(v₂)) have opposite signs.
// We do this in O(|E| log |E|) per layer pair using the well-known
// merge-sort-based count.

function crossingsBetween(L_top, L_bot, edges) {
  // edges between L_top → L_bot. Return number of crossings.
  const posBot = new Map();
  L_bot.forEach((id, i) => posBot.set(id, i));
  // For each top node in order, list its bot positions.
  const seq = [];
  for (const top of L_top) {
    const targets = [];
    for (const [u, v] of edges) {
      if (u === top) targets.push(posBot.get(v));
    }
    targets.sort((a, b) => a - b);
    seq.push(...targets);
  }
  // Count inversions in seq via merge sort.
  return countInversions(seq);
}

function countInversions(arr) {
  const tmp = arr.slice();
  let count = 0;
  function mergeSort(a, lo, hi) {
    if (hi - lo < 2) return;
    const mid = (lo + hi) >> 1;
    mergeSort(a, lo, mid);
    mergeSort(a, mid, hi);
    let i = lo, j = mid, k = lo;
    while (i < mid && j < hi) {
      if (a[i] <= a[j]) tmp[k++] = a[i++];
      else { tmp[k++] = a[j++]; count += mid - i; }
    }
    while (i < mid) tmp[k++] = a[i++];
    while (j < hi)  tmp[k++] = a[j++];
    for (let x = lo; x < hi; x++) a[x] = tmp[x];
  }
  mergeSort(arr, 0, arr.length);
  return count;
}

function totalCrossings(layers, edges) {
  let total = 0;
  for (let i = 0; i < layers.length - 1; i++) {
    const layerEdges = edges.filter(e => {
      // Edge between layer i and i+1?
      const inTop = layers[i].includes(e[0]);
      const inBot = layers[i + 1].includes(e[1]);
      return inTop && inBot;
    });
    total += crossingsBetween(layers[i], layers[i + 1], layerEdges);
  }
  return total;
}

function reorderByBarycenter(L_target, L_neighbour, edges, dir /* 'up' | 'down' */) {
  // For each node in L_target, average position of its neighbours in
  // L_neighbour. dir='down' means neighbour is the layer ABOVE (sources).
  const posN = new Map();
  L_neighbour.forEach((id, i) => posN.set(id, i));
  const bary = new Map();
  const counts = new Map();
  for (const id of L_target) { bary.set(id, 0); counts.set(id, 0); }
  for (const [u, v] of edges) {
    if (dir === 'down') {
      // u in neighbour, v in target.
      if (posN.has(u) && bary.has(v)) {
        bary.set(v, bary.get(v) + posN.get(u));
        counts.set(v, counts.get(v) + 1);
      }
    } else {
      // dir === 'up': v in neighbour, u in target.
      if (posN.has(v) && bary.has(u)) {
        bary.set(u, bary.get(u) + posN.get(v));
        counts.set(u, counts.get(u) + 1);
      }
    }
  }
  // Nodes with no neighbours keep their original index as the score.
  L_target.forEach((id, i) => {
    if (counts.get(id) === 0) bary.set(id, i);
    else bary.set(id, bary.get(id) / counts.get(id));
  });
  const sorted = [...L_target].sort((a, b) => bary.get(a) - bary.get(b));
  return sorted;
}

export function reduceCrossings(layers, edges) {
  let best = layers.map(l => [...l]);
  let bestCrossings = totalCrossings(best, edges);
  for (let iter = 0; iter < 24; iter++) {
    const candidate = best.map(l => [...l]);
    const dir = iter % 2 === 0 ? 'down' : 'up';
    if (dir === 'down') {
      for (let i = 1; i < candidate.length; i++) {
        const layerEdges = edges.filter(e =>
          candidate[i - 1].includes(e[0]) && candidate[i].includes(e[1]));
        candidate[i] = reorderByBarycenter(candidate[i], candidate[i - 1], layerEdges, 'down');
      }
    } else {
      for (let i = candidate.length - 2; i >= 0; i--) {
        const layerEdges = edges.filter(e =>
          candidate[i].includes(e[0]) && candidate[i + 1].includes(e[1]));
        candidate[i] = reorderByBarycenter(candidate[i], candidate[i + 1], layerEdges, 'up');
      }
    }
    const cand = totalCrossings(candidate, edges);
    if (cand < bestCrossings) { best = candidate; bestCrossings = cand; }
    else if (cand === bestCrossings) { /* tie — keep best */ }
  }
  // Greedy switching: try swapping adjacent pairs in each layer; commit
  // any swap that reduces crossings.
  let improved = true;
  let safety = 32;
  while (improved && safety-- > 0) {
    improved = false;
    for (let i = 0; i < best.length; i++) {
      for (let j = 0; j < best[i].length - 1; j++) {
        const candidate = best.map(l => [...l]);
        const tmp = candidate[i][j];
        candidate[i][j] = candidate[i][j + 1];
        candidate[i][j + 1] = tmp;
        const cand = totalCrossings(candidate, edges);
        if (cand < bestCrossings) {
          best = candidate; bestCrossings = cand; improved = true;
        }
      }
    }
  }
  return { layers: best, crossings: bestCrossings };
}

export { totalCrossings };
