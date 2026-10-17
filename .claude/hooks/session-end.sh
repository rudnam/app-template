#!/usr/bin/env bash
# When the Claude Code session in this folder ends, stop its preview
# (./preview.sh) and anything a killed ./verify.sh left running. Only this
# folder's stacks: another worktree's belong to another session. Clearing
# the conversation (/clear) is not an end, so the preview stays then.
input=$(cat)
[[ $input == *'"reason":"clear"'* || $input == *'"reason": "clear"'* ]] && exit 0
cd "${CLAUDE_PROJECT_DIR:-.}" || exit 0
command -v docker >/dev/null 2>&1 || exit 0
project=${PWD##*/}; project=${project,,}
# Down by project name needs no compose file; verify keeps its Go caches.
timeout 60 docker compose -p "$project-preview" down --volumes --remove-orphans --timeout 1 >/dev/null 2>&1
timeout 60 docker compose -p "$project-verify" down --remove-orphans --timeout 1 >/dev/null 2>&1
exit 0
