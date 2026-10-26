#!/usr/bin/env bash
# Verifies the working tree end to end, in Docker, against a throwaway database:
#
#   ./verify.sh                  everything: go vet, Go tests, browser tests
#   ./verify.sh go               go vet and Go tests only (unit + HTTP integration)
#   ./verify.sh e2e [args...]    browser tests only; args go to `playwright test`,
#                                e.g. ./verify.sh e2e -g notes
#
# Needs only Docker. Uses its own Compose project (<folder>-verify) with the
# database in tmpfs, so it never touches the local stack or its data, and it
# tears everything down when it exits, except the Go module and build caches,
# which every checkout shares so a run compiles only what changed. A run that was
# killed outright (or whose Docker crashed) can't tear down; the next run
# clears what it left before starting. Screenshots of
# failed browser tests land in e2e/test-results/.
set -euo pipefail
cd "$(dirname "$0")"

mode=${1:-all}; (( $# )) && shift
case $mode in
  all|go|e2e) ;;
  -h|--help) awk 'NR>1 && /^#/ { sub(/^# ?/, ""); print; next } NR>1 { exit }' "$0"; exit 0 ;;
  *) echo "unknown mode: $mode (see ./verify.sh --help)" >&2; exit 2 ;;
esac

# The Playwright image must match the @playwright/test version exactly.
PW_VERSION=$(sed -n 's/.*"@playwright\/test": *"\([0-9.]*\)".*/\1/p' e2e/package.json)
: "${PW_VERSION:?could not read the @playwright/test version from e2e/package.json}"
# Go tests run on the Dockerfile's Go image, so they use the toolchain that
# builds the app, and Dependabot's bumps of it reach them too.
GO_IMAGE=$(sed -n 's/^FROM \(golang:[^ ]*\).*/\1/p' Dockerfile)
: "${GO_IMAGE:?could not read the golang image from the Dockerfile}"
HOST_UID=$(id -u) HOST_GID=$(id -g)
# BuildKit, for the Dockerfile's build cache (even if the environment says not to).
DOCKER_BUILDKIT=1
export PW_VERSION GO_IMAGE HOST_UID HOST_GID DOCKER_BUILDKIT
# Keep the local stack's .env (COMPOSE_FILE etc.) out of this.
unset COMPOSE_FILE COMPOSE_PROJECT_NAME

project=${PWD##*/}; project=${project,,}-verify
dc() { docker compose -p "$project" -f e2e/compose.yml "$@"; }
# No --volumes: the database is in tmpfs, and the Go caches should stay.
down() { dc --profile go --profile e2e down --remove-orphans --timeout 1 >/dev/null 2>&1 || true; }
down   # whatever an earlier run left behind
docker volume create app-template-gomod >/dev/null
docker volume create app-template-gobuild >/dev/null
golog=$(mktemp) buildlog=$(mktemp)
trap 'down; rm -f "$golog" "$buildlog"' EXIT

# Pull the browser image while the app builds, not after: it's large, and a
# fresh machine (CI) or a new Playwright version has to download it.
pullpid=''
if [[ $mode == all || $mode == e2e ]]; then
  dc --profile e2e pull -q e2e >/dev/null 2>&1 & pullpid=$!
fi

status=0
step() { printf '\n\033[1m== %s\033[0m\n' "$*"; }
gostep="Go: vet and tests (with integration tests)"

if [[ $mode == go ]]; then
  step "$gostep"
  dc --profile go run --rm gotest || status=1
else
  # Each container takes seconds to create on Docker Desktop, so everything
  # that doesn't need the new image starts while it builds. The build's output
  # is shown only if it fails.
  step "Build and start the app"
  dc build server >"$buildlog" 2>&1 & buildpid=$!
  dc up -d --wait db mail >/dev/null 2>&1 || true   # if this failed, the up below says why
  if [[ $mode == all ]]; then
    # The Go tests use their own database (app_test), so they run while the
    # app builds and the browser tests run. Their output is shown when they finish.
    dc --profile go run --rm gotest >"$golog" 2>&1 & gopid=$!
  fi
  wait "$pullpid" || true   # if the pull failed, the up below pulls again
  # Reinstall only when node_modules is missing or out of date.
  if ! dc --profile e2e up -d --no-deps e2e >/dev/null 2>&1 ||
     ! dc --profile e2e exec -T e2e sh -c \
       'npm ls @playwright/test >/dev/null 2>&1 || npm ci --no-audit --no-fund --loglevel=error'; then
    echo "could not start the browser tests' container" >&2
    status=1
  fi
  if ! wait "$buildpid"; then
    cat "$buildlog"
    status=1
  elif ! dc up -d --wait server; then
    dc logs server db | tail -40
    status=1
  elif [[ $status == 0 ]]; then
    step "Browser tests (Playwright $PW_VERSION)"
    tty=(); [[ -t 0 && -t 1 ]] || tty=(-T)
    dc --profile e2e exec "${tty[@]}" e2e npx playwright test "$@" || status=1
  fi
fi

if [[ $mode == all ]]; then
  step "$gostep"
  wait "$gopid" || status=1
  cat "$golog"
fi

if [[ $status == 0 ]]; then step "VERIFY PASSED"; else step "VERIFY FAILED"; fi
exit $status
