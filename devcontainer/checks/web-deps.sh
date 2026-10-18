#!/bin/bash
# Rung 4 of the ladder for a web leaf, first half (spec §9.5, §9.6; v0-T258): the dependency policy. Every direct
# dependency is pinned to a minor line (x.y.z or ~x.y.z), the packages the policy names are locked at or above
# their minimum patched version, and the audit tools the policy names find nothing. The policy is
# config/deps-policy.json, baked beside this script where the agent cannot write; a policy file in the leaf is
# never read. The policy check reads package.json and package-lock.json and runs before any install. Run in the
# leaf's top folder.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
policy="$here/config/deps-policy.json"
"$here/web-protected.sh"
echo "== dependency policy"
node "$here/lib/webdeps.mjs" check "$policy"
# A clean install first: npm ci deletes node_modules, which the agent can write, so no tool it left there runs.
npm ci --ignore-scripts --no-audit --no-fund
# The lock file is the agent's text: check what was installed, too, before any audit.
node "$here/lib/webdeps.mjs" installed "$policy"
tools="$(node "$here/lib/webdeps.mjs" audit "$policy")"
for tool in $tools; do
  case "$tool" in
    npm-audit)
      echo "== npm audit"
      npm audit --audit-level=high
      ;;
    osv)
      echo "== OSV"
      osv-scanner scan source -L package-lock.json
      ;;
    *)
      echo "deps: unknown audit $tool"
      exit 1
      ;;
  esac
done
