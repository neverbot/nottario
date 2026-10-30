---
title: Live and questions
section: Views
nav_order: 4
---

# Live and questions

Agents working in the background, or several conversations at once,
leave their progress and their questions scattered across terminals.
Two views bring them into one place: **Live** shows what is being
worked on right now, and **Questions** collects what the agents need
you to decide.

Both exist per project (next to Board and Gantt) and across every
project you belong to (in the top bar). They update in real time over
the same SSE stream as the board.

## Live

A list of the work in progress, in three blocks:

- **Waiting for you**: open questions addressed to you, oldest first,
  with an **Answer** button that opens the answer form in place.
- **Working now**: every task in `doing`, most recent activity first.
  Each row shows the task, the step in progress (`3/8 · Write
  migration`), the agent conversation working on it, the assignee, how
  long it has been going, and an amber pill when it has open questions.
  Work by humans without an agent shows too.
- **Finished recently**: tasks closed in the last 24 hours, dimmed.

A row with no activity for a while is marked idle in grey. The
threshold is a project setting, **Live: mark work idle after**, from 5
to 1440 minutes (default 30), under the project's Settings.

The conversation chip copies what you need to get back to it: the
resume command for clients Nottario knows (for Claude Code,
`claude --resume <id>`), or the bare conversation id for any other
client.

## Questions

When an agent needs a decision only a human can make, it files a
question on the task it is working on, optionally on one step. A
question is free text, or 2 to 9 options with an optional recommended
one and its reason; you can always answer with your own text instead
and add a note.

- **Only the recipient answers.** A question goes to the human who owns
  the agent's token. Other members of the project see it read-only.
- You get a bell notification when an agent asks; turn it off under
  your profile's notification preferences ("An agent asks me a
  question").
- The **Questions** view has **Open**, **Answered** and **All** tabs.
  Keys 1-9 pick an option in the focused question, j and k move between
  questions, and Cmd/Ctrl+Enter sends a free-text answer.
- The task dialog has a **Questions** section after Steps, and Kanban
  cards with open questions carry an amber pill.
- An answer can be edited until the agent picks it up; then it reads
  **Delivered** and is final. If the agent's conversation has ended,
  the answer shows the resume command (or the conversation id) so you
  can reopen it.

Open questions block nothing: the agent keeps working on what does not
depend on the answer. Closing a task withdraws its open questions.

## How an agent drives these views

Any MCP client works; nothing here is specific to one AI tool. The
agent passes its conversation id and a short label as `session` and
`session_label` when it claims or creates a task, starts a step with
`nottario.tasks.start_step`, and asks with `nottario.questions.ask`.
It collects answers with `nottario.questions.take` at fixed points of
its loop (start of every turn, before picking the next task, before
ending a turn): answers are pulled, never pushed into a running
conversation. See [MCP integration](/mcp/) and the
[questions skill](/skills/questions/).
