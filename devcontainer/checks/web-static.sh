#!/bin/bash
# Rung 1 of the ladder for a web leaf (spec §9.5): the packages are what package-lock.json says, the code
# is linted and type-checked and it builds, and the docs are in order. Run in the leaf's top folder.
set -euo pipefail
# shellcheck source-path=SCRIPTDIR source=web-configs.sh
. "$(dirname "$0")/web-configs.sh"
# shellcheck source-path=SCRIPTDIR source=lib/result.sh
. "$(dirname "$0")/lib/result.sh"
trap 'rc=$?; ygg_configs_remove; ygg_finish "$rc"' EXIT
ygg_protected
# tsc prints its diagnostics on stdout: they are the log and the machine output both.
ygg_tsc() {
  local rc=0
  if ! ygg_on; then node_modules/.bin/tsc --noEmit -p .ygg-tsconfig.json; return; fi
  ygg_tool tsc-text "$ygg_out/tsc.txt"
  node_modules/.bin/tsc --noEmit -p .ygg-tsconfig.json >"$ygg_out/tsc.txt" || rc=$?
  cat "$ygg_out/tsc.txt"
  return "$rc"
}
# A clean install first: npm ci deletes node_modules, which the agent can write, so no tool it left there
# runs. The tools are called by path, not through the scripts of package.json, and with the baked configs.
echo "== npm ci"
ygg_logged build "npm ci failed" npm ci --ignore-scripts --no-audit --no-fund
ygg_stage build "the baked tool configs could not be installed in the leaf"
ygg_configs_install "$PWD"
echo "== eslint"
if ygg_on; then
  # The machine output goes to a file; on a failure the log gets the readable report too.
  ygg_tool eslint-json "$ygg_out/eslint.json"
  node_modules/.bin/eslint --no-config-lookup -c .ygg-eslint.config.js -f json -o "$ygg_out/eslint.json" . ||
    { rc=$?; node_modules/.bin/eslint --no-config-lookup -c .ygg-eslint.config.js . || true; exit "$rc"; }
else
  node_modules/.bin/eslint --no-config-lookup -c .ygg-eslint.config.js .
fi
echo "== tsc"
ygg_tsc
echo "== build"
ygg_tsc
ygg_logged build "vite build failed" node_modules/.bin/vite build --config .ygg-vite.config.ts
echo "== docs"
ygg_lines lint "docs: the documents are not in order" "$(dirname "$0")/docs.sh"
