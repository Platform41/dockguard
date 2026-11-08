# Spec — Multi-Target DockGuard (v0.2.0 to v0.4.0)

**Status:** Draft, waiting for Azrad's review
**Last Updated:** 2026-11-08
**Owner:** One
**Builder:** Three
**Decisions:** D-005 to D-015 in the [board's decision log](../board/KANBAN.md#decision-log).
Proposal P-3 (§10) needs approval before v0.3.0 is built.

DockGuard runs preflight checks before starting Docker Desktop when Docker's storage is on an
external SSD. This spec extends the same protection to any application whose data lives on that
drive: GUI apps such as Xcode, PostgreSQL clusters, and Apple `container` stacks such as Fizzy.

The product promise doesn't change: **check first, fail loudly, start only when it's safe.**

---

## 1. Terms

| Term | Meaning |
| --- | --- |
| **Drive** | The external volume the targets depend on, for example `/Volumes/ExternalSSD` |
| **Target** | One managed application, named in the config (`docker`, `xcode`, `myapp-db`, `fizzy`) |
| **Kind** | How DockGuard checks, starts and stops a target: `docker-desktop`, `app`, `postgres`, `container` |
| **Check** | One preflight test with a name, a pass/fail result and a message |

New applications are **config entries of an existing kind**. Code is only needed for a new kind
(D-007).

---

## 2. Commands

Format: `dockguard <action> [target] [flags]` (D-006). Every command accepts `--config <path>`.

| Command | Does | Exit code |
| --- | --- | --- |
| `dockguard list` | Lists the configured targets: name, kind, running or stopped, and its main path on the drive. Runs no checks, so it's fast. | 0, or 1 if the config is invalid |
| `dockguard status` | `list` plus the full checks for every target. Shows the drive once at the top. | 0 if every target is ready, else 1 |
| `dockguard check <target>` | Runs that target's checks | 0 if all pass, else 1 |
| `dockguard start <target>` | Runs the checks, then starts the target only if they pass | 0 if started, else 1 |
| `dockguard stop <target>` | Stops the target. If it's already stopped, says so and exits 0 | 0, or 1 on error |
| `dockguard stop <target> --eject` | Stops the target, then ejects the drive. **Refuses to eject** while any other target on the drive is running, and names those targets. | 0 if ejected, else 1 |
| `dockguard stop --all [--eject]` | Stops every running target in reverse config order, then ejects if asked (v0.4.0) | 0, or 1 if any stop failed (no eject then) |
| `dockguard help` | Usage | 0 |

**Compatibility with v0.1:** `check`, `start` and `stop` with no target mean `docker`. Existing
shell functions like `dg start` keep working. A usage error (unknown target, missing argument,
`--eject` on a command other than `stop`) prints usage and exits 1.

**Already running:** a target that is already running is a *state*, not a failure, in `list` and
`status`. The already-running guard (`fail_if_already_running`) only applies to `check <target>`
and `start <target>`. This changes v0.1 behaviour: `status` no longer reports "not ready" just
because Docker Desktop is running. It's the only change in behaviour in v0.2.0.

### 2.1 Output

Plain text, one line per check, the same `[OK]` / `[FAIL]` style as v0.1. Example:

```text
$ dockguard list
Config: ~/.config/dockguard/dockguard.yaml
Drive:  /Volumes/ExternalSSD (mounted)

NAME        KIND            STATE     PATH
docker      docker-desktop  running   /Volumes/ExternalSSD/docker-data/DockerDesktop
xcode       app             stopped   ~/Library/Developer/Xcode/DerivedData -> /Volumes/ExternalSSD/...
myapp-db  postgres        running   /Volumes/ExternalSSD/pgdata/myapp
fizzy       container       running   /Volumes/ExternalSSD/container-data
```

If the drive isn't mounted, `list` still lists the targets and shows `drive not mounted` as their
state. When DockGuard can't find out whether a target is running, the state is `unknown`, never a
guess.

---

## 3. Config

Parsed with `gopkg.in/yaml.v3` (D-011), which replaces the hand-written parser.

**The config is the record of what DockGuard manages (D-015).** `list` and `status` print which
file they read. Lookup order:

1. `--config <path>`
2. `./dockguard.yaml`, if present (v0.1 behaviour)
3. `~/.config/dockguard/dockguard.yaml`, the recommended home for the record

If none exists, commands other than `help` fail and name the paths they tried. (v0.1 silently
used built-in defaults; with several targets, a missing record must be visible.)

```yaml
drive: /Volumes/ExternalSSD
minimum_free_space_gb: 50          # default for every target; a target can override it

targets:
  docker:
    kind: docker-desktop
    storage_path: /Volumes/ExternalSSD/docker-data/DockerDesktop
    settings_path: ~/Library/Application Support/Docker/settings-store.json   # optional, auto-detected
    require_cli_start_support: true
    fail_if_already_running: true

  xcode:                            # v0.3.0
    kind: app
    app: Xcode
    symlink_paths:
      - ~/Library/Developer/Xcode/DerivedData
      - ~/Library/Developer/Xcode/Archives

  myapp-db:                       # v0.3.0 (P-3)
    kind: postgres
    data_dir: /Volumes/ExternalSSD/pgdata/myapp
    bin_dir: /opt/homebrew/opt/postgresql@17/bin   # required: the version-specific install
    port: 55432                     # optional
    socket_dir: /tmp                # optional
    log_file: /Volumes/ExternalSSD/pgdata/myapp.log   # optional

  fizzy:                            # v0.4.0
    kind: container
    up: ~/.config/fizzy/fizzy-up.sh
    down: ~/.config/fizzy/fizzy-down.sh
    containers: [fizzy-db, fizzy-app]
    data_path: /Volumes/ExternalSSD/container-data
```

**Rules**

- Target names: lowercase letters, digits and `-`. The order in the file is the order of `list`,
  `status` and `stop --all` (reversed for stopping).
- `~` is expanded in every path field.
- Unknown keys, unknown kinds and missing required keys are errors that name the target and key.
  DockGuard doesn't guess.
- Every target is checked to be on the drive: its main path (`storage_path`, `data_dir`,
  `data_path`, or each `symlink_paths` destination) must resolve to somewhere under `drive`.

**Legacy v0.1 config:** a file with the old top-level keys (`external_mount_path`,
`docker_storage_path`, `docker_desktop:`) and no `targets:` is still read. It maps to `drive` and
a single `docker` target. `status` prints one line suggesting the new format. A file that mixes
both formats is an error.

**Defaults (D-012):** no personal paths in defaults, examples or docs. They use
`/Volumes/ExternalSSD`. Personal configs keep their real paths.

---

## 4. Kinds

Every kind uses the shared checks in §5 and adds its own.

### 4.1 `docker-desktop` (v0.2.0, existing behaviour)

| | |
| --- | --- |
| Required keys | `storage_path` |
| Optional keys | `settings_path` (auto-detected as in v0.1), `require_cli_start_support`, `fail_if_already_running` |
| Checks | Storage path exists and is writable; free space; settings file exists and points at `storage_path`; not already running (guard) |
| Start | `docker desktop start` (needs Docker Desktop 4.37+) |
| Stop | `docker desktop stop` |
| Running | `docker desktop status` |

### 4.2 `app` (v0.3.0)

| | |
| --- | --- |
| Required keys | `app` (the name `open -a` takes) |
| Optional keys | `process` (default: `app`), `symlink_paths`, `fail_if_already_running` (default true) |
| Checks | Each symlink path resolves to somewhere on the drive. A broken link fails. A plain folder that is **not** a link fails too, with "not on the drive", because it means the app's data has silently been recreated on the internal disk. |
| Start | `open -a <app>` |
| Stop | Asks the app to quit (`osascript -e 'quit app "<app>"'`), which lets it save. No force-quit. |
| Running | `pgrep -x <process>` |

**CoreSimulator (D-013):** `~/Library/Developer/CoreSimulator/Devices` is never accepted in
`symlink_paths`; the config is rejected with the reason. Moving it to an external drive breaks
`simctl create` with no workaround.

### 4.3 `postgres` (v0.3.0, proposal P-3)

For a PostgreSQL cluster run directly with `pg_ctl`, which is how the Homebrew PostgreSQL 17 in the
current setup is used (not through `brew services`). Each cluster is its own target, so
several projects can each have one.

| | |
| --- | --- |
| Required keys | `data_dir`, `bin_dir` |
| Optional keys | `port`, `socket_dir`, `log_file`, `fail_if_already_running` (default true) |
| Checks | `pg_ctl` and `postgres` exist in `bin_dir`; `data_dir` exists on the drive and is writable; free space; `data_dir/PG_VERSION` matches the major version of `<bin_dir>/pg_ctl --version`; no stale `postmaster.pid` (a PID file whose process isn't running fails with how to clear it); the port isn't used by another process |
| Start | `pg_ctl -D <data_dir> -l <log_file> -o "-p <port> -k <socket_dir>" -w start` |
| Stop | `pg_ctl -D <data_dir> -m fast -w stop` (no immediate mode, so no recovery needed on next start) |
| Running | `pg_ctl -D <data_dir> status` |

`bin_dir` is required, never taken from the PATH: Homebrew's `libpq` puts its own `pg_ctl` on the
PATH, and its major version can differ from the cluster's (18 against 17 in the current setup).
Running the wrong major version against a cluster fails or, worse, misreports its state.

DockGuard only passes the options in the config. It never edits `postgresql.conf`. A MySQL kind
can follow the same pattern if one is ever needed; Fizzy's MySQL is a container (§4.4).

### 4.4 `container` (v0.4.0)

| | |
| --- | --- |
| Required keys | `up`, `down`, `containers` |
| Optional keys | `data_path`, `fail_if_already_running` (default true) |
| Checks | `up` and `down` exist and are executable; `container` is on the PATH; `data_path` exists on the drive; Apple container's data folder (`~/Library/Application Support/com.apple.container`) resolves to the drive |
| Start | Runs `up` |
| Stop | Runs `down` |
| Running | Every name in `containers` is running (`container list`) |

**Fizzy at login (D-014):** the launchd agent that runs `fizzy-up.sh` at
login is removed. Fizzy starts with `dockguard start fizzy` instead, so it never
starts while the drive is missing. The v0.4.0 release notes include the removal steps.

---

## 5. Shared checks

The building blocks in `internal/checks`, reused by every kind:

| ID | Check |
| --- | --- |
| C1 | Drive is configured and mounted |
| C2 | Path exists |
| C3 | Path is writable |
| C4 | Free space at or above the minimum |
| C5 | Symlink resolves |
| C6 | Path resolves to somewhere under the drive |
| C7 | Command is on the PATH |
| C8 | Target isn't already running (guard) |

C1 always runs first. If the drive isn't mounted, the other checks on that drive are reported as
"skipped: drive not mounted" instead of as separate failures.

---

## 6. Code layout

```text
internal/
├── app/            # command parsing and dispatch only
├── config/         # yaml.v3 loading, validation, legacy mapping
├── checks/         # shared checks C1–C8
├── output/         # text output
├── platform/       # macOS commands: diskutil, open, pgrep, osascript, lsof
└── target/
    ├── target.go   # Target interface and registry
    ├── dockerdesktop/
    ├── app/        # v0.3.0
    ├── postgres/   # v0.3.0
    └── container/  # v0.4.0
```

```go
type Target interface {
    Name() string
    Kind() string
    MainPath() string                 // shown by list
    Checks() []checks.Item            // preflight, without the already-running guard
    IsRunning() (State, error)        // Running, Stopped or Unknown
    Start() error
    Stop() (stopped bool, err error)  // false if it was already stopped
}
```

All operating-system commands go through `internal/platform` behind function variables, so tests
replace them (as `dockerdesktop` does today). Other operating systems would be added there later
(D-010); none are built now.

---

## 7. Releases

### v0.2.0 — Target layer

1. `yaml.v3` config, with legacy mapping and validation (§3)
2. `Target` interface and registry; Docker Desktop moved onto it with **no change in behaviour**
   apart from the already-running rule (§2)
3. `list`; `status` across all targets; target argument on `check`, `start` and `stop`
4. Eject refuses while another target on the drive is running
5. Generic defaults and docs (D-012)
6. Config lookup with `~/.config/dockguard/dockguard.yaml`; `list` and `status` name the file
   (D-015)

### v0.3.0 — Apps and PostgreSQL

1. `app` kind; Xcode in the example config
2. `postgres` kind (P-3); a sample cluster in the example config
3. The CoreSimulator rule (§4.2, D-013)

### v0.4.0 — Containers

1. `container` kind; Fizzy in the example config
2. `stop --all [--eject]`
3. Fizzy's launchd agent removed (D-014); release notes give the steps

Each release: tests written with the change, `go vet` and `go test ./...` clean, CHANGELOG and
release notes, tag after One's review.

---

## 8. Acceptance scenarios

**v0.2.0**

- **AS-01** Given a v0.1 config, when I run `dockguard check`, then I get the same checks and
  result as v0.1.2.
- **AS-02** Given a v0.1 config, when I run `dockguard status`, then it works and suggests the new
  format in one line.
- **AS-03** Given a config with `targets:` and the old top-level keys, then loading fails and names
  both.
- **AS-04** Given an unknown kind or key, then loading fails and names the target and the key.
- **AS-05** Given two targets, when I run `dockguard list`, then both are listed in config order
  with kind, state and path, and no checks run.
- **AS-06** Given the drive is unmounted, when I run `dockguard list`, then every target shows
  `drive not mounted` and the exit code is 0.
- **AS-07** Given Docker Desktop is running, when I run `dockguard status`, then Docker shows
  `running` and isn't counted as not ready.
- **AS-08** Given Docker Desktop is running and the guard is on, when I run `dockguard start`, then
  it refuses, as in v0.1.
- **AS-09** Given an unknown target name, then the command fails, lists the valid names and exits 1.
- **AS-10** Given another target on the drive is running, when I run `dockguard stop docker
  --eject`, then Docker stops, the drive isn't ejected, and the running targets are named.
- **AS-21** Given no `--config` and no `./dockguard.yaml`, when I run `dockguard list` from any
  folder, then it reads `~/.config/dockguard/dockguard.yaml` and prints that path.
- **AS-22** Given no config file anywhere, when I run `dockguard list`, then it fails and names the
  paths it tried.

**v0.3.0**

- **AS-11** Given Xcode's DerivedData links to the drive and the drive is mounted, when I run
  `dockguard start xcode`, then the checks pass and Xcode opens.
- **AS-12** Given the drive is unmounted, when I run `dockguard start xcode`, then it refuses and
  Xcode doesn't open.
- **AS-13** Given DerivedData is a plain folder on the internal disk, then `check xcode` fails with
  "not on the drive".
- **AS-14** Given `symlink_paths` includes CoreSimulator/Devices, then loading fails with the reason.
- **AS-15** Given `myapp-db` with its data folder on the drive, when I run `dockguard start
  myapp-db`, then `pg_ctl ... start` runs only after the checks pass, with the configured port,
  socket folder and log file.
- **AS-16** Given `PG_VERSION` says 17 and `<bin_dir>/pg_ctl --version` says 18, then `check myapp-db`
  fails and shows both values.
- **AS-17** Given the cluster is already stopped, when I run `dockguard stop myapp-db`, then it
  says so and exits 0.
- **AS-25** Given the cluster was started by hand with `pg_ctl`, not by DockGuard, then `list`
  shows it as running and `stop myapp-db --eject` stops it before ejecting.
- **AS-23** Given a `postmaster.pid` whose process isn't running, then `check myapp-db` fails and
  says how to clear it.

**v0.4.0**

- **AS-18** Given the Fizzy containers are running, then `list` shows `fizzy` as running.
- **AS-24** Given I log in with the drive unplugged, then nothing tries to start Fizzy.
- **AS-19** Given Docker, Postgres and Fizzy are running, when I run `dockguard stop --all
  --eject`, then they stop in reverse config order and the drive is ejected.
- **AS-20** Given one target fails to stop, then `stop --all --eject` doesn't eject and names the
  target.

---

## 9. Out of scope

- Linux and Windows (designed for, not built: D-010)
- More than one drive
- Protecting apps from the drive being unplugged while they run
- Changing an app's own settings (Docker's disk location, Postgres's data folder, Xcode's paths)
- Moving data between drives
- A background service, menu bar app or launchd agent
- Renaming the project (D-009)

---

## 10. Proposals needing approval

P-1 and P-2 were approved on 2026-11-08 and are now D-012 and D-013.

| # | Proposal | Why |
| --- | --- | --- |
| P-3 | Replace the `brew-service` kind in D-007 with a `postgres` kind that uses `pg_ctl` (§4.3) | PostgreSQL 17 in the current setup is the Homebrew build but runs directly with `pg_ctl -D <drive>/.../pgdata/<cluster>` on a custom port. `brew services` reports it as stopped, so a `brew-service` kind could never see or control it. Homebrew-managed services can come back as a kind if one is ever used. |

---

## 11. What happens to the uncommitted Xcode work

The June 2026 changes on the `master` checkout (an `xcode` command, symlink checks,
`internal/xcode/`, list support in the config parser) are the starting point for the v0.3.0 `app`
kind:

- `checkSymlinkResolvable` becomes shared check C5 and gains C6 (on the drive).
- `xcode.Open` and `xcode.IsRunning` move to `internal/platform` as generic `open -a` and `pgrep`.
- The top-level `xcode:` config section and the `dockguard xcode` command are dropped in favour of
  `targets.xcode` and `dockguard start xcode`.
- The hand-written list parsing goes away with `yaml.v3`.
- CoreSimulator/Devices is removed from the example, and the personal drive defaults are reverted (D-012, D-013).
- The test Dockerfile and compose file are dropped: tests run with plain `go test ./...`.

Three moves this work onto a `feat/` branch in its own worktree before v0.3.0; nothing is
committed from the `master` checkout.

---

## 12. Open questions

None open. Answered on 2026-11-08:

| # | Question | Answer |
| --- | --- | --- |
| O1 | Fizzy's launchd agent | Remove it; DockGuard starts Fizzy (D-014) |
| O2 | Where PostgreSQL's data lives | One cluster folder per project under `<drive>/Projects/pgdata/`, run with `pg_ctl`, which led to P-3 |
| O3 | User-level config | Yes: `~/.config/dockguard/dockguard.yaml` is the record of managed apps (D-015) |
