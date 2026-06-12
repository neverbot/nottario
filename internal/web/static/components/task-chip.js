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
    task: { type: Object },
    projectId: { type: String, attribute: 'project-id' },
  };

  static styles = css`
    :host { display: inline-block; box-sizing: border-box; }
    a {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 2px 8px 2px 6px;
      border: 1px solid var(--border);
      border-radius: 999px;
      background: var(--bg-subtle);
      color: var(--fg);
      font-size: 12px;
      font-weight: 500;
      text-decoration: none;
      line-height: 1.4;
      max-width: 100%;
    }
    a:hover {
      border-color: var(--accent);
      background: var(--tint-blue);
      color: var(--accent);
    }
    .id {
      font-family: ui-monospace, SFMono-Regular, monospace;
      color: var(--gray-5);
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
    .state-todo  { color: var(--fg-muted); background: var(--gray-2); }
    .state-doing { color: var(--accent); background: var(--tint-blue); }
    .state-done  { color: var(--success-hover); background: var(--tint-green); opacity: 0.7; }

    /* Missing task fallback: the chip is still rendered (so an
       orphan dependency is visible) but in a muted, italic shape. */
    a.missing {
      color: var(--danger);
      background: var(--tint-red);
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
    const short = (t.id || '').slice(0, 7);
    const href = `/projects/${this.projectId}/board/kanban#task=${t.id}`;
    const a11yLabel = `Task ${short}: ${t.title || t.id}` + (t.state ? `, state ${t.state}` : '');
    return html`
      <a href=${href}
         title=${t.title || t.id}
         aria-label=${a11yLabel}>
        <span class="id">#${short}</span>
        <span class="title">${t.title || t.id}</span>
        ${t.state ? html`<span class="state state-${t.state}">${t.state}</span>` : null}
      </a>
    `;
  }
}

customElements.define('nottario-task-chip', NottarioTaskChip);
