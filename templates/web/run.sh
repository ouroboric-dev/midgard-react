#!/bin/sh
# Starts this leaf on $PORT: builds the bundle, then serves it in the foreground (node serve.mjs). It installs
# nothing: ./init.sh installs the packages first. It reads no secret and writes only dist/ in this folder (ignored
# by git) and what npm and tsc keep in their own caches.
set -eu
cd "$(dirname "$0")"

if [ -z "${PORT:-}" ]; then
  echo "run.sh: PORT is not set. Run it as: PORT=3000 ./run.sh" >&2
  exit 1
fi
case "$PORT" in
  *[!0-9]*)
    echo "run.sh: PORT must be a number, not '$PORT'." >&2
    exit 1
    ;;
esac
if [ ! -x node_modules/.bin/vite ]; then
  echo "run.sh: node_modules is missing. Run ./init.sh first; run.sh installs nothing." >&2
  exit 1
fi

if ! npm run build --silent; then
  echo "run.sh: the build failed (npm run build); see the messages above." >&2
  exit 1
fi

# serve.mjs serves dist/ on HOST:PORT (HOST defaults to 127.0.0.1; the product start sets 0.0.0.0). It forwards
# /api/ to API_URL (default http://localhost:8080) and /$S3_BUCKET/ to S3_ENDPOINT, as delta §9.3 says.
HOST="${HOST:-127.0.0.1}"
export HOST PORT
exec node serve.mjs
