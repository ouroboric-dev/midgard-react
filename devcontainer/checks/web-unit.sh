#!/bin/bash
# Rung 2, the tests: every unit test of the leaf. (web-tdd.sh is the other half of the rung: it shows the
# new tests fail on the commit the task started from.)
set -euo pipefail
# shellcheck source=web-configs.sh
. "$(dirname "$0")/web-configs.sh"
trap ygg_configs_remove EXIT
"$(dirname "$0")/web-protected.sh"
# A clean install first: npm ci deletes node_modules, which the agent can write, so no tool it left there runs.
npm ci --ignore-scripts --no-audit --no-fund
ygg_configs_install "$PWD"
node_modules/.bin/vitest run --config .ygg-vite.config.ts
