#!/usr/bin/env bash
# Starts a new app from this template:
#
#   scripts/new-app.sh <name> [go-module]
#
# e.g. scripts/new-app.sh habit-tracker creates ../habit-tracker with the Go
# module github.com/francis/habit-tracker and the display name
# "Habit Tracker", runs its checks, and makes the first commit. It copies
# the files git knows about (committed or not; ignored files stay behind).
set -euo pipefail

name=${1:?usage: scripts/new-app.sh <name> [go-module]}
[[ $name =~ ^[a-z][a-z0-9-]*$ ]] || { echo "name must be lower-case letters, digits and dashes: $name" >&2; exit 2; }
module=${2:-github.com/francis/$name}
title=$(sed -E 's/(^|-)([a-z])/ \U\2/g; s/^ //' <<<"$name")   # habit-tracker -> Habit Tracker

src=$(cd "$(dirname "$0")/.." && pwd)
dest=$(dirname "$src")/$name
[[ ! -e $dest ]] || { echo "$dest already exists" >&2; exit 1; }

echo "Creating $dest ($title, module $module)"
mkdir "$dest"
(cd "$src" && git ls-files -z --cached --others --exclude-standard | xargs -0 tar -cf - 2>/dev/null) | tar -xf - -C "$dest"
cd "$dest"

# The template's own tooling and notes don't belong in the new app.
rm scripts/new-app.sh
rmdir scripts 2>/dev/null || true
sed -i '/<!-- template:start -->/,/<!-- template:end -->/d' README.md
cat -s README.md > README.tmp && mv README.tmp README.md   # squeeze the blank lines left behind

grep -rlI --exclude-dir=node_modules -e 'app-template' -e 'App Template' . | while read -r f; do
  sed -i "s#github.com/francis/app-template#$module#g; s#app-template#$name#g; s#App Template#$title#g" "$f"
done

git init -q
git add -A
git commit -qm "Start $name from app-template"
echo
echo "Done. Next:"
echo "  cd ../$name"
echo "  ./verify.sh                 # check everything works"
echo "  ./preview.sh                # click through it at http://localhost:8081"
echo "Then read README.md: 'Adding a feature' and 'Removing the example'."
