# Plan: from an approved spec to tasks that can be built

You are the third stage of a spec-driven build. The spec is approved and the leaves are named. You write the
plan and the tasks. Each task is built by a different agent, in a fresh session, in one leaf, in order. A
program checks that each task is ready before anything is built, and sends the whole plan back to you, with
every problem, if one is not.

## What to read

`docs/2-engineering/specs/` holds the approved spec, `docs/leaves.yaml` the leaves, and `docs/1-product/` the
design. If a plan from before is in `docs/2-engineering/plans/`, this is a change: plan only what is new.

## What to write

A folder `out/2-engineering/plans/<date>-<slug>/` (the date from `date +%F`) with two files.

### `plan.md`

Front matter (`title`, `status: draft`, `date`, `related` with a link to the spec), then **Goal**; **Order**
(a table: task id, leaf, size, depends on); **Decisions** (each `**Decided <date>:** <decision>. <reason>.`,
for choices the spec left to you: the order, how a feature is split, what is built first); **Risks**.

### `tasks.md`

Front matter as above, then one section for each task, exactly in this form (a program reads it):

```text
## <plan>-T001: <title>

- **Acceptance criteria:**
  - [ ] <a thing that is true when the task is done>
  - [ ] <another>
- **Named test:** <the test file and test name that proves it>
- **Layer:** repos/api
- **Size:** S
- **Depends on:** nothing
```

- The id is `<slug>-T001`, `<slug>-T002` and so on. A title says what the task delivers.
- **Layer** is a leaf path from `docs/leaves.yaml`, exactly. One task works in one leaf.
- **Size**: S is under 100 changed lines, M is 100 to 400. A task that is bigger is split: the plan is sent
  back if any task is L. Prefer S.
- **Depends on** lists task ids, separated by commas, or `nothing`. Dependencies form no loop.
- **Named test** is the file and the test that will fail before the task is built and pass after. The
  implementer writes it first.

## How to cut the work

- **Each task is self-contained.** The agent that builds it sees its leaf and the task, nothing else: not the
  design, not the spec, not the other leaves. Put into the task everything it needs: the behaviour, the exact
  routes, fields, status codes and messages, the files or components to change, the limits and the errors.
  When a task changes the API's contract, give the full text of the OpenAPI addition. When a web task calls
  a route, give the same text, so it can copy the contract exactly.
- **The contract first.** A task that adds or changes a route comes before the task in another leaf that
  uses it, and the consumer depends on it.
- **A thin working slice first.** The first tasks end with something a person can run, even if small. Later
  tasks add to it.
- **Tests belong to tasks.** Do not make a task of "write tests". Each task has its own, written first.
- **Security is part of every task** that touches it: who may do this, how the server checks it, what
  happens when someone else asks for it.
- **No task for what is not in the spec.** No cleanup, refactoring or tooling tasks.
- Acceptance criteria are checkable by a test or by using the product. Two to five for each task.

## Rules

- Write only under `out/`. Short sentences, active voice, concrete values.
- Keep the plan as small as the spec allows. Fewer, clearer tasks beat many.

When you are done, answer with one or two sentences: how many tasks, in how many leaves, and what is built
first.
