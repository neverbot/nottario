---
title: Agent skills
section: Skills
nav_order: 0
---

# Agent skills

Nottario ships a small bundle of markdown files that teach an AI
agent how to work inside a Nottario project: how to identify
itself, how to find the next task, how to record progress, what
each domain (tasks, cycles, docs, architecture) means. The bundle
is the same content a human contributor would read in this
documentation — but written in the imperative voice agents respond
to, and addressable by tool calls.

The files in this section are **mirrored straight from
`internal/skill/files/`** in the source tree at every deploy. What
you read here is exactly what the latest container ships to agents.
Drift between the docs and the bundle is impossible: the build job
copies the markdown from disk every time the site is rebuilt.

## How an agent uses them

Three paths, equivalent in content. The first is the recommended
default; the next two are options when you want the bundle
pre-loaded into the session.

**1. On-demand via MCP** *(recommended)*. Once an agent is wired
through the [MCP integration](/mcp/), it can call
`nottario.skill.list` to see what's available and
`nottario.skill.read` to pull a specific file into its context.
The agent doesn't need anything installed locally — the bundle
travels with the server. Updates ship the moment a new container
is pulled. This keeps the agent's context lean: it only pulls the
domain it needs (tasks, cycles, docs, architecture) for the work
in front of it.

**2. Workspace-scoped install** *(when you're checked into the
tracked repo)*. Drop the contents of `internal/skill/files/` into
`<repo>/.claude/skills/nottario/` and commit them. Claude Code
loads the skill whenever the workspace is opened, scoped to that
repo. Every contributor who clones gets the same rules without
manual setup. Prefer this over the home install whenever the
project is a real checkout — it keeps the rules where the work
lives.

**3. Home install** *(fallback)*. The same tree can go into
`~/.claude/skills/nottario/` instead. The skill loads on every
Claude Code session regardless of the current directory, which is
convenient if you work on the project across multiple unrelated
checkouts or you don't want to version the bundle in any specific
repo. The trade-off is that the skill also loads in sessions on
repos that have nothing to do with Nottario.

Both pre-loaded paths are snapshots: re-copy the files when you
upgrade the server. The MCP path is the only one that stays in
sync automatically.

## Anatomy of a skill

Every file starts with a tiny YAML front-matter block declaring
its `name` and `description`. The description is what a model sees
when it decides whether the skill is relevant — keep it specific
about WHEN to invoke, not just WHAT the skill covers.

```
---
name: nottario-domain-tasks
description: Complete reference for the Nottario tasks domain: schema, semantics, every tool, edge cases.
---

# Tasks domain — full reference
…
```

The body is plain markdown — same conventions as the rest of this
site. Code blocks for tool invocations, tables for schemas,
imperative sentences ("always", "never", "before X, do Y") for
operating rules.

## The bundle

The pages listed in the **Skills** section of the side nav are the
exact files that ship with Nottario. Start with the [overview](/skills/overview/)
and read deeper into a domain as the task at hand requires.
