---
title: Self-hosting reference
section: Reference
nav_order: 1
---

# Self-hosting reference

The full surface for running Nottario on your own host. The
[Getting started](/getting-started/) guide covers the happy path;
this page documents every knob.

## Environment variables

| Variable | Required | Default | What it does |
|---|---|---|---|
| `PUBLIC_URL` | yes | `http://localhost:8080` | The base URL users reach. Must match the OAuth callback host and use `https://` in production for `Secure` cookies. |
| `DATABASE_URL` | yes | — | pgx connection string. Migrations run automatically on startup. |
| `GITHUB_OAUTH_CLIENT_ID` | yes | — | From the GitHub OAuth App. |
| `GITHUB_OAUTH_CLIENT_SECRET` | yes | — | From the GitHub OAuth App. |
| `SESSION_KEY` | yes | — | 32 random bytes, base64-encoded. Generate once with `openssl rand -base64 32` and keep. Rotating logs everyone out. |
| `GITHUB_OAUTH_ORG` | no | (disabled) | Org slug; when set, only active members can sign in via OAuth. API tokens are unaffected. |
| `HTTP_ADDR` | no | `:8080` | Listen address. Change only if you run multiple instances on one host. |
| `NOTTARIO_BACKUP_DIR` | no | (disabled) | Mount-friendly path where periodic `pg_dump` backups land. Empty = no backups. |
| `NOTTARIO_BACKUP_AT` | no | `03:00` | Time of day for the daily backup, `HH:MM` 24h local time. |
| `NOTTARIO_BACKUP_KEEP_DAYS` | no | `7` | Delete dumps older than this many days after each successful run. |
| `SELF_UPDATE_CHECK_ENABLED` | no | `true` | Poll the upstream GitHub repository once a day and surface an admin-only banner when a newer commit lands on master. `false` skips the outbound request entirely. |
| `SELF_UPDATE_CHECK_INTERVAL` | no | `24h` | Go duration between checks. Clamped up to `1h` — the anonymous GitHub API has a 60 req/h/IP limit that leaves plenty of headroom, but there's nothing to gain from a tighter cadence. |
| `SELF_UPDATE_UPSTREAM` | no | `neverbot/nottario` | GitHub `owner/repo` to compare against. Change if you run a fork so the banner points at your own tree. |
| `NOTIFICATIONS_ENABLED` | no | `true` | Produce per-user notifications (bell in the topbar, drawer, preferences under `/me`). `false` hides the bell, refuses writes, and returns empty reads — no rows are inserted. Existing rows are preserved so the feature can be flipped back on without loss. |

## Secret files

Every secret variable also accepts a `_FILE` companion that points
at a file on disk:

- `SESSION_KEY_FILE`
- `GITHUB_OAUTH_CLIENT_SECRET_FILE`

The file's contents are read and trailing whitespace is stripped, so
`openssl rand -base64 32 > /run/secrets/session_key` works without
extra ceremony. The `_FILE` variant takes precedence when both are
set — recommended under Docker secrets or Kubernetes mounted
secrets.

> **Container UID.** The image runs as `nonroot` (UID **65532**, fixed
> by the distroless base image). The `_FILE` targets must be readable
> by that UID. With Docker Compose `file:` secrets the mounted file
> inherits the host's owner; if the host file is `0600 root:root`,
> Nottario can't read it. Either widen to `644` or `chown 65532:65532`.

## Backups

When `NOTTARIO_BACKUP_DIR` is set, an in-process goroutine fires
`pg_dump --format=custom` once a day at `NOTTARIO_BACKUP_AT` local
time. The container ships the matching `postgresql-client` so the
dump runs in-process; no sidecar needed. Files are named
`nottario-YYYY-MM-DDTHH-MM-SS.dump` and `NOTTARIO_BACKUP_KEEP_DAYS`
controls the rotation.

To restore: stop the container, drop the database, and run
`pg_restore --clean --if-exists -d <DATABASE_URL> <file.dump>`.

## Upgrade flow

```
docker compose pull nottario
docker compose up -d nottario
```

Migrations run on every startup. A migration that fails will keep
the previous container running (Compose health check), so a bad
upgrade does not lose data. If you pin to a `vX.Y.Z` tag instead of
`:latest`, upgrades are explicit: bump the tag in your
`compose.yml` and `pull`+`up`.

### Update notifications

To keep you from having to check GitHub yourself, an in-process
poller compares the commit SHA baked into the running binary against
the current `refs/heads/master` on the upstream repository. When
they differ, an admin-only banner appears under the topbar with the
exact `docker compose pull && docker compose up -d` command needed
to update. The poller never touches the container — it is purely
informational.

The check runs once at startup and then every
`SELF_UPDATE_CHECK_INTERVAL` (default 24h). It calls the anonymous
GitHub REST API (`https://api.github.com/repos/{upstream}/commits/master`)
with no authentication and a 5-second timeout; failures are logged
at warn level and the previous known-good state is preserved. Only
admins receive the `update_available: true` signal — regular
members can't run `docker compose pull` and there's no reason to
expose the running SHA to them.

To turn it off entirely (air-gapped hosts, privacy preferences),
set `SELF_UPDATE_CHECK_ENABLED=false`. To point at a fork, set
`SELF_UPDATE_UPSTREAM=your/fork`.
