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

## What is here

- `pack.yaml`: the leaf recipe (template, implementer prompt, the ladder by name), the check scripts, and
  the sandbox: two images built from `devcontainer/`, the leaf's stack (`web`) and the one for acceptance
  checks (`acceptance`, with Go).
- `graph.yaml`: the stages of a web product (spec §4.2). The planning stages run in scratch workspaces and
  Odin publishes their documents after the approval points; a design-conformance reviewer checks the spec
  against the design and the spec gate stops on a deviation; a reuse review reads the plan; the plan is read
  into tasks and the leaves are created from their realms' templates. Then, for every task: the implementer
  in a loop with the check ladder and three reviewers (spec compliance, simplicity, quality), the merge
  gate, the `pointer-contract` gate and the pointer move with the acceptance run.
- `prompts/`: planner, conformance, reuse-review, implementer and reviewer prompts. A planner prompt tells
  where its documents go; a test checks the examples in them against the schemas Odin reads.
- `devcontainer/Containerfile` and `devcontainer/checks/`: the stack and the ladder's scripts. `web-static`,
  `web-unit`, `web-tdd` (new tests fail on the commit the task started from), `web-contract`, `web-deps` (the
  dependency policy of `config/deps-policy.json`: pinned minor lines, minimum patched versions, then the
  audits the policy names), `web-security` and `web-e2e` (Playwright against the built app and a Prism mock of the contract) make the leaf's ladder.
  `accept-e2e` is the acceptance run: on a snapshot of the project it checks the API still keeps what the
  web app's copy of the contract promises (the copy may lag; the API may not break it), builds and starts
  the API against Postgres, and runs the Playwright specs.
- `devcontainer/checks/protected-common`: the files that configure the checks of every product, one git
  pathspec a line: the CI and dev container (`.github/`, `.devcontainer/`), `.pre-commit-config.yaml`, the
  markdown, YAML, shell and secret-scan configs, and the licensing files. `web-protected.sh`, the first step of
  every rung, adds the web names (`.npmrc`, `eslint.config.*`, `.eslintrc*`, `tsconfig*.json`, `vite.config.*`,
  `vitest.config.*`, `vitest.workspace.*`, `playwright.config.*`) and refuses a task's change to any of them:
  added, changed, deleted, moved, swapped for a symlink, untracked or ignored by `.gitignore`, against `main`.
  The `**/` entries skip `node_modules/`, where installed packages keep config files of their own; the
  root-anchored entries (`.github/`, `.devcontainer/`, `.reuse/`, `LICENSES/`) do not. With no `main`, a
  missing or empty list, or a failing `git`, the rung fails. The file is shared: the Go realm keeps a
  byte-for-byte copy, and `check-repos.sh --check shared` fails when they differ. The Containerfile bakes it
  with the folder into `/opt/ygg/checks`. The rungs still run the tool configs baked beside the scripts, not
  the leaf's.
- `templates/web/`: the leaf a new web app starts from: Vite, React, TypeScript, Tailwind, TanStack Router
  and Query, a typed client for the contract, a tested component and a Playwright spec.

## Routing

Odin keeps two indexes of a project (spec §8). The wiring block of `REPOS.md` says who consumes, requires or
pins what. `.ygg/impact.json` says what a change to a leaf, a contract or a package reaches. Odin builds both
from the umbrella, the packs, `go.mod`, `package.json`, the lockfiles, Go imports and OpenAPI files. It calls no
model. Every edge is `extracted` (read from a file) or `inferred` (a guess).

- The `move` node lists both files in `generated`. At each move Odin rebuilds them and logs
  `routing_index_built`. The shrink guard refuses an index that kept under half of its edges.
- The `contract-gate` node judges the move after it. A move that changes an OpenAPI file is judged by
  `extracted` edges of the shipped impact index, or by `oasdiff`. If the only evidence is an `inferred` edge,
  the verdict is `escalate` and a person decides in every mode. The ledger records `pointer_contract_checked`.
- A contract file is a file whose name ends in `openapi.yaml` or `openapi.yml`. The web template keeps its copy
  of the API contract in `contracts/api.openapi.yaml`. A byte-identical copy is an `extracted` edge. A copy
  with the same title and other bytes is `inferred`. Keep the copy current, or the gate escalates.
- The pack sets no `routing` block. A project that wants its indexes measured adds `.ygg/routing-bench.yaml` to
  its umbrella (questions with known answers, scored right first with no model) and a `routing:` block with a
  `threshold` to the pack. Odin then ships an index only at or above the threshold.

## Tests

`go test ./...` checks the pack, the graph and the prompts against Gungnir's schemas; runs the scripts on
small leaves with stand-ins for the image's tools (and python3 for the servers); and builds the template,
which needs the npm registry the first time (set `YGG_REQUIRE_TEMPLATES` to fail instead of skipping when
it cannot be reached). The image and the tools in it are checked on a real engine (Phase 5b, v0-H14).

## CI

`.github/workflows/ci.yml` has one job, `test`, the required check on `main`. It runs in the Node image (the
template tests need `npm`) with `git`, `python3` (the e2e test's servers) and `gcc` (for `-race`) installed by
`apt-get`, and Go from `actions/setup-go`, without `go.work`,
and runs `gofmt`, `go vet`, `go test -race -v` as the non-root user `node`, and `govulncheck`. It sets
`YGG_REQUIRE_TEMPLATES`, so the template tests fail instead of skipping when the npm registry cannot be
reached. The pr-bot cannot change the workflow; the founder pushes changes to it.

The web template carries its own CI (`templates/web/.github/workflows/ci.yml`), so a product's first commit
has the required check `test` too. It runs in the same Node image as the `web` stage of the Containerfile
(no `apt-get` step, no service, no Playwright) and runs `npm ci --ignore-scripts`, `lint`, `typecheck`,
`test` and `build`. `TestTemplateCI` keeps it in step with the Containerfile and `package.json`. Every rung
of the web ladder refuses a task's change under `.github/` (`web-protected.sh`), so only the product's people
change the workflow. Its lint and typecheck use the leaf's own tool configs, which the ladder does not read but
also does not let a task change. The `lint` script is `eslint --no-config-lookup -c eslint.config.js .`
(`TestTemplateCI` pins it): ESLint reads the one protected config at the top and looks for no other.

## Depends on

- Gungnir's schemas.
- Havamal's `ponytail-review` skill, for the reuse review and the simplicity reviewer.
- A lead realm's graph names the roles `planner`, `implementer` and `reviewer`: the operator's
  `factory.yaml` defines them.

## Source

The v0 engineering spec, §2.1, §9, and the change proposal realm leaf naming. When this page and the spec differ, the spec wins.
