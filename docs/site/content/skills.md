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

Two paths, equivalent in content; pick by ergonomics:

**1. On-demand via MCP.** Once an agent is wired through the
[MCP integration](/mcp/), it can call `nottario.skill.list` to see
what's available and `nottario.skill.read` to pull a specific file
into its context. The agent doesn't need anything installed
locally — the skill bundle travels with the server. Updates ship
the moment a new container is pulled.

**2. Pre-loaded into Claude Code.** The repo's
`internal/skill/files/` tree can be dropped into
`~/.claude/skills/nottario/` (or installed via the same MCP
server's helper) so the skill is visible to the agent at session
start without any tool call. Useful when the agent's first action
should already be informed by Nottario conventions, or when the
MCP server might not be reachable.

The on-demand path is the recommended one for self-hosters: it
keeps the agent's context lean by default, and the agent pulls
only the domain it needs (tasks, cycles, docs, architecture) for
the work in front of it.

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
