#!/bin/bash
# Rung 3 of the ladder for a web leaf (spec §9.5): the contract this app uses. The copy of the API's
# contract is linted, the types generated from it are what it says, and the changes of the copy against the
# commit the task started from are listed: a changed copy means the API changed, and the acceptance run
# checks the copy is the API leaf's own file. Run in the leaf's top folder.
set -euo pipefail
# shellcheck source-path=SCRIPTDIR source=lib/result.sh
. "$(dirname "$0")/lib/result.sh"
trap 'rc=$?; ygg_finish "$rc"' EXIT
c=contracts/api.openapi.yaml
ygg_stage other "contract: this leaf has no $c"
[ -f "$c" ] || { echo "contract: this leaf has no $c"; exit 1; }
ygg_protected
# A clean install first: npm ci deletes node_modules, which the agent can write, so no tool it left there runs.
ygg_logged build "npm ci failed" npm ci --ignore-scripts --no-audit --no-fund
echo "== openapi lint"
ygg_logged lint "vacuum: $c has errors" vacuum lint --no-style --details --fail-severity error "$c"
echo "== generated types"
tmp="$(mktemp -d)"
trap 'rc=$?; rm -rf -- "$tmp"; ygg_finish "$rc"' EXIT
ygg_stage build "openapi-typescript could not generate the types from $c"
node_modules/.bin/openapi-typescript "$c" -o "$tmp/schema.d.ts" >/dev/null
ygg_stage build "contract: src/api/schema.d.ts is not what $c generates; run npm run gen:api"
if ! diff -q "$tmp/schema.d.ts" src/api/schema.d.ts >/dev/null; then
  echo "contract: src/api/schema.d.ts is not what $c generates; run npm run gen:api"
  exit 1
fi
echo "== changes of the contract since the task started"
ygg_stage other "contract: this leaf has no main to compare with"
base="$(git merge-base HEAD refs/heads/main)" || { echo "contract: this leaf has no main to compare with"; exit 1; }
ygg_stage other "contract: the changes of the contract could not be listed"
# oasdiff only lists the changes here and the check passes whatever it says, so its output is not a failure: it is not given to ygg-check-result.
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
