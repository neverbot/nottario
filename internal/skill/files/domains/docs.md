---
name: nottario-domain-docs
description: Reference for the Nottario shared-context (markdown documents) domain — how to list, read, write and search shared documentation that the whole team uses.
---

# Documents domain — full reference

The documents domain holds shared markdown that humans and agents both
read. Use it instead of letting context drift across local `.md` files
on each developer's laptop. Three kinds, all stored the same way:

- **`skill`** — operational instructions for agents (testing
  conventions, commit conventions, "how to work on the payments
  module"). Equivalent to a `.claude/skills/` file but living in
  Nottario so every agent on the team sees the same version.
- **`context`** — living project documentation: glossaries, decisions
  (ADRs), onboarding, runbooks. The canonical answer to "what does X
  mean in this codebase".
- **`note`** — scratchpads, half-baked ideas, follow-ups too small to
  be tasks. Filed by whoever (human or agent) wants to remember
  something.

## Scopes

Every document lives in one of two scopes:

- `project` — visible only within one project. Pass `project_id`.
- `global` — visible to every project. No `project_id`. Any token
  can read globals; **no token can modify them**, not even an admin's.
  Global writes need an admin signed in to the web app, because they
  reach every project and a token is scoped to one.

## Paths

A path is a slash-delimited string used by the tree view and by your
own MCP calls. There is **no real filesystem**. Conventions:

```
projects/<project-id>/skills/writing-tests.md
projects/<project-id>/context/glossary.md
projects/<project-id>/context/decisions/2026-05-22-postgres-not-sqlite.md
projects/<project-id>/notes/<github-login>/mcp-ideas.md
global/skills/conventional-commits.md
```

You are free to invent the path structure that fits the project. The
sidebar groups documents alphabetically, so sensible prefixes pay off.

## Frontmatter

Every document may carry YAML frontmatter at the very top:

```yaml
---
title: How to write tests
kind: skill
description: Testing conventions for this project
applies_to_roles: [backend, qa]
tags: [testing, quality]
---

# How to write tests

...body...
```

- `title` (string): falls back to the first `# heading` or the path.
- `description` (string): shown in lists; this is the "blurb" you read
  *before* deciding to load the full body.
- `kind` (string): can also be passed explicitly to `docs.write`.
- Any other keys are yours; they stay in the document.

**Documents are stored whole.** `content` is always the complete
markdown exactly as written, frontmatter included: what you write is
what `read` returns, byte for byte. The server only *reads* the
frontmatter to fill `title`, `description` and `kind`, and returns it
parsed as `frontmatter` for convenience. Search does not index it and
the web view does not render it. A frontmatter block that is not valid
YAML is refused on write.

## Tools

### `nottario.docs.list`

Returns lightweight summaries (no body). Use it to navigate, then
`read` the ones you care about. Filters: `path_prefix`, `kind`.

### `nottario.docs.read`

Returns the whole document (`content`, frontmatter included), the
parsed frontmatter and the current version number. **Capture `current_version`**: you need it to
update the document safely.

Pass `head_only: true` when you only need to check "is this the right
doc" — the response then carries the frontmatter, title, description,
`current_version` and just the first 400 chars of `content` (which
starts with the frontmatter block), with `truncated: true` /
`body_length: N` so you know there is more. Useful
for catalogue-style flows over many documents.

### `nottario.docs.search`

Full-text search over `title`, `description` and body (the
frontmatter block is not indexed). Use
`plainto_tsquery` semantics (treat the query as keywords; the parser
ignores quoting and operators). Filters: `kind`.

### `nottario.docs.stat`

Fingerprints a document without returning it:

```json
{ "path": "…", "current_version": 7, "size_bytes": 9000,
  "content_sha256": "…", "updated_at": "…" }
```

Use it to answer "does this need writing?" for a fraction of a
`docs.read`. Hash your local copy, compare, and skip the write when
the digests agree.

**The digest is the file's.** Documents are stored byte for byte,
frontmatter included, so `content_sha256` equals `sha256sum file.md`
(or `shasum -a 256 file.md`) of the copy you wrote.

### `nottario.docs.append`

Adds markdown to the end of an existing document's body:

```text
nottario.docs.append {
  project_id, path,
  content: "\n## 2026-09-09\n\n- the new entry\n",
  expected_version: 7,
  message: "log today's decision",
}
```

You pay for the entry, not for the whole document. Same slim ack and
same `expected_version` contract as `docs.write`.

Appending is the only partial write on this surface, and it is safe
for a specific reason: there is no anchor to mismatch and no existing
text to overwrite, so a stale caller cannot corrupt a passage it
misread. It only ever adds, at the end, under a version check.

The document must already exist — append never creates. Use
`docs.write` for the first version. The server also guarantees exactly
one newline at the seam, so a body that does not end in a newline
cannot swallow your first line.

Reach for it on anything log-shaped: changelogs, decision records,
running notes. For editing existing text, `docs.write` the whole
document; there is deliberately no line-level patch tool.

### `nottario.docs.write`

Creates or updates the document keyed by `(scope, project_id, path)`.
`content` is the complete file, frontmatter included; it is stored
exactly as sent:

```text
content = "---\ntitle: …\nkind: skill\n---\n\nBody…"
```

**Optimistic concurrency**: always pass `expected_version`.

- For a **new document**, pass `0`. If the path already exists you get
  `version_conflict`.
- For an **update**, pass the `current_version` you most recently
  read. If someone else updated between your read and write you get
  `version_conflict`; the response payload now includes the live
  `current_version` and a human message. Re-read, integrate, retry.

The conflict shape is:

```json
{ "error": "version_conflict", "current_version": 7,
  "message": "re-read the document and retry with the latest current_version" }
```

Omitting `expected_version` is **deprecated**: the server still
accepts the write but logs a warning, and you're racing whoever else
might be editing the same path. Don't.

Always include a short `message` explaining *why* — like a commit
message. It's stored on the version row and helps future readers.

The response is a **slim ack**: `{path, current_version, updated_at}`.
The body you sent is deliberately not echoed back — you already have
it, and on a large document a second copy is the most expensive thing
this tool could do to your context. `current_version` is returned
because you need it for the next write; cache it rather than calling
`docs.read` to look it up again.

### `nottario.docs.upload_url`

Creates or replaces a **whole document from a file on disk**, without
the file's bytes passing through your context and without you ever
handling an API token. You ask for a signed URL, then send the file to
it with a plain HTTP `PUT`.

Use it whenever the content already exists as a file — a repo
document you are mirroring, a generated report, anything you would
otherwise paste into `docs.write`. For text you are composing right
now, `docs.write` is fine: those bytes are in your context anyway. To
add to the end of a document, `docs.append` is cheaper still.

The flow:

```text
1. st = nottario.docs.stat { project_id, path }        // or "not found" for a new doc
2. compare st.content_sha256 with the body hash of your file (see docs.stat)
   → equal: stop, nothing to upload
3. file_sha256 = sha256 of the EXACT file bytes         // e.g. shasum -a 256 file.md
4. u = nottario.docs.upload_url {
     project_id, path,
     expected_version: st.current_version,             // 0 to create
     file_sha256,
     message: "why",
   }
5. curl -fsS -X PUT --data-binary @file.md "<u.upload_url>"
   → {"path": …, "current_version": …, "updated_at": …}
```

**One hash, two uses.** Both are the SHA-256 of the whole file.
`docs.stat`'s `content_sha256` answers "is it already there?";
`file_sha256` locks the URL to one specific file. When they are equal,
skip the upload.

What the URL allows, and nothing more:

- **Valid for 5 minutes** (`expires_in_seconds: 300`). Request it right
  before uploading, not at the start of a long task.
- **One file.** The body must hash to `file_sha256`; anything else is
  refused with `400` and nothing is written.
- **One version, so one use.** The upload replaces the document only if
  it is still at `expected_version`. After a successful upload, the
  same URL returns `409 version_conflict`, which makes it single-use.
- **One project and path**, always a project document; global documents
  cannot be uploaded this way.
- **No credential inside.** Do not add an `Authorization` header: the
  signature is the whole credential. Quote the URL in the shell — it
  contains `&`.

Responses:

| Status | Meaning | What to do |
|---|---|---|
| `200` | Written. Slim ack with the new `current_version`. | Cache the version. |
| `400` | Body does not match `file_sha256`, is not UTF-8, or the frontmatter is invalid. Nothing written. | Re-hash the file you are actually sending. |
| `403` | URL expired, altered, or the token that requested it was revoked. | Request a new URL. If it keeps failing, tell the human. |
| `409` | The document is no longer at `expected_version`. | `docs.stat` again and decide; never just re-sign blindly over someone else's change. |
| `413` | Body over 8 MiB. | Documents that large do not belong in Nottario. |

The tool itself refuses to sign for a version it can already see is
stale, returning the same `version_conflict` shape without a URL.

### `nottario.docs.delete`

Soft delete: the row stays in `document_versions` so history is
preserved. Re-writing the same path resurrects the document with the
next version number. Same `expected_version` semantics as `write`:
pass the `current_version` from your most recent read; omitting it is
deprecated and logs a warning.

### `nottario.docs.history`, `nottario.docs.read_version`

Inspect history and pull a specific version body. Useful when the
human asks "what did this say last week?" or when investigating an
edit that broke an assumption.

## Idiomatic patterns

### "Mirror repo markdown into Nottario"

Any markdown file in the repo that describes the project itself —
not source, not generated output — should have a mirror in the docs
domain so agents on other laptops (or agents that never clone the
repo, like an MCP-only helper) can read it. Concretely:

- **Agent instruction files**: `claude.md`, `AGENTS.md`, `GEMINI.md`,
  anything under `.claude/skills/`, `.cursor/rules/`, `.aider.conf.md`,
  and equivalents. `kind: skill`, typically at
  `projects/<id>/skills/<file>.md`. Rules that should apply across
  projects belong under `global/skills/`, but only an admin in the web
  app can write there — flag it to the human rather than attempting it.
- **Living project context**: architecture notes, ADRs (decision
  records), glossaries, onboarding pages, runbooks, post-mortems,
  incident reports, design briefs, methodology docs (branching
  strategy, release checklist, on-call playbook). `kind: context`,
  typically under `projects/<id>/context/…`.
- **Personal scratchpads and half-baked ideas** that don't warrant a
  task yet. `kind: note`, typically under
  `projects/<id>/notes/<login>/…`.

**Rule of thumb.** If a future teammate (human or agent) would
benefit from reading it, and it's not a task and not source code,
it belongs in Nottario docs. If you edit the repo copy, update the
Nottario copy in the same session — the read → edit → commit → write
sync flow below is the contract; don't let the two copies drift.

Small style tweaks or typo fixes on a mirrored file still count as
edits: bump the Nottario copy too, otherwise the next agent syncing
from Nottario will silently undo your fix.

### "Record this decision"

A short ADR-style document under `context/decisions/`. Two minutes of
your time saves the next agent a lot of context discovery later:

```text
nottario.docs.write {
  scope: "project",
  project_id: "...",
  path: "projects/<id>/context/decisions/2026-05-22-no-sqlite.md",
  content: "---\ntitle: Postgres only\nkind: context\n---\n\n# Postgres only\n\n## Decision\n\n…\n\n## Rationale\n\n…\n",
  expected_version: 0,
  message: "decide to drop sqlite support",
}
```

### "I just learned something, save it as a note"

If it's too small for a task and not a decision, file a note. Future
you will thank present you:

```text
nottario.docs.write {
  scope: "project",
  project_id: "...",
  path: "projects/<id>/notes/<your-login>/<topic>.md",
  content: "Body…",
  expected_version: 0,
}
```

### "Update a doc safely"

```text
doc = nottario.docs.read { ..., path }
edit doc.content locally (it already includes the frontmatter)
nottario.docs.write {
  ..., path,
  content: new_body,
  expected_version: doc.current_version,
  message: "clarify wording",
}
// on version_conflict: re-read and retry.
```

## Keeping a local file in sync with Nottario

Some documents live in **two places at once**: as a `.md` file on disk
(committed to the repo) and as a Nottario document. `claude.md` is the
canonical example, but the same pattern applies to anything an agent
edits in both surfaces — operating manuals, ADRs the team also wants
in the repo, anything you can imagine being touched concurrently by a
teammate's editor and by another agent over MCP.

Without discipline, the two diverge. The flow below keeps them in
lockstep using the optimistic-concurrency primitives:

1. **Before editing** the local file, read the Nottario copy first
   and stash its `current_version`:

   ```text
   doc = nottario.docs.read {
     scope: "project",
     project_id: "...",
     path: "projects/<id>/context/claude.md",
   }
   // remember doc.current_version
   ```

2. **Compare** Nottario's body against the local file.
   - If they match, edit the local file and commit your changes.
   - If Nottario is ahead, merge its changes into your local file
     *first* — somebody (an agent in another session, a teammate)
     wrote to Nottario after the last sync. Resolve the merge, then
     commit.

3. **Push to Nottario** with the version you stashed in step 1:

   ```text
   nottario.docs.write {
     scope: "project",
     project_id: "...",
     path: "projects/<id>/context/claude.md",
     content: <full updated body>,
     expected_version: doc.current_version,
     message: "<why you changed it>",
   }
   ```

   When the local copy is a file on disk, do this step with
   `docs.upload_url` instead of pasting the file into `docs.write`:
   same `expected_version`, but the bytes never pass through you.

4. **On `version_conflict`** the response carries the live
   `current_version` and a message:

   ```json
   { "error": "version_conflict", "current_version": 9,
     "message": "re-read the document and retry with the latest current_version" }
   ```

   Re-read, merge Nottario's newer body into yours, and retry the
   write with the new `current_version`. Never just retry with the
   stale version — you would clobber whatever change made Nottario
   move forward.

**Generic rule:** for *any* document an agent edits both as a file
and via MCP, read-then-write under `expected_version` is the
contract. The repo commit and the Nottario write are two sides of
the same change; finish both before moving on, in this order:
read → edit → commit → write.

This `claude.md`-style sync flow is the same one called out in the
project's own `claude.md` under **Document sync (local files ↔
Nottario)** — keep them aligned if either changes.

## Token discipline

Keep MCP responses small. Default to the slim shapes; opt in to
heavier ones deliberately.

**`docs.list` is body-less.** It returns only path / title / kind /
description / current_version / updated_at per document — no
markdown body. To get a body you `docs.read` the specific path.
This is by design: a docs listing in a large project would otherwise
be dominated by bodies you'll never look at.

**`docs.read { head_only: true }` for catalogue checks.** When you
only need to confirm "is this the doc I want" (right title, right
kind, right frontmatter), pass `head_only: true`. The response then
carries the parsed frontmatter + the first 400 chars of `content` plus
`truncated: true` / `body_length: N` markers. Switch to a full
`docs.read` only when you've decided to actually work with the body.

**Skip `verbose: true` on `docs.search`.** The slim hit already
carries the highlighted snippet (`description_html`). The raw
`description` fallback is for the web UI; in an agent context it's
duplicated noise per hit. Default limit is 20, max 100 — raise it
only when a wider sweep is genuinely needed.

**Always pass `expected_version` on writes.** Without it the server
falls back to last-writer-wins and may silently overwrite a
concurrent change. The shortcut is not worth the recovery time when
two agents clobber each other.

**`docs.stat` before you read, and `docs.append` before you rewrite.**
Two habits kill most of the cost of keeping documents in sync. Reading
a whole document to find out whether it changed is the expensive way
to learn one bit — `docs.stat` answers it with a hash. And rewriting a
document to add a line at the end sends the whole body for the sake of
that line — `docs.append` sends the line.

**Don't re-`docs.read` what you just wrote.** `docs.write` returns
the new `current_version` in its ack. Cache that and pass it to the
next write. Re-reading a body you composed yourself buys nothing.

**Moving a whole file: `docs.upload_url`, never the HTTP API.**
`docs.write`'s `content` is a tool argument, so every byte of the
document crosses your context on the way out. For a file that already
exists on disk, get a signed URL from `docs.upload_url` and `PUT` the
file to it: the bytes go from disk to the server, and all you ever hold
is a URL that expires in five minutes.

Do not work around this by calling Nottario's REST API with your MCP
token. You should never have the token in the first place — see
`skill.md` §1 and `references/identity.md` → "Hands off the token".

## Things you cannot do (today)

- Edit a comment or version body retroactively.
- Move (rename a path) — write the new path and delete the old.
- Attach images or binaries (planned for a later milestone).
- Modify global documents. Tokens are refused even for admins; global
  writes need a signed-in admin in the web app.
