#!/bin/bash
# The acceptance run of a pointer move (spec §4.5, §9.5): the whole product, as it would be built, on a
# snapshot of the project in the acceptance sandbox, which has the services of every leaf. It checks that the
# API still keeps what the web app's copy of the contract promises, builds the API, starts it against
# Postgres, builds the web app and runs its Playwright specs against both. The leaves are at repos/api and repos/web unless
# YGG_API_DIR and YGG_WEB_DIR say otherwise. A product with no server of its own has no API leaf (no folder
# for it): then the oasdiff, build and start steps are skipped, and the web app runs its specs against a mock
# of its own copy of the contract (Prism), or against no API when it has no copy. Run in the top folder of the snapshot.
set -euo pipefail
# shellcheck source-path=SCRIPTDIR source=web-configs.sh
. "$(dirname "$0")/web-configs.sh"
# shellcheck source-path=SCRIPTDIR source=lib/result.sh
. "$(dirname "$0")/lib/result.sh"
trap 'rc=$?; ygg_finish "$rc"' EXIT
api="${YGG_API_DIR:-repos/api}" web="${YGG_WEB_DIR:-repos/web}" wait_s="${YGG_E2E_WAIT:-60}"
api_port="${YGG_API_PORT:-8080}" app_port="${YGG_APP_PORT:-4173}"
db_host="${YGG_DB_HOST:-postgres}" db_port="${YGG_DB_PORT:-5432}"
s3_endpoint="${YGG_S3_ENDPOINT:-http://s3:8333}" s3_bucket=ygg-store
has_api=1
[ -e "$api" ] || has_api=0
if [ "$has_api" -eq 1 ]; then
  ygg_stage other "accept: the API leaf has no $api/openapi.yaml"
  [ -f "$api/openapi.yaml" ] || { echo "accept: the API leaf has no $api/openapi.yaml"; exit 1; }
  ygg_stage other "accept: the web leaf has no $web/contracts/api.openapi.yaml"
  [ -f "$web/contracts/api.openapi.yaml" ] || { echo "accept: the web leaf has no $web/contracts/api.openapi.yaml"; exit 1; }
  echo "== the API keeps what the web app's copy of the contract promises"
  # The copy may lag behind the API (the API's task comes first), but the API may not take away or change what the
  # copy has: oasdiff reads the copy as the old contract and the API's file as the new one.
  if ! ygg_oasdiff "$web/contracts/api.openapi.yaml" "$api/openapi.yaml"; then
    ygg_tool oasdiff-json "$ygg_out/oasdiff.json"
    echo "accept: the API's contract breaks what the web app's copy of it promises. Change the web app to the API's contract: copy $api/openapi.yaml to $web/contracts/api.openapi.yaml, run npm run gen:api in the web leaf, and fix what it breaks."
    exit 1
  fi
fi
busy() { (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; }
for p in "$api_port" "$app_port"; do
  if busy "$p"; then ygg_stage other "accept: port $p is already in use; a server of an earlier run may still be up"; echo "accept: port $p is already in use; a server of an earlier run may still be up"; exit 1; fi
done
log="$(mktemp -d)"
pids=()
cleanup() {
  local rc=$?
  for p in "${pids[@]}"; do kill "$p" 2>/dev/null || true; done
  wait 2>/dev/null || true
  ygg_configs_remove
  rm -rf -- "$log"
  ygg_finish "$rc"
}
trap cleanup EXIT
wait_for() { # port name
  for ((i = 0; i < wait_s; i++)); do
    if (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; then return 0; fi
    sleep 1
  done
  ygg_stage other "accept: $2 did not answer on port $1 in ${wait_s}s: $(tail -n 15 "$log/$2.log" 2>/dev/null || true)"
  echo "accept: $2 did not answer on port $1 in ${wait_s}s"
  tail -n 15 "$log/$2.log" 2>/dev/null || true
  exit 1
}
if [ "$has_api" -eq 1 ]; then
  for ((i = 0; i < wait_s; i++)); do
    if (exec 3<>"/dev/tcp/$db_host/$db_port") 2>/dev/null; then break; fi
    [ "$i" -lt $((wait_s - 1)) ] || { ygg_stage other "accept: Postgres at $db_host:$db_port did not answer in ${wait_s}s"; echo "accept: Postgres at $db_host:$db_port did not answer in ${wait_s}s"; exit 1; }
    sleep 1
  done
  echo "== build the API"
  # shellcheck disable=SC2016 # the script is bash -c's, expanded there
  ygg_logged build "go build of the API failed" bash -c 'cd "$1" && go build -o "$2" ./cmd/api' _ "$api" "$log/api"
  echo "== start the API"
  mkdir -p "$log/data"
  # The sandbox's SeaweedFS has no identity file and accepts any key: the pair is a fixed dummy one.
  PORT="$api_port" HOST=127.0.0.1 DATABASE_URL="postgres://postgres@$db_host:$db_port/app?sslmode=disable" DATA_DIR="$log/data" \
    S3_ENDPOINT="$s3_endpoint" S3_REGION=us-east-1 S3_BUCKET="$s3_bucket" S3_ACCESS_KEY_ID=ygg-accept S3_SECRET_ACCESS_KEY=ygg-accept-secret \
    "$log/api" >"$log/api.log" 2>&1 &
  pids+=($!)
  wait_for "$api_port" api
elif [ -f "$web/contracts/api.openapi.yaml" ]; then
  echo "== no API leaf: a mock of the web app's copy of the contract"
  (cd "$web" && exec prism mock --host 127.0.0.1 --port "$api_port" contracts/api.openapi.yaml) >"$log/api.log" 2>&1 &
  pids+=($!)
  wait_for "$api_port" api
else
  echo "== no API leaf and no contract: the web app runs with no API"
  api_port=""
fi
echo "== build the web app"
# shellcheck disable=SC2016 # the script is bash -c's, expanded there
ygg_logged build "npm ci failed" bash -c 'cd "$1" && npm ci --ignore-scripts --no-audit --no-fund' _ "$web"
# The baked configs go in after the install and are the only ones the tools read (web-configs.sh).
ygg_stage build "the baked tool configs could not be installed in the leaf"
ygg_configs_install "$web"
# shellcheck disable=SC2016 # the script is bash -c's, expanded there
ygg_logged build "tsc or vite build failed" bash -c 'cd "$1" && node_modules/.bin/tsc --noEmit -p .ygg-tsconfig.json && node_modules/.bin/vite build --config .ygg-vite.config.ts' _ "$web"
echo "== start the web app"
store=""
if [ "$has_api" -eq 1 ]; then
  # The baked server, from the leaf's folder (it serves ./dist), never the leaf's own serve.mjs.
  (cd "$web" && exec env PORT="$app_port" HOST=127.0.0.1 API_URL="http://127.0.0.1:$api_port" S3_ENDPOINT="$s3_endpoint" S3_BUCKET="$s3_bucket" node "$ygg_serve") >"$log/app.log" 2>&1 &
  store=1
else
  (cd "$web" && exec env ${api_port:+"API_URL=http://127.0.0.1:$api_port"} node_modules/.bin/vite preview --config .ygg-vite.config.ts --host 127.0.0.1 --port "$app_port" --strictPort) >"$log/app.log" 2>&1 &
fi
pids+=($!)
wait_for "$app_port" app
echo "== playwright"
# YGG_E2E_STORE=1 tells a spec that a real store is behind /<bucket>/; with no API there is none.
if ygg_on; then
  ygg_leaf="$(cd "$web" && pwd)"
  ygg_tool playwright-json "$ygg_out/playwright.json"
  (cd "$web" && unset YGG_E2E_STORE && env E2E_URL="http://127.0.0.1:$app_port" PLAYWRIGHT_JSON_OUTPUT_NAME="$ygg_out/playwright.json" ${store:+YGG_E2E_STORE=1} node_modules/.bin/playwright test --config .ygg-playwright.config.ts --reporter=list,json)
else
  (cd "$web" && unset YGG_E2E_STORE && env E2E_URL="http://127.0.0.1:$app_port" ${store:+YGG_E2E_STORE=1} node_modules/.bin/playwright test --config .ygg-playwright.config.ts)
fi
