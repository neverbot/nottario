-- name: InsertTaskQuestion :one
INSERT INTO task_questions (project_id, task_id, step_id, session_id, asked_by_user_id,
  asked_by_token_id, recipient_user_id, kind, body_md, options, multi, recommended_key,
  recommended_reason)
VALUES (sqlc.arg('project_id')::uuid, sqlc.arg('task_id')::uuid, sqlc.narg('step_id')::uuid,
  sqlc.narg('session_id')::uuid, sqlc.narg('asked_by_user_id')::uuid,
  sqlc.narg('asked_by_token_id')::uuid, sqlc.narg('recipient_user_id')::uuid,
  sqlc.arg('kind')::text, sqlc.arg('body_md')::text, sqlc.narg('options')::jsonb,
  sqlc.arg('multi')::boolean, sqlc.narg('recommended_key')::text,
  sqlc.arg('recommended_reason')::text)
RETURNING *;

-- name: GetTaskQuestion :one
SELECT * FROM task_questions
WHERE id = sqlc.arg('id')::uuid AND project_id = sqlc.arg('project_id')::uuid;

-- name: GetTaskQuestionForUpdate :one
SELECT * FROM task_questions
WHERE id = sqlc.arg('id')::uuid AND project_id = sqlc.arg('project_id')::uuid
FOR UPDATE;

-- name: AnswerTaskQuestion :one
-- Records or replaces an answer. The caller holds the row lock and has
-- checked the question is open, or answered and not yet delivered.
-- answered_at moves on every edit, so Take hands answers over in the
-- order of their last edit.
UPDATE task_questions SET state = 'answered',
  answer_keys = sqlc.arg('keys')::text[], answer_text = sqlc.arg('text')::text,
  answer_note = sqlc.arg('note')::text, answered_by_user_id = sqlc.arg('user_id')::uuid,
  answered_at = now(), updated_at = now()
WHERE id = sqlc.arg('id')::uuid
RETURNING *;

-- name: RecordTaskAnswer :one
-- Records an answer the human gave the asking agent outside Nottario.
-- The caller holds the row lock and has checked the question is open.
-- The agent already has the answer, so it is delivered at once.
UPDATE task_questions SET state = 'answered',
  answer_keys = sqlc.arg('keys')::text[], answer_text = sqlc.arg('text')::text,
  answer_note = sqlc.arg('note')::text, answered_by_user_id = sqlc.narg('user_id')::uuid,
  answered_by_token_id = sqlc.narg('token_id')::uuid, answer_channel = sqlc.arg('channel')::text,
  answered_at = now(), delivered_at = now(), updated_at = now()
WHERE id = sqlc.arg('id')::uuid
RETURNING *;

-- name: TakeTaskAnswers :many
-- Hands a session its new answers and marks them delivered, once:
-- SKIP LOCKED keeps two concurrent takes from returning the same row,
-- and skips a row an answer edit is holding.
UPDATE task_questions SET delivered_at = now(), updated_at = now()
WHERE id IN (
  SELECT q.id FROM task_questions q
  WHERE q.project_id = sqlc.arg('project_id')::uuid
    AND q.session_id = sqlc.arg('session_id')::uuid
    AND q.state = 'answered' AND q.delivered_at IS NULL
  ORDER BY q.answered_at
  FOR UPDATE SKIP LOCKED)
RETURNING *;

-- name: CountOpenQuestionsForSession :one
SELECT count(*)::int FROM task_questions
WHERE project_id = sqlc.arg('project_id')::uuid
  AND session_id = sqlc.arg('session_id')::uuid
  AND state = 'open';

-- name: WithdrawTaskQuestion :one
-- Only the user who asked may withdraw a question.
UPDATE task_questions SET state = 'withdrawn', withdrawn_at = now(),
  withdrawn_reason = sqlc.arg('reason')::text, updated_at = now()
WHERE id = sqlc.arg('id')::uuid AND project_id = sqlc.arg('project_id')::uuid AND state = 'open'
  AND asked_by_user_id = sqlc.arg('user_id')::uuid
RETURNING *;

-- name: WithdrawOpenTaskQuestions :many
-- Run by the tasks package inside the transaction that closes the task.
UPDATE task_questions SET state = 'withdrawn', withdrawn_at = now(),
  withdrawn_reason = sqlc.arg('reason')::text, updated_at = now()
WHERE task_id = sqlc.arg('task_id')::uuid AND state = 'open'
RETURNING id, body_md;

-- name: ListTaskQuestions :many
-- Filters are optional. project_ids scopes the global views to the
-- caller's projects. Open questions come first, oldest first; the rest
-- follow, most recently changed first.
SELECT * FROM task_questions
WHERE project_id = ANY(sqlc.arg('project_ids')::uuid[])
  AND (sqlc.narg('state')::text IS NULL OR state = sqlc.narg('state')::text)
  AND (sqlc.narg('task_id')::uuid IS NULL OR task_id = sqlc.narg('task_id')::uuid)
  AND (sqlc.narg('session_id')::uuid IS NULL OR session_id = sqlc.narg('session_id')::uuid)
  AND (sqlc.narg('recipient_user_id')::uuid IS NULL OR recipient_user_id = sqlc.narg('recipient_user_id')::uuid)
ORDER BY (state = 'open') DESC,
         CASE WHEN state = 'open' THEN created_at END ASC,
         updated_at DESC
LIMIT sqlc.arg('lim')::int;

-- name: CountOpenTaskQuestions :many
-- Open question counts for a page of tasks. Tasks without open
-- questions produce no row.
SELECT task_id, count(*)::int AS open FROM task_questions
WHERE task_id = ANY(sqlc.arg('ids')::uuid[]) AND state = 'open'
GROUP BY task_id;

-- name: ListPendingTaskQuestions :many
-- A task's questions an agent still has to act on: open ones, and
-- answers the asking session has not taken yet. Oldest first.
SELECT * FROM task_questions
WHERE task_id = sqlc.arg('task_id')::uuid AND project_id = sqlc.arg('project_id')::uuid
  AND (state = 'open' OR (state = 'answered' AND delivered_at IS NULL))
ORDER BY created_at;

-- name: GetTasksBrief :many
-- Batch lookup for the question views: no N+1.
SELECT id, project_id, title FROM tasks WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: GetStepsBrief :many
SELECT id, text FROM task_steps WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: GetProjectsBrief :many
SELECT id, name, slug FROM projects WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: GetUsersBrief :many
SELECT id, github_login, display_name, COALESCE(avatar_url, '')::text AS avatar_url
FROM users WHERE id = ANY(sqlc.arg('ids')::uuid[]);
