import { LitElement, html, css } from '/static/vendor/lit/lit.js';

// <nottario-page-header> is the shared chrome below the topbar.
// Layout slots, fixed by name so a page can't improvise:
//
//   - crumbs (property, array): optional breadcrumb trail.
//             Renders as anchors with " / " separators, the last item
//             plain text. Replaces every ad-hoc "← Back" button in
//             the previous design.
//   - title (property, string): the single h1 of the page.
//   - subtitle (property, string): muted secondary line beside title.
//   - <span slot="switcher">: optional segmented control.
//   - <span slot="actions">:  one primary + zero-to-two secondary.
//
// Renders a hairline below the row so every page has the same break
// between header and body.
class NottarioPageHeader extends LitElement {
  static properties = {
    crumbs: { type: Array },
    title: { type: String },
    subtitle: { type: String },
  };

  static styles = css`
    :host {
      display: block;
      box-sizing: border-box;
      margin-bottom: 16px;
      padding-bottom: 12px;
      border-bottom: 1px solid var(--gray-2);
    }
    * { box-sizing: border-box; }

    .crumbs {
      display: flex;
      gap: 4px;
      align-items: center;
      font-size: 12px;
      color: var(--fg-muted);
      margin-bottom: 4px;
      min-height: 16px;
    }
    .crumbs a {
      color: var(--accent);
      text-decoration: none;
    }
    .crumbs a:hover { text-decoration: underline; }
    .crumbs .sep { opacity: 0.5; }
    .crumbs .current { color: var(--fg-muted); }

    .row {
      display: flex;
      align-items: center;
      gap: 12px;
      flex-wrap: wrap;
    }
    .titles {
      display: flex;
      align-items: baseline;
      gap: 10px;
      min-width: 0;
      flex: 1 1 auto;
    }
    h1 {
      margin: 0;
      font-size: 20px;
      font-weight: 600;
      line-height: 1.2;
      color: var(--fg);
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .subtitle {
      color: var(--fg-muted);
      font-size: 13px;
      white-space: nowrap;
    }
    .right {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      flex: 0 0 auto;
    }
    ::slotted([slot="actions"]) {
      display: inline-flex;
      gap: 8px;
      align-items: center;
    }
  `;

  constructor() {
    super();
    this.crumbs = [];
    this.title = '';
    this.subtitle = '';
  }

  _renderCrumb(c, last) {
    if (last || !c.href) {
      return html`<span class="current">${c.label}</span>`;
    }
    return html`<a href=${c.href}
                  @click=${(e) => {
                    e.preventDefault();
                    window.nottarioNavigate(c.href);
                  }}>${c.label}</a>`;
  }

  render() {
    const crumbs = this.crumbs || [];
    return html`
      ${
        crumbs.length
          ? html`<div class="crumbs">
            ${crumbs.flatMap((c, i) => {
              const last = i === crumbs.length - 1;
              const node = this._renderCrumb(c, last);
              return i === 0 ? [node] : [html`<span class="sep">/</span>`, node];
            })}
          </div>`
          : null
      }
      <div class="row">
        <div class="titles">
          <h1>${this.title}</h1>
          ${this.subtitle ? html`<span class="subtitle">${this.subtitle}</span>` : null}
        </div>
        <div class="right">
          <slot name="switcher"></slot>
          <slot name="actions"></slot>
        </div>
      </div>
    `;
  }
}

customElements.define('nottario-page-header', NottarioPageHeader);
