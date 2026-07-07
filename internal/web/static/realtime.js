// Real-time event subscription helper.
//
// Opens an EventSource against /events scoped to a project, parses
// every notification as JSON and dispatches it to the caller's
// callback. Reconnection is automatic (built into EventSource).
//
// Usage from a Lit component:
//
//   import { subscribe } from '/static/realtime.js';
//   connectedCallback() {
//     super.connectedCallback();
//     this._unsub = subscribe(this.projectId, (ev) => {
//       if (ev.type.startsWith('task.')) this.refresh();
//     });
//   }
//   disconnectedCallback() {
//     super.disconnectedCallback();
//     this._unsub?.();
//   }

// Pass an empty/null projectId to open a GLOBAL subscription — used
// by shell-level components (update banner) that care about cross-
// project state (version_status advisories) with no project scope.
// The backend requires auth but skips the project-membership check.
export function subscribe(projectId, onEvent) {
  const url = projectId ? `/events?project_id=${encodeURIComponent(projectId)}` : `/events`;
  const es = new EventSource(url);
  let opened = false;
  es.onopen = () => {
    // EventSource auto-reconnects after a transient disconnect (network
    // blip, container rebuild). Events that fired during the gap are
    // lost — the server doesn't replay them. Surface a synthetic
    // 'realtime.reconnected' on every open after the first so consumers
    // can do a full reload and catch up.
    if (opened) onEvent({ type: 'realtime.reconnected' });
    opened = true;
  };
  es.onmessage = (e) => {
    if (!e.data) return;
    try {
      const ev = JSON.parse(e.data);
      onEvent(ev);
    } catch (_) {
      // Ignore malformed payloads (keep-alive comments fall here
      // silently since they aren't 'data:' lines).
    }
  };
  // Silently swallow errors — EventSource will auto-reconnect.
  es.onerror = () => {};
  return () => es.close();
}
