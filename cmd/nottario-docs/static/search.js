// Client-side search. Loads /search-index.json once on first input,
// filters entries by substring against title + body, and renders up
// to 8 hits in a dropdown under the search box. No external deps.

(function () {
  const form = document.querySelector('[data-search]');
  if (!form) return;
  const input = form.querySelector('[data-search-input]');
  const results = form.querySelector('[data-search-results]');

  const baseMeta = document.querySelector('meta[name="nottario-docs-base"]');
  const base = baseMeta ? (baseMeta.content || '') : '';

  let index = null;
  let loading = null;

  function loadIndex() {
    if (index) return Promise.resolve(index);
    if (loading) return loading;
    loading = fetch(base + '/search-index.json')
      .then(r => r.ok ? r.json() : [])
      .then(j => { index = j || []; return index; })
      .catch(() => { index = []; return index; });
    return loading;
  }

  function escapeHTML(s) {
    return s.replace(/[&<>"']/g, c => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
    })[c]);
  }

  function excerpt(body, q) {
    if (!body) return '';
    const lower = body.toLowerCase();
    const i = lower.indexOf(q);
    if (i < 0) return escapeHTML(body.slice(0, 140));
    const start = Math.max(0, i - 40);
    const end = Math.min(body.length, i + q.length + 80);
    const before = escapeHTML(body.slice(start, i));
    const match = escapeHTML(body.slice(i, i + q.length));
    const after = escapeHTML(body.slice(i + q.length, end));
    return (start > 0 ? '… ' : '') + before + '<mark>' + match + '</mark>' + after + (end < body.length ? ' …' : '');
  }

  function score(entry, q) {
    const t = entry.title.toLowerCase();
    if (t === q) return 1000;
    if (t.startsWith(q)) return 500;
    if (t.includes(q)) return 200;
    if ((entry.body || '').toLowerCase().includes(q)) return 50;
    return 0;
  }

  function render(hits, q) {
    if (!hits.length) {
      results.innerHTML = '<div class="hit"><span class="hit__excerpt">No matches.</span></div>';
      results.hidden = false;
      return;
    }
    results.innerHTML = hits.map(h => {
      const url = (base + h.url).replace(/\/+/g, '/');
      return '<a class="hit" href="' + url + '">'
        + '<div class="hit__title">' + escapeHTML(h.title) + '</div>'
        + '<div class="hit__excerpt">' + excerpt(h.body || '', q) + '</div>'
        + '</a>';
    }).join('');
    results.hidden = false;
  }

  function onInput() {
    const q = input.value.trim().toLowerCase();
    if (!q) { results.hidden = true; results.innerHTML = ''; return; }
    loadIndex().then(idx => {
      const scored = idx
        .map(e => ({ e, s: score(e, q) }))
        .filter(x => x.s > 0)
        .sort((a, b) => b.s - a.s)
        .slice(0, 8)
        .map(x => x.e);
      render(scored, q);
    });
  }

  input.addEventListener('input', onInput);
  input.addEventListener('focus', onInput);
  document.addEventListener('click', e => {
    if (!form.contains(e.target)) { results.hidden = true; }
  });
  input.addEventListener('keydown', e => {
    if (e.key === 'Escape') { results.hidden = true; input.blur(); }
  });
})();
