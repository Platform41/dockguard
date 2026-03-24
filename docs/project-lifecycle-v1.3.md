# DockGuard — Project Lifecycle (v1.3)

**Status:** Active
**Repository:** `dockguard`
**Last Updated:** 2026-03-24
**Primary Platform:** macOS
**Implementation Language:** Go
**Product Type:** CLI

---

## 1. Project Summary

DockGuard is a macOS-first Go CLI that performs preflight checks before starting Docker Desktop when Docker Desktop storage is hosted on an external SSD.

The product goal for v1 is narrow and operational:

- reduce bad startup states before Docker Desktop launches
- verify that the expected external storage is present and healthy
- validate that Docker Desktop is configured to use the expected external storage path
- start Docker Desktop only when checks pass

DockGuard is not a Docker replacement, migration tool, or runtime recovery system.

---

## 2. Correct Mental Model

This model should remain consistent across docs, implementation, and release notes:

- MacBook provides CPU, RAM, macOS, and the Docker Desktop app
- Docker Desktop runs a Linux VM or engine layer on the Mac
- External SSD stores the Docker Desktop disk image, which is where Docker Desktop stores Linux containers and images on macOS

Implication:

- DockGuard is not an "images-only offload" tool
- DockGuard protects a setup where Docker Desktop storage is externalized to an SSD, while compute still runs on the MacBook

---

## 3. Product Positioning

Canonical one-line positioning:

> DockGuard is a preflight safety launcher for Docker Desktop on macOS when Docker storage is hosted on an external SSD.

Working product promise:

- run checks before startup
- fail loudly when the environment is unsafe
- keep startup behavior explicit and deterministic

---

## 4. Scope Definition

### In Scope for v1

- macOS-only CLI
- preflight validation before Docker Desktop startup
- external volume presence checks
- Docker Desktop storage path existence checks
- writability checks
- free-space threshold checks
- Docker Desktop settings validation against the expected path
- controlled startup through `docker desktop start`
- clear terminal output and non-zero exit codes on failure

### Out of Scope for v1

- replacing Docker Desktop
- managing Docker internals directly
- automatic Docker data migration
- protection against SSD disconnects after Docker Desktop is already running
- Windows support
- Linux support
- GUI or menu bar application
- background daemon behavior

---

## 5. Compatibility and Assumptions

DockGuard depends on supported Docker Desktop behavior rather than private patching.

Current implementation assumptions:

- Docker Desktop disk image location is configurable by the user
- `docker desktop start` is the preferred startup path when supported
- Docker Desktop CLI support is version-dependent and requires Docker Desktop 4.37 or later for the documented Desktop CLI flow

Settings file compatibility must be treated defensively.

DockGuard should support:

- overrideable settings file path
- version-aware detection of Docker Desktop settings files
- newer `settings-store.json` layouts
- older `settings.json` layouts when needed

Do not hardcode one settings path as the only supported location.

---

## 6. Architecture Direction

Go is the official implementation language for DockGuard.

Why Go fits this product:

- strong support for filesystem inspection
- clean subprocess execution for `diskutil`, `df`, and `docker`
- straightforward JSON and YAML parsing
- reliable exit code handling for CLI use
- single-binary distribution for macOS users

This is a product-quality distributable CLI, not a personal shell wrapper.

Recommended repo shape:

```text
dockguard/
├── cmd/
│   └── dockguard/
│       └── main.go
├── internal/
│   ├── checks/
│   ├── config/
│   ├── dockerdesktop/
│   ├── platform/
│   └── output/
├── examples/
│   └── dockguard.yaml
├── README.md
├── CONTRIBUTING.md
├── LICENSE
└── go.mod
```

---

## 7. Command Set

Initial command surface for the MVP:

### `dockguard check`

- runs all preflight validations
- exits non-zero if any required check fails

### `dockguard start`

- runs the same preflight validations
- starts Docker Desktop only if checks pass
- fails clearly when Docker Desktop CLI support is unavailable or unsupported

### `dockguard status`

- reports current readiness and key environment state without starting Docker Desktop

Deferred command:

- `doctor` stays out of the MVP unless implementation pressure proves it necessary

---

## 8. MVP Functional Requirements

Required checks in the first milestone:

- expected external mount path exists
- expected Docker storage path exists
- target path is writable
- free space is above configurable minimum
- Docker Desktop settings match the expected external path
- optional guard: Docker Desktop is not already running

Expected behavior:

- all checks produce readable CLI output
- failures identify the exact condition that blocked startup
- exit codes are stable enough for scripting and shell aliases

---

## 9. Risks and Constraints

DockGuard reduces bad startup states. It does not eliminate all failure modes.

Known risk boundaries:

- it does not protect against SSD disconnects after Docker Desktop has already started
- it depends on Docker Desktop CLI and config surfaces remaining compatible
- settings file location and naming may differ by Docker Desktop version
- Docker Desktop CLI support is version-dependent

Operational stance:

- prefer explicit failure over implicit fallback
- prefer documented Docker Desktop surfaces over reverse-engineered internals

---

## 10. Delivery Phases

## Phase 0 — Validation Baseline
**Owner:** One
**Goal:** Lock the product definition against real Docker Desktop behavior.

Deliverables:

- canonical product summary
- corrected Docker Desktop storage mental model
- compatibility notes for Desktop CLI and settings files
- agreed v1 scope and exclusions

Approval checklist:

- [ ] positioning no longer implies "images-only" offload
- [ ] Go is confirmed as implementation language
- [ ] Docker Desktop version dependency is documented
- [ ] settings file path is defined as configurable and version-aware

## Phase 1 — Product and Documentation Foundation
**Owner:** One
**Goal:** Turn the validated proposal into project documentation that can drive execution.

Deliverables:

- lifecycle document aligned to the reviewed proposal
- README direction and positioning language
- initial examples for expected config
- MVP success criteria

Approval checklist:

- [ ] docs use the corrected mental model everywhere
- [ ] scope is CLI-only and macOS-only for v1
- [ ] command set is limited to `check`, `start`, and `status`

## Phase 2 — Technical Skeleton
**Owner:** Three
**Goal:** Establish the Go CLI structure and core integration boundaries.

Deliverables:

- `go.mod`
- `cmd/dockguard/main.go`
- internal package skeleton
- config loading path
- output conventions

Approval checklist:

- [ ] repo structure matches agreed Go layout
- [ ] CLI entrypoint builds locally
- [ ] config path strategy supports overrides

## Phase 3 — Preflight Check Implementation
**Owner:** Three
**Goal:** Implement the minimum safe checks required for `check` and `status`.

Deliverables:

- mount existence check
- storage path existence check
- writability check
- free-space threshold check
- Docker Desktop settings inspection
- test coverage for core failure modes

Approval checklist:

- [ ] each failed check returns clear output
- [ ] non-zero exit behavior is consistent
- [ ] tests cover the critical logic branches

## Phase 4 — Guarded Startup
**Owner:** Three
**Goal:** Implement controlled Docker Desktop startup via the documented CLI.

Deliverables:

- `dockguard start`
- Docker Desktop CLI capability detection
- unsupported-version messaging
- optional already-running guard

Approval checklist:

- [ ] startup occurs only after checks pass
- [ ] unsupported CLI state fails clearly
- [ ] no hidden fallback start path is introduced

## Phase 5 — Release Baseline
**Owner:** One
**Goal:** Ship the first coherent milestone for external testing.

Target milestone:

- `v0.1.0` as the first usable baseline

Release checklist:

- [ ] docs and examples are coherent
- [ ] core commands build and run on macOS
- [ ] failure messaging is acceptable for real operator use
- [ ] release review completed before tagging

---

## 11. Engineering Quality Gates

- use short-lived branches for task work per [docs/versioning-and-git-workflow.md](/Users/nurulazrad/Projects/ningenai/dockguard/docs/versioning-and-git-workflow.md)
- treat the stable branch as release and integration only
- write tests before or alongside behavioral changes
- keep implementation aligned to documented Docker Desktop surfaces
- avoid hidden assumptions about settings paths or Desktop versions
- prefer deterministic CLI output over clever automation

---

## 12. Current Next Step

Immediate next execution step:

- turn the docs baseline into the actual Go repo skeleton and initial CLI commands

Near-term build order:

1. scaffold the Go module and command entrypoint
2. define config loading and output contracts
3. implement `status` and `check`
4. implement guarded `start`
5. tag the first coherent baseline when the workflow criteria are met
