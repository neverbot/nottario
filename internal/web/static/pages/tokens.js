import { LitElement, html, css } from '/static/vendor/lit/lit.js';

class NottarioTokensPage extends LitElement {
  static properties = {
    me: { type: Object },
    tokens: { state: true },
    showIssue: { state: true },
    issued: { state: true },
    error: { state: true },
  };

  static styles = css`
    :host { display: block; }
    .header { display: flex; align-items: baseline; gap: 12px; margin-bottom: 16px; }
    .header h2 { margin: 0; }
    .spacer { flex: 1; }
    .panel {
      background: #fff;
      border: 1px solid #d1d9e0;
      border-radius: 8px;
      padding: 0;
      overflow: hidden;
    }
    table { width: 100%; border-collapse: collapse; }
    th, td { padding: 10px 16px; text-align: left; border-bottom: 1px solid #eaeef2; }
    th { font-size: 12px; text-transform: uppercase; color: #59636e; background: #f6f8fa; }
    tr:last-child td { border-bottom: none; }
    .row-actions { text-align: right; }
    .dialog {
      position: fixed;
      inset: 0;
      background: rgba(0, 0, 0, 0.4);
      display: flex;
      align-items: center;
      justify-content: center;
      z-index: 10;
    }
    .dialog .inner {
      background: #fff;
      border-radius: 8px;
      padding: 24px;
      width: 540px;
      max-width: 90vw;
    }
    .field { margin-bottom: 12px; }
    .field label { display: block; margin-bottom: 4px; font-weight: 500; font-size: 13px; }
    .actions-row {
      margin-top: 16px;
      display: flex;
      gap: 8px;
      justify-content: flex-end;
    }
    .secret-banner {
      background: #fff8c5;
      border: 1px solid #d4a72c;
      border-radius: 6px;
      padding: 12px;
      margin-bottom: 12px;
      color: #7d4e00;
    }
    .secret {
      font-family: ui-monospace, SFMono-Regular, "SF Mono", monospace;
      padding: 8px 12px;
      background: #f6f8fa;
      border: 1px solid #d1d9e0;
      border-radius: 4px;
      word-break: break-all;
      user-select: all;
    }
    .error { color: #cf222e; margin-bottom: 8px; font-size: 13px; }
    .muted { color: #59636e; }
  `;

  constructor() {
    super();
    this.tokens = null;
    this.showIssue = false;
    this.issued = null;
    this.error = '';
  }

  connectedCallback() {
    super.connectedCallback();
    this.load();
  }

  async load() {
    try {
      const res = await fetch('/api/tokens');
      if (!res.ok) throw new Error('failed to load tokens');
      this.tokens = (await res.json()).tokens || [];
    } catch (e) {
      this.error = e.message;
      this.tokens = [];
    }
  }

  open() {
    this.showIssue = true;
    this.issued = null;
    this.error = '';
  }

  close() {
    this.showIssue = false;
    this.issued = null;
  }

  async issue(e) {
    e.preventDefault();
    const form = e.target;
    try {
      const res = await fetch('/api/tokens', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: form.name.value.trim() }),
      });
      if (!res.ok) throw new Error((await res.json()).error || 'failed');
      this.issued = await res.json();
      await this.load();
    } catch (err) {
      this.error = err.message;
    }
  }

  async revoke(id) {
    if (!confirm('Revoke this token? Agents using it will be locked out immediately.')) return;
    try {
      const res = await fetch(`/api/tokens/${id}`, { method: 'DELETE' });
      if (!res.ok) throw new Error('failed');
      await this.load();
    } catch (err) { this.error = err.message; }
  }

  fmt(d) {
    return d ? new Date(d).toLocaleString() : '—';
  }

  render() {
    if (this.tokens === null) return html`<div class="panel" style="padding:16px">Loading…</div>`;
    return html`
      <div class="header">
        <h2>API tokens</h2>
        <div class="spacer"></div>
        <button class="primary" @click=${() => this.open()}>New token</button>
      </div>
      ${this.error ? html`<div class="error">${this.error}</div>` : null}
      <div class="panel">
        <table>
          <thead><tr>
            <th>Name</th><th>Prefix</th><th>Created</th><th>Last used</th><th>Status</th><th></th>
          </tr></thead>
          <tbody>
            ${this.tokens.length === 0
              ? html`<tr><td colspan="6" class="muted" style="text-align:center;padding:24px">No tokens yet.</td></tr>`
              : this.tokens.map(t => html`
                <tr>
                  <td>${t.Name}</td>
                  <td class="mono">${t.Prefix}…</td>
                  <td>${this.fmt(t.CreatedAt)}</td>
                  <td>${this.fmt(t.LastUsedAt)}</td>
                  <td>${t.RevokedAt ? html`<span class="muted">revoked</span>` : html`<span style="color:#1f883d">active</span>`}</td>
                  <td class="row-actions">
                    ${t.RevokedAt ? null : html`<button class="danger" @click=${() => this.revoke(t.ID)}>Revoke</button>`}
                  </td>
                </tr>
              `)}
          </tbody>
        </table>
      </div>
      ${this.showIssue ? this.renderDialog() : null}
    `;
  }

  renderDialog() {
    if (this.issued) {
      return html`
        <div class="dialog">
          <div class="inner">
            <h3>Token issued</h3>
            <div class="secret-banner">
              <strong>Copy this token now.</strong> It will not be shown again.
            </div>
            <div class="secret">${this.issued.plaintext}</div>
            <div class="actions-row">
              <button @click=${() => navigator.clipboard.writeText(this.issued.plaintext)}>Copy</button>
              <button class="primary" @click=${() => this.close()}>Done</button>
            </div>
          </div>
        </div>
      `;
    }
    return html`
      <div class="dialog">
        <div class="inner">
          <h3>New API token</h3>
          ${this.error ? html`<div class="error">${this.error}</div>` : null}
          <form @submit=${(e) => this.issue(e)}>
            <div class="field">
              <label>Name (so you remember which machine uses it)</label>
              <input name="name" required autofocus placeholder="laptop, ci-runner, …">
            </div>
            <div class="actions-row">
              <button type="button" @click=${() => this.close()}>Cancel</button>
              <button type="submit" class="primary">Issue</button>
            </div>
          </form>
        </div>
      </div>
    `;
  }
}

customElements.define('nottario-tokens-page', NottarioTokensPage);
