---
name: nottario-identity
description: Detailed reference on how Nottario authenticates agents and resolves the calling user.
---

# Identity reference

## Authentication

Nottario MCP authenticates agents with **API tokens**:

- Header: `Authorization: Bearer ntr_<random>`.
- Tokens are issued from the web UI by a human under **Tokens → New
  token** and shown to the human only once at issuance time.
- The token belongs to one user; the agent acts on that user's behalf.
- An admin can issue tokens that double as admin credentials by the
  simple fact that the owning user has `is_admin = true`.

The MCP server has no cookies, no sessions, no refresh tokens. If a
token is revoked, the very next call fails with
`401 Unauthorized` carrying `WWW-Authenticate: Bearer realm="nottario"`.

## What `whoami` returns

| Field          | Meaning                                                                        |
|----------------|--------------------------------------------------------------------------------|
| `user_id`      | uuid of the user the token belongs to.                                         |
| `github_login` | the user's GitHub handle (the same one shown in the web UI).                   |
| `display_name` | the user's GitHub display name (falls back to the login).                      |
| `is_admin`     | `true` if the user is the instance admin (first GitHub user to ever log in).   |
| `source`       | always `"token"` for MCP callers (other surfaces may use `"session"`).         |
| `token_id`     | uuid of the specific API token presented.                                      |

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

- Admins see every project on the instance.
- Other users see only the projects where they have at least one
  membership. Calling tools against a project you cannot see returns
  the error `"not a project member"`.

## Per-token defaults (future)

Today the MCP server is stateless. Future iterations may let humans set
a `default_role_id` on the token from the web UI, but **the agent must
still pass `project_id` explicitly on every call.** Do not write code
that assumes server-side "active project" state.
