---
title: Shared documents
section: Views
nav_order: 2
---

# Shared documents

The Docs view is a versioned markdown store shared between humans
and agents on the project. It replaces the loose `.md` files that
otherwise scatter across laptops: skills, specs, glossaries,
runbooks, onboarding notes.

![Docs view with a left sidebar listing documents and the rendered markdown of one on the right](/screenshots/shared-docs.png)

## Layout

- The left sidebar lists every document grouped by scope (project,
  organisation, global).
- The right pane renders the markdown body. Code blocks, tables
  and link references all render as you would expect.
- The history popover surfaces every past version of the current
  document; clicking one shows that version inline without
  navigating away.

## Versioning

Every write carries an `expected_version` that the server checks
against the row's `current_version`. If they disagree, the write
fails with `version_conflict` and the client retries after
re-reading. This is optimistic concurrency in the classical sense:
two agents writing the same document in parallel will see exactly
one win, with the loser told to merge and retry.

## Scopes

- **Project**: documents that belong to one project. Most team
  notes live here.
- **Organisation**: documents shared across every project in an
  organisation. The skill bundle's per-organisation overrides land
  here.
- **Global**: documents shared by every project on the instance.
  The default skill bundle (`internal/skill/files/`) is mirrored
  into this scope at startup.

## How an agent uses the store

The Docs view is the human-facing surface; the agent reads and
writes through the [MCP docs domain](/skills/docs/). Tools, in
the order they typically appear in a session:

- `nottario.docs.list { project_id, scope, path_prefix? }` walks
  the tree to find what's there without pulling bodies.
- `nottario.docs.read { project_id, scope, path }` returns the
  full document including its parsed frontmatter and the
  `current_version` integer. The agent stashes that integer.
- `nottario.docs.search { project_id, scope, query }` is full-text
  across title, description and body.
- `nottario.docs.write { project_id, scope, path, content,
  expected_version, message }` commits an edit. `content` is the
  full markdown including frontmatter; the server splits and
  stores the two halves. `expected_version` is the integer from
  the most recent read — if the server's `current_version` no
  longer matches, the call returns `version_conflict` with the
  fresh number and the agent re-reads, merges, retries. This is
  optimistic concurrency: two agents writing the same path see
  exactly one win.

The pattern is "read → edit → write with expected_version", same
as a git commit on top of a known tip. The web UI follows the
identical contract under the hood.
