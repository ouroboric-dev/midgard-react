#!/bin/bash
# Sourced by the rungs of the ladder for a web leaf (spec §9.5), not run: it puts the tool configs that are
# baked into the image (checks/config, root-owned) into the leaf, where the tools find the packages the config
# imports (a plugin resolves from the folder of the config file; so do tsconfig's include and vite's alias).
# The rung calls each tool with its copy by name (eslint --no-config-lookup -c, tsc -p, vite --config, vitest
# --config, playwright test --config) so no config the leaf has is read: not one added in a folder (ESLint
# looks for the nearest), not a symlink, not one git ignores. The copies are made AFTER npm ci, as fresh
# files: whatever the agent left at their names (a file, a symlink, a folder) is removed first. They are
# removed when the rung ends, so the leaf is as it was.
set -euo pipefail
ygg_config_src="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/config"
ygg_config_names=(eslint.config.js tsconfig.json vite.config.ts playwright.config.ts)
ygg_config_copies=()

ygg_configs_install() { # leaf folder
  local dir="$1" name
  for name in "${ygg_config_names[@]}"; do
    rm -rf -- "${dir:?}/.ygg-$name"
    install -m 0644 -- "$ygg_config_src/$name" "$dir/.ygg-$name"
    ygg_config_copies+=("$dir/.ygg-$name")
  done
}

ygg_configs_remove() {
  [ ${#ygg_config_copies[@]} -eq 0 ] || rm -rf -- "${ygg_config_copies[@]}"
  ygg_config_copies=()
}
