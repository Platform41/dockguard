# DockGuard

DockGuard is a macOS Go CLI that runs preflight checks before starting applications whose data lives
on an external SSD, and stops them safely before the drive is ejected.

Today it manages Docker Desktop and GUI apps such as Xcode. PostgreSQL clusters and Apple
`container` stacks follow in v0.3.0 and v0.4.0 (see
[`docs/specs/multi-target.md`](docs/specs/multi-target.md)).

## Quickstart

1. Create `~/.config/dockguard/dockguard.yaml` from [`examples/dockguard.yaml`](examples/dockguard.yaml).
2. See what DockGuard manages: `dockguard list`.
3. Check everything is ready: `dockguard status`.
4. Start safely: `dockguard start docker`.
5. Stop and eject: `dockguard stop docker --eject`.

## Install

From source (Go 1.23+):

```bash
go install ./cmd/dockguard
```

If `dockguard` is not found after install, verify where Go installed it:

```bash
go env GOBIN
go env GOPATH
ls "$(go env GOPATH)/bin" | grep dockguard
```

If `GOBIN` is empty, add `$(go env GOPATH)/bin` to your shell `PATH`:

```bash
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
hash -r
which dockguard
dockguard --help
```

Recommended config location (found automatically, so `--config` isn't needed):

```text
~/.config/dockguard/dockguard.yaml
```

Optional shorter name:

```bash
dg() { dockguard "$@"; }
```

Add this to `~/.zshrc`, then reload with `source ~/.zshrc`.

Build a local binary:

```bash
go build -o ./bin/dockguard ./cmd/dockguard
```

## Prerequisites

- macOS with Docker Desktop 4.37 or later (for `docker desktop start`)
- An external SSD set as Docker Desktop's storage location

## Correct Mental Model

- Your MacBook provides CPU, RAM, macOS, and the Docker Desktop app.
- Docker Desktop runs a Linux VM or engine layer on the Mac.
- The external SSD stores the Docker Desktop disk image, which is where Docker Desktop stores Linux containers and images on macOS.

DockGuard is not an "images-only offload" tool. It protects a setup where Docker Desktop storage is externalized to an SSD, while compute still runs on the MacBook.

## Commands

| Command | Does |
| --- | --- |
| `dockguard list` | Lists the configured targets with their state (running, partial, stopped or unknown) and path. Runs no checks. |
| `dockguard status` | Every target's state and checks. Exits 0 when every target is ready. A running target counts as ready; one whose state can't be read doesn't. |
| `dockguard check [target]` | Runs one target's checks, including the already-running guard |
| `dockguard start [target]` | Runs the checks, then starts the target only if they all pass |
| `dockguard stop [target] [--eject]` | Stops the target; with `--eject`, ejects the drive under the eject rule below |

With no target, `check`, `start` and `stop` act on the target named `docker`, so v0.1 commands keep
working. Every command accepts `--config <path>`.

**Eject rule.** After stopping, DockGuard checks every configured target again, waiting up to 30
seconds for the one it just stopped. It ejects only when every target is confirmed stopped. A
target that is running, partly running, or whose state can't be read blocks the eject, and DockGuard
names it. There's no override flag.

## Configuration

The config file is the record of which applications DockGuard manages. DockGuard looks for it in
this order:

1. `--config <path>`
2. `./dockguard.yaml`
3. `~/.config/dockguard/dockguard.yaml`

If none exists, DockGuard stops and names the paths it tried. `list` and `status` print the file
they read.

```yaml
drive: /Volumes/ExternalSSD
minimum_free_space_gb: 50

targets:
  docker:
    kind: docker-desktop
    storage_path: /Volumes/ExternalSSD/docker-data/DockerDesktop
    # settings_path: ~/Library/Application Support/Docker/settings-store.json  # auto-detected
    require_cli_start_support: true
    fail_if_already_running: true

  xcode:
    kind: app
    app: Xcode                      # the name `open -a` takes
    # process: Xcode                # process name for pgrep; defaults to app
    symlink_paths:                  # internal-disk paths that link onto the drive
      - ~/Library/Developer/Xcode/DerivedData
```

Unknown keys and kinds are errors. Target names use lowercase letters, digits and `-`.

**v0.1 configs still work.** A file with `external_mount_path`, `docker_storage_path` and
`docker_desktop:` is read as a single `docker` target, and `status` suggests the new format.

For a full setup (external SSD format, rollback backup, install, and safe stop/eject), see
[`docs/docker-local-setup.md`](docs/docker-local-setup.md).

## Docker Desktop checks

- the drive is configured and mounted (if not, the other checks are skipped)
- the storage path exists, is on the drive, and is writable
- free space meets `minimum_free_space_gb`
- the settings file exists and points at the storage path
- with `fail_if_already_running`, `check` and `start` fail if Docker Desktop is already running

Recognized settings keys for storage-path validation:

- `diskImageLocation`
- `diskImagePath`
- `dataFolder`
- `storagePath`
- `virtualMachineDiskPath`

This settings validation is JSON-aware, but it is still based on recognized keys rather than Docker-version-specific schemas.

## App checks

- the drive is configured and mounted
- each `symlink_paths` entry exists and resolves to somewhere on the drive. A plain folder on the
  internal disk fails as "not on the drive": the app's data was recreated off the drive.
- with `fail_if_already_running` (default on), `check` and `start` fail if the app is running

`start` opens the app with `open -a`. `stop` asks it to quit, so it can save first, and only once
it's confirmed running. `~/Library/Developer/CoreSimulator/Devices` (or any folder containing it) is
rejected: simulator devices break on an external drive.

## Common Failures

- Drive not mounted: plug in the SSD; the drive's other checks show `[skip]` until it's mounted.
- Storage path missing or not on the drive: confirm Docker Desktop's disk image location.
- Free space below threshold: lower `minimum_free_space_gb` or free space on the SSD.
- Docker already running: stop Docker Desktop or set `fail_if_already_running: false`.
- Drive not ejected: `dockguard list` shows which target is still running or in an unknown state.

## Project Layout

```text
dockguard/
├── cmd/dockguard/
├── internal/app/          # commands and the eject rule
├── internal/checks/       # shared preflight checks
├── internal/config/       # config loading and validation
├── internal/dockerdesktop/  # Docker Desktop CLI
├── internal/output/
├── internal/platform/     # macOS commands (diskutil, open, osascript, pgrep)
├── internal/target/       # target interface and kinds
└── examples/
```
