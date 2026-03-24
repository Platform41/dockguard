# Kanban Board — DockGuard

**Status:** Active
**Repository:** `dockguard`
**Last Updated:** 2026-03-24
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

- [ ] `[Phase 3] Implement settings file auto-detection for Docker Desktop variants`
  **Owner:** Three
  **Priority:** P1
  **Outcome:** Support overrideable and version-aware settings file lookup
  **Source docs:** `docs/project-lifecycle-v1.3.md`
  **Dependencies:** `[Phase 2] Scaffold Go CLI and config model`
  **Notes:** Must handle `settings-store.json` and older `settings.json` shapes defensively.

- [ ] `[Phase 3] Add free-space and writability checks`
  **Owner:** Three
  **Priority:** P1
  **Outcome:** Implement threshold and path health validation for external storage
  **Source docs:** `docs/project-lifecycle-v1.3.md`
  **Dependencies:** `[Phase 2] Scaffold Go CLI and config model`
  **Notes:** Keep check output deterministic and script-friendly.

- [ ] `[Phase 4] Implement guarded Docker Desktop startup`
  **Owner:** Three
  **Priority:** P1
  **Outcome:** `dockguard start` runs checks and starts Docker Desktop only when safe
  **Source docs:** `docs/project-lifecycle-v1.3.md`
  **Dependencies:** `[Phase 3] Build preflight check suite`
  **Notes:** Prefer `docker desktop start`; fail clearly if unsupported.

- [ ] `[Phase 5] Prepare v0.1.0 release checklist`
  **Owner:** One
  **Priority:** P2
  **Outcome:** Coherent release criteria for the first usable baseline
  **Source docs:** `docs/project-lifecycle-v1.3.md`, `docs/versioning-and-git-workflow.md`
  **Dependencies:** Core commands implemented and reviewed
  **Notes:** Include docs, examples, and command validation.

- [ ] `[Phase 5] Define post-MVP roadmap`
  **Owner:** One
  **Priority:** P3
  **Outcome:** Prioritised follow-up work after `v0.1.0`
  **Source docs:** `docs/project-lifecycle-v1.3.md`
  **Dependencies:** MVP shipped
  **Notes:** Candidate items: `doctor`, richer diagnostics, packaging.

### This Week

- [ ] `[Phase 1] Write README and example config for DockGuard`
  **Owner:** One
  **Priority:** P1
  **Outcome:** Clear project entrypoint plus `examples/dockguard.yaml`
  **Source docs:** `docs/project-lifecycle-v1.3.md`
  **Dependencies:** None
  **Notes:** README must keep the corrected Docker Desktop storage model near the top.

- [ ] `[Phase 2] Scaffold Go CLI and config model`
  **Owner:** Three
  **Priority:** P1
  **Outcome:** `go.mod`, `cmd/dockguard/main.go`, internal package layout, config loading path
  **Source docs:** `docs/project-lifecycle-v1.3.md`
  **Dependencies:** None
  **Notes:** Match the agreed repo shape and keep settings path overrideable.

- [ ] `[Phase 3] Build preflight check suite`
  **Owner:** Three
  **Priority:** P1
  **Outcome:** `dockguard check` and `dockguard status` with mount, path, and settings validation
  **Source docs:** `docs/project-lifecycle-v1.3.md`
  **Dependencies:** `[Phase 2] Scaffold Go CLI and config model`
  **Notes:** Cover mount existence, storage path existence, and readable output.

### Doing

- [ ] `[Phase 1] Align project docs to reviewed Go-based MVP`
  **Owner:** One
  **Priority:** P1
  **Started:** 2026-03-24
  **Branch:** `master`
  **Source docs:** `docs/project-lifecycle-v1.3.md`, reviewed proposal
  **Notes:** Lifecycle is updated; Kanban is being aligned and README is next.

### Review/Test

- [ ] `[Phase 0] Confirm lifecycle reflects reviewed proposal`
  **Owner:** One
  **Priority:** P1
  **Review Type:** Internal
  **Branch:** `master`
  **Commit:** `b0bbeb5`
  **Test command:** `N/A`
  **Test result:** Pending
  **Source docs:** `docs/project-lifecycle-v1.3.md`
  **Notes:** Validate that scope, compatibility notes, and delivery phases match the approved direction.

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

- [x] `[Phase 1] Update project lifecycle for DockGuard`
  **Owner:** One
  **Priority:** P1
  **Completed:** 2026-03-24
  **Branch:** `master`
  **Commit:** `b0bbeb5`
  **Deliverable:** Lifecycle document
  **Notes:** Replaced the generic studio template with a DockGuard-specific lifecycle and MVP path.

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
| Phase 0 | One | Product validation baseline | `docs/project-lifecycle-v1.3.md` |
| Phase 1 | One | Docs foundation and positioning | `docs/project-lifecycle-v1.3.md`, `README.md` |
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
