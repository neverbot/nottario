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

Agents drive the same store via `nottario.docs.list`,
`nottario.docs.read` and `nottario.docs.write`. See the
[MCP integration](/mcp/) page for the tool contract.
