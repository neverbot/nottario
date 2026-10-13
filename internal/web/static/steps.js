// Pure helpers for task steps, shared by the dialog component and the
// kanban card. No DOM: the unit tests import this module directly.

export const MAX_STEP_LEN = 200;
export const MAX_STEPS = 50;

// A leading list or checkbox marker, as pasted from markdown, a plain
// list or an agent's plan: "- ", "* ", "+ ", "1. ", "2) ", "- [ ] ",
// "[x] ".
const MARKER = /^(?:[-*+](?:\s+|$)|\d+[.)](?:\s+|$))?(?:\[[ xX]\](?:\s+|$))?/;

// stepTextsFromPaste splits pasted text into step texts: one per
// non-empty line, list markers stripped, whitespace trimmed.
export function stepTextsFromPaste(text) {
  return String(text || '')
    .split(/\r\n|\r|\n/)
    .map((line) => line.trim().replace(MARKER, '').trim())
    .filter((line) => line !== '');
}

// stepProgress counts a step list into {done, total}.
export function stepProgress(steps) {
  const list = steps || [];
  return { done: list.filter((s) => s.done).length, total: list.length };
}

// moveIndex returns the list with the item at `from` moved to `to`
// (both clamped), positions rewritten. The input is left untouched.
export function moveIndex(list, from, to) {
  const out = [...list];
  const f = Math.max(0, Math.min(from, out.length - 1));
  const t = Math.max(0, Math.min(to, out.length - 1));
  const [item] = out.splice(f, 1);
  out.splice(t, 0, item);
  return out.map((s, i) => ({ ...s, position: i }));
}
