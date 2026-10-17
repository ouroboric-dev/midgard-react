#!/bin/sh
# Prepares a fresh clone for work: installs exactly what package-lock.json says. Run it once.
set -eu
npm ci --no-audit --no-fund
echo "ready: 'npm test' runs the unit tests, 'npm run dev' the app"
