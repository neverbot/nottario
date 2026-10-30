---
name: nottario-domain-questions
description: 'Reference for Nottario questions: asking the human through Nottario, collecting the answers, recording answers given in the chat, helper-agent hand-off, what to say at the end of a turn.'
---

# Questions domain — reference

A question is something only the human can decide, filed on a task
(and optionally one of its steps) by the conversation working on it.
The human answers in the web UI; your conversation collects the
answer the next time it calls `questions.take`. Nothing is pushed to
you: delivery is pull.

**The recipient is the human who owns your token.** Only that user can
answer, and edit the answer until you take it; other project members
see the question read-only. A question about someone else's decision
still goes to your human, who can pass it on.

Open questions block nothing. The task and step show "waiting for
answer" in the UI, `tasks.list` rows carry `questions_open`, and
`tasks.get` lists the open and the answered-but-not-taken ones under
`questions`.

## Sessions

Every question belongs to a session: the conversation that asked it,
which is also the only one that can take its answer. The session is
the id from `skill.md` §1 (your client's own conversation id if it
exposes one, e.g. Claude Code's `$CLAUDE_CODE_SESSION_ID`; otherwise a
UUID you generated once), 1 to 128 characters. `session_label`, up to
80 characters, names the work for the human. Use the same session for
every call of the conversation, or the answers will not find you.

## Tools

The tools sit in the `questions` toolset domain (`?tools=questions,...`
on the MCP URL).

### `nottario.questions.ask`

```text
nottario.questions.ask { project_id, task_id, step_id?, session, session_label?,
                         question, options?, recommended?, recommended_reason?, multi? }
# → {id, state: "open"}
```

- `question`: markdown, 1 to 4000 characters.
- No `options`: a free-text question.
- `options`: 2 to 9 `{label, description?}`; label up to 120
  characters, description up to 300. The human can always answer with
  their own text instead, and add a note.
- `recommended`: the 1-based number of the option you recommend, with
  `recommended_reason` (one line, up to 300 characters).
- `multi: true`: the human may pick several options.

A bad question is rejected before anything is written.

### `nottario.questions.take`

```text
nottario.questions.take { project_id, session }
# → {answers: [{id, task_id, step_id?, question, choices?, text?, note?, answered_by}], open}
```

Returns the answers to your session's questions you have not taken
yet, and marks them delivered: each answer comes back exactly once,
so act on it (or write it down) when you get it. `choices` holds the
labels picked, `text` a free-text or "other" answer, `note` an extra
remark. `open` counts your questions still unanswered. Once taken, the
answer is final; the human can no longer edit it.

### `nottario.questions.record_answer`

```text
nottario.questions.record_answer { project_id, id, choices?, text?, note?, channel? }
# → {id, state: "answered"}
```

**Only** for an answer the human already gave you outside Nottario (in
the chat, or `channel: "other"` for any other medium) to an open
question you asked. Never use it to answer on the human's behalf, to
guess, or to close a question nobody answered. `choices` are the
1-based option numbers they picked, as you listed them in `ask`;
`text` is their own words (a free-text answer, or their wording
instead of the options); `note` anything else they added. The answer
is attributed to the human, marked as given in the chat and recorded
by you, and is delivered at once: `take` will not return it. A question
already answered (in the web UI, say) or withdrawn is refused.

### `nottario.questions.list`

```text
nottario.questions.list { project_id, state?, task_id?, session?, limit? }
# → {questions: [{id, task_id, step_id?, state, kind, question, asked_at}], has_more}
```

`state`: `open` (default), `answered`, `withdrawn` or `all`. `limit`
defaults to 50 (max 200); when `has_more`, narrow the filter. The
`question` is an excerpt. Reading does not mark anything delivered.

### `nottario.questions.withdraw`

```text
nottario.questions.withdraw { project_id, id, reason? }
```

Withdraws an open question you asked; only its asker can. Closing a
task by any path (`tasks.close`, `set_state done` or `wont_do`)
withdraws its open questions; the response adds `withdrawn_questions`
and a `notice`.

## Always ask in Nottario

Every question for the human goes to Nottario with `questions.ask`,
always: it is the record of what was asked and decided, and what the
human sees in the Questions and Live views. If you think the human is
in the chat to answer, you may also ask it there.

When the human answers outside Nottario (in the chat, or anywhere
else), call `questions.record_answer` with their answer straight away,
so the question is answered in Nottario too and nobody answers it
twice. Answers given in Nottario reach you through `questions.take`.

After asking, keep working on whatever does not depend on the answer:
other steps, other tasks. Mention each question you filed in one line
of your reply.

## Writing a good question

Give the context, what you need, and what each answer leads to. The
human reads it cold, maybe hours later, next to other questions.

Free text:

```text
question: "The import job reads `customers.csv` from S3. Staging has
  no bucket and no credentials in the repo. Which bucket and prefix
  should it read in staging? I will add them to `config/staging.yml`;
  until then the job stays disabled there."
```

Choice:

```text
question: "Deleting a project must also deal with its 40 GB of
  uploads. How should that work?"
options: [
  { label: "Delete at once",       description: "Simple; nothing to undo" },
  { label: "Soft-delete 30 days",  description: "Restorable; a nightly job purges" },
  { label: "Keep forever",         description: "Only the database rows go" } ]
recommended: 2
recommended_reason: "Matches how tasks are deleted today"
```

One decision per question. Prefer options when the choices are known:
the human answers in one keystroke.

## When to take answers

Call `questions.take` with your session:

- at the start of every turn;
- after each helper agent returns;
- before picking the next task;
- before ending a turn.

An answer may change the plan: act on it before continuing.

## Helper agents (subagents)

If your client runs helper agents, only the main agent asks and takes:
the answers go back to the session that asked, and a helper's
conversation ends before the human answers.

- Give each helper your session id in its prompt, for `start_step` on
  the steps it works.
- A helper never calls `questions.ask` or `take`. It ends its report
  with a list under `Questions for the human:`, each with its task (and
  step) and options if it has them.
- You file each one with `questions.ask` under your own session.

## Ending a turn with open questions

List each open question in one line (task and the gist) and say they
can be answered in Nottario's Questions view. On the next turn, take
the answers first.

```text
Waiting on you (answer in Nottario, Questions view):
- Import job: which S3 bucket for staging?
- Project deletion: delete at once, soft-delete or keep?
```

## How the human gets back to you

The human answers in the Questions view, the Live view or the task
dialog, and gets a bell notification when you ask. If your
conversation has ended by then, the UI offers the way back: the resume
command for clients Nottario knows (Claude Code: `claude --resume
<id>`), or the bare conversation id to copy for any other client. Once
resumed, take with the session you asked with, even if your client now
reports a new id; `questions.list { session }` shows what is still
open under it.

## Obsolete questions

Withdraw a question as soon as it no longer matters: you found the
answer elsewhere or the plan changed. If the human answered it in the
chat, record that answer (`questions.record_answer`) instead.
A stale question wastes the human's time.
