# App Template

<!-- template:start -->
> **This is the template.** Start a new app with:
>
> ```sh
> scripts/new-app.sh habit-tracker     # creates ../habit-tracker, renamed, verified-ready, first commit made
> ```
>
> Everything below is the new app's README; this box is removed from the copy.
<!-- template:end -->

A web app with a Go API, a React frontend, and PostgreSQL, shipped as one Docker image.

- **Backend:** Go, standard library plus one PostgreSQL driver (`github.com/lib/pq`).
- **Frontend:** React + TypeScript, bundled by Vite and embedded in the Go binary. Installable as a PWA.
- **Database:** PostgreSQL. Migrations are embedded SQL files, applied on startup.
- **Packaging:** one small, non-root Docker image (under 20 MB) with no shell.
- **Android:** the same site, packaged as an app (see [android/README.md](android/README.md)).

## Quick start

You need Docker. Node is only needed for frontend hot reload.

```sh
cp .env.example .env            # the defaults run it on this machine only
docker compose up -d --build    # http://localhost:8080
```

## The three commands

| Command | What it does | When |
|---|---|---|
| `./verify.sh` | Go vet, Go unit and integration tests, and browser tests, in throwaway containers | Before every commit or deploy. CI runs it too. |
| `./preview.sh` | Builds the working tree (or `--ref <branch>`) and runs it privately at http://localhost:8081 with a fresh database | To **see** a change, e.g. what an AI just did, before deploying it |
| `docker compose up -d --build` | Builds and (re)starts the real stack | To deploy |

### Previewing a change before you deploy it

```sh
./preview.sh                          # what's in the working tree right now
./preview.sh --ref ai/new-feature     # a branch, without checking it out
./preview.sh --restore backup.sql.gz  # with a copy of production data (see Backups)
./preview.sh down                     # stop it and delete its data
```

Sign in as `preview@example.com` / `preview-password`. The preview runs exactly the image production would run, in its own Compose project with its own database, so it cannot touch the live app or its data. `/healthz` shows which version you are looking at.

You can also skip running anything: `./verify.sh` saves screenshots of the main screens at phone and desktop width in `e2e/test-results/`, and CI attaches them to every push and pull request as the `e2e-results` artifact.

### Frontend hot reload

With the local stack running (Quick start), start Vite's dev server. It forwards `/api` to `localhost:8080`:

```sh
cd web && npm install && npm run dev  # http://localhost:5173
```

After Go changes, run `docker compose up -d --build` again.

## Deploying

[docs/deploy.md](docs/deploy.md) is a step-by-step path. Each step changes only the infrastructure, never the app:

1. **This machine:** `deploy/local.yml`.
2. **Your PC on the internet:** Cloudflare Tunnel, with no open ports (`deploy/cloudflare.yml`).
3. **A server with a public IP** (any VPS, or AWS EC2/Lightsail): Caddy with automatic HTTPS (`deploy/caddy.yml`).
4. **Managed AWS:** ECS Fargate + RDS + a load balancer, for zero-downtime deploys, several instances and managed backups.

### Backups

```sh
docker compose exec -T db pg_dump -U app app | gzip > backup-$(date +%F).sql.gz   # back up
gunzip -c backup.sql.gz | docker compose exec -T db psql -U app app                # restore into a fresh stack
```

## Configuration

All settings are environment variables (read in `internal/config`).

| Variable | Default | Meaning |
|---|---|---|
| `DATABASE_URL` | required | PostgreSQL connection URL |
| `ADDR` | `:8080` | Listen address |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. Logs are JSON on stdout. |
| `COOKIE_SECURE` | `true` | Send the session cookie over HTTPS only. Set `false` only for local http. |
| `TRUST_PROXY` | `false` | Read the client IP from `CF-Connecting-IP`, or the last `X-Forwarded-For` entry. Enable only when the app is reachable solely through that proxy. `docker-compose.yml` enables it. |
| `PUBLIC_ORIGIN` | empty | The site's public address, e.g. `https://app.example.com`. Accepted by the CSRF check even if a proxy changes `Host`. |
| `ALLOW_SIGNUP` | `true` | Set `false` to stop new accounts being created. |
| `AUTH_RATE_LIMIT` | `10` | Sign-in and sign-up attempts per minute per client IP. |
| `ANDROID_PACKAGE`, `ANDROID_CERT_FINGERPRINTS` | empty | Publish `/.well-known/assetlinks.json` for the Android app. See [android/README.md](android/README.md). |

## How the code is organized

The code is split by **feature**: each one owns its SQL, HTTP handlers and page. The layers underneath are shared and small.

| Path | What it does |
|---|---|
| `cmd/server/` | Entry point: loads config, connects, migrates, serves, and shuts down cleanly on `SIGTERM`. |
| `internal/config/` | Reads and validates every setting from the environment. |
| `internal/db/` | Connection pool, migrations (`migrations/*.sql`, embedded), shared errors. |
| `internal/httpx/` | HTTP plumbing every feature shares: handlers that return errors, JSON in and out, middleware (security headers, CSRF origin check, logging, panic recovery), rate limiting. |
| `internal/account/` | Feature: sign-up, sign-in, sessions, and the `RequireUser` middleware. |
| `internal/notes/` | Feature: **the example**. A per-user list of notes. |
| `internal/server/` | Wires the features into one handler; serves the web app. Its tests run the whole API against a real database. |
| `internal/testdb/` | Gives integration tests an empty, migrated database. |
| `web/src/` | The React app: `api.ts` (typed API client), `App.tsx` (sign-in gate), `components/`, and `features/<name>/` pages. |
| `e2e/` | Playwright browser tests, and the throwaway stack that `verify.sh` and `preview.sh` use. |
| `deploy/` | Swappable ways to expose the app (local, Cloudflare Tunnel, Caddy). |

### Adding a feature

Follow the notes example; a feature is about five files:

1. `internal/db/migrations/00N_<name>.sql`: the tables. Never edit a migration that has been deployed; add a new one.
2. `internal/<name>/<name>.go`: a `Store` (all the SQL, every query scoped by user) and a `Handler` with `Register(mux, requireUser)`. Handlers return errors: `httpx.Errorf(404, "...")` reaches the user, and anything else becomes a logged 500.
3. One line in `internal/server/server.go` to register it.
4. `internal/server/<name>_test.go`: API tests against the real database.
5. `web/src/api.ts` (types and calls), `web/src/features/<name>/` (the page), and `e2e/tests/<name>.spec.js` (a browser test).

### Removing the example

Delete `internal/notes/`, `internal/db/migrations/002_notes.sql`, `internal/server/notes_test.go`, `web/src/features/notes/` and `e2e/tests/notes.spec.js`, then remove its line in `internal/server/server.go`, the note calls in `web/src/api.ts`, and the `<NotesPage />` in `web/src/App.tsx` (plus the note steps in `e2e/tests/screens.spec.js` and `auth.spec.js`). Do this **before the first deploy**; afterwards, add a migration that drops the table instead.

## API

| Method | Path | Auth | Returns |
|---|---|---|---|
| POST | `/api/signup`, `/api/login` | – | `{email}` and a session cookie. Body `{email, password}`. |
| POST | `/api/logout` | – | `204` |
| GET | `/api/me` | user | `{email}` |
| GET | `/api/notes` | user | `[{id, body, created_at}]`, newest first |
| POST | `/api/notes` | user | `201` and the note. Body `{body}`. |
| DELETE | `/api/notes/{id}` | user | `204`, or `404` |
| GET | `/healthz` | – | `{status: "ok", version}` if the database is reachable, else `503` |

Errors are `{"error": "message"}` with a matching status code. Requests that change data must come from the site itself (the `Origin` check), and the session cookie is `HttpOnly` and `SameSite=Lax`.
