#!/bin/bash
# Rung 3 of the ladder for a web leaf (spec §9.5): the contract this app uses. The copy of the API's
# contract is linted, the types generated from it are what it says, and the changes of the copy against the
# commit the task started from are listed: a changed copy means the API changed, and the acceptance run
# checks the copy is the API leaf's own file. Run in the leaf's top folder.
set -euo pipefail
c=contracts/api.openapi.yaml
[ -f "$c" ] || { echo "contract: this leaf has no $c"; exit 1; }
"$(dirname "$0")/web-protected.sh"
# A clean install first: npm ci deletes node_modules, which the agent can write, so no tool it left there runs.
npm ci --ignore-scripts --no-audit --no-fund
echo "== openapi lint"
vacuum lint --no-style --details --fail-severity error "$c"
echo "== generated types"
tmp="$(mktemp -d)"
trap 'rm -rf -- "$tmp"' EXIT
node_modules/.bin/openapi-typescript "$c" -o "$tmp/schema.d.ts" >/dev/null
if ! diff -q "$tmp/schema.d.ts" src/api/schema.d.ts >/dev/null; then
  echo "contract: src/api/schema.d.ts is not what $c generates; run npm run gen:api"
  exit 1
fi
echo "== changes of the contract since the task started"
base="$(git merge-base HEAD refs/heads/main)" || { echo "contract: this leaf has no main to compare with"; exit 1; }
if git cat-file -e "$base:$c" 2>/dev/null; then
  git show "$base:$c" > "$tmp/old.yaml"
  if [ "$(oasdiff breaking "$tmp/old.yaml" "$c" --format json)" = "[]" ]; then
    echo "none"
  else
    echo "BREAKING: the API this app calls changed in ways that break callers; say so in your summary."
    oasdiff breaking "$tmp/old.yaml" "$c"
  fi
else
  echo "$c is new in this change"
fi
