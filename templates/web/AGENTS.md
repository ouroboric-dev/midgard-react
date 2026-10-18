# AGENTS.md: web

The React web app of this product. It talks to the API only through `contracts/api.openapi.yaml`.

Follow the Yggdrasil invariants:
<https://github.com/ouroboric-dev/yggdrasil/blob/main/AGENTS.md#invariants>
This file only adds rules for this leaf. Read `docs/` for the design; this file says how to work here.

## Work here

- `npm ci` first (`./init.sh`). Then `npm test` (unit tests), `npm run lint`, `npm run typecheck`,
  `npm run build`, and `npm run test:e2e` (Playwright, against the built app and an API behind it).
- Stack: React, Vite, TypeScript (strict), Tailwind, shadcn/ui style components in `src/components/ui`,
  TanStack Router for pages and TanStack Query for data. Uploads use Uppy with presigned URLs.
- Pin every dependency to an exact version (`x.y.z`; `~x.y.z` also passes the factory) and commit `package-lock.json`. A new dependency needs a reason
  the standard library, the browser and what is here cannot give.

## Rules for every change

- **Test first.** A new test fails on the parent commit, for the reason the task gives. The factory
  checks this: a change to code without a new or changed test is refused. Unit tests are `*.test.ts(x)`
  beside the code, with Vitest and Testing Library. One Playwright spec in `e2e/` per acceptance criterion
  that a person does in the browser.
- **The contract is a copy.** `contracts/api.openapi.yaml` is the API leaf's contract as this app uses it.
  The task gives you the contract text when the API changes: copy it exactly. After a change, run
  `npm run gen:api`; `src/api/schema.d.ts` is generated and committed, never edited by hand. The
  copy may lag behind the API; the acceptance run fails if the API takes away or changes what the copy
  promises.
- **Call the API through `src/lib/api.ts`.** The client is typed by the contract. Never write `fetch`
  against a URL by hand, and never put a server address in the code: the browser calls `/api`.
- **Photo bytes never pass through the API.** Uploads and downloads use presigned URLs the API hands out.
  Thumbnails are made in the browser. Large downloads stream to disk.
- Handle every state a user can see: loading, empty, error, and the result. Forms validate before they
  send, and the server's answer wins.
- No secret, token or real person's data in the repository, in a test or in a log line.
- Accessible by default: real buttons and links, labels on inputs, text alternatives.

## Layout

- `src/main.tsx`, `src/router.tsx`: the app and its routes. A page is a component in `src/routes`.
- `src/components`: shared components; `src/components/ui` are the base ones (shadcn/ui style).
- `src/lib`: `api.ts` (the typed client) and helpers.
- `src/api/schema.d.ts`: generated from the contract.
- `contracts/`: the API contract. `e2e/`: Playwright specs.
