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

## License

TBD.
