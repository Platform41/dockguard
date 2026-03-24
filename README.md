# DockGuard

DockGuard is a macOS-first Go CLI that performs preflight checks before starting Docker Desktop when Docker Desktop storage is hosted on an external SSD.

## Correct Mental Model

- Your MacBook provides CPU, RAM, macOS, and the Docker Desktop app.
- Docker Desktop runs a Linux VM or engine layer on the Mac.
- The external SSD stores the Docker Desktop disk image, which is where Docker Desktop stores Linux containers and images on macOS.

DockGuard is not an "images-only offload" tool. It protects a setup where Docker Desktop storage is externalized to an SSD, while compute still runs on the MacBook.

## v1 Scope

In scope:

- macOS-only CLI
- preflight validation before Docker Desktop startup
- external mount presence checks
- Docker Desktop storage path checks
- writability checks
- free-space threshold checks
- Docker Desktop settings validation against the expected path
- guarded startup through `docker desktop start`

Out of scope:

- replacing Docker Desktop
- automatic Docker data migration
- runtime protection after Docker Desktop has already started
- Windows or Linux support
- GUI or menu bar app

## Planned Commands

- `dockguard status`: report readiness and key environment state
- `dockguard check`: run all required preflight validations
- `dockguard start`: run checks, then start Docker Desktop only if safe

## Compatibility Notes

- DockGuard targets macOS and Docker Desktop.
- `docker desktop start` is the preferred startup path and depends on Docker Desktop CLI support.
- Docker Desktop settings file location is not assumed to be fixed. DockGuard is designed for overrideable and version-aware settings file detection.

## Configuration

See [examples/dockguard.yaml](/Users/nurulazrad/Projects/ningenai/dockguard/examples/dockguard.yaml) for the intended config shape.

Expected config inputs:

- external mount path
- Docker Desktop storage path
- minimum free space threshold
- optional explicit settings file path override
- optional already-running guard

## Project Layout

```text
dockguard/
├── cmd/dockguard/
├── internal/checks/
├── internal/config/
├── internal/dockerdesktop/
├── internal/output/
├── internal/platform/
└── examples/
```

## Current Status

The repo currently contains:

- project lifecycle and Kanban docs aligned to the reviewed proposal
- a Go CLI skeleton with placeholder command handlers
- an example config file for the first implementation slice

The next implementation milestone is `status` and `check`, followed by guarded `start`.
