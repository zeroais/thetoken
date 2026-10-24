# Building TOKEN

Everything builds inside Docker; you need Docker and about 4 GB of disk.

## The image TOKEN deploys

```bash
docker build -t token:local .
```

The `Dockerfile` builds the web console (Bun) and embeds it in the Go server
(`new-api` binary). The release name in `VERSION` is stamped into both. Base
images are pinned by digest.

## Run it locally

PostgreSQL 17, Valkey 8 and TOKEN on http://localhost:3000:

```bash
cp .env.token.example .env      # replace each placeholder: openssl rand -hex 32
docker compose -f docker-compose.token.yml up -d --build
```

Open http://localhost:3000, create the root account, then add channels
(upstream providers and their keys), model prices and options in the console,
as with New API. Payment gateways are off until their keys are set (Moyasar:
`MOYASAR_*` environment variables; see `controller/token_topup_moyasar.go`).

## Tests

```bash
# Go (root module, relaykit, tokenkit)
docker run --rm -v "$PWD":/src -w /src golang:1.26.10-alpine \
  sh -c 'apk add --no-cache make git >/dev/null && mkdir -p web/dist && touch web/dist/index.html && make test'

# Web console: typecheck and tests (Vitest under Node)
c=$(docker create oven/bun:1.4.0) && docker cp -q $c:/usr/local/bin/bun /tmp/bun && docker rm $c
docker run --rm -v "$PWD":/src -w /src/web -v /tmp/bun:/usr/local/bin/bun:ro node:22-bookworm-slim \
  sh -c 'bun install --frozen-lockfile && bun run typecheck && bun run test'
```

Database code is tested against SQLite, MySQL and PostgreSQL (see `AGENTS.md`).
