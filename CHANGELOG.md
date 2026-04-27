# Changelog

All notable changes to DockGuard are documented in this file.

## [v0.1.2] - 2026-04-27

### Added
- Added state-aware `dockguard stop` output to clearly report when Docker Desktop is already stopped.

### Changed
- Improved stop behavior to check Docker Desktop status first and skip redundant stop requests.
- Updated docs to replace the previous `dg` alias with a safer shell function pattern.

## [v0.1.1] - 2026-04-27

### Added
- Added install troubleshooting for `go install` path visibility (`GOBIN`/`GOPATH` checks).
- Added user-level config guidance for storing `dockguard.yaml` in `~/.config/dockguard/`.

## [v0.1.0] - 2026-04-27

### Added
- Added external SSD setup and operations runbook in `docs/docker-local-setup.md`.
- Added `dockguard stop` command with optional `--eject` flow.
- Added platform-level volume eject support with retry and busy-volume guidance.

### Fixed
- Fixed Docker Desktop settings path matching to support case-insensitive keys such as `DataFolder`.

[v0.1.2]: https://github.com/platform41/dockguard/compare/v0.1.1...v0.1.2
[v0.1.1]: https://github.com/platform41/dockguard/compare/v0.1.0...v0.1.1
[v0.1.0]: https://github.com/platform41/dockguard/releases/tag/v0.1.0
