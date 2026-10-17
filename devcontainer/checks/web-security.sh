#!/bin/bash
# Rung 4 of the ladder for a web leaf (spec §9.5, §9.6): known vulnerabilities in the packages, from npm's
# advisories and from OSV. Secrets are scanned by Odin before every checkpoint. Run in the leaf's top folder.
set -euo pipefail
"$(dirname "$0")/web-protected.sh"
# A clean install first: npm ci deletes node_modules, which the agent can write, so no tool it left there runs.
npm ci --ignore-scripts --no-audit --no-fund
echo "== npm audit"
npm audit --audit-level=high
echo "== OSV"
osv-scanner scan source -L package-lock.json
