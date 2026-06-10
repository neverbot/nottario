---
title: Getting started
section: Start
nav_order: 1
---

# Getting started

The fastest path to a working Nottario instance is Docker Compose.
The image is published to GHCR on every push to master, plus a
versioned tag on every `v*` git tag.

## 1. Clone the repo

```
git clone https://github.com/neverbot/nottario.git
cd nottario
```

You don't strictly need the source to run Nottario, but the
`compose.yml` and `.env.example` files at the root of the repo are
the easiest starting point.

## 2. Create a GitHub OAuth App

Nottario uses GitHub OAuth as the identity provider for humans. From
your GitHub account, open *Settings → Developer settings → OAuth Apps
→ New OAuth App* and fill in:

- **Application name:** something recognisable, e.g. `Nottario (mine)`.
- **Homepage URL:** the URL the instance will be reached at, e.g.
  `http://localhost:8080`.
- **Authorization callback URL:** the same host plus
  `/auth/github/callback`, e.g.
  `http://localhost:8080/auth/github/callback`.

GitHub will give you a Client ID and let you generate a Client
Secret. You'll need both in the next step.

> Use a regular **OAuth App**, not a GitHub App. They look similar in
> the UI but have very different flows; Nottario expects OAuth.

## 3. Wire your `.env`

Copy the example file and fill in the values:

```
cp .env.example .env
```

The required keys are:

```
PUBLIC_URL=http://localhost:8080
DATABASE_URL=postgres://nottario:nottario@db:5432/nottario?sslmode=disable
GITHUB_OAUTH_CLIENT_ID=<your client id>
GITHUB_OAUTH_CLIENT_SECRET=<your client secret>
SESSION_KEY=<32 random bytes, base64>
```

Generate `SESSION_KEY` once and keep it:

```
openssl rand -base64 32
```

Rotating it logs everyone out.

## 4. Bring it up

```
docker compose up -d
```

Compose starts a `postgres:16` and the Nottario container alongside
it. Open `http://localhost:8080` in a browser, sign in with GitHub,
and you'll land on the projects list.

## 5. Create a project and a token

From the projects list, hit **New project**. Open it, go to
**Settings → Tokens**, and issue an API token. The secret is shown
once; copy it now. The Settings tab also gives you a one-line
`claude mcp add …` snippet that wires your local agent into this
specific project — see [MCP integration](/mcp/) for the details.

## 6. Restrict to a GitHub org (optional)

If you only want members of one GitHub organisation to be able to
log in, set:

```
GITHUB_OAUTH_ORG=acme
```

Restart the container. Non-members trying to sign in will land on
`/login` with an explanation. The OAuth consent screen will
additionally request `read:org` so Nottario can verify membership.
API tokens are unaffected by this gate.

See [Self-hosting reference](/self-hosting/) for the full list of
env vars, secret-file conventions, and backup configuration.
