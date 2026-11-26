# DockGuard

DockGuard is a macOS Go CLI that runs preflight checks before starting applications whose data lives
on an external SSD, and stops them safely before the drive is ejected.

It manages Docker Desktop, GUI apps such as Xcode, PostgreSQL clusters and Apple `container`
stacks (see [`docs/specs/multi-target.md`](docs/specs/multi-target.md)).

## Quickstart

1. Create `~/.config/dockguard/dockguard.yaml` from [`examples/dockguard.yaml`](examples/dockguard.yaml).
2. Register your drive (recommended): run `dockguard drive` and add the `drive_uuid:` line it prints
   to the config.
3. See what DockGuard manages: `dockguard list`.
4. Check everything is ready: `dockguard status`.
5. Start safely: `dockguard start docker`.
6. Stop and eject: `dockguard stop docker --eject`. After an eject, `dockguard mount` brings the
   drive back without unplugging it.

## Install

With Go 1.23 or later on macOS, install the latest release:

```bash
go install github.com/platform41/dockguard/cmd/dockguard@latest
```

### Updating

From v0.7.0 on, `dockguard version` shows the version you're running and, in a terminal, offers to
update when a newer release exists (`Update to vX? [y/N]`, default No). It runs the same pinned
`go install` and reports `Updated` only after checking the installed file. Older versions don't
have this command: run the `go install …@latest` line above once.

### From source

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

## Upgrading to v0.9.0

Existing configs keep working. Everyday start and stop behavior stays the same when nothing
blocks the drive or the container-system stop. When a blocker appears, the terminal and dashboard
can ask you to stop it. Use [Drive holders](#drive-holders) to inspect the blockers before ejecting.

## Upgrading to v0.8.0

v0.8.0 makes shared prerequisites explicit with `depends_on` and the new
`container-system` kind. Start and stop ask before acting on additional targets;
without a terminal they refuse with commands to run. The dashboard asks the same
questions. See [Target dependencies](#target-dependencies).

Existing configs keep working for this release. If you use `stop_system`, `list`
and `status` show a deprecation line for each target using it; `stop_system` is removed in
v0.10.0. Follow
[Migrating from `stop_system`](#migrating-from-stop_system) to switch to an explicit shared
target. This changes your config and scripts. Updating the binary
alone does not migrate them. Update with `dockguard version` or the `go install`
command above.

## Upgrading to v0.7.0

v0.7.0 adds drive registration, one-step remounting, self-update and persistent volume display. Your
existing config keeps working unchanged; each step below is optional.
Full list: [release notes](docs/releases/v0.7.0.md) · [CHANGELOG](CHANGELOG.md).

1. **Update DockGuard.** From v0.6.0 or earlier:

   ```bash
   go install github.com/platform41/dockguard/cmd/dockguard@latest
   dockguard version      # confirms v0.7.0; future updates are offered here
   ```

2. **Register your drive.** `dockguard drive` prints the volume's identity and a `drive_uuid:` line.
   Add that line next to `drive:` in your config. From then on DockGuard trusts only that volume at
   that path: a leftover empty folder, a different drive with the same name, or the drive mounting
   as `/Volumes/<name> 1` is reported and treated as not mounted, and eject refuses an unverified
   volume. See [Commands](#commands) for every drive status.
3. **Remount without unplugging.** After an eject, run `dockguard mount`, or press `m` in
   `dockguard ui` when the drive line says `press m to mount`. See [Dashboard](#dashboard).
4. **Show persistent volumes.** For an Apple `container` stack, list its named volumes, for example
   `volumes: [fizzy-mysql]` (requires `data_path`). `dockguard list` then shows each volume under
   its stack with its size on disk. Information only; start, stop and eject are unchanged. See
   [Configuration](#configuration).

Already on v0.6.0 features? `ready` and `ephemeral` for container targets are covered in
[Configuration](#configuration) and [Container checks](#container-checks), with a worked example in
[the AWS Emulator recipe](docs/recipes/aws-emulator.md).

## Prerequisites

- macOS with Docker Desktop 4.37 or later (for `docker desktop start`)
- An external SSD set as Docker Desktop's storage location

## Correct Mental Model

- Your MacBook provides CPU, RAM, macOS, and the Docker Desktop app.
- Docker Desktop runs a Linux VM or engine layer on the Mac.
- The external SSD stores the Docker Desktop disk image, which is where Docker Desktop stores Linux containers and images on macOS.

DockGuard is not an "images-only offload" tool. It protects a setup where Docker Desktop storage is externalized to an SSD, while compute still runs on the MacBook.

## Commands

`dockguard drive` inspects the volume at the configured `drive` path and prints a
`drive_uuid:` line. Paste that line into your current-format configuration to
register the volume; DockGuard leaves the config file for you to edit.

```yaml
drive: /Volumes/ExternalSSD
drive_uuid: 11111111-2222-3333-4444-555555555555 # use the UUID printed by dockguard drive
# targets: ...
```

With a registered UUID, only that volume mounted at the configured path counts
as mounted. `start` and preflight checks require verified identity, and eject
refuses if identity cannot be verified. Configs without `drive_uuid` keep the
existing directory check and show a registration hint in `list` and `status`.
The legacy v0.1 format does not accept `drive_uuid`.

| Drive status | Meaning / next action |
| --- | --- |
| `mounted` | Registered volume is at the configured path. |
| `not mounted` | Registered volume is attached; run `dockguard mount`. |
| `not connected` | Registered UUID is absent from a successful disk inventory; plug it in. |
| `mounted elsewhere` | The volume is mounted at the reported different path. |
| `wrong volume` | A different volume occupies the configured path; both UUIDs are reported. |
| `stray folder` | The configured path exists but is not a mount point. |
| `unknown` | Disk inspection failed or timed out; the error is reported. |

`dockguard mount` requires registration and mounts only an attached, unmounted
volume with an available path. It waits at most 90 seconds for `diskutil mount`,
then rechecks identity; failures and timeouts also report the freshly read state.
It refuses occupied paths, leaves folders and volume names alone, and does not
start targets. Normal macOS auto-mount behavior is unchanged; no `sudo` is needed.
The dashboard shows the same status and offers `m mount` only when the registered
drive is attached but unmounted. The action runs with the usual `Working…` display
and refreshes afterward.

| Command | Does |
| --- | --- |
| `dockguard drive` | Inspects the volume and prints its registration line. |
| `dockguard mount` | Mounts the registered, attached volume and verifies its path. |
| `dockguard holders [--stop]` | Lists processes using the mounted drive. Add `--stop` to review quit requests in a terminal. |
| `dockguard ui` | Live dashboard: the drive, whether it's safe to eject, and every target's state, with keys to start, stop, check, or stop everything and eject |
| `dockguard list` | Lists the configured targets with their state (running, partial, stopped or unknown) and path. Runs no checks. |
| `dockguard status` | Every target's state and checks. Exits 0 when every target is ready. A running target counts as ready; one whose state can't be read doesn't. |
| `dockguard check [target]` | Runs one target's checks, including the already-running guard |
| `dockguard start [target]` | Checks dependencies first; asks before starting missing prerequisites, then runs the target's checks and starts it only if they pass |
| `dockguard stop [target] [--eject]` | Asks before also stopping active dependents; with `--eject`, applies the eject rule below |
| `dockguard stop --all [--eject]` | Stops dependents before prerequisites, with reverse config priority for independent targets; carries on with independent targets after failure, protecting prerequisites still in use; any failure prevents eject |
| `dockguard version` | Shows the running version and checks for a newer release. In a terminal, offers a pinned `go install` update and verifies the installed file; defaults to No |

With no target, `check`, `start` and `stop` act on the target named `docker`, so v0.1 commands keep
working. Every command accepts `--config <path>`.

**Eject rule.** After stopping, DockGuard checks every configured target again, waiting up to 30
seconds for the one it just stopped. It ejects only when every target is confirmed stopped and a
fresh process scan finds nothing else using the drive. A running, partial or unknown target blocks
the eject. A shell whose current directory is on the drive, Finder/Quick Look or another process
also blocks it and is named (for example `zsh (PID 23721)`). If the process scan fails, readiness is
unknown and DockGuard refuses the eject. There's no override flag.

## Drive holders

Run `dockguard holders` to list processes using the mounted drive and see why each can or cannot
be offered a quit. It exits 0 when the drive is clear, 1 when processes are using it, and 2 when
inspection fails. `dockguard holders --stop` asks about each eligible process and checks the drive
again afterward. In a terminal, `stop --eject` offers the same questions after targets stop.
Without a terminal, DockGuard leaves blocking processes alone and lists `kill <pid>` commands
for eligible processes.

- **Quit `[Y/n]`** sends a normal quit request. Enter accepts. DockGuard waits up to five seconds
  for the process to exit or release the drive. Finder is eligible and macOS relaunches it.
- **Force-quit `[y/N]`** appears if the process still holds the drive. Enter declines. Force-quitting
  can lose unsaved work. DockGuard verifies the process and its drive use before either signal.
- **Stop container `[Y/n]`** appears when an unmanaged container blocks a `container-system` stop.
  Enter accepts. DockGuard stops the named container, prints its `container start <name>` command,
  and checks again before stopping the system.

DockGuard leaves root-owned and other users' processes, its own process and ancestors, and
protected macOS processes alone. It uses no `sudo`. Container questions leave container data
intact. Configured stacks stop through their own scripts, and DockGuard leaves container runtime
processes alone.

## Dashboard

`dockguard ui` opens a live dashboard in the terminal:

```
──────────────────────────── DockGuard · ExternalSSD ────────────────────────────
 Drive  /Volumes/ExternalSSD   ● mounted   ▓▓▓▓░░░░░░░░░░  26% used · 685 GB free
 Eject  ✗ blocked by: myapp-db (running), zsh (PID 23721, using drive) [self]

 ▌○ docker       docker-desktop  stopped  …ExternalSSD/docker-data/DockerDesktop
  ○ xcode        app             stopped  …/Library/Developer/Xcode/DerivedData
  ● myapp-db     postgres        running  /Volumes/ExternalSSD/pgdata/myapp
  ● myapp-stack  container       running  /Volumes/ExternalSSD/container-data

 p processes · ↑↓ · s start · x stop · E eject all · q quit
```

It refreshes target and process-holder state every 3 seconds. `safe to eject` means both checks
passed; failure to inspect holders is shown as unknown. The keys run the same code as the matching
commands: `s` start, `x` stop, `c` check, `E` stop everything and eject. Stopping and ejecting ask
you to confirm. Dependencies appear dimmed after the kind, for example
`container  → container`. `s` asks before starting missing prerequisites and `x`
asks before stopping active dependents, using the same questions as the CLI.
Press a fresh `y`/`Y` to accept; any other key cancels without changing targets.
Unknown prerequisite state refuses rather than offering a start. If the targets
needing action change while a confirmation is open, retry to confirm the new set.

When the drive is mounted and the scan finds holders, `p` offers the
[drive-holder questions](#drive-holders) and refreshes without ejecting. `E` offers those questions
before its final eject check. Stopping a `container-system` target can also ask about unmanaged
containers. For these questions press `y` or `n`, or use Enter to take the displayed default.
Esc cancels the remaining questions and the eject, if requested. A process that already received
a signal is still checked and reported. Refresh resumes when the action finishes.

With a registered drive (`drive_uuid`), the Drive line explains any problem, such as
`○ mounted elsewhere — registered volume mounted at /Volumes/ExternalSSD 1, not /Volumes/ExternalSSD`. When the drive is attached but unmounted, for example
right after `E`, it reads `○ not mounted — press m to mount`, and `m` runs `dockguard mount`
without a confirmation (mounting changes no data), then refreshes. `m` is shown and active only in
that state.

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

  container:
    kind: container-system
    data_path: /Volumes/ExternalSSD/container-data   # optional

  myapp-stack:
    kind: container
    depends_on: [container]
    up: ~/bin/myapp-up.sh           # starts only the stack, not the shared system
    down: ~/bin/myapp-down.sh
    containers: [myapp-db, myapp-web]
    data_path: /Volumes/ExternalSSD/container-data   # optional
    volumes: [myapp-mysql]           # optional: persistent volume information; requires data_path
    ready: ~/bin/myapp-ready.sh     # optional: poll after up until the service is usable
    ready_timeout_seconds: 30       # optional: default 30; allowed 1-300
    ephemeral: true                 # optional: warn that down discards this stack's data
```

For an ephemeral container target, `list` and the dashboard append `(ephemeral)` to its state.
Immediately before `down` runs, DockGuard prints `<target> is ephemeral: its data is discarded on
stop.` A stopped stack or a system-only stop does not run `down`, so it prints no discard notice.
The flag adds no confirmation prompt and does not change stop order, eject readiness, or the
existing dashboard stop and eject confirmations.

For persistent storage, list named volumes on the owning `container` target, for
example `volumes: [fizzy-mysql]` on the Fizzy stack. `dockguard list` shows a
sub-row per volume; `dockguard status` shows information lines after the stack's
checks. DockGuard reads `<data_path>/volumes/<name>/volume.img` directly, so this
works even when the container system is stopped. `present, 268 MB` reports
allocated disk space in decimal units, rather than the sparse image's logical
capacity. A missing file shows `missing`; other lookup errors show `unknown`.
An unmounted or unverified drive shows `drive not mounted` without looking up
volume files.

`volumes` requires `data_path`. Names use letters, digits, `_`, `-`, and `.`,
start with a letter or digit, and cannot contain `..` or duplicates. A volume is
information under its stack: it does not change target state, check results,
summary, exit codes, startup, shutdown, eject safety, or dashboard rows. A volume
name cannot be used as a target, and DockGuard never creates or deletes volumes.

For a worked example with `ready` and `ephemeral`, see
[AWS Emulator as a `container` target](docs/recipes/aws-emulator.md).

Unknown keys and kinds are errors. Target names use lowercase letters, digits and `-`. Paths must
be absolute or start with `~/`.

### Target dependencies

Every kind accepts an optional `depends_on: [target-name, ...]`. Names must exist;
self references, duplicate entries and cycles are config errors. A cycle is shown
in order, such as `a → b → a`. A `container-system` target manages Apple's shared
container service; container stacks should declare it as a prerequisite.

One command acts on one target by default. For a stopped prerequisite:

```text
$ dockguard start myapp-stack
myapp-stack needs container, which is stopped.
Start container first? [y/N]
```

Only `y`/`yes`, in any case, proceeds. Enter, other text and EOF mean No.
DockGuard runs each missing prerequisite's preflight and starts it deepest first,
confirming it running before continuing. Running prerequisites are left alone;
an unknown prerequisite refuses with its probe error, without a question. A failed
preflight, start or running-state confirmation stops the sequence.

Without a terminal, or after No, a single missing prerequisite prints:

```text
Not started. Start it first: dockguard start container
```

For a chain the question lists the order (`Start base, then container, first? [y/N]`).
Refusal lists only missing prerequisites, deepest first, one command per line:

```text
Not started. Start these first, in order:
  dockguard start base
  dockguard start container
```

Stopping a prerequisite asks `Stop them first? [y/N]` when active targets depend
on it, directly or through a chain. Running, partial and unknown dependents count.
On yes, dependents stop in reverse dependency order and must be confirmed stopped
before the next action. On No or without a terminal, the message is
`Not stopped. Stop these first: myapp-stack, other-stack`; nothing stops.

`stop --all` stops targets in reverse dependency order without dependency prompts.
A blocked container-system stop or eject can ask about [blockers](#drive-holders).
Among targets ready to stop, the last configured target goes first. A failure or
unconfirmed stop leaves all of that target's prerequisites alone and reports them,
while independent targets can still stop. Any failure prevents eject. With no
dependencies, this is the previous reverse config order. Configs without an explicit
`container-system` target retain the legacy shared-system pass after all targets,
including when they declare dependencies.

### Migrating from `stop_system`

1. Add one shared target, for example `container: {kind: container-system}`. Set its
   optional `data_path` as in the example above.
2. Add `depends_on: [container]` to each container stack that uses it.
3. Remove `container system start` from those stacks' `up` scripts. Their scripts
   should start only their own containers.
4. Remove `stop_system` from every stack. A config with a `container-system` target
   and `stop_system: true` anywhere is rejected, naming the conflicting target.

Use `dockguard start <stack>` to be asked before starting the shared system, and
`dockguard stop container` to be asked before stopping its dependents. Stopping a
stack alone leaves the system running; that running system blocks eject. Use
`stop --all --eject` to stop everything in dependency order and apply the eject rule.

For this release, unmigrated configs keep `stop_system` behaviour: two target/system
passes, with target dependencies honored in the first pass. `list` and `status`
print, once per target using it:
`stop_system is deprecated; add a container-system target and depends_on (see README).`
Without `depends_on`, stop ordering and command output remain as in v0.7.x apart
from these deprecation notices. `stop_system` is removed in v0.10.0.

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
- `ready`, if set, exists and is executable
- `data_path`, if set, exists and is on the drive
- Apple container's data folder (`~/Library/Application Support/com.apple.container`) resolves to
  the drive
- with `fail_if_already_running` (default on), `check` and `start` fail if any of the stack's
  containers is running

State comes from `container list`: `running` if all of `containers` run, `partial` if some do,
`stopped` if none do. When the container system is confirmed down (`unregistered`), the stack is
`stopped`; a system that doesn't answer its health check is `unknown`, which blocks the eject.

With `ready` set, `start` runs the script once per second after `up` succeeds. Each attempt is
limited to five seconds or the remaining overall deadline, whichever is shorter. The overall
`ready_timeout_seconds` deadline defaults to 30 seconds and may be from 1 to 300. Success reports
`started and ready`; timeout reports `started but not ready`, exits 1, and leaves the container
running. Readiness runs only during start, never during `list`, `status`, or dashboard refreshes.
Without `ready`, start behavior is unchanged.

Apple container's system service keeps files open on the drive even after a stack stops. With
the `container-system` kind, `start` runs `container system start` and confirms
`running`; `stop` runs `container system stop`. Its checks require the CLI on the
PATH and Apple's data folder on the drive, plus the usual `data_path` checks when
set. State is running, stopped (`unregistered`) or unknown. A running or unknown
system blocks eject. If active containers outside configured stacks block the system stop,
DockGuard offers to [stop them](#drive-holders) in a terminal or in the dashboard. The system
stop is refused while blockers remain. Declare `depends_on` on every stack using the system.

Legacy `stop_system: true` is deprecated but still works for this release: stop
also stops the service, and a running system makes an otherwise stopped stack
`partial`. Its existing safety rule still refuses while containers outside that
stack run. See [Migrating from `stop_system`](#migrating-from-stop_system).

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
