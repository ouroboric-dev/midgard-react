#!/bin/bash
# Rung 1 of the ladder for a web leaf (spec §9.5): the packages are what package-lock.json says, the code
# is linted and type-checked and it builds, and the docs are in order. Run in the leaf's top folder.
set -euo pipefail
# The ladder reads these files and runs the scripts of package.json, so a task may not change them: a rung would
# turn green by editing the rule instead of the code. The dependency list of package.json may change. The task's
# diff is the working tree against the commit the task started from (main).
base="$(git merge-base HEAD main)" || { echo "protected: this leaf has no main to compare with"; exit 1; }
echo "== protected files"
changed="$(git diff --no-renames --name-only "$base" -- eslint.config.js tsconfig.json vite.config.ts playwright.config.ts)"
scripts() { node -e '
  const sort = (o) => Object.fromEntries(Object.entries(o ?? {}).sort(([a], [b]) => (a < b ? -1 : 1)));
  let t = ""; process.stdin.on("data", (d) => (t += d)).on("end", () => console.log(JSON.stringify(sort(t.trim() ? JSON.parse(t).scripts : {}))));'; }
old_scripts="$(git show "$base:package.json" 2>/dev/null | scripts)"
new_scripts="$(scripts < package.json)"
[ "$old_scripts" = "$new_scripts" ] || changed+=$'\n'"package.json (the scripts)"
if [ -n "$changed" ]; then
  echo "protected: this task changes files the checks rely on; they are not the task's to change:"
  sed '/^$/d; s/^/  /' <<<"$changed"
  echo "Undo these changes. A new dependency may be added to package.json, but its scripts stay as they are."
  exit 1
fi
# A clean install first: npm ci deletes node_modules, which the agent can write, so no tool it left there
# runs. The tools are called by path, not through the scripts of package.json.
echo "== npm ci"
npm ci --no-audit --no-fund
echo "== eslint"
node_modules/.bin/eslint .
echo "== tsc"
node_modules/.bin/tsc --noEmit -p tsconfig.json
echo "== build"
node_modules/.bin/tsc --noEmit -p tsconfig.json
node_modules/.bin/vite build
echo "== docs"
"$(dirname "$0")/docs.sh"
