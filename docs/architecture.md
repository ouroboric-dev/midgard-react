---
title: midgard-react architecture
status: draft
date: 2026-10-12
related: [https://github.com/ouroboric-dev/yggdrasil/blob/main/docs/2-engineering/specs/2026-10-11-yggdrasil-v0-spec.md]
style: reference
---

# midgard-react architecture

The Midgard realm's React flavor: implementer prompt, devcontainer stack layer, templates and checks for
web frontends.

Midgard is the realm of web frontends, and this is its React flavor (realm leaf naming, Yggdrasil
change proposal `2026-10-17-realm-leaf-naming`).

## Responsibilities

- Holds the recipe of a React web leaf (spec §9, as changed by realm leaf naming): the `implementer`
  prompt, the React devcontainer stack layer, the web leaf template and its checks, under the role
  names every build realm shares.
- Holds no graph and no stage prompt. Voluspa's `software` graph holds the stages (spec §4.2) and the
  prompts of the planners and the reviewers; this realm is a flavor that a project's leaf pins. The api
  leaf's recipe is `helheim-go`'s.
- Its templates are copied into products, so they are MIT-licensed (`templates/`).

## What is here

- `pack.yaml`: the leaf recipe (template, implementer prompt, the ladder by name), the check scripts, and
  the sandbox: two images built from `devcontainer/`, the leaf's stack (`web`) and the one for acceptance
  checks (`acceptance`, with Go).
- `prompts/implementer.md`: the implementer prompt, the only prompt here. The graph's other prompts are
  Voluspa's.
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
  `main` is the branch: a tag or another ref called `main` does not count. The `**/` entries skip
  `node_modules/`, where installed packages keep config files of their own, in the listing of untracked files
  only: a tracked file there is a change. The root-anchored entries (`.github/`, `.devcontainer/`, `.reuse/`,
  `LICENSES/`) skip nothing, and go to `git` without their trailing slash, so a folder swapped for a symlink is
  caught too; the root `.npmrc` is named plain as well, so a folder of that name is caught. With no `main`, a
  missing or empty list, or a failing `git`, the rung fails. The file is shared: the Go realm keeps a
  byte-for-byte copy, and `check-repos.sh --check shared` fails when they differ. The Containerfile bakes it
  with the folder into `/opt/ygg/checks`. The rungs still run the tool configs baked beside the scripts, not
  the leaf's.
- `templates/web/`: the leaf a new web app starts from: Vite, React, TypeScript, Tailwind, TanStack Router
  and Query, a typed client for the contract, a tested component and a Playwright spec. It also holds the
  leaf run contract's `run.sh` (change proposal `2026-10-21-leaf-run-contract`): with `PORT` set it builds the
  bundle with `npm run build` and serves it with `serve.mjs` (Node's standard library) on `$HOST:$PORT` (`HOST`
  defaults to `127.0.0.1`; the product start sets `0.0.0.0` and port 3000). `serve.mjs` answers the bundle with an
  `index.html` fallback, forwards `/api/` to `API_URL` with `X-Forwarded-Host` and `X-Forwarded-Proto`, and forwards
  the bucket's path (`/$S3_BUCKET/`, `ygg-store`) to `S3_ENDPOINT` unchanged and with no `X-Forwarded-*` header
  (delta §9.3; `TestServeForwards`). `vite.config.ts` keeps the dev server's own proxy. `run.sh` installs nothing
  (`init.sh` does), exits non-zero with a message when `PORT` is unset or `node_modules` is missing, and is not a
  protected file. `TestRunScriptStarts` starts it from a copy of the template and waits for the port.

## Routing

Voluspa's graph ships the routing indexes (spec §8). A contract file is a file whose name ends in `openapi.yaml` or `openapi.yml`. The web template keeps
its copy of the API contract in `contracts/api.openapi.yaml`, and a test checks the name. A byte-identical copy
of the API's contract is an `extracted` edge. A copy with the same title and other bytes is `inferred`. Keep the
copy current, or the gate escalates. The pack sets no `routing` block.

## Tests

`go test ./...` checks the pack against Gungnir's schemas and the implementer prompt; runs the scripts on
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
- Voluspa's `software` graph, which a run names with `--graph`. The graph's `implement` node takes this
  realm's `implementer` prompt, template and checks from the leaf that pins it.

## Source

The v0 engineering spec, §2.1, §9, and the change proposal realm leaf naming. When this page and the spec differ, the spec wins.
