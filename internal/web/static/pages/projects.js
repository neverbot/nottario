import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { defaultPathFor, viewByKey } from '/static/views.js';
import { EscController } from '/static/components/esc.js';
import { buttonStyles } from '/static/components/buttons.js';
import { surfaceStyles, dialogStyles } from '/static/components/surfaces.js';
import { fieldStyles } from '/static/components/fields.js';
import '/static/components/avatar.js';
import '/static/components/page-header.js';

class NottarioProjectsPage extends LitElement {
  static properties = {
    me: { type: Object },
    projects: { state: true },
    showCreate: { state: true },
    creating: { state: true },
    error: { state: true },
  };

  static styles = [buttonStyles, surfaceStyles, dialogStyles, fieldStyles, css`
    :host { display: block; box-sizing: border-box; }
    * { box-sizing: border-box; }
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
      gap: 14px;
    }
    .card {
      position: relative;
      display: flex;
      flex-direction: column;
      gap: 10px;
      padding: 16px 18px 14px;
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
    .card .top {
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .card h3 {
      margin: 0;
      font-size: 16px;
      font-weight: 600;
      flex: 1;
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .card .dest-chip {
      display: inline-flex;
      align-items: center;
      height: 22px;
      padding: 0 8px;
      border-radius: 999px;
      border: 1px solid #d0d7de;
      background: #f6f8fa;
      color: #59636e;
      font-size: 11px;
      font-weight: 500;
      letter-spacing: 0.02em;
      white-space: nowrap;
    }
    .card .settings-link {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 26px;
      height: 26px;
      border-radius: 6px;
      color: #59636e;
      text-decoration: none;
      border: 1px solid transparent;
      background: transparent;
      font: inherit;
      font-size: 14px;
      line-height: 1;
      cursor: pointer;
      padding: 0;
    }
    .card .settings-link:hover {
      color: #1f2328;
      background: #f3f4f6;
      border-color: #d0d7de;
    }
    .card .desc {
      color: #1f2328;
      font-size: 13px;
      line-height: 1.45;
      display: -webkit-box;
      -webkit-line-clamp: 2;
      line-clamp: 2;
      -webkit-box-orient: vertical;
      overflow: hidden;
      min-height: calc(1.45em * 2);
    }
    .card .desc.placeholder { color: #59636e; opacity: 0.7; font-style: italic; min-height: 0; }
    .card .stats {
      display: flex;
      gap: 14px;
      font-size: 12px;
      color: #59636e;
    }
    .card .stats .stat {
      display: inline-flex;
      align-items: baseline;
      gap: 4px;
    }
    .card .stats .stat .n {
      font-weight: 600;
      font-size: 13px;
      color: #1f2328;
      font-variant-numeric: tabular-nums;
    }
    .card .stats .stat.doing .n { color: #1f883d; }
    .card .stats .activity {
      margin-left: auto;
      font-size: 11px;
      color: #59636e;
      white-space: nowrap;
    }
    .card .stats.empty { font-style: italic; opacity: 0.7; }
    .card .meta {
      display: flex;
      gap: 4px 6px;
      flex-wrap: wrap;
      font-size: 12px;
      color: #59636e;
    }
    .card .meta .sep { opacity: 0.5; }
    .card .meta .lang,
    .card .meta .ptype { white-space: nowrap; }
    .card .meta .lang {
      font-family: ui-monospace, SFMono-Regular, "SF Mono", monospace;
      font-size: 11px;
    }
    .card .repos {
      display: flex;
      flex-direction: column;
      gap: 2px;
      font-family: ui-monospace, SFMono-Regular, "SF Mono", monospace;
      font-size: 11px;
      color: #59636e;
      max-height: calc(1.5em * 3);
      overflow: hidden;
    }
    .card .repos .more { font-family: inherit; color: #8b949e; }
    .card .footer {
      display: flex;
      align-items: center;
      gap: 8px;
      margin-top: auto;
      padding-top: 6px;
      border-top: 1px solid #eaeef2;
    }
    .card .avatars {
      display: inline-flex;
      align-items: center;
    }
    /* Stack: each avatar overlaps the previous by 6px, framed by a
       2px white border so the circles remain distinguishable. */
    .card .avatars nottario-avatar {
      border: 2px solid #fff;
      margin-left: -6px;
    }
    .card .avatars nottario-avatar:first-child { margin-left: 0; }
    .card .avatars .more {
      margin-left: 6px;
      font-size: 11px;
      color: #59636e;
    }
    .card .footer .spacer { flex: 1; }
    .error {
      color: #cf222e;
      font-size: 13px;
      margin-bottom: 8px;
    }
  `];

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

  _renderCard(p) {
    const dest = defaultPathFor(p);
    const destLabel = viewByKey(p.DefaultView || 'board/kanban').label;
    const stop = (e, path) => { e.stopPropagation(); this.goto(path); };
    const stopOnly = (e) => { e.stopPropagation(); };

    const meta = [];
    if (p.PrimaryLanguage) meta.push(html`<span class="lang">${p.PrimaryLanguage}</span>`);
    if (p.ProjectType) meta.push(html`<span class="ptype">${p.ProjectType}</span>`);
    const metaWithSeps = meta.flatMap((node, i) =>
      i === 0 ? [node] : [html`<span class="sep">·</span>`, node]);

    return html`
      <div class="card" role="link" tabindex="0"
           @click=${() => this.goto(dest)}
           @keydown=${(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); this.goto(dest); } }}>
        <div class="top">
          <h3 title=${p.Name}>${p.Name}</h3>
          <span class="dest-chip" title="Default view">${destLabel}</span>
          ${this.me?.is_admin
            ? html`<button class="settings-link" title="Project settings"
                          aria-label="Settings"
                          @click=${(e) => stop(e, `/projects/${p.ID}/settings`)}
                          @keydown=${stopOnly}>⚙</button>`
            : null}
        </div>
        <div class=${p.Description ? 'desc' : 'desc placeholder'} title=${p.Description || ''}>
          ${p.Description || 'No description'}
        </div>
        ${this._renderStats(p)}
        ${meta.length ? html`<div class="meta">${metaWithSeps}</div>` : null}
        ${this._renderRepos(p)}
        ${this._renderFooter(p)}
      </div>
    `;
  }

  _renderStats(p) {
    const s = p.Stats;
    if (!s) return null;
    const total = s.TodoCount + s.DoingCount + s.DoneCount;
    if (total === 0) {
      return html`<div class="stats empty"><span>Empty backlog.</span></div>`;
    }
    return html`
      <div class="stats">
        <span class="stat"><span class="n">${s.TodoCount}</span> todo</span>
        <span class="stat doing"><span class="n">${s.DoingCount}</span> doing</span>
        <span class="stat"><span class="n">${s.DoneCount}</span> done</span>
        ${s.LastActivityAt
          ? html`<span class="activity" title=${new Date(s.LastActivityAt).toLocaleString()}>
                   ${this._relativeTime(s.LastActivityAt)}
                 </span>`
          : null}
      </div>
    `;
  }

  _renderRepos(p) {
    const repos = p.Repos || [];
    if (repos.length === 0) return null;
    const shown = repos.slice(0, 3);
    const extra = repos.length - shown.length;
    return html`
      <div class="repos">
        ${shown.map(r => html`<span>${r}</span>`)}
        ${extra > 0 ? html`<span class="more">+${extra} more</span>` : null}
      </div>
    `;
  }

  _renderFooter(p) {
    const members = p.Members || [];
    if (members.length === 0) return null;
    const shown = members.slice(0, 5);
    const extra = members.length - shown.length;
    return html`
      <div class="footer">
        <div class="avatars">
          ${shown.map(m => html`
            <nottario-avatar
              .src=${m.AvatarURL || ''}
              .name=${m.DisplayName || m.GithubLogin || ''}
              .size=${22}
              title=${m.DisplayName || m.GithubLogin}></nottario-avatar>`)}
          ${extra > 0 ? html`<span class="more">+${extra}</span>` : null}
        </div>
        <div class="spacer"></div>
      </div>
    `;
  }

  // Tiny relative-time formatter: "5m", "3h", "2d", "3w". Falls back
  // to a date for anything older than ~12 weeks. No new dep.
  _relativeTime(iso) {
    const then = new Date(iso).getTime();
    const diff = Date.now() - then;
    if (diff < 60_000) return 'just now';
    const m = Math.floor(diff / 60_000);
    if (m < 60) return `${m}m ago`;
    const h = Math.floor(m / 60);
    if (h < 24) return `${h}h ago`;
    const d = Math.floor(h / 24);
    if (d < 7) return `${d}d ago`;
    const w = Math.floor(d / 7);
    if (w < 12) return `${w}w ago`;
    return new Date(iso).toLocaleDateString();
  }

  render() {
    if (this.projects === null) {
      return html`<div class="empty">Loading projects…</div>`;
    }
    return html`
      <nottario-page-header title="Projects">
        ${this.me?.is_admin
          ? html`<button slot="actions" class="btn primary"
                         @click=${() => this.openCreate()}>New project</button>`
          : null}
      </nottario-page-header>
      ${this.projects.length === 0
        ? html`<div class="empty">
            <strong>No projects yet.</strong>
            ${this.me?.is_admin
              ? html`Click <strong>New project</strong> to seed one with default roles, priorities and an empty backlog.`
              : html`Ask an admin to add you to one, or to create the first project.`}
          </div>`
        : html`
          <div class="grid">
            ${this.projects.map(p => this._renderCard(p))}
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
              <button type="button" class="btn secondary" @click=${() => this.closeCreate()}>Cancel</button>
              <button type="submit" class="btn primary" ?disabled=${this.creating}>
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
