import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { popoverStyles } from '/static/components/surfaces.js';
import '/static/components/avatar.js';

// <nottario-notifications-bell .me=${me}>
//
// Bell icon in the topbar with an unread-dot indicator. Opens a
// popover drawer listing the caller's most recent 20 notifications.
// Hydrates task/actor payloads server-side so each row only knows the
// data it needs.
//
// Realtime is polled, not pushed: the bell refetches its unread count
// on mount, on tab focus (`visibilitychange`), and after client-side
// navigation (`nottario-navigate`). That covers the common paths — a
// user coming back to the tab, moving between pages, or an actor's
// own click triggering a state change — without wiring an SSE
// subscription for a per-user resource. A future iteration can
// upgrade to push if the polling misses feel too coarse.
//
// Kill switch: when `/api/notifications/unread_count` responds with
// `disabled: true` the bell hides itself entirely. Nothing renders,
// no periodic re-fetching, no DOM footprint.
class NottarioNotificationsBell extends LitElement {
  static properties = {
    me: { type: Object },
    _unread: { state: true },
    _open: { state: true },
    _items: { state: true },
    _loading: { state: true },
    _nextAfter: { state: true },
    _disabled: { state: true },
  };

  static styles = [
    popoverStyles,
    css`
    :host { display: inline-block; box-sizing: border-box; position: relative; }
    * { box-sizing: border-box; }

    .trigger {
      position: relative;
      background: transparent;
      border: 1px solid transparent;
      color: inherit;
      padding: 4px;
      border-radius: 6px;
      cursor: pointer;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 30px;
      height: 30px;
      font: inherit;
    }
    .trigger:hover {
      background: rgba(255, 255, 255, 0.08);
    }
    .trigger:focus-visible {
      outline: 2px solid var(--accent);
      outline-offset: 0;
    }
    .trigger svg { display: block; }
    .dot {
      position: absolute;
      top: 3px;
      right: 3px;
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: var(--danger);
      border: 2px solid var(--fg);
    }

    .popover {
      top: calc(100% + 8px);
      right: 0;
      width: 380px;
      max-width: 92vw;
      max-height: 480px;
      display: flex;
      flex-direction: column;
      color: var(--fg);
    }
    .head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 10px 14px;
      border-bottom: 1px solid var(--gray-2);
    }
    .head .title {
      font-weight: 600;
      font-size: 13px;
    }
    .head .mark-all {
      background: none;
      border: none;
      color: var(--accent);
      font-size: 12px;
      cursor: pointer;
      padding: 0;
    }
    .head .mark-all:hover { text-decoration: underline; }
    .head .mark-all[disabled] {
      color: var(--fg-muted);
      cursor: default;
    }

    .list {
      flex: 1 1 auto;
      overflow-y: auto;
    }

    .row {
      display: grid;
      grid-template-columns: auto 1fr auto;
      gap: 10px;
      align-items: start;
      padding: 10px 14px;
      border-top: 1px solid var(--gray-2);
      text-decoration: none;
      color: var(--fg);
      font-size: 13px;
      line-height: 1.35;
    }
    .row:first-child { border-top: none; }
    .row:hover { background: var(--bg-subtle); }
    .row .unread-dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: var(--accent);
      margin-top: 6px;
    }
    .row.read .unread-dot { visibility: hidden; }
    .row .avatar-slot {
      width: 28px;
      height: 28px;
      flex: 0 0 auto;
    }
    .row .body { min-width: 0; }
    .row .body .copy {
      overflow: hidden;
      text-overflow: ellipsis;
      display: -webkit-box;
      -webkit-line-clamp: 2;
      -webkit-box-orient: vertical;
    }
    .row .body .copy strong { font-weight: 600; }
    .row .body .when {
      font-size: 11px;
      color: var(--fg-muted);
      margin-top: 2px;
    }

    .empty {
      padding: 32px 20px;
      text-align: center;
      color: var(--fg-muted);
      font-style: italic;
      font-size: 13px;
    }

    .foot {
      border-top: 1px solid var(--gray-2);
      padding: 8px 14px;
      text-align: center;
    }
    .foot .load-older {
      background: none;
      border: none;
      color: var(--accent);
      font-size: 12px;
      cursor: pointer;
    }
    .foot .load-older:hover { text-decoration: underline; }
    .foot .load-older[disabled] { color: var(--fg-muted); cursor: default; }
  `,
  ];

  constructor() {
    super();
    this.me = null;
    this._unread = 0;
    this._open = false;
    this._items = [];
    this._loading = false;
    this._nextAfter = null;
    this._disabled = false;
    this._onDocClick = this._onDocClick.bind(this);
    this._onDocKey = this._onDocKey.bind(this);
    this._onVisibility = this._onVisibility.bind(this);
    this._onNavigate = this._onNavigate.bind(this);
  }

  connectedCallback() {
    super.connectedCallback();
    document.addEventListener('click', this._onDocClick, true);
    document.addEventListener('keydown', this._onDocKey);
    document.addEventListener('visibilitychange', this._onVisibility);
    window.addEventListener('nottario-navigate', this._onNavigate);
    this._fetchUnread();
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    document.removeEventListener('click', this._onDocClick, true);
    document.removeEventListener('keydown', this._onDocKey);
    document.removeEventListener('visibilitychange', this._onVisibility);
    window.removeEventListener('nottario-navigate', this._onNavigate);
  }

  async _fetchUnread() {
    try {
      const r = await fetch('/api/notifications/unread_count');
      if (!r.ok) return;
      const j = await r.json();
      if (j.disabled) {
        this._disabled = true;
        return;
      }
      this._unread = j.unread || 0;
    } catch (_) {
      // Silent: transient network failures shouldn't spam the console.
    }
  }

  async _fetchList(loadMore = false) {
    this._loading = true;
    try {
      const params = new URLSearchParams({ limit: '20' });
      if (loadMore && this._nextAfter) {
        params.set('after_created_at', this._nextAfter.created_at);
        params.set('after_id', this._nextAfter.id);
      }
      const r = await fetch(`/api/notifications?${params}`);
      if (!r.ok) return;
      const j = await r.json();
      const items = j.notifications || [];
      if (loadMore) {
        this._items = [...this._items, ...items];
      } else {
        this._items = items;
      }
      this._nextAfter = j.next_after || null;
    } finally {
      this._loading = false;
    }
  }

  _onDocClick(e) {
    if (!this._open) return;
    if (e.composedPath().includes(this)) return;
    this._open = false;
  }

  _onDocKey(e) {
    if (e.key === 'Escape' && this._open) {
      this._open = false;
    }
  }

  _onVisibility() {
    if (document.visibilityState === 'visible') this._fetchUnread();
  }

  _onNavigate() {
    this._fetchUnread();
  }

  async _toggle() {
    if (this._disabled) return;
    this._open = !this._open;
    if (this._open) {
      await this._fetchList();
    }
  }

  async _markAllRead() {
    if (this._unread === 0) return;
    try {
      const r = await fetch('/api/notifications/read_all', { method: 'POST' });
      if (!r.ok) return;
      this._unread = 0;
      // Mutate local rows so read_at reflects immediately without a
      // second round-trip.
      this._items = this._items.map((n) => ({ ...n, read_at: new Date().toISOString() }));
    } catch (_) {}
  }

  async _markOne(id) {
    try {
      const r = await fetch('/api/notifications/read', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: [id] }),
      });
      if (!r.ok) return;
      // Decrement unread only if the row was actually unread.
      const row = this._items.find((n) => n.id === id);
      if (row && !row.read_at) {
        this._unread = Math.max(0, this._unread - 1);
        row.read_at = new Date().toISOString();
        this._items = [...this._items];
      }
    } catch (_) {}
  }

  _relTime(iso) {
    if (!iso) return '';
    const then = new Date(iso).getTime();
    if (!Number.isFinite(then)) return '';
    const diff = Date.now() - then;
    if (diff < 60_000) return 'just now';
    const m = Math.floor(diff / 60_000);
    if (m < 60) return `${m}m ago`;
    const h = Math.floor(m / 60);
    if (h < 24) return `${h}h ago`;
    const d = Math.floor(h / 24);
    if (d < 7) return `${d}d ago`;
    const w = Math.floor(d / 7);
    if (w < 12) return `${w}w ago`;
    return new Date(iso).toLocaleDateString();
  }

  _renderCopy(n) {
    const actorName =
      n.actor?.display_name || (n.actor?.github_login ? '@' + n.actor.github_login : 'Someone');
    const title = n.task?.title || 'a task';
    switch (n.kind) {
      case 'task_assigned':
        return html`<span>${actorName} assigned you to <strong>${title}</strong></span>`;
      case 'task_commented':
        return html`<span>${actorName} commented on <strong>${title}</strong></span>`;
      case 'task_closed':
        return html`<span><strong>${title}</strong> was closed by ${actorName}</span>`;
      default:
        return html`<span>${n.body}</span>`;
    }
  }

  _hrefFor(n) {
    if (!n.task?.project_id || !n.task?.id) return '#';
    return `/projects/${n.task.project_id}/board/kanban#task=${n.task.id}`;
  }

  render() {
    if (this._disabled || !this.me) return null;
    const hasUnread = this._unread > 0;
    return html`
      <button class="trigger"
              type="button"
              aria-haspopup="dialog"
              aria-expanded=${this._open ? 'true' : 'false'}
              aria-label=${hasUnread ? `Notifications, ${this._unread} unread` : 'Notifications'}
              title=${hasUnread ? `${this._unread} unread` : 'Notifications'}
              @click=${this._toggle}>
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
          <path d="M4 5.33a4 4 0 0 1 8 0c0 4.67 2 6 2 6H2s2-1.33 2-6"
                stroke="currentColor" stroke-width="1.3"
                stroke-linecap="round" stroke-linejoin="round"/>
          <path d="M6.87 14a1.29 1.29 0 0 0 2.27 0"
                stroke="currentColor" stroke-width="1.3"
                stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
        ${hasUnread ? html`<span class="dot" aria-hidden="true"></span>` : null}
      </button>
      ${
        this._open
          ? html`
        <div class="popover" role="dialog" aria-label="Notifications">
          <div class="head">
            <span class="title">Notifications${hasUnread ? html` <span style="color: var(--fg-muted); font-weight: 400;">(${this._unread})</span>` : ''}</span>
            <button class="mark-all"
                    ?disabled=${!hasUnread}
                    @click=${this._markAllRead}>Mark all read</button>
          </div>
          <div class="list">
            ${
              this._loading && this._items.length === 0
                ? html`<div class="empty">Loading…</div>`
                : this._items.length === 0
                  ? html`<div class="empty">You're all caught up.</div>`
                  : this._items.map(
                      (n) => html`
                <a class=${`row ${n.read_at ? 'read' : ''}`}
                   href=${this._hrefFor(n)}
                   @click=${(e) => {
                     if (this._hrefFor(n) === '#') {
                       e.preventDefault();
                       return;
                     }
                     // Fire-and-forget mark-read; hash change navigates.
                     if (!n.read_at) this._markOne(n.id);
                     this._open = false;
                   }}>
                  <span class="unread-dot" aria-hidden="true"></span>
                  <span class="body">
                    <span class="copy">${this._renderCopy(n)}</span>
                    <span class="when">${this._relTime(n.created_at)}</span>
                  </span>
                  <nottario-avatar class="avatar-slot"
                    .src=${n.actor?.avatar_url || ''}
                    .name=${n.actor?.display_name || n.actor?.github_login || ''}
                    .size=${24}></nottario-avatar>
                </a>
              `,
                    )
            }
          </div>
          ${
            this._nextAfter
              ? html`
            <div class="foot">
              <button class="load-older"
                      ?disabled=${this._loading}
                      @click=${() => this._fetchList(true)}>Load older</button>
            </div>
          `
              : null
          }
        </div>
      `
          : null
      }
    `;
  }
}

customElements.define('nottario-notifications-bell', NottarioNotificationsBell);
