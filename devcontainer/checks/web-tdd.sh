#!/bin/bash
# The TDD check, the other half of rung 2 (spec §9.5): the unit tests a change adds or changes MUST fail on
# the commit it started from. It builds that commit in a folder of its own with the leaf's installed
# packages beside it, puts the change's test files on it, and runs them: they have to fail (a test of code
# that is not there yet fails to load, which counts). A change to app code without a new or changed unit
# test is refused. Playwright specs need a browser and an API, so a change that only has those is passed
# with a note, and the reviewers look at it. Generated and type-only files are not code that needs a test.
# Run in the leaf's top folder, on the run branch; main is where the task started from.
set -euo pipefail
# The git pathspec variables of the environment would change what the src/ and e2e/ pathspecs below match.
unset GIT_LITERAL_PATHSPECS GIT_GLOB_PATHSPECS GIT_NOGLOB_PATHSPECS GIT_ICASE_PATHSPECS
# shellcheck source-path=SCRIPTDIR source=web-configs.sh
. "$(dirname "$0")/web-configs.sh"
# shellcheck source-path=SCRIPTDIR source=lib/result.sh
. "$(dirname "$0")/lib/result.sh"
trap 'rc=$?; ygg_finish "$rc"' EXIT
ygg_stage other "tdd: this leaf has no main to compare with"
base="$(git merge-base HEAD refs/heads/main)" || { echo "tdd: this leaf has no main to compare with"; exit 1; }
ygg_protected
# A clean install first: npm ci deletes node_modules, which the agent can write, so no tool it left there runs.
ygg_logged build "npm ci failed" npm ci --ignore-scripts --no-audit --no-fund
ygg_stage other "tdd: the check could not finish"

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
    ygg_stage test "tdd: app code changed without a new or changed unit test: ${code[*]}. Write the test first, see it fail, then write the code."
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
trap 'rc=$?; ygg_configs_remove; rm -rf -- "$parent"; ygg_finish "$rc"' EXIT
git archive "$base" | tar -x -C "$parent"
ln -s "$PWD/node_modules" "$parent/node_modules"
# The parent runs the baked vite config too, not the one of the commit it was made from.
ygg_stage build "the baked tool configs could not be installed in the leaf"
ygg_configs_install "$parent"
for f in "${tests[@]}"; do
  mkdir -p "$parent/$(dirname "$f")"
  cp -- "$f" "$parent/$f"
done
log="$parent/.tdd.log"
if (cd "$parent" && node_modules/.bin/vitest run --config .ygg-vite.config.ts "${tests[@]}") >"$log" 2>&1; then
  ygg_stage test "tdd: the new tests pass on the commit the task started from, so they test nothing new: ${tests[*]}. A test of new behaviour fails before the code for it exists."
  echo "tdd: the new tests pass on the commit the task started from, so they test nothing new:"
  printf '  %s\n' "${tests[@]}"
  echo "A test of new behaviour fails before the code for it exists. Change the tests so they do."
  exit 1
fi
echo "tdd: the new tests fail on the commit the task started from, as they must:"
printf '  %s\n' "${tests[@]}"
tail -n 12 "$log"
