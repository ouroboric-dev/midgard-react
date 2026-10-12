---
title: realm-web architecture
status: draft
date: 2026-10-12
related: [https://github.com/ouroboric-dev/yggdrasil/blob/main/docs/2-engineering/specs/2026-10-11-yggdrasil-v0-spec.md]
style: reference
---

# realm-web architecture

The web realm: graph, prompts, devcontainer stack layer, templates and checks.

## Responsibilities

- Holds the web recipe (spec §9): `pack.yaml`, `graph.yaml`, prompts, the devcontainer stack
  layer, templates and the check ladder.
- Its templates are copied into products, so they are MIT-licensed (`templates/`).

## Depends on

- Gungnir's schemas.

## Source

The v0 engineering spec, §2.1, §9. When this page and the spec differ, the spec wins.
