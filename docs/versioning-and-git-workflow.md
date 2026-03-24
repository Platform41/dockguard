# 41 Studio — Versioning and Git Workflow

**Owner:** One (Systems Architect)
**Status:** Active
**Last Updated:** [YYYY-MM-DD]

---

## 1. Purpose

This document defines how a project manages:

- day-to-day git workflow
- release versioning
- release tagging
- basic commit discipline

The goal is to keep the repository predictable as the product moves from active build phase toward stable production operations.

## 2. Workflow Choice

Use a small trunk-based workflow.

This means:

- one primary branch is treated as the stable integration branch
- work happens in short-lived branches
- branches are merged back quickly
- release tags are created from the stable branch

## 3. Primary Branch

Default stable branch:

- `main`

Policy:

- until explicitly changed, `main` is the stable branch
- if the team later renames the stable branch, the workflow stays the same
- `main` is integration-only; persona work must not be committed directly to it

## 4. Branch Rules

### Use Short-Lived Branches

Create a branch for meaningful work:

- `feat/[feature-name]`
- `fix/[bug-name]`
- `docs/[doc-name]`
- `chore/[maintenance-name]`

Branch prefixes:

- `feat/` for product features
- `fix/` for bugs or regressions
- `docs/` for documentation work
- `chore/` for maintenance or workflow changes

This is mandatory for persona work in this repo:

- `Zero`, `One`, `Two`, and `Three` must start from a short-lived branch
- direct-to-`main` commits are not allowed for normal task work, including docs-only work
- work is merged back to `main` only after the task is coherent

### Branch Lifetime

- keep branches short-lived
- merge when the work is coherent and reviewed
- avoid long-running branches that drift from the stable branch

## 5. Local Multi-Agent Workflow

When multiple agents work on the same machine, they must not share one active working tree for edits.

### Core Rule

- one agent = one worktree
- one worktree = one active branch

This avoids collisions around:

- `HEAD`
- the git index
- the local working tree
- shared uncommitted changes

### Recommended Local Layout

Keep the original checkout as the stable integration checkout on `main`.

Create separate worktrees for each active agent task.

Example:

```bash
cd /path/to/project
git worktree add ../project-claude feat/claude-task
git worktree add ../project-chatgpt feat/chatgpt-task
```

Suggested mapping:

- Claude terminal works only in `../project-claude`
- ChatGPT terminal works only in `../project-chatgpt`
- original repo stays on `main`

### Operational Rules

1. do not let two agents edit the same checkout
2. do not switch branches inside an active agent worktree mid-task
3. keep the original checkout as the stable branch checkout
4. claim the task in `docs/KANBAN.md` before editing
5. avoid simultaneous edits to `docs/KANBAN.md`
6. commit approved or completed work promptly in the agent's own branch
7. merge back to `main` only after the task is coherent
8. only One creates release tags, and only from `main`

### Tool Ownership

Projects may define a tool split in `docs/versioning-and-git-workflow.md`.

If they do:

- each tool must stay within its assigned role
- implementation must not be written from the wrong tool context
- handoff prompts should explicitly name which tool owns the next task

If no project-specific split exists, use normal persona ownership and the worktree rules above.

### Stable Branch Rule

Treat `main` as:

- integration branch
- review merge target
- release tagging branch

Do not use `main` as:

- an active persona work branch
- a scratch branch
- a docs-only shortcut branch

### Safety Check Before Starting Work

Before an agent starts a task, verify:

```bash
pwd
git branch --show-current
git status --short
```

If the current checkout is the stable branch checkout, stop and create or move to a task worktree before continuing.

## 6. Release Versioning

Use Semantic Versioning while pre-launch work remains in the `0.x.y` range.

Format:

- `MAJOR.MINOR.PATCH`

Examples:

- `0.1.0`
- `0.2.0`
- `0.2.1`
- `1.0.0`

## 7. Pre-1.0 Rules

Until the product is public and stable, version numbers should be interpreted this way:

- `0.MINOR.0` = meaningful product milestone
- `0.MINOR.PATCH` = bugfix or polish release on top of that milestone

Practical rule:

- new coherent feature set: bump `MINOR`
- bugfix-only changes: bump `PATCH`
- docs-only changes: no release bump unless they are part of a tagged milestone

Do not overuse `MAJOR` before launch.

## 8. Release Tagging

Releases are represented by git tags on the stable branch.

Tag format:

- `v0.1.0`
- `v0.2.0`
- `v0.2.1`

Policy:

- tag only from the stable branch
- tag only when the milestone is coherent enough to be referenced later
- do not tag every small commit

### Tagging Ownership

Tagging is owned by One.

This means:

- `Three` may complete implementation work
- `Two` and `Zero` may confirm design or product readiness
- but only `One` should decide the release boundary and create the version tag

Practical process:

1. target work is merged to `main`
2. relevant docs and `docs/KANBAN.md` are updated
3. release readiness is reviewed
4. user approves the release decision if needed
5. One creates the git tag on the stable branch

## 9. Planned Milestone Shape

Initial working model:

- `v0.1.0` for the first coherent baseline
- `v0.2.0` for the next meaningful capability milestone
- `v0.3.0` for the next planned expansion
- `v1.0.0` for the public launch baseline

These are planning anchors, not rigid promises.

## 10. Commit Discipline

Commits should be:

- small enough to understand
- scoped to one coherent change
- written so later release notes are easy to reconstruct

Recommended style:

- `feat: add [feature summary]`
- `fix: resolve [bug summary]`
- `docs: define versioning and release policy`
- `chore: align workflow templates`

For persona-driven work in this repo:

- create a short-lived branch first
- record the work in `docs/KANBAN.md`
- update status when the task is approved or completed
- commit approved or completed work promptly on that branch
- merge to `main` only after the branch is coherent

### Test-First Rule For Three

`Three` should start implementation work from tests before coding whenever the task changes behavior, fixes a bug, or closes a reviewed finding.

### Spec Gate For Three

`Three` should start only after:

- Phase 2 architecture is approved
- Phase 3 is coherent enough for implementation handoff

Phase 3 does not need to be perfect, but the core user flow, main screens, and interaction model must already be defined. If those are still ambiguous, stop and request clarification before coding.

Default order for `Three`:

1. write the failing test first
2. confirm the current behavior fails as expected
3. implement the minimum code change
4. rerun the targeted tests
5. update `docs/KANBAN.md`
6. commit approved or completed work promptly

This is especially required for:

- bug fixes
- regression-prone flows
- reviewed findings from One
- behavior changes in forms, controllers, calculations, and dashboard logic

If a task is pure documentation or structural cleanup with no behavior change, this rule can be applied pragmatically.

## 11. What We Are Not Using

This workflow is not using:

- Git Flow
- long-lived release branches
- permanent develop branch
- complicated merge choreography

Reason:

- the repo is still early-stage
- the team footprint is small
- simpler process creates better discipline than heavyweight process

## 12. Minimum Release Checklist

Before tagging a release milestone:

- relevant work is reflected in `docs/KANBAN.md`
- related design or architecture docs are updated
- the stable branch is clean
- tests for the released scope have been run where applicable
- the release can be described in 2-5 short points

## 13. Current Policy Decision

Approved baseline workflow:

1. use trunk-based development with `main` as the stable branch for now
2. use short-lived topic branches for meaningful work
3. use separate worktrees when multiple agents work locally in parallel
4. use SemVer tags and stay in `0.x.y` until public launch
5. tag milestones, not every commit
6. release tags are created by One, not by every persona
7. Three starts from failing tests before coding for behavior-changing work
8. keep Kanban status and commits aligned with completed or approved work
