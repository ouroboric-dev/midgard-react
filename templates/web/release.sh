#!/bin/sh
# Builds this leaf's release into $OUT: OUT/dist (the bundle), OUT/serve.mjs and OUT/run.sh. The release starts
# with Node alone: OUT/run.sh needs no npm and no node_modules. It installs the packages as the acceptance run
# does (npm ci, exactly as package-lock.json says; this is the one step that reaches the registry), builds the
# bundle, and writes only into $OUT and this folder's dist/ and node_modules/ (both ignored by git).
set -eu

if [ -z "${OUT:-}" ]; then
  echo "release.sh: OUT is not set. Run it as: OUT=/path/to/release ./release.sh" >&2
  exit 1
fi
# A relative OUT is relative to the caller's folder, not to this one.
case "$OUT" in
  /*) ;;
  *) OUT="$PWD/$OUT" ;;
esac
cd "$(dirname "$0")"
mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd)"

npm ci --ignore-scripts --no-audit --no-fund
npm run build --silent

rm -rf "$OUT/dist"
mkdir "$OUT/dist"
cp -RL dist/. "$OUT/dist/"
# Remove first: a symlink planted at either name must not be followed.
rm -f "$OUT/run.sh" "$OUT/serve.mjs"
cp serve.mjs "$OUT/serve.mjs"

cat >"$OUT/run.sh" <<'RUN'
#!/bin/sh
# Starts this release on $PORT with Node alone (HOST defaults to 127.0.0.1; API_URL, S3_ENDPOINT and S3_BUCKET
# are serve.mjs's own settings). It installs and builds nothing.
set -eu
case "$0" in
  */*) cd "${0%/*}" ;;
esac

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

HOST="${HOST:-127.0.0.1}"
export HOST PORT
exec node serve.mjs
RUN
chmod 755 "$OUT/run.sh"
