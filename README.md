# DockGuard

DockGuard is a macOS-first Go CLI that performs preflight checks before starting Docker Desktop when Docker Desktop storage is hosted on an external SSD.

## Quickstart

1. Create a config file (`./dockguard.yaml`) based on the example below.
2. Run `dockguard status` to verify readiness.
3. Start Docker Desktop safely with `dockguard start`.

```bash
dockguard status
dockguard start
```

## Install

From source (Go 1.21+):

```bash
go install ./cmd/dockguard
```

Build a local binary:

```bash
go build -o ./bin/dockguard ./cmd/dockguard
```

## Prerequisites

- macOS with Docker Desktop installed
- External SSD configured as the Docker Desktop storage location

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

## Commands

- `dockguard status [--config path]`: report readiness and key environment state
- `dockguard check [--config path]`: run all required preflight validations
- `dockguard start [--config path]`: run checks, then start Docker Desktop only if safe

If `--config` is omitted, DockGuard looks for `./dockguard.yaml` and falls back to built-in defaults when that file is absent.

## Compatibility Notes

- DockGuard targets macOS and Docker Desktop.
- `docker desktop start` is the preferred startup path and depends on Docker Desktop CLI support.
- Docker Desktop settings file location is not assumed to be fixed. DockGuard is designed for overrideable and version-aware settings file detection.

## Configuration

See `examples/dockguard.yaml` for the intended config shape.

Expected config inputs:

- external mount path
- Docker Desktop storage path
- minimum free space threshold
- optional explicit settings file path override
- optional already-running guard

Example:

```bash
dockguard check --config ./dockguard.yaml
```

Minimal config example:

```yaml
external_mount_path: /Volumes/DockerSSD
docker_storage_path: /Volumes/DockerSSD/DockerDesktop
minimum_free_space_gb: 50
fail_if_already_running: true
```

## Current Checks

`status` and `check` currently validate:

- external mount path is configured and exists
- Docker storage path is configured and exists
- Docker storage path is writable
- available free space meets the configured threshold
- Docker Desktop settings file exists
- Docker Desktop settings JSON contains the expected storage path
- Docker Desktop is not already running when `fail_if_already_running` is enabled

Recognized settings keys for storage-path validation:

- `diskImageLocation`
- `diskImagePath`
- `dataFolder`
- `storagePath`
- `virtualMachineDiskPath`

This settings validation is JSON-aware, but it is still based on recognized keys rather than Docker-version-specific schemas.

## Common Failures

- External mount not found: ensure the SSD is mounted at `external_mount_path`.
- Storage path missing: confirm Docker Desktop is configured to use the external path.
- Free space below threshold: lower `minimum_free_space_gb` or free space on the SSD.
- Docker already running: stop Docker Desktop or set `fail_if_already_running: false`.

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
- a working config loader with `--config` support
- implemented preflight checks for filesystem state and Docker Desktop settings
- guarded startup through `docker desktop start`
- an example config file for the first implementation slice

The next implementation milestone is tightening Docker Desktop compatibility handling and expanding test coverage around real-world settings variants.
