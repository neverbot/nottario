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
//
// Optional `agent` property marks the avatar as belonging to an action
// done by an MCP token (an agent) on behalf of this human. The badge
// overlays the bottom-right corner; its hue is hash-derived from the
// token name so different agents read distinct at a glance, and the
// tooltip reveals the token name. Below 18px we fall back to a plain
// coloured dot — letters get illegible at that size.
class NottarioAvatar extends LitElement {
  static properties = {
    src: { type: String },
    name: { type: String },
    size: { type: Number },
    color: { type: String },
    agent: { type: Object },
  };

  static styles = css`
    :host {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      box-sizing: border-box;
      position: relative;
      color: #fff;
      font-weight: 600;
      text-transform: uppercase;
      flex: 0 0 auto;
      line-height: 1;
      font-family: inherit;
    }
    * { box-sizing: border-box; }
    .frame {
      width: 100%;
      height: 100%;
      border-radius: 50%;
      overflow: hidden;
      background: var(--avatar-bg, var(--fg-muted));
      display: inline-flex;
      align-items: center;
      justify-content: center;
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
    .agent-badge {
      position: absolute;
      right: -3px;
      bottom: -3px;
      border-radius: 50%;
      background: var(--agent-bg, var(--accent));
      color: #fff;
      box-shadow: 0 0 0 1.5px var(--bg, #fff);
      display: inline-flex;
      align-items: center;
      justify-content: center;
      font-weight: 700;
      line-height: 1;
      pointer-events: auto;
    }
  `;

  constructor() {
    super();
    this.src = '';
    this.name = '';
    this.size = 24;
    this.color = '';
    this.agent = null;
  }

  static initials(name) {
    if (!name) return '?';
    const parts = name.trim().split(/\s+/).slice(0, 2);
    return parts.map((p) => p.charAt(0)).join('');
  }

  // Map a token name to one of the brand palette hues. Same palette as
  // role / kind colours so the page stays inside the documented vocab.
  // Empty / missing name falls to a neutral grey so anonymous-agent
  // badges read different from "named" ones.
  static _agentColor(name) {
    if (!name) return '#6e7681';
    const palette = ['#1f6feb', '#2da44e', '#bf8700', '#8250df', '#cf222e', '#bc4c00'];
    let h = 0;
    for (let i = 0; i < name.length; i++) {
      h = (h * 31 + name.charCodeAt(i)) | 0;
    }
    return palette[(h >>> 0) % palette.length];
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
      this.style.setProperty('--avatar-bg', this.color || 'var(--fg-muted)');
    }
    if (changed.has('agent')) {
      this.style.setProperty('--agent-bg', NottarioAvatar._agentColor(this.agent?.name || ''));
    }
  }

  render() {
    const frame = this.src
      ? // alt="" because every consumer renders the user's name as text
        // next to the avatar. Letting `alt` repeat the name would
        // double the accessible name of the parent control.
        html`<div class="frame"><img src=${this.src} alt=""></div>`
      : html`<div class="frame"><span class="initials" aria-hidden="true">${NottarioAvatar.initials(this.name)}</span></div>`;
    if (!this.agent) return frame;
    // Badge geometry scales with the avatar so it stays balanced from
    // 22px stacks up to 56px profile cards. Dot fallback under 18px.
    const badgeSize = Math.max(8, Math.round(this.size * 0.42));
    const showLetter = this.size >= 18;
    const letter = (this.agent.name || '').trim().charAt(0).toUpperCase() || '?';
    const tip = this.agent.name ? `Agent: ${this.agent.name}` : 'Agent (token revoked)';
    const badgeStyle = `width:${badgeSize}px;height:${badgeSize}px;font-size:${Math.max(7, Math.round(badgeSize * 0.6))}px`;
    return html`
      ${frame}
      <span class="agent-badge" style=${badgeStyle} title=${tip} aria-label=${tip}>
        ${showLetter ? letter : ''}
      </span>
    `;
  }
}

customElements.define('nottario-avatar', NottarioAvatar);
