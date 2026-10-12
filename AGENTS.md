# AGENTS.md: realm-web

The web realm: graph, prompts, devcontainer stack layer, templates and checks.

Follow the Yggdrasil invariants:
https://github.com/ouroboric-dev/yggdrasil/blob/main/AGENTS.md#invariants
This file only adds rules for this repo.

## This repo

- Build and test: `go test ./...`. Lint: `go vet ./...`.
- Interfaces live in `gungnir`, never here (spec §1).
- Architecture: `docs/architecture.md`.
