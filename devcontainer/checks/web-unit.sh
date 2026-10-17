#!/bin/bash
# Rung 2, the tests: every unit test of the leaf. (web-tdd.sh is the other half of the rung: it shows the
# new tests fail on the commit the task started from.)
set -euo pipefail
"$(dirname "$0")/web-protected.sh"
# A clean install first: npm ci deletes node_modules, which the agent can write, so no tool it left there runs.
npm ci --ignore-scripts --no-audit --no-fund
node_modules/.bin/vitest run
