# Notes for AI agents

Go API (`internal/`, one package per feature), PostgreSQL, and a React + TypeScript frontend (`web/src/`, built by Vite into `web/dist/` and embedded in the binary). README.md explains the layout and how to add a feature; follow the `notes` example.

## Verify your work before calling it done

```sh
./verify.sh                 # everything: go vet, all Go tests, browser tests
./verify.sh go              # Go only
./verify.sh e2e -g notes    # browser tests only; extra args go to `playwright test`
```

- It needs only Docker. It builds the working tree, runs it against a throwaway database, and tears everything down. It ends with `VERIFY PASSED` or `VERIFY FAILED`.
- Plain `go test ./...` on the host **skips** the integration tests (they need `TEST_DATABASE_URL`). `./verify.sh go` runs them.
- The Docker build type-checks the frontend, so a TypeScript error fails `./verify.sh`. Quick check on the host: `cd web && npm run build`.
- When a browser test fails, its screenshot is at `e2e/test-results/<test>/test-failed-1.png`. Read it before guessing.
- Report what the run actually printed. If something was skipped or couldn't run, say so.

## Show the user what changed

The user reviews changes by looking at them, not by deploying them. When you change anything visible:

1. Extend a browser test in `e2e/tests/` (one file per feature; import `{ test, expect }` from `./fixtures`, which fails on any console error or CSP violation). For new screens, add a step to `e2e/tests/screens.spec.js`, which saves phone and desktop screenshots.
2. Run `./verify.sh`, then **Read the screenshots** in `e2e/test-results/` and check the change looks right at both widths.
3. Run `./preview.sh` and tell the user: "Preview at http://localhost:8081, sign in as preview@example.com / preview-password", plus what to click to see the change.

## Rules

- **Never touch the live stack.** The Compose project named after this folder (`docker compose ...` in the repo root, which reads `.env`) may be production. Don't start, stop, rebuild or exec into it. `verify.sh` and `preview.sh` use their own projects (`<folder>-verify`, `<folder>-preview`).
- Never edit a migration in `internal/db/migrations/` that may have been deployed; add a new numbered file. Keep each migration compatible with the previous release (see docs/deploy.md, "Migrations").
- Every query on user data is scoped by user ID in the feature's `Store`. Another user's row is a 404, not a 403.
- Handlers return errors: `httpx.Errorf(status, msg)` for messages the user should see, and any other error for a logged 500. Don't leak internal errors to the client.
- Keep dependencies minimal: standard library first in Go; no new npm packages without a clear reason.

## Environment gotchas (WSL + Docker Desktop)

- Containers can't reach a server started in WSL, and `--network host` means the Docker VM, not WSL. Run things in Docker, as the scripts do.
- Playwright's browsers can't launch directly on this host (missing system libraries). Use `./verify.sh e2e`.
