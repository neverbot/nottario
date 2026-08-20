// Time formatting shared by every page and component.
//
// Before this module the frontend carried six copy-pasted relative-time
// helpers under three names, and they disagreed: the same two-hour-old
// timestamp rendered as "2h ago" in most places and "2 hours ago" in
// project settings, and a forty-day-old one as "5w ago" on the projects
// list but a bare locale date on the board. One implementation, one
// output.

// formatRelativeTime renders an ISO timestamp as a short, compact
// "time since" string: "just now", "5m ago", "3h ago", "2d ago",
// "6w ago", falling back to a locale date past twelve weeks.
//
// The weeks step matters: without it, anything older than a month
// collapses straight to an absolute date, which reads as a much bigger
// jump in specificity than the age difference warrants.
//
// Returns '' for a missing or unparseable input, so callers that want
// a placeholder ("—") supply their own.
export function formatRelativeTime(iso) {
  if (!iso) return '';
  const then = new Date(iso).getTime();
  if (!Number.isFinite(then)) return '';
  const diff = Date.now() - then;
  if (diff < 60_000) return 'just now';
  const minutes = Math.floor(diff / 60_000);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 7) return `${days}d ago`;
  const weeks = Math.floor(days / 7);
  if (weeks < 12) return `${weeks}w ago`;
  return new Date(iso).toLocaleDateString();
}
