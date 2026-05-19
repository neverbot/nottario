// Minimal placeholder shell. Lit is introduced in milestone M1
// alongside login UI; for M0 we keep zero external dependencies.

class NottarioShell extends HTMLElement {
  connectedCallback() {
    this.innerHTML = `
      <header style="padding: 16px 24px; border-bottom: 1px solid var(--border); background: var(--bg-subtle);">
        <strong>Nottario</strong>
        <span style="color: var(--fg-muted); margin-left: 8px;">foundation milestone</span>
      </header>
      <main style="padding: 24px;">
        <p>The server is running.</p>
        <p>See <a href="/healthz">/healthz</a> and <a href="/version">/version</a>.</p>
      </main>
    `;
  }
}

customElements.define('nottario-shell', NottarioShell);
