import { LitElement, html, css } from '/static/vendor/lit/lit.js';

class NottarioLogin extends LitElement {
  static properties = {
    _error: { state: true },
    _org: { state: true },
  };

  static styles = css`
    :host {
      box-sizing: border-box;
      display: flex;
      flex-direction: column;
      min-height: 80vh;
      align-items: center;
      justify-content: center;
      gap: 24px;
      padding: 32px 16px;
    }
    .card {
      box-sizing: border-box;
      padding: 32px;
      max-width: 380px;
      width: 100%;
      background: var(--bg);
      border: 1px solid var(--border);
      border-radius: 8px;
      box-shadow: var(--shadow-sm);
      text-align: center;
    }
    /* Brand mark: the same green→blue gradient chip that lives in
       the topbar (components/topbar.js). Repeated here so the first
       touchpoint already carries the visual identity — the login
       has no topbar so without this the user lands on a brandless
       neutral card. */
    .mark {
      width: 40px;
      height: 40px;
      border-radius: 9px;
      background: linear-gradient(135deg,
        var(--brand-green) 0%, var(--brand-blue) 100%);
      margin: 0 auto 12px;
    }
    h1 {
      margin: 0 0 6px 0;
      font-size: 22px;
      font-weight: 600;
      letter-spacing: -0.01em;
    }
    .lede {
      margin: 0 0 24px 0;
      color: var(--fg-muted);
      font-size: 13.5px;
      line-height: 1.5;
    }
    .flash {
      box-sizing: border-box;
      margin: 0 0 20px 0;
      padding: 8px 12px;
      border: 1px solid var(--tint-red-border);
      background: var(--tint-red);
      color: var(--danger-text);
      border-radius: 6px;
      font-size: 13px;
      line-height: 1.4;
      text-align: left;
    }
    .flash strong { font-weight: 600; }
    /* GitHub sign-in CTA. Dark on light by design (mirrors GitHub's
       own button so users recognise the affordance). The two greys
       below are intentionally NOT tokenised — they're brand mimicry
       of GitHub's button palette, not Nottario's. */
    a.gh {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      padding: 10px 16px;
      background: #24292f;
      color: var(--fg-on-accent);
      border-radius: 6px;
      font-weight: 500;
      text-decoration: none;
    }
    a.gh:hover {
      background: #32383f;
      text-decoration: none;
    }
    a.gh:focus-visible {
      text-decoration: none;
    }
    svg { width: 18px; height: 18px; fill: currentColor; }
    /* Footer row anchors the card to the rest of the product — even
       a user who hasn't signed in yet should know what Nottario is
       and where the code lives. */
    .footer {
      display: flex;
      align-items: center;
      gap: 14px;
      font-size: 12px;
      color: var(--fg-muted);
    }
    .footer a {
      color: var(--fg-muted);
      text-decoration: none;
    }
    .footer a:hover {
      color: var(--fg);
      text-decoration: underline;
    }
    .footer .sep {
      color: var(--gray-4);
    }
  `;

  constructor() {
    super();
    this._error = '';
    this._org = '';
  }

  connectedCallback() {
    super.connectedCallback();
    const q = new URLSearchParams(window.location.search);
    this._error = q.get('error') || '';
    this._org = q.get('org') || '';
  }

  _flash() {
    if (this._error !== 'org_required') return null;
    const org = this._org
      ? html`<strong>${this._org}</strong>`
      : html`a specific GitHub organisation`;
    return html`
      <div class="flash" role="alert">
        Restricted to ${org} members. Use a GitHub account in the
        org, or ask your admin.
      </div>
    `;
  }

  render() {
    return html`
      <div class="card">
        <div class="mark" aria-hidden="true"></div>
        <h1>Welcome to Nottario</h1>
        <p class="lede">
          Task coordination for humans and the AI agents that work
          with them. Sign in with GitHub to continue — passwords
          stay at GitHub.
        </p>
        ${this._flash()}
        <a class="gh" href="/auth/github/start" aria-label="Sign in with GitHub">
          <svg viewBox="0 0 16 16" aria-hidden="true">
            <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38
              0-.19-.01-.82-.01-1.49-2 .37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13
              -.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66
              .07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15
              -.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0
              1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82
              1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01
              1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"></path>
          </svg>
          Sign in with GitHub
        </a>
      </div>
      <div class="footer">
        <a href="https://github.com/neverbot/nottario"
           target="_blank" rel="noopener">github.com/neverbot/nottario</a>
        <span class="sep">·</span>
        <a href="/docs" target="_blank" rel="noopener">docs</a>
      </div>
    `;
  }
}

customElements.define('nottario-login', NottarioLogin);
