import { LitElement, html, css } from '/static/vendor/lit/lit.js';

// <nottario-tabs .options=${[{id, label}, ...]} value="general" @change=${...}>
//
// Tab strip with a thin underline on the active tab. Emits a
// 'change' CustomEvent ({ detail: { value } }) when the user picks
// a different tab. Stateless beyond the `value` prop — the host
// owns the active state.
//
// Used by project-settings today; designed so other settings hubs
// (user profile, org admin, …) can adopt it without copying CSS.
class NottarioTabs extends LitElement {
  static properties = {
    options: { type: Array },
    value:   { type: String },
  };

  static styles = css`
    :host {
      display: block;
      box-sizing: border-box;
      margin-bottom: 16px;
    }
    .tabs {
      display: flex;
      gap: 4px;
      border-bottom: 1px solid #d1d9e0;
    }
    button {
      padding: 8px 14px;
      background: transparent;
      border: none;
      border-bottom: 2px solid transparent;
      cursor: pointer;
      color: #59636e;
      font: inherit;
      font-size: 13px;
      font-weight: 500;
      margin-bottom: -1px;
    }
    button:hover { color: #1f2328; }
    button.active {
      color: #1f2328;
      border-bottom-color: #ff8c42;
    }
    button:focus-visible {
      outline: 2px solid #0969da;
      outline-offset: 2px;
      border-radius: 4px;
    }
  `;

  constructor() {
    super();
    this.options = [];
    this.value = '';
  }

  _pick(id) {
    if (id === this.value) return;
    this.dispatchEvent(new CustomEvent('change', {
      detail: { value: id },
      bubbles: true,
      composed: true,
    }));
  }

  _onKey(e) {
    if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft'
        && e.key !== 'Home' && e.key !== 'End') return;
    const opts = this.options || [];
    if (opts.length < 2) return;
    const idx = opts.findIndex(o => o.id === this.value);
    let next = idx;
    if (e.key === 'ArrowRight') next = (idx + 1) % opts.length;
    else if (e.key === 'ArrowLeft') next = (idx - 1 + opts.length) % opts.length;
    else if (e.key === 'Home') next = 0;
    else if (e.key === 'End') next = opts.length - 1;
    if (next !== idx) {
      e.preventDefault();
      this._pick(opts[next].id);
      requestAnimationFrame(() => {
        const buttons = this.shadowRoot?.querySelectorAll('[role="tab"]');
        buttons?.[next]?.focus();
      });
    }
  }

  render() {
    return html`
      <div class="tabs" role="tablist" @keydown=${(e) => this._onKey(e)}>
        ${(this.options || []).map(o => html`
          <button class=${o.id === this.value ? 'active' : ''}
                  role="tab"
                  aria-selected=${o.id === this.value ? 'true' : 'false'}
                  tabindex=${o.id === this.value ? '0' : '-1'}
                  @click=${() => this._pick(o.id)}>${o.label}</button>
        `)}
      </div>
    `;
  }
}

customElements.define('nottario-tabs', NottarioTabs);
