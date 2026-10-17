#!/bin/bash
# Rung 1 of the ladder for a web leaf (spec §9.5): the packages are what package-lock.json says, the code
# is linted and type-checked and it builds, and the docs are in order. Run in the leaf's top folder.
set -euo pipefail
if [ ! -d node_modules ]; then
  echo "== npm ci"
  npm ci --no-audit --no-fund
fi
echo "== eslint"
npm run lint
echo "== tsc"
npm run typecheck
echo "== build"
npm run build
echo "== docs"
"$(dirname "$0")/docs.sh"
