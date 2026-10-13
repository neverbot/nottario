// Pure helpers for the MCP usage section of the project settings. No
// DOM: the unit tests import this module directly.

// formatBytes renders a byte count the way the usage tables show it:
// B under 1 KB, then KB and MB with one decimal under 10.
export function formatBytes(n) {
  const b = Math.max(0, Number(n) || 0);
  if (b < 1024) return `${b} B`;
  const units = ['KB', 'MB', 'GB'];
  let v = b / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v < 10 ? v.toFixed(1) : Math.round(v)} ${units[i]}`;
}

// approxTokens estimates model tokens from bytes of JSON or markdown,
// at about four bytes per token. It is a rough guide, labelled as such.
export function approxTokens(bytes) {
  return Math.round(Math.max(0, Number(bytes) || 0) / 4);
}

// formatCount renders an integer with thousands separators and k/M
// above ten thousand.
export function formatCount(n) {
  const v = Math.max(0, Math.round(Number(n) || 0));
  if (v >= 1e6) return `${(v / 1e6).toFixed(v < 1e7 ? 1 : 0)}M`;
  if (v >= 1e4) return `${Math.round(v / 1e3)}k`;
  return v.toLocaleString('en-US');
}

// perCall is the average bytes per call, 0 when there were no calls.
export function perCall(bytes, calls) {
  return calls > 0 ? Math.round(bytes / calls) : 0;
}

// shortToolName drops the "nottario." prefix every tool shares.
export function shortToolName(name) {
  return String(name || '').replace(/^nottario\./, '');
}
