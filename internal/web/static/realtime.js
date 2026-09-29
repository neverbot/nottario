// Real-time event subscription helper.
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
//
// Pass an empty/null projectId for a GLOBAL subscription (the update
// banner): it receives instance-wide events only.
//
// Every subscription in the browser shares one connection. Over plain
// http a browser allows 6 connections per origin across all tabs, and
// an event stream holds one for as long as the page is open, so a
// stream per subscriber ran the pool dry with three tabs and froze
// every other request. A SharedWorker keeps one stream for all tabs;
// where SharedWorker is missing (Chrome on Android) each tab keeps one
// of its own. Either way callers see the same API, and a synthetic
// { type: 'realtime.reconnected' } after the stream recovers from a
// drop, so they can reload what they missed.
import { Multiplexer } from '/static/realtime-core.js';

// Bumped whenever the worker protocol changes: a SharedWorker outlives
// deploys while any tab keeps it alive, and a new URL starts a new one
// rather than talking to an old worker.
const WORKER_URL = '/static/realtime-worker.js?v=4';

let nextId = 0;
let transport = null;

function sharedTransport() {
  const worker = new SharedWorker(WORKER_URL, { type: 'module', name: 'nottario-realtime' });
  const port = worker.port;
  const handlers = new Map();
  port.onmessage = (msg) => {
    const { id, event } = msg.data || {};
    handlers.get(id)?.(event);
  };
  port.start();
  // Tell the worker when the tab goes away, so it can stop following
  // projects nobody is looking at any more.
  addEventListener('pagehide', () => port.postMessage({ op: 'bye' }));
  return {
    subscribe(id, projectId, onEvent) {
      handlers.set(id, onEvent);
      port.postMessage({ op: 'sub', id, projectId: projectId || null });
    },
    unsubscribe(id) {
      handlers.delete(id);
      port.postMessage({ op: 'unsub', id });
    },
  };
}

function localTransport() {
  const mux = new Multiplexer((url) => new EventSource(url));
  return {
    subscribe: (id, projectId, onEvent) => mux.subscribe(id, projectId, onEvent),
    unsubscribe: (id) => mux.unsubscribe(id),
  };
}

function getTransport() {
  if (transport) return transport;
  try {
    transport = typeof SharedWorker === 'function' ? sharedTransport() : localTransport();
  } catch (_) {
    // A browser that has SharedWorker but refuses a module worker.
    transport = localTransport();
  }
  return transport;
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const resolved = new Map(); // slug -> Promise<uuid|null>

// Pages pass the project segment of their URL, which can be a slug.
// Streams and events speak uuids: the server rejects a slug in
// ?project_id, and events are routed to subscribers by comparing
// project ids, so a slug subscription would never receive anything.
// Resolve once per slug and remember it.
function resolveProjectId(projectId) {
  if (!projectId || UUID_RE.test(projectId)) return Promise.resolve(projectId || null);
  if (!resolved.has(projectId)) {
    resolved.set(
      projectId,
      fetch(`/api/projects/${encodeURIComponent(projectId)}`)
        .then((r) => (r.ok ? r.json() : null))
        .then((p) => p?.id || null)
        .catch(() => null),
    );
  }
  return resolved.get(projectId);
}

export function subscribe(projectId, onEvent) {
  const id = nextId++;
  const t = getTransport();
  let active = false;
  let cancelled = false;
  resolveProjectId(projectId).then((uuid) => {
    // Unsubscribed while the slug was being resolved, or a slug that
    // does not resolve (deleted project): nothing to follow.
    if (cancelled || (projectId && !uuid)) return;
    t.subscribe(id, uuid, onEvent);
    active = true;
  });
  return () => {
    cancelled = true;
    if (active) t.unsubscribe(id);
  };
}
