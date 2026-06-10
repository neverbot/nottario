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
