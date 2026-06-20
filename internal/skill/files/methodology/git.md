---
name: nottario-methodology-git
description: How agents drive git across the three concurrency shapes a Nottario project can live in (single-agent, parallel agents, multi-dev) — branch choice, worktrees, push policy, exact commands.
---

# Git methodology for agents

Nottario coordinates work but **does not store code**. Every agent
eventually has to choose: which branch, when to branch, when to PR,
how not to step on a parallel agent. The skill below tells you which
of the three modes you are in, and the exact commands to use.

## TL;DR — the rules that never change

These apply across every mode. Memorise them; the rest is shape.

- **You never push.** Pushing is a human action. Even when you finish
  a task, you commit and stop. The human pushes (or asks you to
  explicitly, in which case go ahead). This rule survives mode A, B
  and C. Local commits are the agent's job; the network is the
  human's.
- **No force-push.** Ever. Not to your own branch, not to a shared
  one. If your rebase has finished and the only way to publish is
  `--force`, stop and ask the human.
- **No `--no-verify`, no `--no-gpg-sign`.** Hooks exist for a reason.
  If a hook fails, fix the underlying issue.
- **No amend on pushed commits.** Amending local-only commits is fine
  per the `feedback-amend-small-doc-fixups` pattern; verify the commit
  is local with `git log @{u}..HEAD` first.
- **Conventional Commits, single line, no body, no trailers.** Match
  the project's existing log style; the global rule lives in
  `~/.claude/CLAUDE.md`.
- **`git add <specific files>`**, never `git add -A` / `git add .`.
  Avoid staging `.env`, credentials, large binaries, or unrelated
  noise by accident.

## Decide the mode at session start

Run this discovery once at the beginning of your session. Cache the
answer for the rest of the conversation.

```text
1. git config remote.origin.url
     → empty? You are in mode A. Done.
     → present? continue.
2. nottario.projects.get { project_id }
     → count distinct humans (memberships, deduped by user_id).
     → 1 human → candidate for mode A or B.
     → >1 human → mode C. Done.
3. mode A vs B (single human):
     → If there is no signal of a sibling agent on this repo
       (no other recent worktrees, no other live MCP sessions on
       the same project, the human did not say "I have another
       agent running"), assume mode A.
     → If the human told you "I am running parallel agents on
       this project" or you can see a sibling worktree under
       `../<repo>-agent-*`, use mode B.
4. When the signal is ambiguous (one human now but the project is
   shared, or you don't know if siblings exist), default to the
   **safer** mode: B over A, C over B. Wider mode never breaks
   narrower; narrower mode breaks wider.
```

Same-task multi-agent coordination (a "main" agent dispatching
helpers) is NOT this skill's problem — that orchestration happens
above Nottario and the helpers agree among themselves how not to
collide. This document is about **different tasks** running in
parallel on the same repo.

## Mode A — single human, single (or sequential) agent

The simplest shape, and what most Nottario projects live in today.
One human, one agent session at a time.

### Where you commit

Directly on the project's **integration branch** (`master` for
Nottario; some projects use `main` or `develop`). Discover it once:

```bash
git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's@^refs/remotes/origin/@@'
# fallback if no remote / no symbolic ref:
echo master
```

Cache the result.

### The committing loop

```bash
# Before each commit, sync to avoid stranding history on a stale
# base if the human pushed something behind your back.
git pull --rebase

# Stage only the files you changed.
git add internal/foo/bar.go internal/foo/bar_test.go
git commit -m "feat(foo): explain bar in one line"

# No push. The human pushes when they want to.
```

If `git pull --rebase` reports conflicts:

1. Open each conflicted file. If the resolution is mechanical
   (same line edited two different ways with one obviously correct),
   resolve it.
2. If any chunk is unclear (intent mismatch, unfamiliar code), abort
   with `git rebase --abort` and report to the human with the
   conflict markers preserved in the message. Do not guess.

### No branches, no PRs

Mode A doesn't use branches per task. The task lives in Nottario,
not in git. Branch noise on the integration line is the cost you
pay in modes B/C to get isolation — when there's no isolation
problem, you don't pay it.

## Mode B — single human, multiple parallel agents

The human runs 2+ agent sessions against the same repo at the
same time (multiple terminals of the same client, or different
clients). Each session is working on a **different Nottario task**.
The risk is two agents touching the same files concurrently and
producing a fight that the human has to untangle.

### Worktree per session

Before any code work, create a git worktree on a branch named for
your task:

```bash
# Replace 5ff29046 with the first 8 chars of your task id.
git worktree add ../$(basename $PWD)-agent-5ff29046 -b agent/5ff29046
cd ../$(basename $PWD)-agent-5ff29046
```

The worktree shares the `.git` database with the original checkout
but has its own working tree and HEAD, so two agents can be on two
different branches editing two different file sets without
stepping on each other's working copies.

### The committing loop inside the worktree

```bash
# Sync the branch's base with master before each commit.
git fetch origin
git rebase origin/master   # use the integration branch you discovered

# Stage only your files.
git add internal/foo/bar.go
git commit -m "feat(foo): explain bar in one line"

# No push.
```

### Closing the task: fast-forward merge

When the task is done and the work is committed on `agent/5ff29046`:

```bash
# Inside the worktree — rebase one last time on freshly-fetched master.
git fetch origin
git rebase origin/master

# From the MAIN checkout (not the worktree) — fast-forward merge.
# Switch to the main checkout's working directory first.
cd ../<original-repo-dir>
git checkout master
git merge --ff-only agent/5ff29046
```

If `git merge --ff-only` fails — because another sibling agent
fast-forwarded master between your rebase and your merge — repeat:
re-rebase the agent branch on the now-newer `origin/master`, then
re-merge. The retry converges because each merge advances master by
exactly one chain of commits. **No human intervention needed for
clean diffs.**

For diffs that are not clean (your changes conflict with what a
sibling merged), abort the rebase, report to the human with the
conflicting files, and let them resolve.

### Cleanup

```bash
git worktree remove ../<repo>-agent-5ff29046
git branch -d agent/5ff29046
```

The task is already closed in Nottario at this point. The worktree
and branch are bookkeeping; remove them so the next agent has a
clean view.

### Same-task siblings: not your problem

If the human spawns two agents to work on the **same** task, that
choice was made above Nottario — they have orchestrated themselves
(typically through a "main" agent that dispatches sub-tasks to
helpers). You don't try to coordinate with siblings on the same
task; you trust the orchestration that put you there. Nottario's
visible signal — `assignee_user_id` + `via_mcp.name` on the task
row — is the breadcrumb a human can read after the fact to see
which token did what.

## Mode C — multiple devs (and their agents)

The repo is shared across humans. Direct commits to the integration
branch don't fly: code review is part of the contract.

### Branch per task

```bash
# 5ff29046 = first 8 chars of the Nottario task id.
# arch-versioning-skeleton = short kebab-case slug of the title.
git checkout -b 5ff29046/arch-versioning-skeleton origin/master
```

The naming convention `<task-id-short>/<short-slug>` lets any future
agent reading `git log --oneline` map a branch back to a Nottario
task without guessing.

### Committing inside the branch

```bash
git fetch origin
git rebase origin/master    # rebase, do not merge — keep PR diff clean
git add internal/foo/bar.go
git commit -m "feat(foo): explain bar in one line"

# No push. The human pushes the branch when they want to.
```

### Opening a PR

Before opening the PR the **human** pushes the branch:

```bash
# Human does this — not you.
git push -u origin 5ff29046/arch-versioning-skeleton
```

After the human pushes, **you** open the PR via `gh`:

```bash
gh pr create --title "feat(foo): explain bar in one line" --body "$(cat <<'EOF'
## Summary
- One-line description tying back to the Nottario task.

## Task
Closes [`<task title>`](https://nottario.example/projects/<id>/tasks/<task-id>).

## Linked commits
- abc1234 — first commit
- def5678 — second commit

## Test plan
- [ ] Bullet list of what to verify.
EOF
)"
```

The PR description must carry:

- The Nottario task id (the human reading the PR needs the back-link).
- The list of commits already linked to that task in Nottario.
- A test plan as a markdown checklist.

### Review and merge

The merge is a human action. You do not auto-merge. Once merged:

```bash
git checkout master
git pull
git branch -d 5ff29046/arch-versioning-skeleton
# The human deletes the remote branch if their merge didn't auto-delete it.
```

## Cross-mode quick reference

| Concern | Mode A | Mode B | Mode C |
|---|---|---|---|
| Where you commit | `master` directly | `agent/<task-id>` in worktree | `<task-id>/<slug>` branch |
| Sync cadence | `git pull --rebase` before each commit | `git fetch && git rebase origin/master` | same as B |
| Closes via | nothing — task closed in Nottario | `--ff-only` merge in main checkout | PR via `gh` |
| Push | human only | human only | human pushes branch, you open PR |
| Force push | banned | banned | banned |
| Branch cleanup | n/a | `git worktree remove` + `git branch -d` | local + remote delete after merge |

## What changes nothing across modes

- `nottario.tasks.link_commit { repo, sha }` for every commit the
  task produced. The audit trail is independent of branch shape.
- Conventional Commits, single line, no trailers, no body. The log
  reads the same in all three modes.
- The closing-the-loop ritual (find/file related tasks, walk the
  arch diagram, capture side-comments as new tasks). See `skill.md`
  §4.
- "Never push without explicit human request." Wider versions of
  the rule (Mode C requires no auto-merge etc.) are layered on top
  of this base.

## What this document does NOT cover

- **How siblings on the same task coordinate.** The orchestrating
  agent decides. Nottario shows you who did what via
  `assignee_user_id` + `via_mcp.name` so a human can read the trail
  after the fact; that is the only Nottario surface for it.
- **`default_branch` as project-level data.** Today you discover it
  with `git symbolic-ref refs/remotes/origin/HEAD` and fall back to
  `master`. Adding a column on `projects` to make it explicit is a
  separate (not yet filed) change; this document tracks the current
  state.
- **Force-push escape hatches.** There are none. Anything that
  requires force-push is a conversation with the human, not a path
  for the agent.
