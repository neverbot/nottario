// Priority helpers shared by the views that render task priorities.
//
// Priorities are stored as a raw 0-100 integer, but every project also
// defines named buckets (low / medium / high by default, editable in
// project settings). The UI shows the bucket key when the stored value
// lands exactly on one.

// priorityLabel maps a raw priority value to its bucket key, falling
// back to `p<value>` when no bucket matches. A value can miss every
// bucket legitimately: someone set a raw integer through the API, or
// the project's buckets were edited after the task was created.
//
// `buckets` is the project's priority list as returned by
// `/api/projects/:id/priorities`; a missing or empty list just means
// every value falls back.
export function priorityLabel(value, buckets) {
  const match = buckets?.find((p) => p.value === value);
  return match ? match.key : `p${value}`;
}

// priorityBand ranks a value against the project's own catalogue and
// returns one of 'low' / 'medium' / 'high', the three tints the card's
// coloured dot is painted with.
//
// The ranking is relative, not absolute: the lowest bucket anchors 0
// and the highest anchors 1, and the span between them is cut in
// thirds. That keeps the colours meaningful for a project whose
// buckets are 200/500/800 just as much as for the seeded 30/60/90/100,
// which fixed cutoffs could not. Values that miss every bucket (or sit
// past both ends) land in the band their number falls into.
export function priorityBand(value, buckets) {
  const values = (buckets ?? []).map((p) => p.value);
  // With no catalogue loaded yet, fall back to the nominal 0-100
  // scale so the first paint is not uniformly grey.
  const min = values.length ? Math.min(...values) : 0;
  const max = values.length ? Math.max(...values) : 100;
  // One bucket, or a catalogue where every bucket shares a value,
  // offers no spread to rank against.
  if (max === min) return 'medium';
  const ratio = (value - min) / (max - min);
  if (ratio >= 2 / 3) return 'high';
  if (ratio >= 1 / 3) return 'medium';
  return 'low';
}
