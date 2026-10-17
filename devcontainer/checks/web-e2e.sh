#!/bin/bash
# Rung 5 of the ladder for a web leaf (spec §9.5): Playwright runs against the built app. In a task's loop the
# API behind the app is a mock made from the leaf's copy of the contract (Prism), so the task needs no API
# of its own; the real API is part of the acceptance run (accept-e2e.sh). The ports are YGG_MOCK_PORT
# (4010) and YGG_APP_PORT (4173); it refuses to start if either is taken. Run in the leaf's top folder. Every
# server it starts is stopped.
set -euo pipefail
mock_port="${YGG_MOCK_PORT:-4010}" app_port="${YGG_APP_PORT:-4173}" wait_s="${YGG_E2E_WAIT:-60}"
log="$(mktemp -d)"
busy() { (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; }
for p in "$mock_port" "$app_port"; do
  if busy "$p"; then echo "e2e: port $p is already in use; a server of an earlier run may still be up"; exit 1; fi
done
pids=()
cleanup() {
  for p in "${pids[@]}"; do kill "$p" 2>/dev/null || true; done
  wait 2>/dev/null || true
  rm -rf -- "$log"
}
trap cleanup EXIT
wait_for() { # port name
  for ((i = 0; i < wait_s; i++)); do
    if (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; then return 0; fi
    sleep 1
  done
  echo "e2e: $2 did not answer on port $1 in ${wait_s}s"
  tail -n 15 "$log/$2.log" 2>/dev/null || true
  exit 1
}
"$(dirname "$0")/web-protected.sh"
# A clean install first: npm ci deletes node_modules, which the agent can write, so no tool it left there runs.
npm ci --ignore-scripts --no-audit --no-fund
echo "== mock API from contracts/api.openapi.yaml"
prism mock --host 127.0.0.1 --port "$mock_port" contracts/api.openapi.yaml >"$log/mock.log" 2>&1 &
pids+=($!)
wait_for "$mock_port" mock
echo "== build"
node_modules/.bin/tsc --noEmit -p tsconfig.json
node_modules/.bin/vite build
echo "== app"
# vite itself, not npx: the pid kept is the server's, so stopping it stops the server.
(API_URL="http://127.0.0.1:$mock_port" exec node_modules/.bin/vite preview --host 127.0.0.1 --port "$app_port" --strictPort) >"$log/app.log" 2>&1 &
pids+=($!)
wait_for "$app_port" app
echo "== playwright"
E2E_URL="http://127.0.0.1:$app_port" node_modules/.bin/playwright test
