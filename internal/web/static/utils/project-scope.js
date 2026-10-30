import { subscribe } from '/static/realtime.js';

// Shared by the pages that work on one project or, with no project in
// the route, on every project the viewer belongs to (Live, Questions):
// resolve what the page covers and follow its live events.

// projectError is the message for a project that would not load.
export function projectError(status) {
  if (status === 403) return 'You are not a member of this project.';
  if (status === 404) return 'Project not found.';
  return `Could not load the project (HTTP ${status}).`;
}

// loadScope resolves a route's project, given as id or slug, into
// { project, projects }: the project for a project page, or
// project null and the viewer's projects for a global page (an empty
// list when they cannot be read). Throws an Error carrying the
// message to show when the project does not load.
export async function loadScope(projectId) {
  if (!projectId) {
    let projects = [];
    try {
      const r = await fetch('/api/projects');
      const j = r.ok ? await r.json() : {};
      projects = j.projects || [];
    } catch (_) {
      projects = [];
    }
    return { project: null, projects };
  }
  let r;
  try {
    r = await fetch(`/api/projects/${encodeURIComponent(projectId)}`);
  } catch (_) {
    throw new Error('Could not reach the server.');
  }
  if (!r.ok) throw new Error(projectError(r.status));
  return { project: await r.json(), projects: [] };
}

// subscribeScope follows the live events of the scope's project, or of
// each of its projects, and returns the unsubscribe functions. A
// project the viewer joins later shows up on the next visit.
export function subscribeScope(scope, onEvent) {
  const ids = scope.project ? [scope.project.id] : scope.projects.map((p) => p.id);
  return ids.map((id) => subscribe(id, onEvent));
}
