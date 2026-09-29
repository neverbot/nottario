// Realtime multiplexing, shared by the SharedWorker and the in-page
// fallback. No DOM, no globals: the EventSource constructor is passed
// in, so the logic runs (and is tested) under plain Node.
//
// Why it exists: over plain http a browser allows 6 connections per
// origin, shared by every tab, and each EventSource holds one forever.
// Two per tab meant three tabs left no connection for anything else,
// and every other request — API calls, markdown renders, even a new
// tab's HTML — waited in the browser's queue indefinitely. The
// multiplexer keeps ONE stream per browser (per tab where SharedWorker
// is unavailable) carrying every project any subscriber follows.

// streamURL returns the /events URL for a set of project ids. Global
// events reach every stream, so a stream with projects also serves the
// subscribers that only want global ones.
export function streamURL(projectIds) {
  const ids = [...new Set([...projectIds].filter(Boolean))].sort();
  if (ids.length === 0) return '/events';
  return `/events?${ids.map((id) => `project_id=${encodeURIComponent(id)}`).join('&')}`;
}

// wants reports whether a subscriber should receive an event. Events
// without a project (instance-wide notices, the synthetic
// realtime.reconnected) go to everyone; project events only to the
// subscribers following that project.
export function wants(subscription, event) {
  if (!event.project_id) return true;
  return subscription.projectId === event.project_id;
}

// Multiplexer owns the single physical stream.
//
// subscribe()/unsubscribe() change the set of projects; when the set
// changes the stream is replaced make-before-break: the new one is
// opened first and only becomes active once it has connected, so no
// event is lost in the switch and none is delivered twice.
// EventSource retries on its own after a network drop, but NOT after
// an HTTP error response (a 401 before signing in, a 502 from a proxy
// mid-deploy): it closes for good. In a page that was survivable,
// because the next load opened a fresh stream. A shared worker
// outlives page loads, so a stream that died once stayed dead for
// every tab until all of them were closed. These bound the retry.
const RETRY_MIN_MS = 1000;
const RETRY_MAX_MS = 30000;
const CLOSED = 2; // EventSource.CLOSED

export class Multiplexer {
  constructor(openStream, schedule = (fn, ms) => setTimeout(fn, ms)) {
    this._open = openStream; // (url) => EventSource-like
    this._schedule = schedule;
    this._subs = new Map(); // id -> { projectId, deliver }
    this._active = null; // { es, url }
    this._pending = null; // { es, url }
    this._retryMs = RETRY_MIN_MS;
    this._missed = false; // a stream died: tell everyone once one is back
  }

  subscribe(id, projectId, deliver) {
    this._subs.set(id, { projectId: projectId || null, deliver });
    this._reconcile();
  }

  unsubscribe(id) {
    if (!this._subs.delete(id)) return;
    this._reconcile();
  }

  // Drop every subscription whose id satisfies pred — used when a tab
  // goes away and takes all of its subscriptions with it.
  unsubscribeWhere(pred) {
    let changed = false;
    for (const id of [...this._subs.keys()]) {
      if (pred(id)) {
        this._subs.delete(id);
        changed = true;
      }
    }
    if (changed) this._reconcile();
  }

  get url() {
    return this._active?.url ?? null;
  }

  // What the multiplexer is doing right now, for diagnosing a stream
  // that is not delivering: from the console, post { op: 'state' } to
  // the worker (see realtime-worker.js).
  state() {
    return {
      subscriptions: [...this._subs.entries()].map(([id, s]) => ({ id, projectId: s.projectId })),
      active: this._active
        ? { url: this._active.url, readyState: this._active.es.readyState }
        : null,
      pending: this._pending
        ? { url: this._pending.url, readyState: this._pending.es.readyState }
        : null,
      retryMs: this._retryMs,
    };
  }

  get size() {
    return this._subs.size;
  }

  _wantedURL() {
    if (this._subs.size === 0) return null;
    return streamURL([...this._subs.values()].map((s) => s.projectId));
  }

  _reconcile() {
    const url = this._wantedURL();
    if (url === null) {
      this._close(this._pending);
      this._close(this._active);
      this._pending = null;
      this._active = null;
      return;
    }
    if (this._active?.url === url) {
      // Back to what is already live (a project added and removed
      // before its stream connected): drop the half-open replacement.
      this._close(this._pending);
      this._pending = null;
      return;
    }
    if (this._pending?.url === url) return;
    this._close(this._pending);
    this._pending = this._connect(url);
  }

  _connect(url) {
    const es = this._open(url);
    const conn = { es, url, opened: false };
    es.onopen = () => {
      if (!conn.opened) {
        conn.opened = true;
        this._retryMs = RETRY_MIN_MS;
        if (this._pending === conn) {
          // The replacement is live: switch to it, drop the old one.
          const previous = this._active;
          this._active = conn;
          this._pending = null;
          this._close(previous);
        }
        if (this._missed && this._active === conn) {
          this._missed = false;
          this._broadcast({ type: 'realtime.reconnected' });
        }
        return;
      }
      // A second open is EventSource recovering from a drop. Events
      // sent during the gap are gone, so tell everyone to reload.
      if (this._active === conn) this._broadcast({ type: 'realtime.reconnected' });
    };
    es.onmessage = (e) => {
      if (this._active !== conn || !e.data) return;
      let event;
      try {
        event = JSON.parse(e.data);
      } catch (_) {
        return;
      }
      this._broadcast(event);
    };
    es.onerror = () => {
      // A transient drop: EventSource reconnects by itself and onopen
      // reports it. Only a stream the browser gave up on needs us.
      if (es.readyState !== CLOSED) return;
      this._retry(conn);
    };
    return conn;
  }

  _retry(conn) {
    if (this._active === conn) {
      this._active = null;
      this._missed = true; // events may have been sent while it was down
    } else if (this._pending === conn) {
      this._pending = null;
    } else {
      return; // already replaced
    }
    this._close(conn);
    const wait = this._retryMs;
    this._retryMs = Math.min(this._retryMs * 2, RETRY_MAX_MS);
    this._schedule(() => this._reconcile(), wait);
  }

  _broadcast(event) {
    for (const sub of this._subs.values()) {
      if (wants(sub, event)) sub.deliver(event);
    }
  }

  _close(conn) {
    if (!conn) return;
    conn.es.onopen = null;
    conn.es.onmessage = null;
    conn.es.close();
  }
}
