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

export function subscribe(projectId, onEvent) {
  if (!projectId) return () => {};
  const url = `/events?project_id=${encodeURIComponent(projectId)}`;
  const es = new EventSource(url);
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
