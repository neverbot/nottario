---
name: nottario
description: Use when working on a software project tracked in Nottario. Establishes the agent's identity, locates the active project, finds the next task to work on, and records progress (state changes, linked commits, comments).
---

# Nottario skill

Nottario coordinates developers and their agents around three domains:
**tasks**, shared **markdown context**, and an **architecture** diagram.
This file is the entry point. The full reference for each domain lives
under `domains/`; call `nottario.skill.read` to pull the one you need.

## 1. Identify yourself

Always start with `nottario.whoami`. It tells you the **user** you act
on behalf of (`user_id`, `github_login`), whether you are an **admin**,
how you authenticated (`source`), and the **single project** the
token is scoped to (`memberships[0].project_id` and friends — token
callers always see exactly one membership: the one belonging to the
token's project). On failure the token is missing or revoked — stop
and ask the human for a fresh one (web UI → open the project →
**Settings → Tokens → New token**).

Tokens are **per-project**. One token = one project. Passing
`project_id` of a different project on any subsequent call returns
`"token scoped to project X, request targets Y"`. Cache the project
id from `whoami` and reuse it on every tool call — never let the user
or another step override it without re-running `whoami`.

**The token belongs to your MCP client, not to you.** Never read it,
look for it, print it, or use it to call Nottario's HTTP API.
Everything you need is an MCP tool; for whole files use
`nottario.docs.upload_url`. If a task seems to require the token, stop
and ask the human. See `references/identity.md` → "Hands off the token".

After `whoami` and before any code work, also decide your **git mode**
for the session: solo agent, one of several parallel agents under the
same human, or one of many agents across many humans. The shape of
your commits, branches and pushes is set by that decision. Full
playbook: `methodology/git.md`.

Deep dive: `references/identity.md`.

## 2. Locate the project

Every tool call that touches a project needs `project_id` as an
explicit argument. With per-project tokens, the answer comes straight
from step 1: `whoami.memberships[0].project_id` is the only project the
token can reach. Cache it once and pass it on every call.

`nottario.projects.list` exists for completeness but for a token
caller it returns only that one project. Use it if you want the slug
or name; the id is the same one `whoami` already gave you.

If you need the role catalogue (to assign to "any backend"), call
`nottario.projects.list_roles` once and cache.

## 3. Work a task end-to-end

**Two rules hold for every task, however it came to exist:**

1. **A task you are working on is assigned to you.** Take it with
   `tasks.claim` / `claim_next`, or, when you are filing work you are
   about to start, with `tasks.create { claim: true }` — one call that
   creates it already yours and in `doing`. The server assigns you
   anyway when you move an unowned task out of `todo`, but that is a
   safety net catching a step you skipped, not the way to work.
2. **A task that reaches `done` carries a closing comment**, and the
   commit links when the work produced commits. `tasks.close` does the
   comment, the links and the transition in one call.

They apply just as much to the common case — you are working, you file
the task for what you are doing, you finish it and close it — as to
taking something off the backlog.

### Preflight: surface your own `doing` first

Whenever the human asks "what's next", "cuáles son las siguientes
tareas", "carry on", or any pickup-shaped question, ALWAYS list
your own open pickup BEFORE previewing new work:

```text
nottario.tasks.list {
  project_id,
  assignee_user_id: whoami.user_id,
  state: 'doing'
}
```

Any row here is a task you (or a previous agent under the same
identity) claimed in an earlier session and never closed. Surface
these at the top of your reply, labelled as "still open from before".
Then, and only then, preview the next eligible `todo` via
`tasks.next` / `claim_next`.

Starting a fresh pickup while a `doing` row lingers leaves two
half-done tasks and pollutes the "who is on what" view the human
uses to steer. If the human explicitly parks the old row, fine —
but the choice has to be theirs, not silently skipped.

### The pickup loop

The loop when the human says "carry on" or "do the next thing":

1. **Atomic claim.** `nottario.tasks.claim_next { project_id }`
   (optionally `assignee_user_id` = your user_id, or `role_id`). It
   picks the highest-priority eligible `todo` task AND sets
   `assignee = you`, `state = doing` in a single SQL transaction
   (`SELECT … FOR UPDATE SKIP LOCKED`). Safe to run from many agents
   in parallel. Returns `{task: null}` when nothing is eligible.
   **Do not** add a comment like "starting this" / "claimed" / "I'm
   on it" — the `state=doing` IS the record. Empty pickup comments
   add noise every future reader has to skim past.
2. **Do the work** in the local repo. Nottario does not store code.
3. **Close in one call.** `nottario.tasks.close { state: 'done',
   comment: '...', commits: [{repo, sha}, ...] }` runs the closing
   comment, the commit links and the state transition atomically.
   On precondition failure the whole thing rolls back (no orphaned
   comment, no orphaned commit links). For ordinary tasks the
   closing comment is one sentence + the commit sha; for bugs, a
   terse Repro / Fix / Test triplet. The diff is truth; the comment
   is the story future readers need to understand the diff. Full
   rules: `domains/tasks.md` §"Always leave a closing comment".
4. **Close the loop** (see below), then go back to step 1.

**Do not close a task with separate `link_commit` + `add_comment` +
`set_state` calls.** They still work, so nothing stops you, but a
failure part-way through leaves a comment and links on a task that
never closed. `tasks.close` is one round-trip and rolls back cleanly.

When the human narrows the pickup ("the next task about topic X" or
"work on this id"), discover candidates with `nottario.tasks.list` and
take one atomically with `nottario.tasks.claim { task_id }`. Read the
409 conflict shape if you lose the race.

`nottario.tasks.next` is a **preview only** — no side effects.
Useful to inspect what `claim_next` would pick. Never the first step
of pickup.

Deep dive: `domains/tasks.md`.

## 4. Close the loop after substantial work

Every time you finish a meaningful change — whether you took it via
`claim_next` or just did it because the human asked directly — run
these three checks before moving on:

1. **Is there already a task for this work?** Search the backlog
   (`nottario.tasks.list` or `nottario.search`) for whatever you just
   delivered. If a matching task is open and your commits cover its
   acceptance, link the commits and `set_state done`. Do not leave
   a duplicate row sitting in `todo`; do not file a new task that
   describes the same delivery.
2. **Did the work add, remove or substantially modify any software
   component, or change how components relate?** If yes, walk the
   architecture with `nottario.arch.list_nodes` and
   `nottario.arch.get_node`, then `upsert_node` / `upsert_edge` /
   `remove_*` to match the new reality. The diagram is the team's
   shared mental model — let it lag and the next agent reads a lie.
3. **Did the human mention side-work along the way?** A "we should
   also…", a half-formed bug repro, a future-page idea. File it as
   a task NOW with verbatim context, before you forget. See "Filing
   work as you discover it" below.
4. **Did you create or change any markdown about the project itself
   — architecture notes, ADRs, runbooks, glossaries, post-mortems,
   design briefs, methodology, or instructions for agents (`claude.md`,
   `AGENTS.md`, `GEMINI.md`, files under `.claude/skills/`)?** These
   are exactly what the docs domain exists for. Publish or update
   the matching document in `nottario.docs` (`kind: context` for
   project knowledge, `kind: skill` for agent instructions) using the
   read → edit → commit → write flow with `expected_version`. When
   the file lives in the repo AND in Nottario, both copies are the
   same truth — don't let them drift. Full flow and idioms:
   `domains/docs.md` → "Mirror repo markdown into Nottario" and
   "Keeping a local file in sync with Nottario".

## Filing work as you discover it

While working, you will spot things the human cares about but did not
ask for: bugs, follow-ups, missing features, side-comments dropped
into chat. **File them as tasks** rather than dropping them in your
reply:

```text
nottario.tasks.create {
  project_id,
  title: "Fix null deref in auth callback on duplicate state",
  type: "bug",
  priority_key: "high",
  target_role_id: "..."   // optional: route to a role rather than a person
}
```

That shape files work for later: it lands in `todo` with no owner,
which is right for something you are not about to do. **When you file
work you are starting now, add `claim: true`** — the task is created
assigned to you and in `doing`, and you never end up with a row that
went from `todo` to `done` with nobody on it.

Pick `type`:

- `task` — ordinary work,
- `bug` — defects you found,
- `chore` — cleanup,
- `spike` — time-boxed investigation,
- `feature` — *parent* whose children are the real work.

**Multi-role work → one task per role.** Create a `type=feature`
parent and one child task per affected role (design / backend /
frontend / qa), linked with `add_dependency` in execution order. The
parent rolls up to `done` automatically when all children are done.

Capture in the description:

- Verbatim what the human said (the repro, the design hunch).
  Future-you will not remember the phrasing.
- The current state-of-the-code that triggered it (a file path, a
  screenshot reference, the URL of the broken view).
- The proposed direction if you have one — mark "tentative" rather
  than omit.
- Role split when the work crosses multiple roles.

The bar: "if I had to leave the session right now, could someone else
pick this up?" If no, add more context.

Deep dive on the patterns: `domains/tasks.md` ("I found a bug while
doing my task", "The user just mentioned a different task / bug /
feature", "Block this until X is done").

## Rules of thumb

- **Never touch the API token.** Don't look for it, don't print it,
  don't call Nottario's HTTP API with it. MCP tools only; for whole
  files, `docs.upload_url`.
- **Re-confirm the project on every call.** Pass `project_id`
  explicitly; do not infer from conversation memory.
- **Never invent ids.** Always look them up via `list` / `get`.
- **Don't touch tasks belonging to other people without being asked.**
  If you must, leave a comment explaining why.
- **Prefer small, frequent updates over silent batching.** Set
  `state = doing` when you start, `state = done` when you finish.
- **If you are unsure whether to file a comment or not, file it.**
  Short comments are cheap; missing context is expensive.
- **Never `set_state done` if the task is not actually finished.** Use
  a comment to record mid-way state instead.
- **Keep responses small.** MCP responses are SLIM by default
  (mutations omit description / body, `tasks.get` omits deps/commits/
  comments unless you ask). Closing comments are one line, not a
  paragraph — the commit message and the diff carry the detail.
  Don't re-`tasks.get` what you already have in memory; don't re-read
  a skill page this session. See `domains/tasks.md` → "Token
  discipline" for the full rules.

## Deeper guides

These pages all live inside the same bundle you installed; open them
from your local skills directory once they are on disk:

- `references/identity.md` — token and identity mechanics.
- `methodology/git.md` — how to drive git across solo, parallel-agent
  and multi-dev projects: branch choice, worktrees, push policy,
  exact commands per mode.
- `domains/tasks.md` — full task API surface, edge cases, pagination,
  claim conflict shape, "carry on" / "block this" / "found a bug"
  patterns.
- `domains/docs.md` — the shared-markdown domain: skills, context,
  notes, frontmatter, optimistic concurrency, local↔Nottario sync.
- `domains/architecture.md` — the textual architecture diagram:
  nodes, edges, kinds, when to touch it, when to leave it alone.

## Installing and updating the bundle

The bundle ships as a single zip and you install it once per project
checkout. Call `nottario.skill.install` to get back a small JSON
descriptor:

```text
{
  "download_url": "https://<host>/skill.zip?exp=…&sig=…",
  "format": "zip",
  "bundle_version": "sha256:…",
  "install": {
    "name": "nottario",
    "manifest": "SHA256SUMS",
    "preferred_dir": "<workspace>/.claude/skills/nottario",
    "fallback_dir":  "~/.claude/skills/nottario",
    "instructions":  "…"
  }
}
```

What to do with it (the response carries this prose too — keep it
authoritative):

0. **Check** whether you already have this version (see below). If
   you do, stop here.
1. **Fetch** the `download_url` with whatever HTTP tool your host has.
   The URL carries an embedded signature with a 5-minute TTL, so no
   `Authorization` header is needed. Examples:
   ```bash
   curl -fSL "$URL" -o /tmp/nottario-skill.zip
   ```
   ```powershell
   Invoke-WebRequest -Uri $URL -OutFile $env:TEMP\nottario-skill.zip
   ```
   ```python
   urllib.request.urlretrieve(url, "/tmp/nottario-skill.zip")
   ```
2. **Extract** the zip into `preferred_dir`, replacing what was
   there. If your environment can't write to a workspace path, fall
   back to `fallback_dir`. Create the directory if it doesn't exist.
   ```bash
   mkdir -p "$DIR" && unzip -qo /tmp/nottario-skill.zip -d "$DIR"
   ```
   ```powershell
   New-Item -ItemType Directory -Force -Path $DIR | Out-Null
   Expand-Archive -Path "$env:TEMP\nottario-skill.zip" -DestinationPath $DIR -Force
   ```
3. **Restart your host application** — whatever agent runtime is
   actually loading you (Claude Code, another IDE plugin, a custom
   harness, anything that reads `.claude/skills/` or an equivalent
   skill directory at startup). Most agent runtimes read the skill
   bundle once when the session opens and do not re-scan, so the new
   content only takes effect after a fresh session. **Tell the human
   about this explicitly** when you finish the install — they cannot
   infer it from your tool output. Wording like "I've updated the
   Nottario skill bundle in `<dir>`. Restart whatever agent
   application is loading me (Claude Code or whichever client you
   are using) so the new rules take effect." Pick the actual name
   when you know it; otherwise leave it generic. Do not assume Claude
   Code specifically.

**Check before downloading.** `bundle_version` is `sha256:` followed
by the SHA-256 of the `SHA256SUMS` file that ships inside the zip. It
is **not** a hash of the zip, of `skill.md`, or of any other single
file. To know whether you are up to date, hash that one file where the
bundle is installed:

```bash
echo "sha256:$(sha256sum "$DIR/SHA256SUMS" | cut -d' ' -f1)"   # Linux
echo "sha256:$(shasum -a 256 "$DIR/SHA256SUMS" | cut -d' ' -f1)" # macOS
```

```powershell
"sha256:" + (Get-FileHash "$DIR\SHA256SUMS" -Algorithm SHA256).Hash.ToLower()
```

Equal to `bundle_version`: stop, there is nothing to download and no
restart to ask for. Different, or `SHA256SUMS` missing (a first
install, or a bundle from before the manifest existed): install.

`SHA256SUMS` lists every other file as `<sha256>  <path>`, the format
`sha256sum -c` reads, so `(cd "$DIR" && sha256sum -c SHA256SUMS)`
verifies an install and reveals files edited by hand. Files in the
directory that it does not list are leftovers of an older bundle.

The bundle content **never flows through your MCP response context**
— only the URL + descriptor does. The bytes go straight from server
to disk.

## Overrides

Each instance can **override or extend** any file without rebuilding:
an admin writes a document with `scope=global`, `kind=skill`,
`path=global/skills/<file>`. The next `skill.install` snapshot
includes the override transparently — overrides change
`bundle_version`, so the next sync picks them up.

**Agents cannot write overrides.** Global documents reach every
project on the instance, and an API token is scoped to one project, so
`docs.write` / `docs.append` / `docs.delete` with `scope=global` are
refused for every token — including an admin's. Overrides are changed
by an admin signed in to the web app. If you think an override is
needed, say so to the human instead of attempting the write.

Use this to tighten a shipped file for your team, or to add bundle-
absent files (`by-language/go.md`, `by-role/security.md`,
`recipes/deploying-to-our-k8s.md`).

For pre-loaded installs, prefer the workspace path
`<repo>/.claude/skills/nottario/` (commit it; every contributor who
clones the repo gets the rules for free, scoped to that repo). Fall
back to `~/.claude/skills/nottario/` only when you work on the
project across multiple unrelated checkouts or do not want to version
the bundle in any specific repo.
