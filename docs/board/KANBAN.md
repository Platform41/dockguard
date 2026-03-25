# Kanban Board — DockGuard

**Status:** Active
**Repository:** `dockguard`
**Last Updated:** 2026-03-25
**Board Owner:** One

---

## Board Rules

- **Single source of execution:** This file is the active task board for DockGuard.
- **Card format:** `[Phase X] Title`
- **Owner format:** `One / Three / PM`
- **Priority format:** `P1 / P2 / P3`
- **WIP limit:** Max 3 cards in `Doing`.
- **Definition of done:** A card is done only when the output exists in the repo and the handoff is clear.
- **Test-first rule:** For behavior changes and command logic, write or update tests before or alongside implementation.
- **Release rule:** Tag only after One reviews the release candidate.
- **Versioning rule:** Use `0.x.y` until the first public production release.

---

## Column Definitions

- **Backlog:** Valid work, not yet prioritised for the current slice.
- **This Week:** Next up for execution.
- **Doing:** Actively in progress.
- **Review/Test:** Implemented and awaiting review or validation.
- **Blocked:** Waiting on a dependency, decision, or environment constraint.
- **Done:** Completed, documented, and committed.

---

## Priority Legend

- **P1:** Required for the Go-based MVP
- **P2:** Important but not blocking `v0.1.0`
- **P3:** Post-MVP or exploratory work

---

## Kanban Board

### Backlog

- [ ] `[Phase 5] Prepare v0.1.0 release checklist`
  **Owner:** One
  **Priority:** P2
  **Outcome:** Coherent release criteria for the first usable baseline
  **Source docs:** `docs/project-lifecycle.md`, `docs/versioning-and-git-workflow.md`
  **Dependencies:** Core commands implemented and reviewed
  **Notes:** Include docs, examples, and command validation.

- [ ] `[Phase 5] Define post-MVP roadmap`
  **Owner:** One
  **Priority:** P3
  **Outcome:** Prioritised follow-up work after `v0.1.0`
  **Source docs:** `docs/project-lifecycle.md`
  **Dependencies:** MVP shipped
  **Notes:** Candidate items: `doctor`, richer diagnostics, packaging.

### This Week

### Doing

_None._

### Review/Test

_None._

### Blocked

- [ ] `[Phase 4] Validate Docker Desktop CLI behavior on target machine`
  **Owner:** Three
  **Priority:** P2
  **Blocked On:** 2026-03-24
  **Blocker:** Requires a real macOS Docker Desktop environment with externalized storage configured
  **Next Action:** Run live validation once the Go CLI skeleton exists and a target setup is available.

### Done

- [x] `[Phase 0] Review and tighten product proposal`
  **Owner:** One
  **Priority:** P1
  **Completed:** 2026-03-24
  **Branch:** `master`
  **Commit:** `49724b5`
  **Deliverable:** Initial repo baseline and proposal-aligned direction
  **Notes:** Proposal corrected the Docker Desktop storage model, narrowed scope, and confirmed Go.

- [x] `[Phase 0] Confirm lifecycle reflects reviewed proposal`
  **Owner:** One
  **Priority:** P1
  **Completed:** 2026-03-25
  **Branch:** `docs/kanban-phase1`
  **Commit:** `6f3b6a2`
  **Deliverable:** Lifecycle review closure
  **Notes:** Scope, compatibility notes, and delivery phases confirmed.

- [x] `[Phase 1] Update project lifecycle for DockGuard`
  **Owner:** One
  **Priority:** P1
  **Completed:** 2026-03-24
  **Branch:** `master`
  **Commit:** `b0bbeb5`
  **Deliverable:** Lifecycle document
  **Notes:** Replaced the generic studio template with a DockGuard-specific lifecycle and MVP path.

- [x] `[Phase 1] Align project docs to reviewed Go-based MVP`
  **Owner:** One
  **Priority:** P1
  **Completed:** 2026-03-25
  **Branch:** `docs/kanban-phase1`
  **Commit:** `b8a9c9e`
  **Deliverable:** Kanban and docs alignment
  **Notes:** Phase 1 docs closed out.

- [x] `[Phase 1] Write README and example config for DockGuard`
  **Owner:** One
  **Priority:** P1
  **Completed:** 2026-03-25
  **Branch:** `docs/kanban-phase1`
  **Commit:** `d5b87a1`
  **Deliverable:** README + `examples/dockguard.yaml`
  **Notes:** Corrected Docker Desktop storage model and config usage reflected.

- [x] `[Phase 2] Scaffold Go CLI and config model`
  **Owner:** Three
  **Priority:** P1
  **Completed:** 2026-03-24
  **Branch:** `master`
  **Commit:** `9e4b0f7`
  **Deliverable:** Go module, CLI entrypoint, config loader
  **Notes:** Config path supports overrides and settings detection.

- [x] `[Phase 3] Build preflight check suite`
  **Owner:** Three
  **Priority:** P1
  **Completed:** 2026-03-24
  **Branch:** `master`
  **Commit:** `9e4b0f7`
  **Deliverable:** `dockguard check` and `dockguard status`
  **Notes:** Includes mount/path checks, free space, and settings validation.

- [x] `[Phase 3] Implement settings file auto-detection for Docker Desktop variants`
  **Owner:** Three
  **Priority:** P1
  **Completed:** 2026-03-24
  **Branch:** `master`
  **Commit:** `9e4b0f7`
  **Deliverable:** Version-aware settings path detection
  **Notes:** Supports `settings-store.json` and `settings.json` candidates.

- [x] `[Phase 3] Add free-space and writability checks`
  **Owner:** Three
  **Priority:** P1
  **Completed:** 2026-03-24
  **Branch:** `master`
  **Commit:** `9e4b0f7`
  **Deliverable:** Free-space + writable path checks
  **Notes:** Deterministic output for scripting.

- [x] `[Phase 3] Tighten Docker Desktop compatibility handling + expand tests`
  **Owner:** Three
  **Priority:** P1
  **Completed:** 2026-03-25
  **Branch:** `feat/phase2-scaffold`
  **Commit:** `f3e698f`
  **Test command:** `go test ./...`
  **Test result:** Passed
  **Deliverable:** Compatibility detection and settings key coverage
  **Notes:** Switched running guard to `docker desktop status`, added CLI unsupported messaging, expanded settings key detection tests.

- [x] `[Phase 4] Implement guarded Docker Desktop startup`
  **Owner:** Three
  **Priority:** P1
  **Completed:** 2026-03-24
  **Branch:** `master`
  **Commit:** `611370e`
  **Deliverable:** `dockguard start` with guarded startup
  **Notes:** Fails clearly when CLI support is missing.

---

## Decision Log

| Date | Decision | Owner | Notes |
| --- | --- | --- | --- |
| 2026-03-24 | Go is the official implementation language for DockGuard | One | Replaces any implicit script-first direction |
| 2026-03-24 | v1 remains macOS-only CLI with `check`, `start`, and `status` | One | GUI, runtime protection, and non-macOS support deferred |
| 2026-03-24 | Docker Desktop settings path must be configurable and version-aware | One | Do not assume one fixed settings file path |
| 2026-03-24 | Direct commits were used for initial repo setup and doc baseline | One | Commits `49724b5` and `b0bbeb5` on `master` |

---

## Lifecycle Mapping

| Phase | Owner | Typical Deliverable | Target Doc |
| --- | --- | --- | --- |
| Phase 0 | One | Product validation baseline | `docs/project-lifecycle.md` |
| Phase 1 | One | Docs foundation and positioning | `docs/project-lifecycle.md`, `README.md` |
| Phase 2 | Three | Go CLI skeleton and config model | Code |
| Phase 3 | Three | Preflight checks and status flow | Code + tests |
| Phase 4 | Three | Guarded Docker Desktop startup | Code + tests |
| Phase 5 | One | Release review and tagging baseline | Docs + git tag |

---

## Weekly Cadence

- **Start of week:** Confirm the next MVP-critical cards and clear blockers.
- **Midweek:** Update card notes with concrete output or dependency changes.
- **End of week:** Move completed work to `Done`, prepare review items, and tighten the next slice.

---

## Change Log

| Date | Change |
| --- | --- |
| 2026-03-24 | Replaced generic board template with DockGuard Go-based MVP board |
