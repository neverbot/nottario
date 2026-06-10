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
- `global` — visible to every project. No `project_id`. Only admins
  can modify globals.

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
- Any other keys are stored verbatim in `frontmatter` and round-trip
  through `read` / `write`.

When you read a document, the response splits the body from the
parsed frontmatter (as an object). When you write, you send the full
markdown including frontmatter in `content`; the server splits and
stores them.

## Tools

### `nottario.docs.list`

Returns lightweight summaries (no body). Use it to navigate, then
`read` the ones you care about. Filters: `path_prefix`, `kind`.

### `nottario.docs.read`

Returns the full document including body, parsed frontmatter and the
current version number. **Capture `current_version`**: you need it to
update the document safely.

### `nottario.docs.search`

Full-text search over `title`, `description` and body. Use
`plainto_tsquery` semantics (treat the query as keywords; the parser
ignores quoting and operators). Filters: `kind`.

### `nottario.docs.write`

Creates or updates the document keyed by `(scope, project_id, path)`.
The body you send should include any frontmatter you want preserved:

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
edit the body locally (regenerate full markdown with frontmatter)
nottario.docs.write {
  ..., path,
  content: new_body,
  expected_version: doc.CurrentVersion,
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
   and stash its `CurrentVersion`:

   ```text
   doc = nottario.docs.read {
     scope: "project",
     project_id: "...",
     path: "projects/<id>/context/claude.md",
   }
   // remember doc.CurrentVersion
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
     expected_version: doc.CurrentVersion,
     message: "<why you changed it>",
   }
   ```

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

## Things you cannot do (today)

- Edit a comment or version body retroactively.
- Move (rename a path) — write the new path and delete the old.
- Attach images or binaries (planned for a later milestone).
- Modify global documents unless you are admin.
