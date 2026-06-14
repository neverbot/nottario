import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { defaultPathFor, viewByKey } from '/static/views.js';
import { EscController } from '/static/components/esc.js';
import { toast } from '/static/components/toast.js';
import { formButton } from '/static/components/form-button.js';
import { buttonStyles } from '/static/components/buttons.js';
import { surfaceStyles, dialogStyles } from '/static/components/surfaces.js';
import { formStyles } from '/static/components/forms.js';
import '/static/components/field.js';
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

  static styles = [
    buttonStyles,
    surfaceStyles,
    dialogStyles,
    formStyles,
    css`
    :host { display: block; box-sizing: border-box; }
    * { box-sizing: border-box; }
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
      gap: 14px;
      /* Each card sizes to its own content. Without this, grid's
         default stretch forces every card in a row to the tallest
         one, so an empty project ends up as a wall of whitespace
         next to a populated card. */
      align-items: start;
    }
    .card {
      position: relative;
      display: flex;
      flex-direction: column;
      gap: 10px;
      padding: 16px 18px 14px;
      background: #fff;
      border: 1px solid var(--border);
      border-radius: 8px;
      box-shadow: 0 1px 0 rgba(31, 35, 40, 0.04);
      cursor: pointer;
      transition: border-color 80ms ease-out, box-shadow 80ms ease-out;
    }
    .card:hover {
      border-color: var(--border-strong);
      box-shadow: 0 1px 0 rgba(31, 35, 40, 0.04),
                  0 4px 12px rgba(31, 35, 40, 0.06);
    }
    .card:focus-visible {
      outline: 2px solid var(--accent);
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
      border: 1px solid var(--border);
      background: var(--bg-subtle);
      color: var(--fg-muted);
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
      color: var(--fg-muted);
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
      color: var(--fg);
      background: var(--bg-hover);
      border-color: var(--border);
    }
    .card .desc {
      color: var(--fg);
      font-size: 13px;
      line-height: 1.45;
      display: -webkit-box;
      -webkit-line-clamp: 2;
      line-clamp: 2;
      -webkit-box-orient: vertical;
      overflow: hidden;
      min-height: calc(1.45em * 2);
    }
    .card .desc.placeholder { color: var(--fg-muted); opacity: 0.7; font-style: italic; min-height: 0; }
    .card .stats {
      display: flex;
      gap: 14px;
      font-size: 12px;
      color: var(--fg-muted);
    }
    .card .stats .stat {
      display: inline-flex;
      align-items: baseline;
      gap: 4px;
    }
    .card .stats .stat .n {
      font-weight: 600;
      font-size: 13px;
      color: var(--fg);
      font-variant-numeric: tabular-nums;
    }
    .card .stats .stat.doing .n { color: var(--success); }
    .card .stats .activity {
      margin-left: auto;
      font-size: 11px;
      color: var(--fg-muted);
      white-space: nowrap;
    }
    /* "empty" chip lives next to the dest-chip in the top row when
       a project has no tasks yet. Quiet (no fill), so it doesn't
       compete with the kanban/gantt chip. */
    .card .dest-chip.empty-chip {
      color: var(--gray-5);
      background: transparent;
    }
    .card .meta {
      display: flex;
      /* Lang chip uses mono 11px, project-type chip uses inherited
         sans 12px. Different font metrics → different cap heights
         and ascender extents. align-items: baseline lines the text
         baselines so the row reads as one even with mixed faces. */
      align-items: baseline;
      gap: 4px 6px;
      flex-wrap: wrap;
      font-size: 12px;
      color: var(--fg-muted);
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
      color: var(--fg-muted);
      max-height: calc(1.5em * 3);
      overflow: hidden;
    }
    .card .repos .more { font-family: inherit; color: var(--gray-5); }
    .card .footer {
      display: flex;
      align-items: center;
      gap: 8px;
      margin-top: auto;
      padding-top: 6px;
      border-top: 1px solid var(--gray-2);
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
      color: var(--fg-muted);
    }
    .card .footer .spacer { flex: 1; }
    .error {
      color: var(--danger);
      font-size: 13px;
      margin-bottom: 8px;
    }
  `,
  ];

  constructor() {
    super();
    this.projects = null;
    this.showCreate = false;
    this.creating = false;
    this.error = '';
    new EscController(this, (e) => {
      if (this.showCreate) {
        this.closeCreate();
        e.stopPropagation();
      }
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
    const form = e.target;
    this.creating = true;
    this.error = '';
    const payload = {
      name: form.name.value.trim(),
      description: form.description.value.trim(),
      primary_language: form.primary_language.value.trim(),
      project_type: form.project_type.value.trim(),
      repos: form.repos.value
        .split(/\s*,\s*|\n+/)
        .map((s) => s.trim())
        .filter(Boolean),
    };
    try {
      await formButton(e, async () => {
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
      });
      toast.success(`Project "${payload.name}" created.`);
    } catch (err) {
      this.error = err.message;
      toast.error(`Couldn't create project: ${err.message}`);
    } finally {
      this.creating = false;
    }
  }

  goto(path) {
    window.nottarioNavigate(path);
  }

  _renderCard(p) {
    const dest = defaultPathFor(p);
    const destLabel = viewByKey(p.default_view || 'board/kanban').label;
    const stop = (e, path) => {
      e.stopPropagation();
      this.goto(path);
    };
    const stopOnly = (e) => {
      e.stopPropagation();
    };

    const meta = [];
    if (p.primary_language) meta.push(html`<span class="lang">${p.primary_language}</span>`);
    if (p.project_type) meta.push(html`<span class="ptype">${p.project_type}</span>`);
    const metaWithSeps = meta.flatMap((node, i) =>
      i === 0 ? [node] : [html`<span class="sep">·</span>`, node],
    );

    return html`
      <div class="card" role="link" tabindex="0"
           @click=${() => this.goto(dest)}
           @keydown=${(e) => {
             if (e.key === 'Enter' || e.key === ' ') {
               e.preventDefault();
               this.goto(dest);
             }
           }}>
        <div class="top">
          <h3 title=${p.name}>${p.name}</h3>
          <span class="dest-chip" title="Default view">${destLabel}</span>
          ${
            p.stats && p.stats.todo_count + p.stats.doing_count + p.stats.done_count === 0
              ? html`<span class="dest-chip empty-chip" title="No tasks yet">empty</span>`
              : null
          }
          ${
            this.me?.is_admin
              ? html`<button class="settings-link" title="Project settings"
                          aria-label="Settings"
                          @click=${(e) => stop(e, `/projects/${p.id}/settings`)}
                          @keydown=${stopOnly}>⚙</button>`
              : null
          }
        </div>
        <div class=${p.description ? 'desc' : 'desc placeholder'} title=${p.description || ''}>
          ${p.description || 'No description'}
        </div>
        ${this._renderStats(p)}
        ${meta.length ? html`<div class="meta">${metaWithSeps}</div>` : null}
        ${this._renderRepos(p)}
        ${this._renderFooter(p)}
      </div>
    `;
  }

  _renderStats(p) {
    const s = p.stats;
    if (!s) return null;
    const total = s.todo_count + s.doing_count + s.done_count;
    // Empty backlog: render nothing. The "empty" chip in the top
    // row already conveys the state, and skipping the stats line
    // keeps the card honestly short instead of inflating it with
    // a placeholder that grows the whole grid row.
    if (total === 0) return null;
    return html`
      <div class="stats">
        <span class="stat"><span class="n">${s.todo_count}</span> todo</span>
        <span class="stat doing"><span class="n">${s.doing_count}</span> doing</span>
        <span class="stat"><span class="n">${s.done_count}</span> done</span>
        ${
          s.last_activity_at
            ? html`<span class="activity" title=${new Date(s.last_activity_at).toLocaleString()}>
                   ${this._relativeTime(s.last_activity_at)}
                 </span>`
            : null
        }
      </div>
    `;
  }

  _renderRepos(p) {
    const repos = p.repos || [];
    if (repos.length === 0) return null;
    const shown = repos.slice(0, 3);
    const extra = repos.length - shown.length;
    return html`
      <div class="repos">
        ${shown.map((r) => html`<span>${r}</span>`)}
        ${extra > 0 ? html`<span class="more">+${extra} more</span>` : null}
      </div>
    `;
  }

  _renderFooter(p) {
    const members = p.members || [];
    if (members.length === 0) return null;
    const shown = members.slice(0, 5);
    const extra = members.length - shown.length;
    return html`
      <div class="footer">
        <div class="avatars">
          ${shown.map(
            (m) => html`
            <nottario-avatar
              .src=${m.avatar_url || ''}
              .name=${m.display_name || m.github_login || ''}
              .size=${22}
              title=${m.display_name || m.github_login}></nottario-avatar>`,
          )}
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
        ${
          this.me?.is_admin
            ? html`<button slot="actions" class="btn primary"
                         @click=${() => this.openCreate()}>New project</button>`
            : null
        }
      </nottario-page-header>
      ${
        this.projects.length === 0
          ? html`<div class="empty">
            <strong>No projects yet.</strong>
            ${
              this.me?.is_admin
                ? html`Click <strong>New project</strong> to seed one with default roles, priorities and an empty backlog.`
                : html`Ask an admin to add you to one, or to create the first project.`
            }
          </div>`
          : html`
          <div class="grid">
            ${this.projects.map((p) => this._renderCard(p))}
          </div>
        `
      }
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
            <nottario-field label="Name">
              <input name="name" required autofocus>
            </nottario-field>
            <nottario-field label="Description">
              <input name="description">
            </nottario-field>
            <nottario-field label="Primary language" hint="optional">
              <input name="primary_language" placeholder="go, typescript, python…">
            </nottario-field>
            <nottario-field label="Project type" hint="optional">
              <input name="project_type" placeholder="web-app, cli-tool, library…">
            </nottario-field>
            <nottario-field label="Repos" hint="comma or newline separated, format owner/repo">
              <textarea name="repos" rows="3"></textarea>
            </nottario-field>
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
