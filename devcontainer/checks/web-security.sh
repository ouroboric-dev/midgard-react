#!/bin/bash
# Rung 4 of the ladder for a web leaf (spec §9.5, §9.6): known vulnerabilities in the packages, from npm's
# advisories and from OSV. Secrets are scanned by Odin before every checkpoint. Run in the leaf's top folder.
set -euo pipefail
# shellcheck source-path=SCRIPTDIR source=lib/result.sh
. "$(dirname "$0")/lib/result.sh"
trap 'rc=$?; ygg_finish "$rc"' EXIT
ygg_protected
# A clean install first: npm ci deletes node_modules, which the agent can write, so no tool it left there runs.
ygg_logged build "npm ci failed" npm ci --ignore-scripts --no-audit --no-fund
echo "== npm audit"
ygg_npm_audit
echo "== OSV"
ygg_osv
