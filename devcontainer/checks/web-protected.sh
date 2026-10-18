#!/bin/bash
# The first step of every rung of the ladder for a web leaf (spec §9.5), so a rung run alone is protected too.
# The tool configs (ESLint, TypeScript, Vite, Vitest, Playwright) are not checked here: the rungs run copies
# baked into the image and never read the leaf's (web-configs.sh). What is left is what the baked configs do
# not cover: the scripts of package.json, which npm runs, and a leaf .npmrc, which changes how npm runs the
# scripts of the packages. The dependency list of package.json may change. The task's diff is the working
# tree against the commit the task started from (main). Run in the leaf's top folder.
set -euo pipefail
base="$(git merge-base HEAD main)" || { echo "protected: this leaf has no main to compare with"; exit 1; }
echo "== protected files"
files=('.npmrc')
# Added, changed, deleted or renamed since the task started, and files git has not seen yet, ignored ones too.
changed="$(git diff --no-renames --name-only "$base" -- "${files[@]}")"
untracked="$(git ls-files --others -- "${files[@]}")"
[ -z "$untracked" ] || changed+=$'\n'"$untracked"
scripts() { node -e '
  const sort = (o) => Object.fromEntries(Object.entries(o ?? {}).sort(([a], [b]) => (a < b ? -1 : 1)));
  let t = ""; process.stdin.on("data", (d) => (t += d)).on("end", () => console.log(JSON.stringify(sort(t.trim() ? JSON.parse(t).scripts : {}))));'; }
old_scripts="$(git show "$base:package.json" 2>/dev/null | scripts)"
new_scripts="$(scripts < package.json)"
[ "$old_scripts" = "$new_scripts" ] || changed+=$'\n'"package.json (the scripts)"
if [ -n "$changed" ]; then
  echo "protected: this task changes files the checks rely on; they are not the task's to change:"
  sed '/^$/d' <<<"$changed" | sort -u | sed 's/^/  /'
  echo "Undo these changes. A new dependency may be added to package.json, but its scripts stay as they are."
  exit 1
fi
