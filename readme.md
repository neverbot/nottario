# Nottario

Open source, self-hosted service that coordinates human developers
and their AI agents.

## Status

Pre-alpha. Foundation, identity and tasks milestones complete.

## Prerequisites

- Docker and Docker Compose.
- A **GitHub OAuth App** for the instance:
  https://github.com/settings/developers → New OAuth App.
  - **Homepage URL**: `http://localhost:8080`
  - **Authorization callback URL**: `http://localhost:8080/auth/github/callback`
  - Leave Device Flow disabled and the webhook section empty.
  - Generate a client secret and keep it for the `.env` file.

## First-time setup

Copy the example env file and fill in the secrets:

```bash
cp .env.example .env
```

Then edit `.env` with:

```env
PUBLIC_URL=http://localhost:8080
HTTP_ADDR=:8080
DATABASE_URL=postgres://nottario:nottario@db:5432/nottario?sslmode=disable

GITHUB_OAUTH_CLIENT_ID=<your client id>
GITHUB_OAUTH_CLIENT_SECRET=<your client secret>
SESSION_KEY=<32 random bytes, base64>
```

Generate `SESSION_KEY` with:

```bash
openssl rand -base64 32
```

## Run

Everything runs through Docker Compose:

```bash
docker compose up --build
```

That builds the `nottario` image, brings up Postgres, applies all
pending migrations on first boot and exposes the web UI at
http://localhost:8080.

The first GitHub account that logs in becomes the instance admin.

To stop and discard state:

```bash
docker compose down -v
```

To keep state (the `db-data` volume) between runs:

```bash
docker compose down
```

## Self-hosting

The same binary that runs locally is what ships to production. Below is
the minimum needed to put it behind a real domain with TLS, a real
Postgres, and a sensible secret hygiene story.

### Behind a reverse proxy (Traefik example)

Nottario does **not** terminate TLS. A reverse proxy in front of it
handles certificates and forwards plain HTTP to `:8080`. Set
`PUBLIC_URL=https://<your-domain>`; the binary detects the `https://`
prefix and turns on the `Secure` flag for session cookies
automatically. Anything else (cookies on `http`, mixed-content) is not
supported.

Minimal Traefik snippet — drop into the compose file that already runs
Traefik on your host. No `ports:` block on Nottario; the proxy is the
only thing the public reaches.

```yaml
services:
  nottario:
    image: ghcr.io/neverbot/nottario:latest
    restart: unless-stopped
    networks:
      - <your-traefik-network>
    depends_on:
      - postgres
    environment:
      PUBLIC_URL: https://nottario.example.com
      HTTP_ADDR: ":8080"
      DATABASE_URL: postgres://nottario@postgres:5432/nottario?sslmode=disable
      GITHUB_OAUTH_CLIENT_ID: <your client id>
      GITHUB_OAUTH_CLIENT_SECRET_FILE: /run/secrets/nottario_github_secret
      SESSION_KEY_FILE: /run/secrets/nottario_session_key
    secrets:
      - nottario_github_secret
      - nottario_session_key
    labels:
      - traefik.enable=true
      - traefik.http.routers.nottario.rule=Host(`nottario.example.com`)
      - traefik.http.routers.nottario.entrypoints=websecure
      - traefik.http.routers.nottario.tls.certresolver=<your-resolver>
      - traefik.http.services.nottario.loadbalancer.server.port=8080
```

For nginx or Caddy: forward `https://nottario.example.com → http://127.0.0.1:8080`
preserving `Host` and `X-Forwarded-Proto`. Nothing else is required.

### Required environment

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `PUBLIC_URL` | yes | `http://localhost:8080` | The base URL users reach. Must match the OAuth callback host and have `https://` in production for `Secure` cookies. |
| `DATABASE_URL` | yes | — | pgx connection string. Migrations run automatically on startup. |
| `GITHUB_OAUTH_CLIENT_ID` | yes | — | From the GitHub OAuth App. |
| `GITHUB_OAUTH_CLIENT_SECRET` | yes | — | From the GitHub OAuth App. |
| `SESSION_KEY` | yes | — | 32 random bytes, base64-encoded. Generate once and keep: `openssl rand -base64 32`. Rotating it logs everyone out. |
| `HTTP_ADDR` | no | `:8080` | Listen address. Change only if you run multiple instances on one host. |

Every secret variable also accepts a `_FILE` companion that points at
a file on disk: `SESSION_KEY_FILE`, `GITHUB_OAUTH_CLIENT_SECRET_FILE`.
The file's contents are read and trailing whitespace is stripped — so
`openssl rand -base64 32 > /run/secrets/session_key` works without
extra ceremony. The `_FILE` variant takes precedence when both are
set; this is the recommended path under Docker secrets, Kubernetes
mounted secrets, or any host where you'd rather not have secrets in
the process environment.

### GitHub OAuth App

1. Sign in to GitHub and open **Settings → Developer settings → OAuth Apps → New OAuth App**.
2. Fill in:
   - **Application name**: anything (e.g. `nottario-yourorg`).
   - **Homepage URL**: your `PUBLIC_URL`.
   - **Authorization callback URL**: `${PUBLIC_URL}/auth/github/callback`.
   - Leave Device Flow disabled and the webhook section empty.
3. Save, then **Generate a new client secret**. Copy it once — GitHub
   only shows it once.
4. Put the Client ID and Client Secret into the environment of the
   running container (see the table above).

Required OAuth scopes: `read:user`. The MCP server never asks for
elevated GitHub scopes.

### First admin

The very first user to complete the OAuth login flow on a fresh
instance is promoted to **instance admin** automatically. After that,
new logins are regular users until granted admin via project
membership.

Adding or revoking instance-admin on subsequent users currently
requires direct SQL (`UPDATE users SET is_admin = true WHERE
github_login = '…';`). A UI for this is on the roadmap; until then,
keep that one human you trust as the first login.

### Database

Two supported setups:

- **Bring your own Postgres** (production). Point `DATABASE_URL` at an
  external Postgres 16+ instance you already operate. Nottario applies
  its own migrations on first boot; no manual SQL needed. A dedicated
  database and role for Nottario is recommended:

  ```sql
  CREATE ROLE nottario LOGIN PASSWORD '…';
  CREATE DATABASE nottario OWNER nottario;
  ```

- **Embedded compose Postgres** (dev only). The `compose.yml` in this
  repo ships a `db` service with a hardcoded `nottario:nottario`
  password. Convenient for local development; do **not** run a
  production instance against it.

Migrations are written as goose files under `internal/db/migrations/`
and are embedded into the binary. There is no separate migration
command — booting the container is the migration.

### Backups

There is no built-in backup mechanism yet. Running real data without
an external backup is **at your own risk**. Until the backup feature
lands, a minimal cron from outside the container:

```bash
# /etc/cron.daily/nottario-backup
docker exec <postgres-container> \
  pg_dump -U nottario -Fc nottario > /backups/nottario-$(date +%F).dump
find /backups -name 'nottario-*.dump' -mtime +14 -delete
```

Restore with `pg_restore -U nottario -d nottario /backups/<file>.dump`.

### Upgrades

```bash
docker compose pull nottario
docker compose up -d nottario
```

Migrations apply automatically on the new container's boot. A failed
migration leaves the previous schema intact; the new container will
exit non-zero and the proxy will fall back to whatever it was last
serving — but the database state is consistent.

Rolling **back** is not automatic: goose Down migrations are written
defensively but data shape changes (e.g. dropped columns) cannot be
reconstructed. Test upgrades on a copy of the production database
before applying.

## Connect an AI agent

Nottario exposes its full surface area to AI agents through an MCP
server bundled inside the same binary. Any MCP-capable client (Claude
Code, Claude Desktop, Cursor, etc.) connects over HTTP with a Bearer
token.

### 1. Issue an API token

Tokens in Nottario are **scoped to a single project**: one token = one
project. An agent using a token issued for project A can never read or
modify project B, even if the underlying user is a member of both.
Agents working across multiple projects need one token per project.

In the web UI:

1. Sign in with GitHub.
2. Open the project you want the agent to work on.
3. Go to **Settings → Tokens → New token**.
4. Give it a name (e.g. `claude-code-laptop`) and an optional default
   role.
5. Copy the secret — it is shown **once** and starts with `ntr_…`. It
   is hashed in the database; if you lose it, revoke and issue a new
   one.

The token authenticates as the user that created it. Admin powers
require that the underlying user is an instance admin; admin status is
*not* a bypass of project scope — an admin token issued for project A
is still rejected against project B.

### 2. Add the server to your client

**Claude Code:**

```bash
claude mcp add nottario http://localhost:8080/mcp \
  --transport http \
  --header "Authorization: Bearer ntr_…" \
  --scope local
```

`--scope local` stores the config per-project (still outside git) and
is the recommended setup: each project gets its own token, so an
agent working on project A can't accidentally touch project B.
`--scope user` puts the same token in every project from
`~/.claude.json` — only use it if you knowingly want one shared
token. Avoid `--scope project` (it would commit the token into the
repo).

Verify:

```bash
claude mcp get nottario
```

**Claude Desktop:** edit `claude_desktop_config.json` (Settings →
Developer → Edit Config) and add:

```json
{
  "mcpServers": {
    "nottario": {
      "type": "http",
      "url": "http://localhost:8080/mcp",
      "headers": {
        "Authorization": "Bearer ntr_…"
      }
    }
  }
}
```

Restart Claude Desktop after saving.

**Cursor:** in `~/.cursor/mcp.json` (or the project-level `.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "nottario": {
      "url": "http://localhost:8080/mcp",
      "headers": { "Authorization": "Bearer ntr_…" }
    }
  }
}
```

For a remote deployment, swap `http://localhost:8080` for your public
URL. The transport must stay `http` (Streamable HTTP); plain SSE is
not supported.

### 3. Verify the connection

From the agent, the first call should be `nottario.whoami`. If it
returns your `github_login` and your `memberships` (roles in the
**single project the token is scoped to**), the connection is
healthy. A `401 Unauthorized` with a `WWW-Authenticate: Bearer
realm="nottario"` header means the token is missing, malformed or
revoked. A `"token scoped to project X, request targets Y"` error on a
later tool call means the agent passed the wrong `project_id`; cache
the value `whoami` returns and pass it on every subsequent call.

### 4. Pull the skill bundle

Nottario ships a skill bundle that teaches agents how to use the
tools (filing tasks, picking up work, writing docs, editing the
architecture graph). Download and unzip it into your client's skills
directory:

```bash
curl -fsSL http://localhost:8080/skill.zip -o nottario-skill.zip
unzip nottario-skill.zip -d ~/.claude/skills/nottario
```

The bundle is regenerated on every release; pull it again after an
upgrade to get new conventions and the latest tool descriptions.

### Troubleshooting

- **`401 Unauthorized`** — token missing, malformed (must start with
  `ntr_`) or revoked. Issue a new one.
- **`404` on `/mcp`** — wrong path or the binary is older than the
  MCP milestone. Check `GET /version`.
- **`Mcp-Session-Id missing` or session errors** — your client is not
  preserving the session header between requests. Update the client
  or check its transport config; Nottario follows the standard
  Streamable HTTP transport.
- **The agent sees no projects** — the user behind the token has no
  membership in the token's project. An admin must add them via the
  web UI (Project settings → Members) or grant `is_admin`.
- **`token scoped to project X, request targets Y`** — the agent
  passed a `project_id` that doesn't match the token's project. Tokens
  are per-project; either re-issue against the right project or pass
  the correct id. Cache `whoami`'s `memberships[0].ProjectID` and use
  it everywhere.

## Day-to-day commands

```bash
docker compose up -d              # background
docker compose logs -f nottario   # follow logs
docker compose restart nottario   # restart only the app
docker compose exec db psql -U nottario -d nottario  # inspect the database
```

After editing Go code, rebuild and restart only the app:

```bash
docker compose up -d --build nottario
```

## Tests

The test suite is run on the host with the Go toolchain (no Docker
required):

```bash
go test ./...
```

## Tech stack

Backend (Go):

- [pgx/v5](https://github.com/jackc/pgx) — Postgres driver.
- [sqlc](https://sqlc.dev) — type-safe Go from SQL.
- [goose](https://github.com/pressly/goose) — schema migrations.
- [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) — MCP server.
- [golang.org/x/oauth2](https://pkg.go.dev/golang.org/x/oauth2) — GitHub OAuth.
- [google/uuid](https://github.com/google/uuid) — UUIDs.
- [yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3) — YAML parsing.
- [joho/godotenv](https://github.com/joho/godotenv) — `.env` loader.
- [golang.org/x/tools](https://pkg.go.dev/golang.org/x/tools) — analyzer framework for the custom SQL-injection lint.

Frontend (vanilla, no build step):

- [Lit](https://lit.dev) — web-components framework.

Infrastructure:

- [Postgres](https://www.postgresql.org/) — primary datastore.
- [Docker](https://www.docker.com/) / [Docker Compose](https://docs.docker.com/compose/) — local and deploy runtime.

Tooling:

- [gofmt](https://pkg.go.dev/cmd/gofmt), [go vet](https://pkg.go.dev/cmd/vet), [golangci-lint](https://golangci-lint.run) — lint stack.

## License

MIT — see [`license.md`](license.md).
