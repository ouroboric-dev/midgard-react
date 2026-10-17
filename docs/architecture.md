---
title: midgard-react architecture
status: draft
date: 2026-10-12
related: [https://github.com/ouroboric-dev/yggdrasil/blob/main/docs/2-engineering/specs/2026-10-11-yggdrasil-v0-spec.md]
style: reference
---

# midgard-react architecture

The Midgard realm's React flavor: graph, prompts, devcontainer stack layer, templates and checks for
web frontends.

Midgard is the realm of web frontends, and this is its React flavor (realm leaf naming, Yggdrasil
change proposal `2026-10-17-realm-leaf-naming`).

## Responsibilities

- Holds the recipe of a React web leaf (spec §9, as changed by realm leaf naming): the `implementer`
  prompt, the React devcontainer stack layer, the web leaf template and its checks, under the role
  names every build realm shares.
- Is the client gallery's lead realm: it holds the graph (`graph.yaml`, spec §4.2) and the prompts not
  tied to one leaf (planner, initializer, reviewers). The api leaf's recipe is `helheim-go`'s.
- Its templates are copied into products, so they are MIT-licensed (`templates/`).

## Depends on

- Gungnir's schemas.

## Source

The v0 engineering spec, §2.1, §9, and the change proposal realm leaf naming. When this page and the spec differ, the spec wins.
