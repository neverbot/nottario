import { LitElement, html, css } from '/static/vendor/lit/lit.js';

// <nottario-markdown> renders a block of markdown as styled HTML.
//
// Two modes, mutually exclusive:
//   - .html  — pre-rendered HTML (set by the parent because the
//              payload already carries it, e.g. /api/docs/read). No
//              network round-trip. This is the path the docs reader
//              uses.
//   - .source — raw markdown. The component POSTs it to
//              /api/markdown/render and inlines the response. Used
//              by task descriptions, comments, and any future
//              surface where shipping rendered HTML alongside raw
//              markdown is too heavy.
//
// project-id is optional. When provided, the server resolves the
// `[[task:N]]`, `[[doc:path]]`, `[[arch:slug]]` chips inside the
// markdown against that project. Without it the chips render as
// inert "no project context" spans (the server still does the
// sanitisation work).
//
// All prose styling lives in this component's shadow CSS so the same
// chrome appears everywhere markdown is shown: typography hierarchy,
// link chip pills, code-block treatment, table chrome, blockquotes.
// Width is capped to 76ch by default; set `wide` to remove the cap.
//
// Syntax highlighting is lazy. If the rendered HTML contains
// `code[class^="language-"]` blocks, the component dynamically
// imports highlight.js (from the vendor folder) and runs it on those
// blocks only. Documents without code blocks pay no JS cost.
class NottarioMarkdown extends LitElement {
  static properties = {
    html:      { type: String },
    source:    { type: String },
    projectId: { type: String, attribute: 'project-id' },
    wide:      { type: Boolean },

    _resolvedHTML: { state: true },
    _loading:      { state: true },
    _error:        { state: true },
  };

  static styles = css`
    :host { display: block; box-sizing: border-box; }
    * { box-sizing: border-box; }

    .prose {
      max-width: 76ch;
      color: #1f2328;
      font-size: 14px;
      line-height: 1.65;
    }
    :host([wide]) .prose { max-width: none; }

    /* Typography hierarchy — tighter than brand defaults, calibrated
       to GitHub's reading scale. */
    .prose h1, .prose h2, .prose h3, .prose h4, .prose h5, .prose h6 {
      color: #1f2328;
      font-weight: 600;
      line-height: 1.25;
      margin: 24px 0 12px;
    }
    .prose h1 { font-size: 22px; padding-bottom: 6px; border-bottom: 1px solid #d1d9e0; }
    .prose h2 { font-size: 19px; padding-bottom: 4px; border-bottom: 1px solid #eaeef2; }
    .prose h3 { font-size: 16px; }
    .prose h4 { font-size: 14px; }
    .prose h5, .prose h6 { font-size: 13px; color: #59636e; }
    .prose > :first-child { margin-top: 0; }
    .prose > :last-child { margin-bottom: 0; }

    .prose p { margin: 0 0 12px; }

    .prose a {
      color: #0969da;
      text-decoration: none;
    }
    .prose a:hover { text-decoration: underline; }

    .prose ul, .prose ol {
      margin: 0 0 12px;
      padding-left: 24px;
    }
    .prose li { margin: 2px 0; }
    .prose li > p { margin: 0; }

    .prose input[type="checkbox"] {
      margin: 0 6px 0 -22px;
      vertical-align: middle;
    }

    .prose blockquote {
      margin: 0 0 12px;
      padding: 0 12px;
      border-left: 3px solid #d1d9e0;
      color: #59636e;
    }

    .prose code {
      font-family: ui-monospace, SFMono-Regular, "SF Mono", monospace;
      font-size: 0.88em;
      background: #f6f8fa;
      padding: 0.15em 0.4em;
      border-radius: 4px;
    }

    .prose pre {
      margin: 0 0 12px;
      padding: 12px;
      background: #f6f8fa;
      border: 1px solid #d1d9e0;
      border-radius: 6px;
      overflow: auto;
      font-size: 12.5px;
      line-height: 1.55;
    }
    .prose pre code {
      background: transparent;
      padding: 0;
      border-radius: 0;
      font-size: inherit;
      display: block;
    }

    .prose table {
      border-collapse: separate;
      border-spacing: 0;
      border: 1px solid #d1d9e0;
      border-radius: 6px;
      overflow: hidden;
      margin: 0 0 12px;
      font-size: 13px;
    }
    .prose th, .prose td {
      padding: 6px 10px;
      border-bottom: 1px solid #eaeef2;
      text-align: left;
    }
    .prose th {
      background: #f6f8fa;
      font-weight: 600;
      font-size: 12px;
      text-transform: uppercase;
      letter-spacing: 0.04em;
      color: #59636e;
    }
    .prose tr:last-child td { border-bottom: none; }

    .prose hr {
      border: none;
      border-top: 1px solid #d1d9e0;
      margin: 24px 0;
    }

    .prose img {
      max-width: 100%;
      border-radius: 4px;
    }

    /* Cross-domain link chips ([[task:N]] etc.). The server emits
       <a class="chip chip-task">, etc. .chip-missing renders red. */
    .prose .chip {
      display: inline-flex;
      align-items: center;
      gap: 4px;
      padding: 0 8px;
      height: 20px;
      border-radius: 999px;
      border: 1px solid #d1d9e0;
      background: #f6f8fa;
      color: #1f2328;
      font-size: 12px;
      font-weight: 500;
      text-decoration: none;
      vertical-align: 1px;
      line-height: 1;
      white-space: nowrap;
    }
    .prose a.chip:hover {
      border-color: #0969da;
      color: #0969da;
      background: #ddf4ff;
      text-decoration: none;
    }
    .prose .chip-task { color: #0a3069; }
    .prose .chip-doc  { color: #1a7f37; }
    .prose .chip-arch { color: #8250df; }
    .prose .chip-missing {
      color: #cf222e;
      background: #ffebe9;
      border-color: rgba(207, 34, 46, 0.4);
      font-style: italic;
    }
    .prose .chip-state-done { opacity: 0.6; }

    .status {
      color: #59636e;
      font-size: 13px;
      font-style: italic;
    }
    .status.error { color: #cf222e; font-style: normal; }

    /* highlight.js GitHub theme — vendored from highlight.js@11.10.0/styles/github.css.
       Included in the component's shadow so every consumer gets the same
       palette without a separate link rel="stylesheet". */
    .prose pre code.hljs { display: block; overflow-x: auto; padding: 0; }
    .prose code.hljs { padding: 0; }
    .prose .hljs { color: #24292e; background: transparent; }
    .prose .hljs-doctag, .prose .hljs-keyword, .prose .hljs-meta .hljs-keyword,
    .prose .hljs-template-tag, .prose .hljs-template-variable, .prose .hljs-type,
    .prose .hljs-variable.language_ { color: #d73a49; }
    .prose .hljs-title, .prose .hljs-title.class_, .prose .hljs-title.class_.inherited__,
    .prose .hljs-title.function_ { color: #6f42c1; }
    .prose .hljs-attr, .prose .hljs-attribute, .prose .hljs-literal, .prose .hljs-meta,
    .prose .hljs-number, .prose .hljs-operator, .prose .hljs-selector-attr,
    .prose .hljs-selector-class, .prose .hljs-selector-id, .prose .hljs-variable { color: #005cc5; }
    .prose .hljs-meta .hljs-string, .prose .hljs-regexp, .prose .hljs-string { color: #032f62; }
    .prose .hljs-built_in, .prose .hljs-symbol { color: #e36209; }
    .prose .hljs-code, .prose .hljs-comment, .prose .hljs-formula { color: #6a737d; }
    .prose .hljs-name, .prose .hljs-quote, .prose .hljs-selector-pseudo,
    .prose .hljs-selector-tag { color: #22863a; }
    .prose .hljs-subst { color: #24292e; }
    .prose .hljs-section { color: #005cc5; font-weight: 700; }
    .prose .hljs-bullet { color: #735c0f; }
    .prose .hljs-emphasis { color: #24292e; font-style: italic; }
    .prose .hljs-strong { color: #24292e; font-weight: 700; }
    .prose .hljs-addition { color: #22863a; background-color: #f0fff4; }
    .prose .hljs-deletion { color: #b31d28; background-color: #ffeef0; }
  `;

  constructor() {
    super();
    this.html = '';
    this.source = '';
    this.projectId = '';
    this.wide = false;
    this._resolvedHTML = '';
    this._loading = false;
    this._error = '';
    this._lastFetch = '';
  }

  updated(changed) {
    // If a parent passes pre-rendered HTML, skip the fetch entirely.
    if (changed.has('html') && this.html) {
      this._resolvedHTML = this.html;
      this._error = '';
      this._loading = false;
      this._writeProse();
      this._highlightAfterRender();
      return;
    }
    // Otherwise (re-)fetch the rendered HTML when source changes.
    if (changed.has('source') || changed.has('projectId')) {
      const key = `${this.projectId} ${this.source || ''}`;
      if (key === this._lastFetch) return;
      this._lastFetch = key;
      if (!this.source) {
        this._resolvedHTML = '';
        this._writeProse();
        return;
      }
      this._fetchRender();
    }
    if (changed.has('_resolvedHTML')) {
      this._writeProse();
    }
  }

  // The rendered markdown is server-sanitized HTML, but Lit's html
  // tag refuses to interpolate raw HTML strings. So instead of
  // dragging in the unsafe-html directive (not in our vendored Lit
  // bundle), we render an empty .prose container in the template
  // and write innerHTML ourselves after every update. The content
  // is already sanitized by bluemonday on the server.
  _writeProse() {
    const node = this.renderRoot?.querySelector('.prose');
    if (!node) return;
    node.innerHTML = this._resolvedHTML || '';
  }

  async _fetchRender() {
    this._loading = true;
    this._error = '';
    try {
      const res = await fetch('/api/markdown/render', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          project_id: this.projectId || '',
          content_md: this.source,
        }),
      });
      if (!res.ok) throw new Error((await res.json()).error || 'render failed');
      const data = await res.json();
      this._resolvedHTML = data.html || '';
      this._highlightAfterRender();
    } catch (e) {
      this._error = e.message;
    } finally {
      this._loading = false;
    }
  }

  // Lazy-load highlight.js the first time we render a doc that has
  // language-flagged code blocks. Pages without code blocks never
  // pay the highlight.js cost.
  async _highlightAfterRender() {
    if (!this._resolvedHTML) return;
    if (!/code[^>]*class="[^"]*language-/.test(this._resolvedHTML)) return;
    await this.updateComplete;
    const blocks = this.renderRoot?.querySelectorAll('code[class*="language-"]') || [];
    if (!blocks.length) return;
    try {
      const mod = await import('/static/vendor/highlight/highlight.js');
      const hljs = mod.default || mod.hljs || mod;
      blocks.forEach(b => {
        try { hljs.highlightElement(b); } catch (_) { /* skip bad block */ }
      });
    } catch (_) {
      // highlight.js failed to load: plain code blocks remain.
    }
  }

  render() {
    if (this._loading) {
      return html`<div class="prose"><p class="status">Rendering...</p></div>`;
    }
    if (this._error) {
      return html`<div class="prose"><p class="status error">${this._error}</p></div>`;
    }
    // Container is intentionally empty; _writeProse fills it.
    return html`<div class="prose"></div>`;
  }
}

customElements.define('nottario-markdown', NottarioMarkdown);
