# Changelog

All notable changes to DockGuard are documented in this file.

## [Unreleased] - v0.4.0

### Added
- `container` target kind for Apple `container` stacks started and stopped by your own `up` and `down` scripts. State comes from `container list` (`running`, `partial` or `stopped`); a container system confirmed down (`unregistered`) means the stack is stopped, and anything unconfirmed is `unknown`.
- `stop_system` option for `container` targets: `stop` also stops Apple container's system service, which keeps files open on the drive, and refuses while containers outside the stack are running. While the system runs, the stack counts as `partial` for the eject rule.
- `dockguard stop --all [--eject]`: stops every target that isn't confirmed stopped, in reverse config order, carrying on past failures. It ejects only when every target is stopped.
- When macOS refuses the eject because the drive is busy, the message names the processes with files open on it (from `lsof`).

## [v0.3.0] - 2026-11-08

### Added
- `app` target kind for macOS GUI apps such as Xcode: `symlink_paths` must each resolve to the drive, `start` opens the app with `open -a`, and `stop` asks it to quit only once it's confirmed running.
- `~/Library/Developer/CoreSimulator/Devices`, anything inside it, and any folder containing it are rejected in `symlink_paths`, including through symlinked folders.
- `postgres` target kind for clusters run with `pg_ctl` from a required, version-specific `bin_dir`. Checks the data folder is on the drive, `PG_VERSION` matches `bin_dir` (including 9.x two-part versions), no stale `postmaster.pid`, and the port is free. The server log goes to `log_file`, defaulting to `<data_dir>.log`. `stop` uses fast mode.

### Changed
- Paths in the current config format must be absolute or start with `~/`.

### Fixed
- Expanding `~` in config paths no longer collapses `..` before symlinks are followed.

## [v0.2.0] - 2026-11-08

### Added
- Targets: the config lists the applications DockGuard manages under `targets:`, each with a `kind`. This release supports `docker-desktop`; `app`, `postgres` and `container` are planned.
- `dockguard list` shows every target with its state (running, partial, stopped or unknown) and path.
- `check`, `start` and `stop` take an optional target name; without one they act on `docker`.
- Config lookup also checks `~/.config/dockguard/dockguard.yaml`; `list` and `status` print the file used.
- Eject rule: `stop --eject` ejects only when every target is confirmed stopped after a fresh check, waiting up to 30 seconds for the target just stopped.
- A new check that the Docker storage path is on the drive (new-format configs).

### Changed
- Config is parsed with `gopkg.in/yaml.v3`; unknown keys and kinds are errors.
- `status` covers every target and no longer reports "not ready" just because Docker Desktop is running. A target whose state can't be read is not ready.
- A missing config file is now an error instead of a silent fallback to built-in defaults.
- When the drive isn't mounted, the checks that depend on it are shown as skipped.
- The example config and README use the current format and a generic drive path.

### Compatibility
- v0.1 config files still load as a single `docker` target; `status` suggests the new format.

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

[v0.3.0]: https://github.com/platform41/dockguard/compare/v0.2.0...v0.3.0
[v0.2.0]: https://github.com/platform41/dockguard/compare/v0.1.2...v0.2.0
[v0.1.2]: https://github.com/platform41/dockguard/compare/v0.1.1...v0.1.2
[v0.1.1]: https://github.com/platform41/dockguard/compare/v0.1.0...v0.1.1
[v0.1.0]: https://github.com/platform41/dockguard/releases/tag/v0.1.0
