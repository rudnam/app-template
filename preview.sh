#!/usr/bin/env bash
# Runs a build in a private preview stack so you can click through a change
# before you deploy it. It never touches the local stack or its data.
#
#   ./preview.sh                  build the working tree
#   ./preview.sh --ref <git-ref>  build a branch, tag or commit instead, e.g. the
#                                 branch an AI worked on, without checking it out
#   ./preview.sh --restore FILE   start from a backup (pg_dump output, .sql or
#                                 .sql.gz) instead of an empty database
#   ./preview.sh logs             follow the app's logs
#   ./preview.sh down             stop the preview and delete its data
#
# Then open http://localhost:8081 (PREVIEW_PORT changes it) and sign in as
# preview@example.com / preview-password. Each start begins from a fresh database.
set -euo pipefail
cd "$(dirname "$0")"

PW_VERSION=unused   # e2e/compose.yml needs it set; the preview runs no tests
PREVIEW_PORT=${PREVIEW_PORT:-8081}
export PW_VERSION PREVIEW_PORT
unset COMPOSE_FILE COMPOSE_PROJECT_NAME   # keep the local stack's .env out of this

project=${PWD##*/}; project=${project,,}-preview
dc() { docker compose -p "$project" -f e2e/compose.yml -f e2e/preview.yml "$@"; }

ref='' restore=''
while (( $# )); do
  case $1 in
    down) dc down --volumes --remove-orphans; exit ;;
    logs) dc logs -f server; exit ;;
    --ref) ref=${2:?--ref needs a branch, tag or commit}; shift 2 ;;
    --restore) restore=${2:?--restore needs a backup file}; shift 2 ;;
    -h|--help) awk 'NR>1 && /^#/ { sub(/^# ?/, ""); print; next } NR>1 { exit }' "$0"; exit 0 ;;
    *) echo "unknown argument: $1 (see ./preview.sh --help)" >&2; exit 2 ;;
  esac
done
[[ -z $restore || -f $restore ]] || { echo "no such file: $restore" >&2; exit 2; }

if [[ -n $ref ]]; then
  # Build from a clean export of the ref; the working tree stays as it is.
  commit=$(git rev-parse --short "$ref^{commit}")
  BUILD_CONTEXT=$(mktemp -d)
  trap 'rm -rf "$BUILD_CONTEXT"' EXIT
  git archive "$commit" | tar -x -C "$BUILD_CONTEXT"
  export BUILD_CONTEXT
  APP_VERSION="$ref@$commit"
else
  APP_VERSION="$(git rev-parse --short HEAD 2>/dev/null || echo none)"
  [[ -n $(git status --porcelain 2>/dev/null) ]] && APP_VERSION+="+changes"
fi
export APP_VERSION

echo "Previewing $APP_VERSION"
dc down --volumes --remove-orphans >/dev/null 2>&1 || true
if [[ -n $restore ]]; then
  dc up -d --wait db
  echo "Restoring $restore"
  if [[ $restore == *.gz ]]; then gunzip -c "$restore"; else cat "$restore"; fi |
    dc exec -T db psql -q -v ON_ERROR_STOP=1 -U app app >/dev/null
fi
if ! dc up -d --build --wait server; then
  dc logs server | tail -40
  exit 1
fi

url=http://localhost:$PREVIEW_PORT
curl -fsS -o /dev/null -H 'Content-Type: application/json' -H "Origin: $url" \
  -d '{"email":"preview@example.com","password":"preview-password"}' "$url/api/signup" ||
  echo "(could not create the preview user; it may exist in the restored backup)"

printf '\n\033[1mPreview of %s is running at %s\033[0m\n' "$APP_VERSION" "$url"
echo "Sign in as preview@example.com / preview-password"
echo "Logs: ./preview.sh logs    Stop: ./preview.sh down"
