import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { subscribe } from '/static/realtime.js';

// <nottario-update-banner .me=${me}>
//
// Full-width strip that surfaces "a newer commit is available on
// upstream master" to admins. Fetches `/api/version/status` once on
// mount; when the endpoint reports `update_available: true` and the
// caller is an admin, renders the strip between the topbar and the
// main content area.
//
// Product-register design: neutral palette (bg-subtle + 1px border),
// no side-stripe, no accent color, no animation. The banner never
// prescribes a specific upgrade command — self-hosters may run any
// combination of `docker compose`, k8s, systemd wrappers, Ansible,
// etc. — and instead links to the canonical documentation page
// that covers each shape. The two 7-char SHAs are exposed as a
// tooltip on the leading icon, not as body text — the operator
// only needs them for confirmation, not as primary reading.
//
// Both doc links hardcode the canonical neverbot.github.io host
// because the docs site is not co-hosted with the Nottario instance
// and there is no runtime way to discover a fork's own docs URL. A
// fork operator can PR an env-var override once they hit this.
//
// Dismiss persistence keys off the LATEST sha (not the current time
// or a boolean) so that once a NEW upstream commit lands, the banner
// re-arms automatically instead of staying dismissed forever.
class NottarioUpdateBanner extends LitElement {
  static properties = {
    me: { type: Object },
    // Internal state: null while loading, object once the endpoint
    // resolves, false when the fetch fails or the check is disabled.
    _status: { state: true },
    _dismissed: { state: true },
  };

  static styles = css`
    :host { display: block; box-sizing: border-box; }

    .bar {
      display: grid;
      grid-template-columns: auto 1fr auto auto;
      gap: 12px;
      align-items: center;
      padding: 10px 20px;
      background: var(--bg-subtle);
      border-bottom: 1px solid var(--border);
      font-size: 13px;
      color: var(--fg);
      box-sizing: border-box;
    }

    .icon {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 20px;
      height: 20px;
      color: var(--fg-muted);
    }
    .icon svg { display: block; }

    .msg { line-height: 1.4; }
    .msg strong { font-weight: 600; }

    .actions {
      display: inline-flex;
      align-items: center;
      gap: 16px;
    }
    .actions a {
      color: var(--accent);
      text-decoration: none;
      font-size: 13px;
      white-space: nowrap;
    }
    .actions a:hover { text-decoration: underline; }

    .dismiss {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 24px;
      height: 24px;
      border: 1px solid transparent;
      background: transparent;
      border-radius: 4px;
      color: var(--fg-muted);
      cursor: pointer;
      padding: 0;
      font: inherit;
    }
    .dismiss:hover {
      color: var(--fg);
      background: var(--bg);
      border-color: var(--border);
    }
    .dismiss:focus-visible {
      outline: 2px solid var(--accent);
      outline-offset: 0;
    }
    .dismiss svg { display: block; }
  `;

  constructor() {
    super();
    this.me = null;
    this._status = null;
    this._dismissed = sessionStorage.getItem('nottario-update-dismissed') || '';
  }

  connectedCallback() {
    super.connectedCallback();
    this._fetchStatus();
    // Subscribe to the global SSE stream. The server pushes
    // `version_status` whenever the selfupdate poller records a real
    // state change (new upstream SHA, error transition, or first
    // successful check). We re-fetch on that signal AND on
    // realtime.reconnected so a watchtower-driven container restart
    // auto-clears the banner: EventSource reconnects to the fresh
    // container, the new state (running.sha == latest.sha) lands,
    // banner hides without a page reload.
    this._unsub = subscribe(null, (ev) => {
      if (ev.type === 'version_status' || ev.type === 'realtime.reconnected') {
        this._fetchStatus();
      }
    });
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this._unsub?.();
    this._unsub = null;
  }

  async _fetchStatus() {
    // The endpoint returns 401 for anonymous. If the shell hasn't
    // resolved `me` yet the fetch still succeeds via the session
    // cookie, but for cleanliness we skip when we already know
    // there's no logged-in user.
    if (!this.me) {
      this._status = false;
      return;
    }
    try {
      const r = await fetch('/api/version/status');
      if (!r.ok) {
        this._status = false;
        return;
      }
      this._status = await r.json();
    } catch (_) {
      this._status = false;
    }
  }

  _dismiss() {
    const latest = this._status?.latest?.sha || '';
    sessionStorage.setItem('nottario-update-dismissed', latest);
    this._dismissed = latest;
  }

  _shortSha(s) {
    return typeof s === 'string' && s.length >= 7 ? s.slice(0, 7) : s || '';
  }

  render() {
    const s = this._status;
    if (!s || !s.update_available) return null;
    const latestSha = s.latest?.sha || '';
    if (this._dismissed && this._dismissed === latestSha) return null;

    const running = this._shortSha(s.running?.sha);
    const latest = this._shortSha(latestSha);
    const shaTitle = running && latest ? `${running} → ${latest}` : '';

    return html`
      <div class="bar" role="status" aria-live="polite">
        <span class="icon" title=${shaTitle} aria-hidden="true">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
            <path d="M8 1.5a.75.75 0 0 1 .53.22l4.25 4.25a.75.75 0 1 1-1.06 1.06L8.75 4.06V14a.75.75 0 0 1-1.5 0V4.06L4.28 7.03A.75.75 0 1 1 3.22 5.97l4.25-4.25A.75.75 0 0 1 8 1.5Z"/>
          </svg>
        </span>
        <span class="msg">
          <strong>Update available.</strong>
        </span>
        <span class="actions">
          <a href="https://neverbot.github.io/nottario/whats-new/"
             target="_blank"
             rel="noopener noreferrer">What's new</a>
          <a href="https://neverbot.github.io/nottario/self-hosting/#upgrade-flow"
             target="_blank"
             rel="noopener noreferrer">How to upgrade</a>
        </span>
        <button class="dismiss"
                type="button"
                title="Dismiss for this session"
                aria-label="Dismiss update banner"
                @click=${this._dismiss}>
          <svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor">
            <path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.75.75 0 1 1 1.06 1.06L9.06 8l3.22 3.22a.75.75 0 1 1-1.06 1.06L8 9.06l-3.22 3.22a.75.75 0 0 1-1.06-1.06L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/>
          </svg>
        </button>
      </div>
    `;
  }
}

customElements.define('nottario-update-banner', NottarioUpdateBanner);
