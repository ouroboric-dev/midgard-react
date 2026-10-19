#!/bin/bash
# The first step of every rung of the ladder for a web leaf (spec §9.5), so a rung run alone is protected too.
# The files that configure the checks of a product are protected: the dev container, the CI, the lint, licence
# and base-CI configuration, and the web tool configuration (.npmrc, ESLint, TypeScript, Vite, Vitest,
# Playwright). The list is protected-common beside this script (the same file in every build realm) plus the
# web names below. A task's change to one is refused, against the commit the task started from (main): added,
# changed, deleted, moved, swapped for a symlink (a type change), not added to git yet, or hidden by .gitignore.
# The rungs read none of the leaf's tool configs (they run copies baked into the image, web-configs.sh), so a
# config changed, hidden or planted in the leaf changes nothing; it is refused all the same. A leaf .npmrc changes
# how npm runs the scripts of the packages. The scripts of package.json, which npm runs, are checked below; its
# dependency list may change. With no main to compare to (a branch called main: a tag or any other ref of that
# name does not count), no list, or a failing git, the rung fails: it never passes unchecked. Run in the leaf's top folder.
set -euo pipefail
base="$(git merge-base HEAD refs/heads/main 2>/dev/null)" || { echo "protected: this leaf has no main to compare with, so the protected files cannot be checked."; exit 1; }
echo "== protected files"
list="$(dirname "${BASH_SOURCE[0]}")/protected-common"
roots=()
globs=()
if [ -r "$list" ]; then
  while IFS= read -r line || [ -n "$line" ]; do
    line="${line#"${line%%[![:space:]]*}"}"
    line="${line%"${line##*[![:space:]]}"}"
    case "$line" in
      '' | '#'*) ;;
      ':(glob)'*) globs+=("$line") ;;
      *) roots+=("$line") ;;
    esac
  done <"$list"
fi
if [ "$((${#roots[@]} + ${#globs[@]}))" -eq 0 ]; then
  echo "protected: the list of protected files, protected-common, is missing or empty (looked for $list), so they cannot be checked."
  exit 1
fi
# The root .npmrc is named plain as well as by the glob below: the plain pathspec also matches a folder of that name.
roots+=('.npmrc')
globs+=(':(glob)**/.npmrc' ':(glob)**/eslint.config.*' ':(glob)**/.eslintrc*' ':(glob)**/tsconfig*.json'
  ':(glob)**/vite.config.*' ':(glob)**/vitest.config.*' ':(glob)**/vitest.workspace.*' ':(glob)**/playwright.config.*')
# Installed packages carry their own config files and no tool reads one from there for the leaf, so the glob
# entries skip node_modules, in the listing of the files git does not track only: a tracked file is a change
# wherever it is. The root-anchored entries (.github/, .devcontainer/, ...) skip nothing: whatever is in them is
# protected. They run in their own git calls, and never with an empty pathspec.
gitfail() {
  echo "protected: git failed while the protected files were checked ($*), so they cannot be checked."
  exit 1
}
# No --diff-filter (a type change is a change), and no --exclude-standard (an ignored file is listed too).
# Each git command is checked: a failing one fails the rung.
changed=""
# The root folders go to git without their trailing slash: a pathspec with one matches a folder only, so a
# folder added as a symlink would pass. A plain folder name matches the folder and all that is in it.
if [ "${#roots[@]}" -gt 0 ]; then
  changed+="$(git diff --no-renames --name-only "$base" -- "${roots[@]%/}")" || gitfail diff
  changed+=$'\n'"$(git ls-files --others -- "${roots[@]%/}")" || gitfail ls-files
fi
if [ "${#globs[@]}" -gt 0 ]; then
  changed+=$'\n'"$(git diff --no-renames --name-only "$base" -- "${globs[@]}")" || gitfail diff
  changed+=$'\n'"$(git ls-files --others -- "${globs[@]}" ':(exclude,glob)**/node_modules/**')" || gitfail ls-files
fi
changed="$(printf '%s\n' "$changed" | sed '/^$/d' | sort -u)"
scripts() { node -e '
  const sort = (o) => Object.fromEntries(Object.entries(o ?? {}).sort(([a], [b]) => (a < b ? -1 : 1)));
  let t = ""; process.stdin.on("data", (d) => (t += d)).on("end", () => console.log(JSON.stringify(sort(t.trim() ? JSON.parse(t).scripts : {}))));'; }
old_scripts="$(git show "$base:package.json" 2>/dev/null | scripts)"
new_scripts="$(scripts < package.json)"
[ "$old_scripts" = "$new_scripts" ] || changed+=$'\n'"package.json (the scripts)"
if [ -n "$changed" ]; then
  echo "protected: this task changes files the checks rely on; they are not the task's to change:"
  printf '%s\n' "$changed" | sed '/^$/d' | sort -u | sed 's/^/  /'
  echo "Put them back as they were; if the check is wrong, say so in your report."
  echo "A new dependency may be added to package.json, but its scripts stay as they are."
  exit 1
fi
