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
# preview@example.com / preview-password. Each start begins from a fresh database
# with some sample data (e2e/seed), so screens look as they do in real use.
#
# The preview stops by itself, data and all, after PREVIEW_MINUTES (default
# 120; 0 keeps it until `down`), when a pull request from the branch it shows
# is merged (checked every minute with the GitHub CLI, if it's signed in), and
# when the Claude Code session in this folder ends (.claude/hooks/session-end.sh).
# Starting it again restarts the clock.
set -euo pipefail
cd "$(dirname "$0")"

PW_VERSION=unused   # e2e/compose.yml needs it set; the preview runs no tests
GO_IMAGE=$(sed -n 's/^FROM \(golang:[^ ]*\).*/\1/p' Dockerfile)   # for the seed, as in verify.sh
: "${GO_IMAGE:?could not read the golang image from the Dockerfile}"
PREVIEW_PORT=${PREVIEW_PORT:-8081}
PREVIEW_MINUTES=${PREVIEW_MINUTES:-120}
[[ $PREVIEW_MINUTES =~ ^[0-9]+$ ]] || { echo "PREVIEW_MINUTES must be a number of minutes" >&2; exit 2; }
DOCKER_BUILDKIT=1   # for the Dockerfile's build cache, even if the environment says not to
unset COMPOSE_FILE COMPOSE_PROJECT_NAME   # keep the local stack's .env out of this

project=${PWD##*/}; project=${project,,}-preview
PREVIEW_PROJECT=$project   # for the reaper in preview.yml
export PW_VERSION GO_IMAGE PREVIEW_PORT PREVIEW_MINUTES PREVIEW_PROJECT DOCKER_BUILDKIT
dc() { docker compose -p "$project" -f e2e/compose.yml -f e2e/preview.yml "$@"; }

# The merge watcher (see watch_merge) runs on this machine, not in Docker: it
# needs this machine's GitHub sign-in. Its process ID is kept per worktree.
watcher=$(git rev-parse --git-path preview-merge-watch.pid 2>/dev/null || echo .preview-merge-watch.pid)
stop_watcher() {
  [[ -f $watcher ]] || return 0
  kill "$(cat "$watcher")" 2>/dev/null || true
  rm -f "$watcher"
}

# Stops the preview once a pull request from $1 merged after $2 (UTC, as
# GitHub writes it): what it shows is in main now. Gives up when the preview
# is gone for another reason.
watch_merge() {
  local branch=$1 since=$2 merged
  while sleep "${PREVIEW_WATCH_SECONDS:-60}"; do
    [[ -n $(docker compose -p "$project" ps -q 2>/dev/null) ]] || break
    merged=$(gh pr list --head "$branch" --state merged --json mergedAt \
      --jq "map(select(.mergedAt > \"$since\")) | length" 2>/dev/null) || continue
    if (( merged > 0 )); then
      docker compose -p "$project" down --volumes --remove-orphans --timeout 1 >/dev/null 2>&1
      break
    fi
  done
  rm -f "$watcher"
}

ref='' restore=''
while (( $# )); do
  case $1 in
    down) stop_watcher; dc down --volumes --remove-orphans; exit ;;
    _watch-merge) watch_merge "$2" "$3"; exit ;;   # started by preview.sh itself
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
stop_watcher
dc down --volumes --remove-orphans >/dev/null 2>&1 || true
docker volume create app-template-gomod >/dev/null   # Go caches for the seed,
docker volume create app-template-gobuild >/dev/null # shared with verify.sh
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
# Stops the preview after PREVIEW_MINUTES, even if nobody is around to.
dc up -d reaper >/dev/null 2>&1 || echo "(could not start the timer; stop the preview with ./preview.sh down)"

# And when the branch it shows is merged. Main itself, or a commit that's no
# branch, never is.
if [[ -n $ref ]]; then
  branch=${ref#origin/}
  git show-ref -q --verify "refs/heads/$branch" || git show-ref -q --verify "refs/remotes/origin/$branch" || branch=''
else
  branch=$(git symbolic-ref -q --short HEAD 2>/dev/null || true)
fi
merge_note=''
if [[ -n $branch && $branch != main ]] && command -v gh >/dev/null && gh auth status >/dev/null 2>&1; then
  setsid "$PWD/${0##*/}" _watch-merge "$branch" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" </dev/null >/dev/null 2>&1 &
  echo $! >"$watcher"
  merge_note=", when $branch is merged"
fi

url=http://localhost:$PREVIEW_PORT
curl -fsS -o /dev/null -H 'Content-Type: application/json' -H "Origin: $url" \
  -d '{"email":"preview@example.com","password":"preview-password"}' "$url/api/signup" ||
  echo "(could not create the preview user; it may exist in the restored backup)"

# On a fresh database, give the preview user some sample data, so there is
# something to review. Dates come out in this machine's time zone.
if [[ -z $restore ]]; then
  PREVIEW_TZ=${TZ:-$(cat /etc/timezone 2>/dev/null || readlink /etc/localtime 2>/dev/null | sed 's|.*zoneinfo/||' || true)}
  export PREVIEW_TZ=${PREVIEW_TZ:-UTC}
  dc --profile seed run --rm seed || echo "(could not add sample data)"
fi

printf '\n\033[1mPreview of %s is running at %s\033[0m\n' "$APP_VERSION" "$url"
echo "Sign in as preview@example.com / preview-password"
echo "Emails the app sends (e.g. confirmation links): http://localhost:${PREVIEW_MAIL_PORT:-8025}"
echo "Logs: ./preview.sh logs    Stop: ./preview.sh down"
if (( PREVIEW_MINUTES > 0 )); then
  echo "It stops by itself at $(date -d "+$PREVIEW_MINUTES minutes" +%H:%M)$merge_note, or when this folder's Claude Code session ends."
elif [[ -n $merge_note ]]; then
  echo "It stops by itself${merge_note#,}, or when this folder's Claude Code session ends."
fi
