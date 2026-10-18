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
- **Frontend:** React + TypeScript, bundled by Vite and embedded in the Go binary. An introduction page at `/` and the app at `/app/`, both in English and Japanese. Installable as a PWA.
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
| `./preview.sh` | Builds the working tree (or `--ref <branch>`) and runs it privately at http://localhost:8081 with a fresh database of sample data | To **see** a change, e.g. what an AI just did, before deploying it |
| `docker compose up -d --build` | Builds and (re)starts the local stack at http://localhost:8080 | To use it on this machine |

### Previewing a change before you deploy it

```sh
./preview.sh                          # what's in the working tree right now
./preview.sh --ref ai/new-feature     # a branch, without checking it out
./preview.sh --restore backup.sql.gz  # with a copy of production data (see Backups)
./preview.sh down                     # stop it and delete its data
PREVIEW_MINUTES=0 ./preview.sh        # keep it until `down` (default: it stops after 120 minutes)
```

Sign in as `preview@example.com` / `preview-password`. A fresh preview comes with some sample data (`e2e/seed`, which adds it through the API). The preview runs exactly the image production would run, in its own Compose project with its own database, so it cannot touch the live app or its data. `/healthz` shows which version you are looking at. It cleans up after itself: it stops, data and all, after two hours, or when the Claude Code session in that folder ends. `verify.sh` tears its stack down when it exits, and clears what a killed run left before it starts. Both share one Go module and build cache per app (Docker volumes `app-template-gomod` and `app-template-gobuild`), so a run compiles only what changed.

You can also skip running anything: `./verify.sh` saves screenshots of the main screens at phone and desktop width in `e2e/test-results/`, and CI attaches them to every push and pull request as the `e2e-results` artifact.

### Frontend hot reload

With the local stack running (Quick start), start Vite's dev server. It forwards `/api` to `localhost:8080`:

```sh
cd web && npm install && npm run dev  # http://localhost:5173 (the introduction), /index.html (the app)
```

After Go changes, run `docker compose up -d --build` again.

## Production

Production runs on AWS: one EC2 instance behind CloudFront, shared with the other rudnam apps. The infrastructure, backups and server access are in [rudnam/rudnam-infra](https://github.com/rudnam/rudnam-infra).

- **Deploy:** push to `main`. CI (`.github/workflows/ci.yml`) runs `./verify.sh`, builds the image, pushes it to ECR and starts it on the instance with `deploy/aws.yml`. A failed check stops the deploy.
- **Roll back:** Actions → CI → **Run workflow** on `main`, with the full SHA of an earlier commit. It reuses that commit's image, or rebuilds it.
- **Settings and secrets:** the production `.env` is the SSM parameter `/rudnam/app-template/env`, not a file in this repo.
- **Backups:** nightly database dumps (kept 30 days) and daily disk snapshots (7 days). `./preview.sh --restore <dump>.sql.gz` runs a change against one.
- **Migrations** run on startup and only go forward, and a deploy briefly runs the old and new versions side by side. So every migration must keep the previous release working: adding a table, a nullable column or a column with a default is safe; renaming or dropping a column takes two releases (stop using it, then drop it). Don't roll back past a commit that changed the schema.

### Put a new app on AWS

Until these steps are done, CI only verifies: the deploy job waits for the repository's `AWS_DEPLOY_ROLE_ARN` variable.

1. In rudnam-infra, add the app to `apps` in `terraform/variables.tf` with `live = false`, then `terraform plan` and `apply`. That creates its ECR repository and lets the deploy role and the server use it.
2. Store its production settings: `.env.example`'s variables with real values, as the SecureString `/rudnam/app-template/env` (rudnam-infra README, "App secrets"). Give it a new `POSTGRES_PASSWORD` (`openssl rand -base64 32`).
3. Give the repository the deploy credentials: `scripts/rotate-github-key.sh` in rudnam-infra (it covers every app in `apps`), then set the variables `AWS_DEPLOY_ROLE_ARN` and `DEPLOY_BUCKET` from `terraform output`.
4. Push to `main` and check the deploy job.
5. Set `live = true`, plan and apply. That points the app's DNS name at CloudFront and adds it to Caddy.

## Contributing

1. Fork the repository (or branch, if you have write access) and run it locally (Quick start).
2. Make the change, following `CLAUDE.md` (it applies to people too).
3. Run `./verify.sh` until it prints `VERIFY PASSED`, and look at the change with `./preview.sh`.
4. Open a pull request against `main`. CI runs the same checks and attaches screenshots; nothing deploys until it is merged.

You never need production credentials: pull requests, including ones from forks, run without them.

## Configuration

All settings are environment variables (read in `internal/config`).

| Variable | Default | Meaning |
|---|---|---|
| `DATABASE_URL` | required | PostgreSQL connection URL |
| `ADDR` | `:8080` | Listen address |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. Logs are JSON on stdout. |
| `COOKIE_SECURE` | `true` | Send the session cookie over HTTPS only. Set `false` only for local http. |
| `TRUST_PROXY` | `false` | Read the client IP from the last `X-Forwarded-For` entry, which the reverse proxy adds. Enable only when the app is reachable solely through that proxy. `docker-compose.yml` enables it; `deploy/local.yml` turns it off. |
| `PUBLIC_ORIGIN` | empty | The site's public address, e.g. `https://app.example.com`. Accepted by the CSRF check even if a proxy changes `Host`. |
| `ALLOW_SIGNUP` | `true` | Set `false` to stop new accounts being created. |
| `AUTH_RATE_LIMIT` | `10` | Sign-in and sign-up attempts per minute per client IP. |
| `ANDROID_PACKAGE`, `ANDROID_CERT_FINGERPRINTS` | empty | Publish `/.well-known/assetlinks.json` for the Android app. See [android/README.md](android/README.md). |
| `DEMO` | as `ALLOW_SIGNUP` | Offer "Try the demo": each visitor gets a throwaway account with sample data in their language (`internal/demo`), without signing up. It has no password and is deleted when they end it or after a day. |
| `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` | empty | Show "Continue with Google" on the sign-in page. Needs `PUBLIC_ORIGIN`. See [Sign in with Google](#sign-in-with-google). |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `MAIL_FROM` | empty, `587` | Email a link to confirm each new account's address, and links to reset forgotten passwords (port 587 with STARTTLS, or 465). `MAIL_FROM` is e.g. `App Template <no-reply@example.com>`. Needs `PUBLIC_ORIGIN`. Without these, nobody is asked to confirm, and "Forgot your password?" isn't offered. |

### Sign in with Google

1. In the [Google Cloud console](https://console.cloud.google.com/apis/credentials), create a project, set up the OAuth consent screen (External; it asks only for the email address, so Google doesn't need to verify the app), then create an **OAuth client ID** of type *Web application*.
2. Add the authorized redirect URI `https://<your domain>/api/auth/google/callback`, the same origin as `PUBLIC_ORIGIN`.
3. Put the client ID and secret in `.env` as `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET`, and redeploy.

A Google sign-in uses the account with the same email if there is one. If that account's email was never confirmed, its password stops working and its other sessions end, because whoever set the password never proved they own the address.

### Confirming email addresses

With the `SMTP_*` settings, new accounts get an email with a link, and the app shows a reminder until it's opened. People can keep using the app meanwhile. Signing in with Google counts as confirmed. Any SMTP provider works (Resend, Amazon SES, Postmark, or a Gmail account with an app password). The email is in Japanese for someone using the app in Japanese.

The same settings turn on **Forgot your password?** on the sign-in page: it emails a link that works once, for an hour, to choose a new password, which signs out every other session.


## How the code is organized

The code is split by **feature**: each one owns its SQL, HTTP handlers and page. The layers underneath are shared and small.

| Path | What it does |
|---|---|
| `cmd/server/` | Entry point: loads config, connects, migrates, serves, and shuts down cleanly on `SIGTERM`. |
| `internal/config/` | Reads and validates every setting from the environment. |
| `internal/db/` | Connection pool, migrations (`migrations/*.sql`, embedded), shared errors. |
| `internal/httpx/` | HTTP plumbing every feature shares: handlers that return errors, JSON in and out, middleware (security headers, CSRF origin check, logging, panic recovery), rate limiting. |
| `internal/account/` | Feature: sign-up, sign-in, sessions and the `RequireUser` middleware; also Sign in with Google (`google.go`), confirming emails (`verify.go`), password reset (`reset.go`), demo accounts (`demo.go`) and an Android app's bearer-token sign-in (`app.go`). |
| `internal/mailer/` | Sends the app's emails over SMTP (`SMTP_*`). |
| `internal/demo/` | Fills a demo account with sample data: the example's notes, in English and Japanese. Fill your own feature's instead. |
| `internal/notes/` | Feature: **the example**. A per-user list of notes. |
| `internal/server/` | Wires the features into one handler; serves the web app. Its tests run the whole API against a real database. |
| `internal/testdb/` | Gives integration tests an empty, migrated database. |
| `web/src/` | The React app: `api.ts` (typed API client), `App.tsx` (sign-in gate), `components/`, and `features/<name>/` pages. `i18n.tsx`, `language.ts` and `locales/ja.ts` translate it (below). |
| `web/landing.html`, `web/src/landing.*` | The introduction page at `/`. Its Japanese is in each element's `data-ja`. |
| `e2e/` | Playwright browser tests, the throwaway stack that `verify.sh` and `preview.sh` use, and `seed/`, the preview's sample data. |
| `.claude/` | Claude Code hooks: make Docker work in cloud sessions, and stop this folder's preview when a session ends. |
| `deploy/` | Where the app runs: `local.yml` (this machine) and `aws.yml` (production). |

### Adding a feature

Follow the notes example; a feature is about five files:

1. `internal/db/migrations/00N_<name>.sql`: the tables, numbered one above the highest. Never edit a migration that has been deployed, and never reuse a number, even a deleted one's: a database that ran it would skip the new file.
2. `internal/<name>/<name>.go`: a `Store` (all the SQL, every query scoped by user) and a `Handler` with `Register(mux, requireUser)`. Handlers return errors: `httpx.Errorf(404, "...")` reaches the user, and anything else becomes a logged 500.
3. One line in `internal/server/server.go` to register it.
4. `internal/server/<name>_test.go`: API tests against the real database.
5. `web/src/api.ts` (types and calls), `web/src/features/<name>/` (the page), and `e2e/tests/<name>.spec.js` (a browser test).
6. Optionally, sample data for the preview in `e2e/seed/main.go`.

### Translating

The app is written in English, and the English is the key: `t('Save')` shows "Save", or its entry in `web/src/locales/ja.ts` in Japanese. A key missing from `ja.ts` fails the type check, so nothing ships half-translated. Use `tn()` for counts and `tx()` for text with elements in it. The server's messages stay in English (old clients show them as they are); list each in `jaServer`, and `errorMessage()` shows it in Japanese. The app sends its language as `Accept-Language`, and `httpx.Language(r)` reads it, for anything the server writes, such as emails.

The language follows the device until someone picks one (the picker in the top bar, or the switch on the introduction page); the choice is kept in the browser and shared by both pages.

### Removing the example

Delete `internal/notes/`, `internal/db/migrations/002_notes.sql`, `internal/server/notes_test.go`, `web/src/features/notes/` and `e2e/tests/notes.spec.js`, then remove its line in `internal/server/server.go`, the note calls in `web/src/api.ts`, the notes strings in `web/src/locales/ja.ts`, and the `<NotesPage />` in `web/src/App.tsx` (plus the note steps in `e2e/tests/screens.spec.js` and `auth.spec.js`, the sample notes in `e2e/seed/main.go`, and `internal/demo`'s). Do this **before the first deploy**; afterwards, add a migration that drops the table instead.

## API

| Method | Path | Auth | Returns |
|---|---|---|---|
| POST | `/api/signup`, `/api/login` | – | `{email}` and a session cookie. Body `{email, password}`. |
| POST | `/api/logout` | – | `204` |
| GET | `/api/me` | user | `{email, has_password, confirm_email, demo}` |
| DELETE | `/api/me` | user | `204`. Deletes the account and all its data. Body `{password}`, or `{email}` for accounts without a password. |
| GET | `/api/auth/options` | – | `{google, demo, reset}`: which ways to sign in exist, whether visitors can try a demo, and whether a forgotten password can be reset by email |
| POST | `/api/demo` | – | Body `{tz}` (the visitor's time zone). Signs in to a new demo account filled with sample data, in the `Accept-Language` language; `201` with `/api/me`'s answer. Only when `DEMO` is on; rate-limited like sign-up. |
| GET | `/api/auth/google`, `/api/auth/google/callback` | – | Sign in with Google (redirects) |
| GET | `/api/verify-email?token=` | – | The link in a confirmation email; redirects to the app |
| POST | `/api/verify-email` | user | `204`. Sends the confirmation email again. |
| POST | `/api/password/forgot` | – | Body `{email}`. `204` whether or not an account uses it; if one does, a link to `/app/#/reset?token=…` is emailed (at most one a minute). `404` without the `SMTP_*` settings. Rate-limited like sign-in. |
| POST | `/api/password/reset` | – | Body `{token, password}`, the token from that link (single use, an hour). Sets the password, confirms the email, ends every other session and signs in: returns `/api/me`'s answer. `400` if the link expired or was used. |
| POST | `/api/auth/app-token` | – | For an Android app: trades the one-time code from Google sign-in in the phone's browser for a session token (`internal/account/app.go`). |
| GET | `/api/notes` | user | `[{id, body, created_at}]`, newest first |
| POST | `/api/notes` | user | `201` and the note. Body `{body}`. |
| DELETE | `/api/notes/{id}` | user | `204`, or `404` |
| GET | `/healthz` | – | `{status: "ok", version}` if the database is reachable, else `503` |

Errors are `{"error": "message"}` with a matching status code. Requests that change data must come from the site itself (the `Origin` check), and the session cookie is `HttpOnly` and `SameSite=Lax`.
