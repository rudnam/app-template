#!/usr/bin/env bash
# Verifies the working tree end to end, in Docker, against a throwaway database:
#
#   ./verify.sh                  everything: go vet, Go tests, browser tests
#   ./verify.sh go               go vet and Go tests only (unit + HTTP integration)
#   ./verify.sh e2e [args...]    browser tests only; args go to `playwright test`,
#                                e.g. ./verify.sh e2e -g notes
#
# Needs only Docker. Uses its own Compose project (<folder>-verify) with the
# database in tmpfs, so it never touches the live stack or its data, and it
# tears everything down when it exits. Screenshots of failed browser tests
# land in e2e/test-results/.
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
HOST_UID=$(id -u) HOST_GID=$(id -g)
export PW_VERSION HOST_UID HOST_GID
# Keep the live stack's .env (COMPOSE_FILE etc.) out of this.
unset COMPOSE_FILE COMPOSE_PROJECT_NAME

project=${PWD##*/}; project=${project,,}-verify
dc() { docker compose -p "$project" -f e2e/compose.yml "$@"; }
trap 'dc --profile go --profile e2e down --volumes --remove-orphans --timeout 1 >/dev/null 2>&1 || true' EXIT

status=0
step() { printf '\n\033[1m== %s\033[0m\n' "$*"; }

if [[ $mode == all || $mode == go ]]; then
  step "Go: vet and tests (with integration tests)"
  dc --profile go run --rm gotest || status=1
fi

if [[ $mode == all || $mode == e2e ]]; then
  step "Build and start the app"
  if dc up -d --build --wait server; then
    step "Browser tests (Playwright $PW_VERSION)"
    # Reinstall only when node_modules is missing or out of date.
    dc --profile e2e run --rm e2e sh -c \
      'npm ls @playwright/test >/dev/null 2>&1 || npm ci --no-audit --no-fund --loglevel=error; npx playwright test "$@"' \
      sh "$@" || status=1
  else
    dc logs server db | tail -40
    status=1
  fi
fi

if [[ $status == 0 ]]; then step "VERIFY PASSED"; else step "VERIFY FAILED"; fi
exit $status
