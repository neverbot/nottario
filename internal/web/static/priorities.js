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
