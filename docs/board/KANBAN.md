# Kanban Board — DockGuard

**Status:** Active
**Repository:** `dockguard`
**Last Updated:** 2026-11-08
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

- [ ] `[v0.2.0] Target layer: yaml.v3 config, Target interface, Docker Desktop on it`
  **Owner:** Three
  **Priority:** P1
  **Outcome:** Docker behaves as in v0.1.2 (AS-01, AS-08); legacy config still loads (AS-02, AS-03, AS-04)
  **Source docs:** `docs/specs/multi-target.md` §3, §4.1, §6
  **Dependencies:** Spec approved

- [ ] `[v0.2.0] list command, status across targets, target argument on check/start/stop`
  **Owner:** Three
  **Priority:** P1
  **Outcome:** AS-05, AS-06, AS-07, AS-09, AS-21, AS-22
  **Source docs:** `docs/specs/multi-target.md` §2, §3 (config lookup, D-015)
  **Dependencies:** Target layer

- [ ] `[v0.2.0] Eject refuses while another target on the drive is running`
  **Owner:** Three
  **Priority:** P1
  **Outcome:** AS-10
  **Source docs:** `docs/specs/multi-target.md` §2
  **Dependencies:** Target layer

- [ ] `[v0.3.0] app kind (Xcode) from the June Xcode work`
  **Owner:** Three
  **Priority:** P2
  **Outcome:** AS-11 to AS-14
  **Source docs:** `docs/specs/multi-target.md` §4.2, §11
  **Dependencies:** v0.2.0; move the uncommitted Xcode work off the `master` checkout first

- [ ] `[v0.3.0] postgres kind (pg_ctl clusters)`
  **Owner:** Three
  **Priority:** P2
  **Outcome:** AS-15 to AS-17, AS-23
  **Source docs:** `docs/specs/multi-target.md` §4.3
  **Dependencies:** v0.2.0; proposal P-3 approved

- [ ] `[v0.4.0] container kind (Fizzy) and stop --all --eject`
  **Owner:** Three
  **Priority:** P3
  **Outcome:** AS-18 to AS-20
  **Source docs:** `docs/specs/multi-target.md` §4.4
  **Dependencies:** v0.3.0. Includes removing Fizzy's launchd agent (D-014, AS-24)

### This Week

### Doing

_None._

### Review/Test

- [ ] `[Spec] Multi-target DockGuard (v0.2.0 to v0.4.0)`
  **Owner:** One
  **Priority:** P1
  **Branch:** `docs/multi-target-spec`
  **Deliverable:** `docs/specs/multi-target.md`
  **Waiting on:** Azrad's review of the spec and proposal P-3 (`postgres` kind replaces `brew-service`)

### Blocked

- [ ] `[Phase 4] Validate Docker Desktop CLI behavior on target machine`
  **Owner:** Three
  **Priority:** P2
  **Blocked On:** 2026-03-24
  **Blocker:** Requires a real macOS Docker Desktop environment with externalized storage configured
  **Next Action:** Run live validation once the Go CLI skeleton exists and a target setup is available.

### Done

- [x] `[Phase 5] Prepare v0.1.0 release checklist`
  **Owner:** One
  **Priority:** P2
  **Completed:** 2026-04-27
  **Deliverable:** `v0.1.0`, `v0.1.1` and `v0.1.2` tagged; notes in `docs/releases/` and `CHANGELOG.md`
  **Notes:** Closed on 2026-11-08 from the release record; the board wasn't updated at release time.

- [x] `[Phase 5] Define post-MVP roadmap`
  **Owner:** One
  **Priority:** P3
  **Completed:** 2026-11-08
  **Branch:** `docs/multi-target-spec`
  **Deliverable:** `docs/specs/multi-target.md` (v0.2.0 to v0.4.0)
  **Notes:** Replaces the earlier candidates (`doctor`, richer diagnostics, packaging) with multi-target support.

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

| ID | Date | Decision | Owner | Notes |
| --- | --- | --- | --- | --- |
| D-001 | 2026-03-24 | Go is the official implementation language for DockGuard | One | Replaces any implicit script-first direction |
| D-002 | 2026-03-24 | v1 remains macOS-only CLI with `check`, `start`, and `status` | One | GUI, runtime protection, and non-macOS support deferred |
| D-003 | 2026-03-24 | Docker Desktop settings path must be configurable and version-aware | One | Do not assume one fixed settings file path |
| D-004 | 2026-03-24 | Direct commits were used for initial repo setup and doc baseline | One | Commits `49724b5` and `b0bbeb5` on `master` |
| D-005 | 2026-11-08 | Scope extends beyond Docker Desktop to any app whose data lives on the external drive: GUI apps (Xcode), Homebrew services (PostgreSQL), Apple `container` stacks (Fizzy) | Azrad | Amends D-002: still macOS-only and CLI-only, but no longer Docker-only or limited to three commands. Approved in a planning session, 2026-11-08 |
| D-006 | 2026-11-08 | Command format is `dockguard <action> [target]`; no target means `docker` for `check`, `start` and `stop` | Azrad | Not `dockguard <appname>`. Keeps v0.1 commands working. Planning session, 2026-11-08 |
| D-007 | 2026-11-08 | Targets are config entries of a fixed set of kinds (`docker-desktop`, `app`, `brew-service`, `container`); a new app needs config, only a new kind needs code | Azrad | Planning session, 2026-11-08 |
| D-008 | 2026-11-08 | Add `dockguard list` to show the configured targets and whether each is running | Azrad | Requested by Azrad. Planning session, 2026-11-08 |
| D-009 | 2026-11-08 | Keep the name DockGuard for now | Azrad | "Jaga" was the leading rename candidate; revisit before any public promotion |
| D-010 | 2026-11-08 | Build for macOS only; keep OS commands in `internal/platform` so other systems can be added later | Azrad | Cross-OS is designed for, not built. Planning session, 2026-11-08 |
| D-011 | 2026-11-08 | Parse config with `gopkg.in/yaml.v3` | Azrad | First external dependency; the hand-written parser can't handle nested target sections. Planning session, 2026-11-08 |
| D-012 | 2026-11-08 | Defaults, examples and docs use a generic drive path (`/Volumes/ExternalSSD`), never a personal drive name or project | Azrad | Was proposal P-1. The repo is public |
| D-013 | 2026-11-08 | DockGuard rejects `~/Library/Developer/CoreSimulator/Devices` as an external path | Azrad | Was proposal P-2. Moving it to an external drive breaks `simctl create` with no fix |
| D-014 | 2026-11-08 | Remove Fizzy's start-at-login launchd agent; Fizzy starts through `dockguard start fizzy` | Azrad | Fizzy no longer starts while the drive is missing. Done with v0.4.0 |
| D-015 | 2026-11-08 | The config file is the record of which apps DockGuard manages; also look it up at `~/.config/dockguard/dockguard.yaml`, and `list`/`status` print the file used | Azrad | Lands in v0.2.0 |

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
