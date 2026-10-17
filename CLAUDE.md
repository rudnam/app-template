# Notes for AI agents

Go API (`internal/`, one package per feature), PostgreSQL, and a React + TypeScript frontend (`web/src/`, built by Vite into `web/dist/` and embedded in the binary). README.md explains the layout and how to add a feature; follow the `notes` example.

## Verify your work before calling it done

```sh
./verify.sh                 # everything: go vet, all Go tests, browser tests
./verify.sh go              # Go only
./verify.sh e2e -g notes    # browser tests only; extra args go to `playwright test`
```

- It needs only Docker. It builds the working tree, runs it against a throwaway database, and tears everything down. It ends with `VERIFY PASSED` or `VERIFY FAILED`. CI runs the same script on every pull request and on pushes to `main`.
- Plain `go test ./...` on the host **skips** the integration tests (they need `TEST_DATABASE_URL`). `./verify.sh go` runs them.
- The Docker build type-checks the frontend, so a TypeScript error fails `./verify.sh`. Quick check on the host: `cd web && npm run build`.
- When a browser test fails, its screenshot is at `e2e/test-results/<test>/test-failed-1.png`. Read it before guessing.
- Report what the run actually printed. If something was skipped or couldn't run, say so.
- **If this machine can't run it, let CI run it.** That means: `docker compose ls` lists `<folder>-verify` (another run in this folder is in progress, and starting one would tear it down, since `verify.sh` begins by removing that project), Docker won't start, or the user says the machine is busy. Then commit, push the branch, and read the `verify` check on its pull request: the job log ends with `VERIFY PASSED` or `VERIFY FAILED`, and the screenshots are in the run's `e2e-results` artifact. CI doesn't run on branch pushes, so if the branch has no pull request, open a draft one. In your report, say the result came from CI, not from this machine.

## Show the user what changed

The user reviews changes by looking at them, not by deploying them. When you change anything visible:

1. Extend a browser test in `e2e/tests/` (one file per feature; import `{ test, expect }` from `./fixtures`, which fails on any console error or CSP violation). For new screens, add a step to `e2e/tests/screens.spec.js`, which saves phone and desktop screenshots.
2. Run `./verify.sh`, then **Read the screenshots** in `e2e/test-results/` and check the change looks right at both widths.
3. On the user's machine, run `./preview.sh` and tell the user: "Preview at http://localhost:8081, sign in as preview@example.com / preview-password", plus what to click to see the change. In a cloud session the user can't open that address, so send them the screenshots instead. You don't need to stop it: it stops by itself after two hours (`PREVIEW_MINUTES`) and when this session ends (`.claude/hooks/session-end.sh`, which also clears anything a killed `verify.sh` left).

## Rules

- **Production is on AWS, deployed only by CI** when `main` changes (`.github/workflows/ci.yml`; infrastructure in rudnam/rudnam-infra). Nothing on this machine is production: the Compose project named after this folder is local development. `verify.sh` and `preview.sh` use their own projects (`<folder>-verify`, `<folder>-preview`). Don't push to `main` or run AWS commands against production unless asked.
- Never edit a migration in `internal/db/migrations/` that may have been deployed; add a new file numbered one above the highest. The number is the version recorded in `schema_migrations`, so never reuse one, even a deleted migration's: a database that ran it would silently skip the new file. Keep each migration compatible with the previous release (README.md, "Production").
- **Keep the API backward compatible** once anything but this server's own web page calls it (an installed mobile app, which can be months old, or an MCP client): add fields and endpoints; don't rename, remove or change the meaning of existing ones.
- Every query on user data is scoped by user ID in the feature's `Store`. Another user's row is a 404, not a 403.
- Handlers return errors: `httpx.Errorf(status, msg)` for messages the user should see, and any other error for a logged 500. Don't leak internal errors to the client.
- Keep dependencies minimal: standard library first in Go; no new npm packages without a clear reason.
- When you add or change an endpoint, a setting or a package, update its table in README.md (API, Configuration, or How the code is organized).

## Environment gotchas

On the user's machine (WSL + Docker Desktop):

- Containers can't reach a server started in WSL, and `--network host` means the Docker VM, not WSL. Run things in Docker, as the scripts do.
- Playwright's browsers can't launch directly on this host (missing system libraries). Use `./verify.sh e2e`.

In Claude Code cloud sessions, `.claude/hooks/session-start.sh` starts Docker, and makes the base images trust the sandbox's TLS proxy. It also sets `DOCKER_BUILDKIT=0`, which the scripts override: BuildKit uses those local images as long as the Dockerfile has no `# syntax=` line, so don't add one. The scripts then work as above. If Docker isn't running, run the hook again (`CLAUDE_CODE_REMOTE=true CLAUDE_PROJECT_DIR=$PWD .claude/hooks/session-start.sh`).
