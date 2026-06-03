---
name: nottario-identity
description: Detailed reference on how Nottario authenticates agents and resolves the calling user.
---

# Identity reference

## Authentication

Nottario MCP authenticates agents with **per-project API tokens**:

- Header: `Authorization: Bearer ntr_<random>`.
- Each token is bound to **exactly one project**. One token = one
  project. An agent using a token issued for project A cannot read or
  write anything in project B, even if the underlying user is a
  member of both projects.
- Tokens are issued from the web UI under **project → Settings →
  Tokens → New token** and shown to the human only once at issuance
  time.
- The token belongs to one user; the agent acts on that user's behalf
  *within the token's project*.
- An admin token still respects project scope. `is_admin` is an
  instance-wide concept (used for cross-project admin actions via the
  web UI); it does **not** bypass per-project token scoping for MCP
  calls.

The MCP server has no cookies, no sessions, no refresh tokens. If a
token is revoked, the very next call fails with
`401 Unauthorized` carrying `WWW-Authenticate: Bearer realm="nottario"`.

If you pass a `project_id` argument that doesn't match the token's
project, the tool returns `"token scoped to project X, request
targets Y"`. Cache the project id from `whoami` and reuse it
everywhere.

## What `whoami` returns

| Field          | Meaning                                                                        |
|----------------|--------------------------------------------------------------------------------|
| `user_id`      | uuid of the user the token belongs to.                                         |
| `github_login` | the user's GitHub handle (the same one shown in the web UI).                   |
| `display_name` | the user's GitHub display name (falls back to the login).                      |
| `is_admin`     | `true` if the user is the instance admin (first GitHub user to ever log in). Does not grant cross-project access for token callers. |
| `source`       | always `"token"` for MCP callers (other surfaces may use `"session"`).         |
| `token_id`     | uuid of the specific API token presented.                                      |
| `memberships`  | array of `{project_id, project_slug, project_name, role_id, role_key, role_label, role_color, role_position}`. For token callers this list is **filtered to the token's project only** — every entry refers to the same `project_id`, one per role the user holds in that project. Use `memberships[0].ProjectID` as the canonical `project_id` for the rest of the session. |

## Roles

A user can hold zero or more roles in any given project. Roles are
defined per-project (commonly `backend`, `frontend`, `qa`, `design`,
and any custom ones the team adds).

- A task targeted at a role (`target_role_id`) is eligible for anyone
  holding that role in that project.
- A task assigned to a user (`assignee_user_id`) is eligible only for
  that user.
- A task with both fields can still be picked up by the assignee; the
  `target_role` becomes informational.

## Project visibility

- For **session (cookie) callers** in the web UI: admins see every
  project; other users see projects where they have at least one
  membership.
- For **token (MCP) callers**: visibility is restricted to the
  token's project, regardless of how many memberships the user
  actually has. `projects.list` returns just that one project;
  `projects.get` rejects any other id.

## Recovering from a scope mismatch

If you see `"token scoped to project X, request targets Y"`:

1. Re-run `nottario.whoami` to make sure you're talking to the right
   project.
2. Replace your cached `project_id` with the value `whoami` returned.
3. Retry the call.

If the human needs the agent to work on a different project, they
must issue a new token from that project's Settings → Tokens panel
and point the MCP client at the new token (`claude mcp remove
nottario -s local` then `claude mcp add … --scope local` with the
fresh secret).

## Per-token defaults

Humans can set a `default_role_id` on the token at issuance time
(picked from the catalogue of the token's project). The agent does
not need to remember it — calls like `tasks.claim_next` accept the
role filter explicitly; the default is only used by the web UI's
"create with these defaults" affordance.
