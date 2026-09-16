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
to, and addressable by a single MCP tool.

The files in this section are **mirrored straight from
`internal/skill/files/`** in the source tree at every deploy. What
you read here is exactly what the latest container ships to agents.
Drift between the docs and the bundle is impossible: the build job
copies the markdown from disk every time the site is rebuilt.

## How an agent installs the bundle

There is **one** tool: `nottario.skill.install`. It returns a small
JSON descriptor — a short-lived signed URL for the bundle as a single
zip, plus an install plan. The bundle content itself **never flows
through the MCP response**; the agent reads ~200 tokens of metadata
and the bytes travel server → disk directly.

Response shape:

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
    "instructions":  "Fetch the URL with any HTTP tool, unzip into preferred_dir, restart the client."
  }
}
```

The signed URL is valid for five minutes and needs no
`Authorization` header. The agent picks whatever HTTP tool the host
exposes (`curl`, `wget`, PowerShell's `Invoke-WebRequest`, Python's
`urllib`, Node's `fetch`, …), saves the zip, then extracts it into the
client's skill directory. **Pre-loaded installs are the only mode** —
the bundle lives on disk because most agent hosts (Claude Code and
similar clients) load skills at session start.

Workspace path is preferred (`<repo>/.claude/skills/nottario/` —
scoped to one repo and committed alongside the code so every
contributor gets it); the home path (`~/.claude/skills/nottario/`)
is the fallback when you work on Nottario across unrelated checkouts
or do not want to version the bundle.

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

The session has to be restarted after a sync — most agent hosts read
the skill bundle once at session start and do not re-scan. The agent
should remind the human of this explicitly when it finishes the
install, naming the host application if it knows which one it is.

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

## Overrides

Each Nottario instance can extend or tighten the bundle without
rebuilding the binary: an admin writes a document with `scope=global`,
`kind=skill`, `path=global/skills/<file>`. That override is included
in the next `skill.install` snapshot transparently — overrides change
`bundle_version`, so the next sync picks them up.

Overrides can only be written by an admin signed in to the web app.
API tokens — including an admin's — are refused for any global
document, because an override changes the instructions of agents on
every project while a token is scoped to a single one.

Use this to add bundle-absent files (`by-language/go.md`,
`by-role/security.md`, `recipes/deploying-to-our-k8s.md`).

## The bundle

The pages listed in the **Skills** section of the side nav are the
exact files that ship with Nottario. Start with the [overview](/skills/overview/)
and read deeper into a domain as the task at hand requires.
