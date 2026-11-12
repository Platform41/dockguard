# DockGuard

DockGuard is a macOS Go CLI that runs preflight checks before starting applications whose data lives
on an external SSD, and stops them safely before the drive is ejected.

It manages Docker Desktop, GUI apps such as Xcode, PostgreSQL clusters and Apple `container`
stacks (see [`docs/specs/multi-target.md`](docs/specs/multi-target.md)).

## Quickstart

1. Create `~/.config/dockguard/dockguard.yaml` from [`examples/dockguard.yaml`](examples/dockguard.yaml).
2. See what DockGuard manages: `dockguard list`.
3. Check everything is ready: `dockguard status`.
4. Start safely: `dockguard start docker`.
5. Stop and eject: `dockguard stop docker --eject`.

## Install

With Go 1.23 or later on macOS, install the latest release:

```bash
go install github.com/platform41/dockguard/cmd/dockguard@latest
```

Or from a clone of this repo:

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
| `dockguard ui` | Live dashboard: the drive, whether it's safe to eject, and every target's state, with keys to start, stop, check, or stop everything and eject |
| `dockguard list` | Lists the configured targets with their state (running, partial, stopped or unknown) and path. Runs no checks. |
| `dockguard status` | Every target's state and checks. Exits 0 when every target is ready. A running target counts as ready; one whose state can't be read doesn't. |
| `dockguard check [target]` | Runs one target's checks, including the already-running guard |
| `dockguard start [target]` | Runs the checks, then starts the target only if they all pass |
| `dockguard stop [target] [--eject]` | Stops the target; with `--eject`, ejects the drive under the eject rule below |
| `dockguard stop --all [--eject]` | Stops every target that isn't confirmed stopped, in reverse config order, carrying on past failures; container systems (`stop_system`) last, after every stack; ejects only if all stopped |

With no target, `check`, `start` and `stop` act on the target named `docker`, so v0.1 commands keep
working. Every command accepts `--config <path>`.

**Eject rule.** After stopping, DockGuard checks every configured target again, waiting up to 30
seconds for the one it just stopped. It ejects only when every target is confirmed stopped and a
fresh process scan finds nothing else using the drive. A running, partial or unknown target blocks
the eject. A shell whose current directory is on the drive, Finder/Quick Look or another process
also blocks it and is named (for example `zsh (PID 23721)`). If the process scan fails, readiness is
unknown and DockGuard refuses the eject. There's no override flag.

## Dashboard

`dockguard ui` opens a live dashboard in the terminal:

```
──────────────────────────── DockGuard · ExternalSSD ────────────────────────────
 Drive  /Volumes/ExternalSSD   ● mounted   ▓▓▓▓░░░░░░░░░░  26% used · 685 GB free
 Eject  ✗ blocked by: myapp-db (running), zsh (PID 23721, using drive)

 ▌○ docker       docker-desktop  stopped  …ExternalSSD/docker-data/DockerDesktop
  ○ xcode        app             stopped  …/Library/Developer/Xcode/DerivedData
  ● myapp-db     postgres        running  /Volumes/ExternalSSD/pgdata/myapp
  ● myapp-stack  container       running  /Volumes/ExternalSSD/container-data

 ↑↓ move · s start · x stop · c check · E eject all · r refresh · q quit
```

It refreshes target and process-holder state every 3 seconds. `safe to eject` means both checks
passed; failure to inspect holders is shown as unknown. The keys run the same code as the matching
commands: `s` start, `x` stop, `c` check, `E` stop everything and eject. Stopping and ejecting ask
you to confirm.

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

  myapp-db:
    kind: postgres
    data_dir: /Volumes/ExternalSSD/pgdata/myapp
    bin_dir: /opt/homebrew/opt/postgresql@17/bin   # required: the cluster's own version
    port: 55432                     # optional
    socket_dir: /tmp                # optional
    log_file: /Volumes/ExternalSSD/pgdata/myapp.log  # optional; defaults to <data_dir>.log

  myapp-stack:
    kind: container
    up: ~/bin/myapp-up.sh           # your scripts start and stop the stack
    down: ~/bin/myapp-down.sh
    containers: [myapp-db, myapp-web]
    data_path: /Volumes/ExternalSSD/container-data   # optional
    stop_system: true               # optional: also stop Apple container's system service
```

Unknown keys and kinds are errors. Target names use lowercase letters, digits and `-`. Paths must
be absolute or start with `~/`.

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

## PostgreSQL checks

For a cluster run directly with `pg_ctl` (not `brew services`). Each cluster is its own target, so
any version, or several side by side, is just another entry with its own `bin_dir`, `data_dir` and
`port`.

- `pg_ctl` and `postgres` exist in `bin_dir`. `bin_dir` is required and never taken from the PATH:
  Homebrew's `libpq` can put a different major version of `pg_ctl` there.
- `data_dir` exists, is on the drive, is writable, and has enough free space
- `PG_VERSION` matches the major version of `<bin_dir>/pg_ctl` (`17`, or `9.6` for pre-10 clusters)
- no stale `postmaster.pid` (one naming a process that isn't running)
- with `port`, nothing else is listening on it
- with `fail_if_already_running` (default on), `check` and `start` fail if the cluster is running

`start` runs `pg_ctl -D <data_dir> -l <log_file> [-o "-p port -k socket_dir"] -w start`. The server
log goes to `log_file`, or `<data_dir>.log` when it isn't set. `stop` runs
`pg_ctl -m fast -w stop`, only once the cluster is confirmed running. DockGuard never edits
`postgresql.conf`.

## Container checks

For an Apple `container` stack started and stopped by your own `up` and `down` scripts.

- `up` and `down` exist and are executable; the `container` CLI is on the PATH
- `data_path`, if set, exists and is on the drive
- Apple container's data folder (`~/Library/Application Support/com.apple.container`) resolves to
  the drive
- with `fail_if_already_running` (default on), `check` and `start` fail if any of the stack's
  containers is running

State comes from `container list`: `running` if all of `containers` run, `partial` if some do,
`stopped` if none do. When the container system is confirmed down (`unregistered`), the stack is
`stopped`; a system that doesn't answer its health check is `unknown`, which blocks the eject.

Apple container's system service keeps files open on the drive even after a stack stops. With
`stop_system: true`, `stop` also runs `container system stop`, and a system still running counts as
`partial`, so it blocks the eject. Because stopping the system stops every container, DockGuard
refuses (and names them) while containers outside the stack are running.

## Common Failures

- Drive not mounted: plug in the SSD; the drive's other checks show `[skip]` until it's mounted.
- Storage path missing or not on the drive: confirm Docker Desktop's disk image location.
- Free space below threshold: lower `minimum_free_space_gb` or free space on the SSD.
- Docker already running: stop Docker Desktop or set `fail_if_already_running: false`.
- Drive not ejected: the dashboard shows configured target blockers and processes using the drive.
- Drive busy: quit the named process or move its terminal out of the drive with `cd ~`, then retry.

## Project Layout

```text
dockguard/
├── cmd/dockguard/
├── internal/app/          # commands and the eject rule
├── internal/checks/       # shared preflight checks
├── internal/config/       # config loading and validation
├── internal/dockerdesktop/  # Docker Desktop CLI
├── internal/output/
├── internal/platform/     # macOS commands (diskutil, open, osascript, pgrep, pg_ctl, container)
├── internal/target/       # target interface and kinds
├── internal/ui/           # the `dockguard ui` dashboard
└── examples/
```

## License

DockGuard is released under the [MIT License](LICENSE).
