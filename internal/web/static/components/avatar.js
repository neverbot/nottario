import { LitElement, html, css } from '/static/vendor/lit/lit.js';

// <nottario-avatar> renders a circular user avatar. When `src` is set
// it shows the image; otherwise it falls back to up-to-two initials
// computed from `name` (display name preferred, github_login as
// fallback caller-side).
//
// Size is configurable via the `size` attribute (px). Defaults to 24px
// to match the topbar pill and projects-card stack. Larger uses (the
// profile identity card) pass `size="56"`.
//
// Color of the fallback circle defaults to neutral grey. Pass an
// explicit `color` attribute to tint it (e.g. role colour). The avatar
// is rendered with `display: inline-flex` so it sits inline with text.
class NottarioAvatar extends LitElement {
  static properties = {
    src: { type: String },
    name: { type: String },
    size: { type: Number },
    color: { type: String },
  };

  static styles = css`
    :host {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      box-sizing: border-box;
      overflow: hidden;
      border-radius: 50%;
      background: var(--avatar-bg, #59636e);
      color: #fff;
      font-weight: 600;
      text-transform: uppercase;
      flex: 0 0 auto;
      line-height: 1;
      font-family: inherit;
    }
    img {
      width: 100%;
      height: 100%;
      object-fit: cover;
      display: block;
    }
    span.initials {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 100%;
      height: 100%;
    }
  `;

  constructor() {
    super();
    this.src = '';
    this.name = '';
    this.size = 24;
    this.color = '';
  }

  static initials(name) {
    if (!name) return '?';
    const parts = name.trim().split(/\s+/).slice(0, 2);
    return parts.map(p => p.charAt(0)).join('');
  }

  updated(changed) {
    if (changed.has('size')) {
      const px = `${this.size}px`;
      this.style.width = px;
      this.style.height = px;
      // Font scale ~40% of the box so initials look balanced.
      this.style.fontSize = `${Math.max(10, Math.round(this.size * 0.4))}px`;
    }
    if (changed.has('color')) {
      this.style.setProperty('--avatar-bg', this.color || '#59636e');
    }
  }

  render() {
    if (this.src) {
      return html`<img src=${this.src} alt=${this.name || ''}>`;
    }
    return html`<span class="initials">${NottarioAvatar.initials(this.name)}</span>`;
  }
}

customElements.define('nottario-avatar', NottarioAvatar);
