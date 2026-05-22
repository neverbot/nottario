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

## Connect an AI agent

Nottario exposes its full surface area to AI agents through an MCP
server bundled inside the same binary. Any MCP-capable client (Claude
Code, Claude Desktop, Cursor, etc.) connects over HTTP with a Bearer
token.

### 1. Issue an API token

In the web UI:

1. Sign in with GitHub.
2. Open **Tokens** in the top-right menu → **New token**.
3. Give it a name (e.g. `claude-code-laptop`) and an optional default
   role.
4. Copy the secret — it is shown **once** and starts with `ntr_…`. It
   is hashed in the database; if you lose it, revoke and issue a new
   one.

The token authenticates as the user that created it. Admin powers
require that the underlying user is an instance admin.

### 2. Add the server to your client

**Claude Code:**

```bash
claude mcp add nottario http://localhost:8080/mcp \
  --transport http \
  --header "Authorization: Bearer ntr_…" \
  --scope user
```

`--scope user` stores the config (and therefore the token) in
`~/.claude.json`, not in the repo. Use `--scope local` to keep it per
project but still outside git, or `--scope project` only when the
server has no secrets to share (Nottario does, so prefer `user` or
`local`).

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
returns your `github_login` and your `memberships` (roles per
project), the connection is healthy. A `401 Unauthorized` with a
`WWW-Authenticate: Bearer realm="nottario"` header means the token is
missing, malformed or revoked.

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
  memberships. An admin must add them via the web UI (Project
  settings → Members) or grant `is_admin`.

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

TBD.
