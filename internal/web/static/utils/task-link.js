// Task links that open the task dialog over whatever page is showing.
//
// The task dialog is driven by the URL hash on any page:
//
//   /projects/{pid}/<anything>#task=<taskId>
//       the task belongs to the project of the current route.
//   /<anything>#task=<taskId>&project=<pid>
//       global pages (/live, /questions, ...), or a task of another
//       project than the route's.
//
// Other hash keys a page keeps for itself (the board's cycle and
// filters, the docs path, ...) are preserved next to task/project, so
// opening and closing a task leaves the page's own state as it was.
// Hashes that are not key=value pairs (the questions page's #q-<id>)
// are dropped: they are one-shot scroll targets, not state.
//
// The pure functions take the location parts as arguments so they can
// be unit-tested in Node; taskHref and openTask read window.location.

// projectFromPath returns the project id of a /projects/{pid}/... route,
// or null on global pages.
export function projectFromPath(pathname) {
  const m = /^\/projects\/([^/#?]+)(?:[/?#]|$)/.exec(pathname || '');
  return m ? decodeURIComponent(m[1]) : null;
}

// keptParams is the page's own hash state: every key=value pair except
// task and project.
function keptParams(hash) {
  const src = new URLSearchParams((hash || '').replace(/^#/, ''));
  const out = new URLSearchParams();
  for (const [k, v] of src) {
    if (v === '' || k === 'task' || k === 'project') continue;
    out.append(k, v);
  }
  return out;
}

// taskHash is the hash that opens taskId on the page at pathname,
// keeping that page's own hash state.
export function taskHash(taskId, projectId, { pathname = '', hash = '' } = {}) {
  const p = keptParams(hash);
  p.set('task', taskId);
  if (projectId && projectFromPath(pathname) !== projectId) p.set('project', projectId);
  return `#${p.toString()}`;
}

// buildTaskHref is the full in-app href that opens taskId without
// leaving the page at loc.
export function buildTaskHref(taskId, projectId, loc = {}) {
  return (loc.pathname || '/') + (loc.search || '') + taskHash(taskId, projectId, loc);
}

// hashWithoutTask is the hash with the dialog's keys removed ('' when
// nothing else is left).
export function hashWithoutTask(hash) {
  const s = keptParams(hash).toString();
  return s ? `#${s}` : '';
}

// Task and project ids are UUIDs. Anything else in a hash (a typo, a
// crafted "../../me") is ignored rather than spliced into an API path.
const ID_RE = /^[0-9a-f-]{36}$/i;

// parseTaskHash reads the open task from a location: { taskId,
// projectId } or null when the hash names no task, no project can be
// resolved for it, or either id is not UUID-shaped.
export function parseTaskHash(pathname, hash) {
  const p = new URLSearchParams((hash || '').replace(/^#/, ''));
  const taskId = p.get('task');
  if (!taskId || !ID_RE.test(taskId)) return null;
  const projectId = p.get('project') || projectFromPath(pathname);
  if (!projectId || !ID_RE.test(projectId)) return null;
  return { taskId, projectId };
}

// parseTaskURL recognises a task link in any href (relative or
// absolute, the helper's form or the older /board/kanban#task= one
// the server still renders in markdown chips). Returns { taskId,
// projectId } for a same-origin link that names a task, else null.
export function parseTaskURL(href, origin) {
  if (!href) return null;
  let u;
  try {
    u = new URL(href, origin);
  } catch (_) {
    return null;
  }
  if (u.origin !== new URL(origin).origin) return null;
  return parseTaskHash(u.pathname, u.hash);
}

// taskHref builds the href for a task from wherever the caller is
// rendered. Clicks on it are intercepted by the app shell, so the
// dialog opens in place; a middle click or "open in new tab" still
// gets a real URL that opens the same page with the dialog.
export function taskHref(taskId, projectId) {
  return buildTaskHref(taskId, projectId, window.location);
}

// openTask opens the task dialog over the current page. The app shell
// owns the history entry.
export function openTask(taskId, projectId) {
  window.dispatchEvent(new CustomEvent('nottario-open-task', { detail: { taskId, projectId } }));
}
