import { LitElement, html, css, nothing } from '/static/vendor/lit/lit.js';
import { subscribe } from '/static/realtime.js';
import { buttonStyles } from '/static/components/buttons.js';
import '/static/components/page-header.js';
import '/static/components/segmented-control.js';
import '/static/components/markdown.js';
import '/static/components/search-input.js';
import '/static/components/task-chip.js';
import './arch-graph.js';

class NottarioArchPage extends LitElement {
  static properties = {
    me: { type: Object },
    projectId: { type: String },
    // 'diagram' (default, was 'graph') or 'tree'. Driven by URL.
    view: { type: String },
    project: { state: true },
    kinds: { state: true },
    rootNodes: { state: true },
    // slug → node (every node, flat, for parent/ancestor walks).
    nodeBySlug: { state: true },
    // parentSlug → ChildNode[]. Built once at load.
    childrenBySlug: { state: true },
    selectedSlug: { state: true },
    selectedDetail: { state: true },
    expanded: { state: true },
    // taskId → task. Lazy-filled when the reader needs to resolve
    // Linked items.
    taskCache: { state: true },
    filter: { state: true },
    error: { state: true },
  };

  static styles = [
    buttonStyles,
    css`
    :host { display: block; }
    .header h2 { margin: 0; }
    .header .muted { color: #59636e; }

    /* Two-column desktop. Single-column mobile (<720px): only one
       of sidebar/reader visible at a time, switched by the
       selection state. */
    .layout {
      display: grid;
      grid-template-columns: 340px 1fr;
      gap: 16px;
      min-height: 70vh;
    }
    .sidebar, .reader {
      background: #fff;
      border: 1px solid #d1d9e0;
      border-radius: 8px;
      padding: 12px;
      overflow: auto;
      max-height: 80vh;
    }
    .sidebar-head {
      display: flex;
      gap: 8px;
      margin-bottom: 8px;
      align-items: center;
    }
    .sidebar-head strong { font-size: 14px; }
    .sidebar-head .muted {
      color: #59636e;
      margin-left: auto;
      font-size: 11px;
    }
    .filter-row { margin-bottom: 8px; }
    .filter-row nottario-search-input { width: 100%; }

    /* Tree */
    .tree { list-style: none; padding-left: 0; margin: 0; }
    .tree ul { list-style: none; padding-left: 14px; margin: 0; }
    [role="treeitem"] { outline: none; }
    [role="treeitem"]:focus-visible > .node {
      box-shadow: 0 0 0 2px var(--color-focus-ring, #0969da);
    }
    .node {
      display: flex;
      align-items: center;
      gap: 6px;
      padding: 4px 6px;
      border-radius: 6px;
      cursor: pointer;
      line-height: 1.4;
      transition: background 100ms ease-out;
    }
    /* Leaves get a visual indent equal to (toggle 14px + gap 6px)
       so their names line up with siblings that DO have a toggle. */
    .node.leaf { padding-left: 26px; }
    .node:hover { background: #f6f8fa; }
    .node.active {
      background: #ddf4ff;
      color: #0969da;
    }
    .node.active .name { font-weight: 600; }
    .node.dim { opacity: 0.35; }

    .toggle {
      flex: 0 0 14px;
      text-align: center;
      color: #59636e;
      user-select: none;
      font-family: ui-monospace, monospace;
      font-size: 11px;
      background: transparent;
      border: 0;
      cursor: pointer;
      padding: 0;
      line-height: 1;
    }
    .toggle:focus-visible {
      outline: 2px solid var(--color-focus-ring, #0969da);
      outline-offset: 1px;
      border-radius: 3px;
    }
    .kind-dot {
      display: inline-block;
      flex: 0 0 8px;
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: #999;
    }
    .name {
      font-size: 14px;
      font-weight: 500;
      color: #1f2328;
    }
    .node.active .name { color: #0969da; }
    .kind-label {
      font-size: 10.5px;
      color: #8b949e;
      text-transform: uppercase;
      letter-spacing: 0.04em;
      margin-left: 2px;
    }

    /* Reader */
    .reader {
      display: flex;
      flex-direction: column;
    }
    .reader.empty {
      align-items: center;
      justify-content: center;
      text-align: center;
      color: #59636e;
      padding: 32px;
      gap: 4px;
    }
    .reader.empty h3 {
      margin: 0;
      font-size: 15px;
      font-weight: 600;
      color: #1f2328;
    }
    .reader.empty p { margin: 0; max-width: 36ch; line-height: 1.5; }

    .reader-back {
      display: none;
      margin: -4px 0 8px;
      background: transparent;
      border: 0;
      color: #0969da;
      cursor: pointer;
      padding: 4px 6px;
      border-radius: 6px;
      font: inherit;
      align-self: flex-start;
    }
    .reader-back:hover { background: #f6f8fa; }

    .reader header {
      display: flex;
      align-items: baseline;
      gap: 8px;
      flex-wrap: wrap;
      border-bottom: 1px solid #eaeef2;
      padding-bottom: 8px;
      margin-bottom: 12px;
    }
    .reader header h3 {
      margin: 0;
      font-size: 18px;
      font-weight: 600;
      line-height: 1.3;
    }
    .reader .meta { color: #59636e; font-size: 12px; }
    .kind-pill {
      display: inline-flex;
      align-items: center;
      gap: 4px;
      padding: 1px 8px;
      border-radius: 6px;
      font-size: 11px;
      font-weight: 600;
      background: #eaeef2;
      color: #1f2328;
      letter-spacing: 0.02em;
    }
    .reader .meta-grid {
      display: grid;
      grid-template-columns: max-content 1fr;
      gap: 6px 16px;
      font-size: 13px;
      margin-bottom: 12px;
    }
    .reader .meta-grid > div:nth-child(odd) {
      color: #59636e;
      font-size: 11px;
      text-transform: uppercase;
      letter-spacing: 0.04em;
    }
    .reader .section { margin-top: 16px; }
    .reader .section h4 {
      margin: 0 0 8px 0;
      font-size: 11px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.04em;
      color: #59636e;
    }
    .reader .edge-line {
      padding: 6px 8px;
      border-radius: 6px;
      font-size: 13px;
      display: flex;
      align-items: center;
      gap: 6px;
      flex-wrap: wrap;
    }
    .reader .edge-line + .edge-line { margin-top: 2px; }
    .reader .edge-line:hover { background: #f6f8fa; cursor: pointer; }
    .reader .edge-line .kind {
      font-family: ui-monospace, monospace;
      font-size: 11.5px;
      color: #59636e;
    }
    .reader .edge-line .label {
      color: #59636e;
      font-size: 12px;
    }
    .reader .edge-line .endpoint {
      color: #0969da;
      cursor: pointer;
      font-weight: 500;
    }
    .reader .edge-line .endpoint:hover { text-decoration: underline; }
    .arrow { color: #8b949e; }

    .linked-items {
      display: flex;
      flex-direction: column;
      gap: 6px;
    }
    .doc-link {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 4px 10px;
      border-radius: 6px;
      background: #f6f8fa;
      border: 1px solid #d1d9e0;
      font-family: ui-monospace, monospace;
      font-size: 12px;
      color: #1f2328;
      text-decoration: none;
      width: max-content;
      max-width: 100%;
    }
    .doc-link:hover { background: #eaeef2; }
    .doc-link::before {
      content: '';
      width: 6px;
      height: 6px;
      border-radius: 50%;
      background: #6e7781;
    }

    .empty-pane {
      color: #8b949e;
      font-size: 13px;
      font-style: italic;
      margin: 0;
    }

    .error {
      color: #cf222e;
      background: #ffebe9;
      padding: 8px 12px;
      border-radius: 6px;
      margin-bottom: 8px;
    }
    code { font-size: 12px; }

    /* Mobile: single column. The sidebar and reader collapse to the
       same cell; selection state drives which one is shown. */
    @media (max-width: 720px) {
      .layout {
        grid-template-columns: 1fr;
      }
      .layout[data-mode="reader"] .sidebar { display: none; }
      .layout[data-mode="list"]   .reader  { display: none; }
      .reader-back { display: inline-flex; }
      .sidebar, .reader { max-height: none; }
    }
  `,
  ];

  constructor() {
    super();
    this.view = 'diagram';
    this.project = null;
    this.kinds = [];
    this.rootNodes = null;
    this.nodeBySlug = {};
    this.childrenBySlug = {};
    this.selectedSlug = null;
    this.selectedDetail = null;
    this.expanded = {};
    this.taskCache = {};
    this.filter = '';
    this.error = '';
    this._reduced =
      typeof window !== 'undefined' && window.matchMedia
        ? window.matchMedia('(prefers-reduced-motion: reduce)').matches
        : false;
  }

  connectedCallback() {
    super.connectedCallback();
    this.load().then(() => this._applyHash());
    this._subscribe();
    this._hashHandler = () => this._applyHash();
    window.addEventListener('hashchange', this._hashHandler);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this._unsub?.();
    window.removeEventListener('hashchange', this._hashHandler);
  }

  updated(c) {
    if (c.has('projectId')) {
      this.load().then(() => this._applyHash());
      this._subscribe();
    }
    // Whenever the selected slug or root data changes, scroll the
    // selected row into view in the sidebar.
    if (c.has('selectedSlug') || c.has('rootNodes')) {
      this._scrollSelectedIntoView();
    }
  }

  _applyHash() {
    const h = new URLSearchParams(window.location.hash.slice(1));
    const slug = h.get('node');
    if (slug) this.select(slug);
  }

  _subscribe() {
    this._unsub?.();
    if (!this.projectId) return;
    this._unsub = subscribe(this.projectId, (ev) => {
      if (ev.type !== 'realtime.reconnected' && !ev.type?.startsWith('arch.')) return;
      this.load();
      if (this.selectedSlug) {
        fetch(`/api/projects/${this.projectId}/arch/nodes/${encodeURIComponent(this.selectedSlug)}`)
          .then((r) => (r.ok ? r.json() : null))
          .then((d) => {
            if (d) this.selectedDetail = d;
          });
      }
    });
  }

  async load() {
    if (!this.projectId) return;
    try {
      const [pr, kr, nr] = await Promise.all([
        fetch(`/api/projects/${this.projectId}`),
        fetch(`/api/projects/${this.projectId}/arch/kinds`),
        fetch(`/api/projects/${this.projectId}/arch/nodes`),
      ]);
      if (!pr.ok) throw new Error('project not found');
      this.project = await pr.json();
      this.kinds = (await kr.json()).kinds || [];
      const all = (await nr.json()).nodes || [];
      const byId = new Map(all.map((n) => [n.ID, n]));
      const bySlug = {};
      const children = {};
      const roots = [];
      for (const n of all) {
        bySlug[n.Slug] = n;
        if (n.ParentID) {
          const parent = byId.get(n.ParentID);
          if (parent) {
            if (!children[parent.Slug]) children[parent.Slug] = [];
            children[parent.Slug].push(n);
            continue;
          }
        }
        roots.push(n);
      }
      const sortFn = (a, b) =>
        a.Position - b.Position || (a.Name || '').localeCompare(b.Name || '');
      roots.sort(sortFn);
      for (const k of Object.keys(children)) children[k].sort(sortFn);
      this.nodeBySlug = bySlug;
      this.rootNodes = roots;
      this.childrenBySlug = children;
    } catch (e) {
      this.error = e.message;
    }
  }

  kindByKey(key) {
    return this.kinds.find((k) => k.Key === key);
  }
  hasChildren(slug) {
    return (this.childrenBySlug[slug] || []).length > 0;
  }

  // Flat list of visible rows in tree order, used for keyboard nav.
  _visibleRows(nodes = this.rootNodes || [], depth = 1, out = []) {
    for (const n of nodes) {
      out.push({ node: n, depth });
      if (this.expanded[n.Slug]) {
        const kids = this.childrenBySlug[n.Slug] || [];
        if (kids.length) this._visibleRows(kids, depth + 1, out);
      }
    }
    return out;
  }

  _focusRow(slug) {
    if (!slug) return;
    requestAnimationFrame(() => {
      const el = this.shadowRoot?.querySelector(`[data-slug="${CSS.escape(slug)}"]`);
      if (el && el.getAttribute('role') === 'treeitem') {
        el.focus();
      } else {
        // The row is hidden — find the closest visible ancestor instead.
        const ancestors = this._ancestorSlugs(slug);
        for (const a of ancestors) {
          const ael = this.shadowRoot?.querySelector(`[data-slug="${CSS.escape(a)}"]`);
          if (ael?.getAttribute('role') === 'treeitem') {
            ael.focus();
            break;
          }
        }
      }
    });
  }

  // Single keydown handler on the whole tree. Implements WAI-ARIA
  // tree navigation: arrows, Home/End, Enter/Space, type-ahead.
  _onTreeKey(e) {
    const target = e.composedPath().find((el) => el?.getAttribute?.('role') === 'treeitem');
    if (!target) return;
    const slug = target.dataset.slug;
    const rows = this._visibleRows();
    const idx = rows.findIndex((r) => r.node.Slug === slug);
    if (idx < 0) return;
    const cur = rows[idx];

    const moveTo = (newIdx) => {
      if (newIdx < 0 || newIdx >= rows.length) return;
      e.preventDefault();
      this._focusRow(rows[newIdx].node.Slug);
    };

    switch (e.key) {
      case 'ArrowDown':
        moveTo(idx + 1);
        return;
      case 'ArrowUp':
        moveTo(idx - 1);
        return;
      case 'Home':
        moveTo(0);
        return;
      case 'End':
        moveTo(rows.length - 1);
        return;
      case 'Enter':
      case ' ':
        e.preventDefault();
        this.select(slug);
        return;
      case 'ArrowRight':
        if (!this.hasChildren(slug)) return;
        if (!this.expanded[slug]) {
          e.preventDefault();
          this.expanded = { ...this.expanded, [slug]: true };
        } else {
          // Move to first child.
          moveTo(idx + 1);
        }
        return;
      case 'ArrowLeft':
        if (this.expanded[slug] && this.hasChildren(slug)) {
          e.preventDefault();
          this.expanded = { ...this.expanded, [slug]: false };
          return;
        }
        // Else move to parent. Walk rows backwards looking for a
        // shallower depth.
        for (let i = idx - 1; i >= 0; i--) {
          if (rows[i].depth < cur.depth) {
            moveTo(i);
            return;
          }
        }
        return;
      default:
        // Type-ahead: single printable character → first visible row
        // whose name starts with it (case-insensitive), starting
        // after the current row.
        if (e.key.length === 1 && /\S/.test(e.key)) {
          const ch = e.key.toLowerCase();
          for (let step = 1; step <= rows.length; step++) {
            const cand = rows[(idx + step) % rows.length];
            if ((cand.node.Name || '').toLowerCase().startsWith(ch)) {
              e.preventDefault();
              this._focusRow(cand.node.Slug);
              return;
            }
          }
        }
    }
  }

  // Walk parents from a node up to a root, returning the slug chain
  // (excluding the node itself).
  _ancestorSlugs(slug) {
    const out = [];
    let cur = this.nodeBySlug[slug];
    while (cur && cur.ParentID) {
      const parent = Object.values(this.nodeBySlug).find((n) => n.ID === cur.ParentID);
      if (!parent) break;
      out.push(parent.Slug);
      cur = parent;
    }
    return out;
  }

  toggle(slug) {
    if (!this.hasChildren(slug)) return;
    this.expanded = { ...this.expanded, [slug]: !this.expanded[slug] };
  }

  async select(slug) {
    this.selectedSlug = slug;
    // Auto-expand the ancestor chain so the selected row is
    // actually visible in the tree.
    const ancestors = this._ancestorSlugs(slug);
    if (ancestors.length) {
      const next = { ...this.expanded };
      for (const a of ancestors) next[a] = true;
      this.expanded = next;
    }
    try {
      const r = await fetch(
        `/api/projects/${this.projectId}/arch/nodes/${encodeURIComponent(slug)}`,
      );
      if (!r.ok) throw new Error('node not found');
      this.selectedDetail = await r.json();
      // Pre-fetch any task-linked items so the reader can render
      // chips with titles instead of bare UUIDs.
      const taskIds = (this.selectedDetail?.links || [])
        .filter((l) => l.LinkType === 'task')
        .map((l) => l.TargetID)
        .filter((id) => !this.taskCache[id]);
      if (taskIds.length) {
        const fetched = await Promise.all(
          taskIds.map(async (id) => {
            const tr = await fetch(`/api/projects/${this.projectId}/tasks/${id}`);
            return tr.ok ? [id, await tr.json()] : null;
          }),
        );
        const next = { ...this.taskCache };
        for (const e of fetched) if (e) next[e[0]] = e[1];
        this.taskCache = next;
      }
    } catch (e) {
      this.error = e.message;
    }
  }

  _backToList() {
    this.selectedSlug = null;
    this.selectedDetail = null;
  }

  _onFilterInput(e) {
    this.filter = (e.detail?.value || '').trim().toLowerCase();
    // Auto-expand ancestors of every match so they're visible.
    if (!this.filter) return;
    const next = { ...this.expanded };
    for (const slug of Object.keys(this.nodeBySlug)) {
      if (this._matchesFilter(this.nodeBySlug[slug])) {
        for (const a of this._ancestorSlugs(slug)) next[a] = true;
      }
    }
    this.expanded = next;
  }

  _matchesFilter(n) {
    if (!this.filter) return true;
    return (
      (n.Name || '').toLowerCase().includes(this.filter) ||
      (n.Slug || '').toLowerCase().includes(this.filter)
    );
  }

  // A node is "visible" (full opacity) if it matches or if any of
  // its descendants match. Otherwise it's dim.
  _isHighlighted(n) {
    if (!this.filter) return true;
    if (this._matchesFilter(n)) return true;
    for (const c of this.childrenBySlug[n.Slug] || []) {
      if (this._isHighlighted(c)) return true;
    }
    return false;
  }

  _scrollSelectedIntoView() {
    if (!this.selectedSlug) return;
    requestAnimationFrame(() => {
      const row = this.shadowRoot?.querySelector(`[data-slug="${CSS.escape(this.selectedSlug)}"]`);
      if (!row) return;
      row.scrollIntoView({
        block: 'nearest',
        behavior: this._reduced ? 'auto' : 'smooth',
      });
    });
  }

  // Single row of the tree. The <li> itself is the focus target
  // (role="treeitem"), so screen readers see depth, expansion, and
  // selection state on one element.
  renderNode(n, depth = 1) {
    const children = this.childrenBySlug[n.Slug] || [];
    const isLeaf = children.length === 0;
    const open = !isLeaf && !!this.expanded[n.Slug];
    const kind = this.kindByKey(n.Kind);
    const isDim = !this._isHighlighted(n);
    const isSelected = this.selectedSlug === n.Slug;
    // Roving tabindex: only the currently-selected row sits in the
    // tab order. The first root takes that slot when nothing is
    // selected, so keyboard users can always Tab into the tree.
    const isFocusTarget =
      isSelected || (!this.selectedSlug && depth === 1 && this.rootNodes?.[0]?.Slug === n.Slug);
    const cls = ['node', isLeaf ? 'leaf' : '', isSelected ? 'active' : '', isDim ? 'dim' : '']
      .filter(Boolean)
      .join(' ');
    return html`
      <li role="treeitem"
          data-slug=${n.Slug}
          aria-level=${depth}
          aria-selected=${isSelected ? 'true' : 'false'}
          aria-expanded=${isLeaf ? nothing : open ? 'true' : 'false'}
          tabindex=${isFocusTarget ? '0' : '-1'}>
        <div class=${cls}>
          ${
            isLeaf
              ? null
              : html`<button class="toggle"
                  type="button"
                  tabindex="-1"
                  aria-label=${open ? `Collapse ${n.Name}` : `Expand ${n.Name}`}
                  @click=${(e) => {
                    e.stopPropagation();
                    this.toggle(n.Slug);
                  }}
            >${open ? '▾' : '▸'}</button>`
          }
          <span class="kind-dot" style=${`background: ${kind?.Color || '#999'}`} title=${kind?.Label || n.Kind} aria-hidden="true"></span>
          <span class="name" @click=${() => this.select(n.Slug)}>${n.Name}</span>
          <span class="kind-label">${n.Kind}</span>
        </div>
        ${
          open
            ? html`
          <ul role="group">${children.map((c) => this.renderNode(c, depth + 1))}</ul>
        `
            : null
        }
      </li>
    `;
  }

  renderSidebar() {
    if (this.rootNodes === null) return html`<div class="sidebar">Loading…</div>`;
    return html`
      <div class="sidebar">
        <div class="sidebar-head">
          <strong>Architecture</strong>
          <span class="muted">read only</span>
        </div>
        <div class="filter-row">
          <nottario-search-input
            placeholder="Filter…"
            .value=${this.filter}
            @input=${(e) => this._onFilterInput(e)}>
          </nottario-search-input>
        </div>
        ${
          this.rootNodes.length === 0
            ? html`
          <p class="empty-pane">No architecture defined yet. Ask an agent to start with <code>nottario.arch.upsert_node</code>.</p>
        `
            : html`
          <ul class="tree" role="tree" aria-label="Architecture tree"
              @keydown=${(e) => this._onTreeKey(e)}>
            ${this.rootNodes.map((n) => this.renderNode(n))}
          </ul>
        `
        }
      </div>
    `;
  }

  renderReader() {
    if (!this.selectedDetail) {
      return html`
        <div class="reader empty">
          <h3>Pick a node</h3>
          <p>Select anything on the left to see its description, edges and the tasks or docs it links to.</p>
        </div>`;
    }
    const { node, children, edges, links } = this.selectedDetail;
    const kind = this.kindByKey(node.Kind);
    const incoming = edges.filter((e) => e.ToSlug === node.Slug);
    const outgoing = edges.filter((e) => e.FromSlug === node.Slug);
    const taskLinks = (links || []).filter((l) => l.LinkType === 'task');
    const docLinks = (links || []).filter((l) => l.LinkType === 'doc');
    return html`
      <div class="reader">
        <button class="reader-back" @click=${() => this._backToList()}>← Back to list</button>
        <header>
          <h3>${node.Name}</h3>
          <span class="kind-pill" style=${`background: ${kind?.Color || '#eaeef2'}1a; color: ${kind?.Color || '#1f2328'}`}>${kind?.Label || node.Kind}</span>
          <div style="flex:1"></div>
          <span class="meta"><code>${node.Slug}</code></span>
        </header>
        <div class="meta-grid">
          ${node.LinkedRepo ? html`<div>Repo</div><div><code>${node.LinkedRepo}</code></div>` : null}
          ${node.LinkedPath ? html`<div>Path</div><div><code>${node.LinkedPath}</code></div>` : null}
          ${
            Object.keys(node.Metadata || {}).length
              ? html`
            <div>Metadata</div>
            <div><code>${JSON.stringify(node.Metadata)}</code></div>
          `
              : null
          }
        </div>
        ${
          node.DescriptionMD
            ? html`
          <div class="section">
            <h4>Description</h4>
            <nottario-markdown
              project-id=${this.projectId}
              .source=${node.DescriptionMD}></nottario-markdown>
          </div>
        `
            : null
        }
        ${
          children && children.length
            ? html`
          <div class="section">
            <h4>Children (${children.length})</h4>
            ${children.map(
              (c) => html`
              <div class="edge-line" @click=${() => this.select(c.Slug)}>
                <span class="kind-dot" style=${`background: ${this.kindByKey(c.Kind)?.Color || '#999'}`}></span>
                <span class="endpoint">${c.Name}</span>
                <span class="kind-label">${c.Kind}</span>
              </div>
            `,
            )}
          </div>
        `
            : null
        }
        ${
          outgoing.length
            ? html`
          <div class="section">
            <h4>Outgoing edges (${outgoing.length})</h4>
            ${outgoing.map(
              (e) => html`
              <div class="edge-line">
                <span class="kind">${e.Kind}</span>
                <span class="arrow">→</span>
                <span class="endpoint" @click=${() => this.select(e.ToSlug)}>${e.ToName}</span>
                ${e.Label ? html`<span class="label">${e.Label}</span>` : null}
              </div>
            `,
            )}
          </div>
        `
            : null
        }
        ${
          incoming.length
            ? html`
          <div class="section">
            <h4>Incoming edges (${incoming.length})</h4>
            ${incoming.map(
              (e) => html`
              <div class="edge-line">
                <span class="endpoint" @click=${() => this.select(e.FromSlug)}>${e.FromName}</span>
                <span class="arrow">→</span>
                <span class="kind">${e.Kind}</span>
                ${e.Label ? html`<span class="label">${e.Label}</span>` : null}
              </div>
            `,
            )}
          </div>
        `
            : null
        }
        ${
          taskLinks.length || docLinks.length
            ? html`
          <div class="section">
            <h4>Linked items (${taskLinks.length + docLinks.length})</h4>
            <div class="linked-items">
              ${taskLinks.map((l) => {
                const t = this.taskCache[l.TargetID];
                return t
                  ? html`
                  <nottario-task-chip
                    project-id=${this.projectId}
                    .task=${t}></nottario-task-chip>
                `
                  : html`
                  <div class="edge-line"><span class="kind">task</span><code>${l.TargetID}</code></div>
                `;
              })}
              ${docLinks.map(
                (l) => html`
                <a class="doc-link"
                   href=${`/projects/${this.projectId}/docs/${l.TargetID}`}
                   @click=${(e) => {
                     e.preventDefault();
                     window.nottarioNavigate(`/projects/${this.projectId}/docs/${l.TargetID}`);
                   }}>${l.TargetID}</a>
              `,
              )}
            </div>
          </div>
        `
            : null
        }
      </div>
    `;
  }

  render() {
    if (!this.project) return html`<p>Loading…</p>`;
    const mobileMode = this.selectedSlug ? 'reader' : 'list';
    return html`
      <nottario-page-header title="Architecture">
        <nottario-segmented-control slot="switcher"
          .options=${[{ value: 'diagram', label: 'Diagram' }, { value: 'tree', label: 'Tree' }]}
          .value=${this.view === 'tree' ? 'tree' : 'diagram'}
          @change=${(e) =>
            window.nottarioNavigate(`/projects/${this.projectId}/arch/${e.detail.value}`)}>
        </nottario-segmented-control>
      </nottario-page-header>
      ${this.error ? html`<div class="error">${this.error}</div>` : null}
      ${
        this.view === 'diagram'
          ? html`<nottario-arch-graph .projectId=${this.projectId}></nottario-arch-graph>`
          : html`<div class="layout" data-mode=${mobileMode}>${this.renderSidebar()}${this.renderReader()}</div>`
      }
    `;
  }
}

customElements.define('nottario-arch-page', NottarioArchPage);
