import { LitElement, html, css } from '/static/vendor/lit/lit.js';

// <nottario-task-chip .task=${task} project-id=${pid}>
//
// Compact inline reference to a task. Renders as a pill:
//   #c4dbf4 · SQL safety Tier 2: migrate all hand-written SQL …  [doing]
//
// Used by the board task-detail dialog to render the Dependencies list
// (previously raw uuid <code> tags), and by the kanban itself once the
// markdown renderer's [[task:N]] chip resolves to a real link
// (the server already emits compatible markup; this is the matching
// component for client-side renders).
//
// The chip links to the same project's kanban with the task id in the
// URL hash, which the board page picks up via the existing
// `_applyHash` handler.
class NottarioTaskChip extends LitElement {
  static properties = {
    task:      { type: Object },
    projectId: { type: String, attribute: 'project-id' },
  };

  static styles = css`
    :host { display: inline-block; box-sizing: border-box; }
    a {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 2px 8px 2px 6px;
      border: 1px solid #d1d9e0;
      border-radius: 999px;
      background: #f6f8fa;
      color: #1f2328;
      font-size: 12px;
      font-weight: 500;
      text-decoration: none;
      line-height: 1.4;
      max-width: 100%;
    }
    a:hover {
      border-color: #0969da;
      background: #ddf4ff;
      color: #0969da;
    }
    .id {
      font-family: ui-monospace, SFMono-Regular, monospace;
      color: #8b949e;
      font-size: 11px;
    }
    .title {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      max-width: 38ch;
    }
    .state {
      font-size: 10px;
      letter-spacing: 0.04em;
      text-transform: uppercase;
      padding: 1px 5px;
      border-radius: 4px;
      border: 1px solid transparent;
      line-height: 1.2;
      flex: 0 0 auto;
    }
    .state-todo  { color: #59636e; background: #eaeef2; }
    .state-doing { color: #0969da; background: #ddf4ff; }
    .state-done  { color: #1a7f37; background: #dafbe1; opacity: 0.7; }

    /* Missing task fallback: the chip is still rendered (so an
       orphan dependency is visible) but in a muted, italic shape. */
    a.missing {
      color: #cf222e;
      background: #ffebe9;
      border-color: rgba(207, 34, 46, 0.4);
      font-style: italic;
    }
  `;

  constructor() {
    super();
    this.task = null;
    this.projectId = '';
  }

  render() {
    if (!this.task) {
      return html`<a class="missing" title="unknown task">unknown task</a>`;
    }
    const t = this.task;
    const short = (t.ID || '').slice(0, 7);
    const href = `/projects/${this.projectId}/board/kanban#task=${t.ID}`;
    const a11yLabel = `Task ${short}: ${t.Title || t.ID}` +
      (t.State ? `, state ${t.State}` : '');
    return html`
      <a href=${href}
         title=${t.Title || t.ID}
         aria-label=${a11yLabel}>
        <span class="id">#${short}</span>
        <span class="title">${t.Title || t.ID}</span>
        ${t.State ? html`<span class="state state-${t.State}">${t.State}</span>` : null}
      </a>
    `;
  }
}

customElements.define('nottario-task-chip', NottarioTaskChip);
