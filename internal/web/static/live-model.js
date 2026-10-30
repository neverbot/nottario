// Pure helpers behind the Live view and the question card. No DOM, no
// fetch: they take the API's JSON and return strings, booleans or
// sorted copies, so they run (and are tested) under plain Node.

// RESUME_COMMANDS maps an MCP client name (lowercase) to the function
// that builds the shell command reopening that client's conversation
// from its session key. Clients without an entry get no command; the
// UI then offers the bare session key.
//
// The server stores the MCP clientInfo name, or the first token of the
// User-Agent, filtered to [A-Za-z0-9._-], so one client can show up
// under more than one name. Add a client only after confirming its
// real name in agent_sessions.client and its resume syntax.
const claudeResume = (key) => `claude --resume ${key}`;
export const RESUME_COMMANDS = Object.freeze({
  'claude-code': claudeResume,
  'claude-cli': claudeResume,
});

// resumeCommand returns the shell command that reopens an agent's
// conversation, or null when its client has no known way back.
export function resumeCommand(session) {
  if (!session?.key || typeof session.client !== 'string') return null;
  const build = Object.hasOwn(RESUME_COMMANDS, session.client.toLowerCase())
    ? RESUME_COMMANDS[session.client.toLowerCase()]
    : null;
  return build ? build(session.key) : null;
}

// sessionCopyText is what a session's copy button puts on the
// clipboard: the resume command when the client has one, the bare
// session key otherwise, '' without a session.
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

// RECENT_MS is how fresh activity must be for the green dot.
export const RECENT_MS = 2 * 60_000;

// activityState classifies a Working row for its dot: 'recent' (green)
// when the last activity is under two minutes old, 'idle' (grey) past
// the project's idle threshold or when the server flagged it, 'quiet'
// (neutral) in between.
export function activityState(item, idleMinutes, now) {
  if (item?.idle || isIdle(item, idleMinutes, now)) return 'idle';
  const at = Date.parse(item?.last_activity_at);
  if (Number.isFinite(at) && now - at < RECENT_MS) return 'recent';
  return 'quiet';
}
