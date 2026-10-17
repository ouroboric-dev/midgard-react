#!/bin/bash
# Rung 2, the tests: every unit test of the leaf. (web-tdd.sh is the other half of the rung: it shows the
# new tests fail on the commit the task started from.)
set -euo pipefail
[ -d node_modules ] || npm ci --no-audit --no-fund
npm test
