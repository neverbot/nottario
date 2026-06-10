// Canonical registry of every per-project view the UI knows about.
// One source of truth for route construction, navigation menus and the
// "default view" selector in project settings. Anything that
// hardcodes a view path is a candidate to migrate here.
//
// Entry shape:
//   - key:        the value stored in `projects.default_view` and used
//                 as an identifier across the codebase.
//   - label:      short human-readable label shown in nav controls.
//   - description:longer string used in tooltips and the settings select.
//   - path(pid):  function that returns the URL for a given project id.
//
// The server-side allowlist mirrors this in
// internal/identity/projects.go (ValidDefaultViews) and the CHECK
// constraint on projects.default_view. Keep all three in sync when
// adding/removing a view.

export const PROJECT_VIEWS = [
  {
    key: 'board/kanban',
    label: 'Kanban',
    description: 'Cards grouped by state — todo, doing, done.',
    path: (pid) => `/projects/${pid}/board/kanban`,
  },
  {
    key: 'board/gantt',
    label: 'Gantt',
    description: 'Time-ordered chart with priority sub-columns and dependencies.',
    path: (pid) => `/projects/${pid}/board/gantt`,
  },
  {
    key: 'docs',
    label: 'Docs',
    description: 'Shared markdown context: skills, decisions, notes.',
    path: (pid) => `/projects/${pid}/docs`,
  },
  {
    key: 'arch/diagram',
    label: 'Architecture',
    description: 'Diagram of expandable boxes and arrows.',
    path: (pid) => `/projects/${pid}/arch/diagram`,
  },
  {
    key: 'arch/tree',
    label: 'Architecture (tree)',
    description: 'Sidebar tree variant of the architecture view.',
    path: (pid) => `/projects/${pid}/arch/tree`,
  },
];

// viewByKey returns the registry entry for a given key, or the default
// entry when the key is unknown (defensive: a project row could carry
// a stale key after a registry rename).
export function viewByKey(key) {
  return PROJECT_VIEWS.find((v) => v.key === key) || PROJECT_VIEWS[0];
}

// defaultPathFor returns the URL of a project's chosen default view,
// falling back to the first registry entry if the project carries an
// unknown key or none at all.
export function defaultPathFor(project) {
  if (!project) return '/';
  const v = viewByKey(project.DefaultView || 'board/kanban');
  return v.path(project.ID);
}
