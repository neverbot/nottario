import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { defaultPathFor, viewByKey } from '/static/views.js';
import { EscController } from '/static/components/esc.js';

class NottarioProjectsPage extends LitElement {
  static properties = {
    me: { type: Object },
    projects: { state: true },
    showCreate: { state: true },
    creating: { state: true },
    error: { state: true },
  };

  static styles = css`
    :host { display: block; }
    .header {
      display: flex;
      align-items: center;
      gap: 12px;
      margin-bottom: 16px;
    }
    h2 { margin: 0; }
    .spacer { flex: 1; }
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
      gap: 12px;
    }
    .card {
      position: relative;
      padding: 16px;
      background: #fff;
      border: 1px solid #d1d9e0;
      border-radius: 8px;
      box-shadow: 0 1px 0 rgba(31, 35, 40, 0.04);
      cursor: pointer;
      transition: border-color 80ms ease-out, box-shadow 80ms ease-out;
    }
    .card:hover {
      border-color: #afb8c1;
      box-shadow: 0 1px 0 rgba(31, 35, 40, 0.04),
                  0 4px 12px rgba(31, 35, 40, 0.06);
    }
    .card:focus-visible {
      outline: 2px solid #0969da;
      outline-offset: 2px;
    }
    .card h3 { margin: 0 0 4px 0; font-size: 16px; }
    .card .default-hint {
      margin-top: 10px;
      font-size: 12px;
      color: #59636e;
    }
    .card .repos {
      margin-top: 8px;
      font-family: ui-monospace, SFMono-Regular, "SF Mono", monospace;
      font-size: 12px;
      color: #59636e;
    }
    .card .repos span {
      display: block;
    }
    .card .actions {
      margin-top: 12px;
      display: flex;
      gap: 8px;
    }
    .empty {
      padding: 32px;
      text-align: center;
      color: #59636e;
      background: #fff;
      border: 1px dashed #d1d9e0;
      border-radius: 8px;
    }
    .dialog {
      position: fixed;
      inset: 0;
      background: rgba(0, 0, 0, 0.4);
      display: flex;
      align-items: center;
      justify-content: center;
      z-index: 10;
    }
    .dialog .panel {
      background: #fff;
      border-radius: 8px;
      padding: 24px;
      width: 480px;
      max-width: 90vw;
    }
    .dialog h3 { margin: 0 0 16px 0; }
    .field { margin-bottom: 12px; }
    .field label {
      display: block;
      margin-bottom: 4px;
      font-weight: 500;
      font-size: 13px;
    }
    .actions-row {
      margin-top: 16px;
      display: flex;
      gap: 8px;
      justify-content: flex-end;
    }
    .error {
      color: #cf222e;
      font-size: 13px;
      margin-bottom: 8px;
    }
  `;

  constructor() {
    super();
    this.projects = null;
    this.showCreate = false;
    this.creating = false;
    this.error = '';
    new EscController(this, (e) => {
      if (this.showCreate) { this.closeCreate(); e.stopPropagation(); }
    });
  }

  connectedCallback() {
    super.connectedCallback();
    this.load();
  }

  async load() {
    try {
      const res = await fetch('/api/projects');
      if (!res.ok) throw new Error('failed to load projects');
      const j = await res.json();
      this.projects = j.projects || [];
    } catch (e) {
      this.error = e.message;
      this.projects = [];
    }
  }

  openCreate() {
    this.showCreate = true;
    this.error = '';
  }

  closeCreate() {
    this.showCreate = false;
  }

  async submitCreate(e) {
    e.preventDefault();
    const form = e.target;
    this.creating = true;
    this.error = '';
    const payload = {
      name: form.name.value.trim(),
      description: form.description.value.trim(),
      primary_language: form.primary_language.value.trim(),
      project_type: form.project_type.value.trim(),
      repos: form.repos.value.split(/\s*,\s*|\n+/).map(s => s.trim()).filter(Boolean),
    };
    try {
      const res = await fetch('/api/projects', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.error || 'failed to create');
      }
      this.showCreate = false;
      await this.load();
    } catch (e) {
      this.error = e.message;
    } finally {
      this.creating = false;
    }
  }

  goto(path) {
    window.nottarioNavigate(path);
  }

  render() {
    if (this.projects === null) {
      return html`<div class="empty">Loading projects…</div>`;
    }
    return html`
      <div class="header">
        <h2>Projects</h2>
        <div class="spacer"></div>
        ${this.me?.is_admin
          ? html`<button class="primary" @click=${() => this.openCreate()}>New project</button>`
          : null}
      </div>
      ${this.projects.length === 0
        ? html`<div class="empty">No projects yet.${this.me?.is_admin ? html` Click <strong>New project</strong> to create one.` : ''}</div>`
        : html`
          <div class="grid">
            ${this.projects.map(p => {
              const dest = defaultPathFor(p);
              const destLabel = viewByKey(p.DefaultView || 'board/kanban').label;
              const stop = (e, path) => { e.stopPropagation(); this.goto(path); };
              return html`
                <div class="card" role="link" tabindex="0"
                     @click=${() => this.goto(dest)}
                     @keydown=${(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); this.goto(dest); } }}>
                  <h3>${p.Name}</h3>
                  <div class="muted">${p.Description || html`<span style="opacity:0.6">no description</span>`}</div>
                  ${p.Repos && p.Repos.length
                    ? html`<div class="repos">${p.Repos.map(r => html`<span>${r}</span>`)}</div>`
                    : null}
                  <div class="default-hint">Opens <strong>${destLabel}</strong></div>
                  <div class="actions">
                    <button @click=${(e) => stop(e, `/projects/${p.ID}/board/kanban`)}>Kanban</button>
                    <button @click=${(e) => stop(e, `/projects/${p.ID}/board/gantt`)}>Gantt</button>
                    <button @click=${(e) => stop(e, `/projects/${p.ID}/docs`)}>Docs</button>
                    <button @click=${(e) => stop(e, `/projects/${p.ID}/arch`)}>Architecture</button>
                    <button @click=${(e) => stop(e, `/projects/${p.ID}/settings`)}>Settings</button>
                  </div>
                </div>
              `;
            })}
          </div>
        `}
      ${this.showCreate ? this.renderCreateDialog() : null}
    `;
  }

  renderCreateDialog() {
    return html`
      <div class="dialog" @click=${(e) => e.target.classList.contains('dialog') && this.closeCreate()}>
        <div class="panel">
          <h3>New project</h3>
          ${this.error ? html`<div class="error">${this.error}</div>` : null}
          <form @submit=${(e) => this.submitCreate(e)}>
            <div class="field">
              <label>Name</label>
              <input name="name" required autofocus>
            </div>
            <div class="field">
              <label>Description</label>
              <input name="description">
            </div>
            <div class="field">
              <label>Primary language (optional)</label>
              <input name="primary_language" placeholder="go, typescript, python…">
            </div>
            <div class="field">
              <label>Project type (optional)</label>
              <input name="project_type" placeholder="web-app, cli-tool, library…">
            </div>
            <div class="field">
              <label>Repos (comma or newline separated, format owner/repo)</label>
              <textarea name="repos" rows="3"></textarea>
            </div>
            <div class="actions-row">
              <button type="button" @click=${() => this.closeCreate()}>Cancel</button>
              <button type="submit" class="primary" ?disabled=${this.creating}>
                ${this.creating ? 'Creating…' : 'Create'}
              </button>
            </div>
          </form>
        </div>
      </div>
    `;
  }
}

customElements.define('nottario-projects-page', NottarioProjectsPage);
