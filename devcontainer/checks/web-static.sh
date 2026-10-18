#!/bin/bash
# Rung 1 of the ladder for a web leaf (spec §9.5): the packages are what package-lock.json says, the code
# is linted and type-checked and it builds, and the docs are in order. Run in the leaf's top folder.
set -euo pipefail
# shellcheck source=web-configs.sh
. "$(dirname "$0")/web-configs.sh"
trap ygg_configs_remove EXIT
"$(dirname "$0")/web-protected.sh"
# A clean install first: npm ci deletes node_modules, which the agent can write, so no tool it left there
# runs. The tools are called by path, not through the scripts of package.json, and with the baked configs.
echo "== npm ci"
npm ci --ignore-scripts --no-audit --no-fund
ygg_configs_install "$PWD"
echo "== eslint"
node_modules/.bin/eslint --no-config-lookup -c .ygg-eslint.config.js .
echo "== tsc"
node_modules/.bin/tsc --noEmit -p .ygg-tsconfig.json
echo "== build"
node_modules/.bin/tsc --noEmit -p .ygg-tsconfig.json
node_modules/.bin/vite build --config .ygg-vite.config.ts
echo "== docs"
"$(dirname "$0")/docs.sh"
