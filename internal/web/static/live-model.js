// Pure helpers behind the Live view and the question card. No DOM, no
// fetch: they take the API's JSON and return strings, booleans or
// sorted copies, so they run (and are tested) under plain Node.

// isClaudeCode reports whether a session's client is Claude Code, the
// one client whose conversations reopen with `claude --resume <id>`.
//
// The server stores the MCP clientInfo name, or the first token of the
// User-Agent, filtered to [A-Za-z0-9._-]. Claude Code names itself
// "claude-code" in clientInfo and "claude-cli" in its User-Agent, which
// is what a stateless server usually sees. Not yet confirmed against
// real rows: check agent_sessions.client and adjust this list.
const CLAUDE_CODE_CLIENTS = new Set(['claude-code', 'claude-cli']);

export function isClaudeCode(client) {
  return typeof client === 'string' && CLAUDE_CODE_CLIENTS.has(client.toLowerCase());
}

// resumeCommand returns the shell command that reopens an agent's
// conversation, or null when the client gives no way back.
export function resumeCommand(session) {
  if (!session?.key) return null;
  return isClaudeCode(session.client) ? `claude --resume ${session.key}` : null;
}

// sessionCopyText is what a session's copy button puts on the
// clipboard: the resume command for Claude Code, the bare id for any
// other client, '' without a session.
export function sessionCopyText(session) {
  if (!session?.key) return '';
  return resumeCommand(session) || session.key;
}

// sessionName is what the UI calls a session: its label, else the
// first 8 characters of its key.
export function sessionName(session) {
  if (!session) return '';
  return session.label || (session.key || '').slice(0, 8);
}

// stepLine renders "3/8 · Write migration" for the newest in-progress
// step, "3/8" when none is in progress, or "" without steps.
export function stepLine(item) {
  const p = item?.progress;
  if (!p) return '';
  const count = `${p.done}/${p.total}`;
  const step = item.steps?.[0];
  return step ? `${count} · ${step.text}` : count;
}

// sortWaiting orders questions oldest first, without touching the input.
export function sortWaiting(qs) {
  return [...qs].sort((a, b) => Date.parse(a.created_at) - Date.parse(b.created_at));
}

// isIdle mirrors the server flag for rows refreshed client-side
// between reloads: now - last_activity_at > idle minutes. A row with no
// recorded activity is not idle; the server's flag covers it.
export function isIdle(item, idleMinutes, now) {
  const at = Date.parse(item?.last_activity_at);
  if (!Number.isFinite(at)) return false;
  return now - at > idleMinutes * 60_000;
}
