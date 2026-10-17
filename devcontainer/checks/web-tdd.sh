#!/bin/bash
# The TDD check, the other half of rung 2 (spec §9.5): the unit tests a change adds or changes MUST fail on
# the commit it started from. It builds that commit in a folder of its own with the leaf's installed
# packages beside it, puts the change's test files on it, and runs them: they have to fail (a test of code
# that is not there yet fails to load, which counts). A change to app code without a new or changed unit
# test is refused. Playwright specs need a browser and an API, so a change that only has those is passed
# with a note, and the reviewers look at it. Generated and type-only files are not code that needs a test.
# Run in the leaf's top folder, on the run branch; main is where the task started from.
set -euo pipefail
base="$(git merge-base HEAD main)" || { echo "tdd: this leaf has no main to compare with"; exit 1; }
# A clean install first: npm ci deletes node_modules, which the agent can write, so no tool it left there runs.
npm ci --no-audit --no-fund

tests=() code=() specs=()
while IFS= read -r f; do
  [ -f "$f" ] || continue
  case "$f" in e2e/*) specs+=("$f") ;; *.test.ts | *.test.tsx) tests+=("$f") ;; esac
done < <(git diff --no-renames --name-only --diff-filter=d "$base" HEAD -- 'src/**' 'e2e/**')
while IFS= read -r f; do
  [ -f "$f" ] || continue
  case "$f" in *.test.ts | *.test.tsx | *.d.ts | src/test-setup.ts | src/index.css) continue ;; esac
  case "$f" in *.ts | *.tsx) code+=("$f") ;; esac
done < <(git diff --no-renames --name-only --diff-filter=d "$base" HEAD -- 'src/**')

if [ ${#tests[@]} -eq 0 ]; then
  if [ ${#code[@]} -gt 0 ] && [ ${#specs[@]} -eq 0 ]; then
    echo "tdd: app code changed without a new or changed unit test:"
    printf '  %s\n' "${code[@]}"
    echo "Write the test first, see it fail, then write the code."
    exit 1
  fi
  if [ ${#specs[@]} -gt 0 ]; then
    echo "tdd: only Playwright specs changed, and they need a browser and an API; they cannot be shown to fail here:"
    printf '  %s\n' "${specs[@]}"
  else
    echo "tdd: no app code changed"
  fi
  exit 0
fi

parent="$(mktemp -d)"
trap 'rm -rf -- "$parent"' EXIT
git archive "$base" | tar -x -C "$parent"
ln -s "$PWD/node_modules" "$parent/node_modules"
for f in "${tests[@]}"; do
  mkdir -p "$parent/$(dirname "$f")"
  cp -- "$f" "$parent/$f"
done
log="$parent/.tdd.log"
if (cd "$parent" && node_modules/.bin/vitest run "${tests[@]}") >"$log" 2>&1; then
  echo "tdd: the new tests pass on the commit the task started from, so they test nothing new:"
  printf '  %s\n' "${tests[@]}"
  echo "A test of new behaviour fails before the code for it exists. Change the tests so they do."
  exit 1
fi
echo "tdd: the new tests fail on the commit the task started from, as they must:"
printf '  %s\n' "${tests[@]}"
tail -n 12 "$log"
