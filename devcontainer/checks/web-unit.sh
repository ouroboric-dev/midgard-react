#!/bin/bash
# Rung 2, the tests: every unit test of the leaf. (web-tdd.sh is the other half of the rung: it shows the
# new tests fail on the commit the task started from.)
set -euo pipefail
# shellcheck source-path=SCRIPTDIR source=web-configs.sh
. "$(dirname "$0")/web-configs.sh"
# shellcheck source-path=SCRIPTDIR source=lib/result.sh
. "$(dirname "$0")/lib/result.sh"
trap 'rc=$?; ygg_configs_remove; ygg_finish "$rc"' EXIT
ygg_protected
# A clean install first: npm ci deletes node_modules, which the agent can write, so no tool it left there runs.
ygg_logged build "npm ci failed" npm ci --ignore-scripts --no-audit --no-fund
ygg_configs_install "$PWD"
if ygg_on; then
  ygg_tool vitest-json "$ygg_out/vitest.json"
  node_modules/.bin/vitest run --config .ygg-vite.config.ts --reporter=default --reporter=json --outputFile.json="$ygg_out/vitest.json"
else
  node_modules/.bin/vitest run --config .ygg-vite.config.ts
fi
