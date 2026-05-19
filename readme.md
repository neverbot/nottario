# Nottario

Open source, self-hosted service that coordinates human developers
and their AI agents. See `docs/initial/` (excluded from version
control) for the design discussion.

## Status

Pre-alpha. Foundation milestone in progress.

## Quick start (local)

```bash
docker compose up --build
```

Then open http://localhost:8080.

## Build from source

```bash
make build
DATABASE_URL=postgres://... ./bin/nottario
```

## License

TBD.
