# Proposal — Persistent Volumes on Container Targets

**Status:** Proposed
**Target:** next minor release
**Owner:** One
**Builder:** Three
**Date:** 2026-11-15

## 1. Problem

A container stack's persistent data is invisible in DockGuard. Example: the `fizzy` target runs
`fizzy-app` and `fizzy-db`; the database lives in the Apple container named volume `fizzy-mysql`,
stored as a sparse disk image under Apple container's data folder on the drive
(`<data_path>/volumes/fizzy-mysql/volume.img`). `dockguard list` shows `fizzy` and its state, but
nothing tells the user that a database volume exists, where it is, whether it is still there, or
how much space it uses.

## 2. Decision on shape

**A volume is shown as part of its target, not as a target of its own.**

A second lifecycle target for the same database would overlap the stack's `up`/`down`, `State`,
`stop --all` ordering and the eject rule (D-019): two rows could disagree about one container,
and stopping one could stop the other. A volume has no lifecycle DockGuard should drive; it only
needs to be **visible**. So this proposal adds information only. It changes no state, check,
action, stop order or eject decision.

## 3. Product behaviour

### 3.1 Config

New optional key on the `container` kind only:

```yaml
fizzy:
  kind: container
  up: ~/.config/fizzy/fizzy-up.sh
  down: ~/.config/fizzy/fizzy-down.sh
  containers: [fizzy-db, fizzy-app]
  data_path: /Volumes/ExternalSSD/container-data
  volumes: [fizzy-mysql]
```

- `volumes` is a list of Apple container named-volume names, shown in config order.
- Names must match `^[A-Za-z0-9][A-Za-z0-9_.-]*$` (no `/`, no `..`), with no duplicates or empty
  entries.
- `volumes` requires `data_path`: the volume file is found at
  `<data_path>/volumes/<name>/volume.img`. Missing `data_path` is a config error naming the target.
- Other kinds reject `volumes` (unknown key).

### 3.2 Volume status

Derived from `stat` of `<data_path>/volumes/<name>/volume.img` only. DockGuard never runs
`container volume …`, so the status is available whether or not the container system is running.

| Status | Condition | Shown as |
| --- | --- | --- |
| present | Regular file exists | `present, <used>` where `<used>` is the allocated size (`st_blocks × 512`, e.g. `268 MB`), not the sparse logical size |
| missing | File does not exist | `missing` |
| unknown | Any other `stat` error | `unknown (<error>)` |
| drive not mounted | The drive isn't mounted | `drive not mounted`, as targets show today |

`missing` is information, not an error: a first start may create the volume.

### 3.3 `dockguard list`

After a target's row, one sub-row per volume, in config order, in the same table:

```text
NAME             KIND       STATE              PATH
fizzy            container  running            /Volumes/ExternalSSD/container-data
  └ fizzy-mysql  volume     present, 268 MB    /Volumes/ExternalSSD/container-data/volumes/fizzy-mysql
warisan-db       postgres   stopped            /Volumes/ExternalSSD/Projects/pgdata/warisan
```

- PATH is the volume's folder, `<data_path>/volumes/<name>`.
- Targets without `volumes` print exactly as today.

### 3.4 `dockguard status`

Under the target's heading, after its checks, one information line per volume:

```text
fizzy (container): running
- [ok] …existing checks…
  volume fizzy-mysql: present, 268 MB (/Volumes/ExternalSSD/container-data/volumes/fizzy-mysql)
```

Volume lines are not checks: they never change the summary or the exit code, including `missing`
and `unknown`.

### 3.5 What does not change

- A volume is not a target: `dockguard start|stop|check fizzy-mysql` is an unknown target, as today.
- `start`, `stop`, `stop --all`, preflight, guard, eject readiness, the dashboard and `ephemeral`
  behave exactly as in v0.6.0. No volume is created, deleted, pruned or resized.

## 4. Architecture

- `internal/config`: `Volumes []string` on `config.Container`; `volumes` in `allowedKeys` for the
  container kind; validation per §3.1.
- `internal/target/container`: `Volumes() []VolumeStatus` using an injectable `stat` seam
  (default `os.Stat` plus `syscall.Stat_t.Blocks`); no CLI calls.
- `internal/app`: `runList` appends sub-rows; `runStatus` prints info lines. Both skip the lookup
  and print `drive not mounted` when the drive isn't mounted.
- `internal/output`: a sub-row form of `Row` (name prefix `  └ `, kind `volume`) and a volume info
  line printer; size formatting in decimal units (`268 MB`, `1.2 GB`).

## 5. Out of scope

| Not proposed | Why |
| --- | --- |
| A separate lifecycle target for the database or volume | Overlapping state and stop with the stack (§2) |
| Discovering volumes from `container inspect` | Needs the system running; relies on CLI output details |
| Dashboard volume rows | Keep the first change to `list`/`status`; revisit on request |
| Volume backup, prune, delete or resize | Data-destructive; not DockGuard's job |
| Free-space checks per volume | Drive free space is already checked |
| Volumes for other kinds (Postgres data_dir already is the path) | No need |

## 6. Acceptance scenarios

| ID | Scenario | Expected result |
| --- | --- | --- |
| AS-78 | `volumes: [fizzy-mysql]`, file present | `list` sub-row `└ fizzy-mysql  volume  present, <used>  <path>` directly under `fizzy` |
| AS-79 | Volume file missing | Sub-row `missing`; `fizzy` row, state and exit code unchanged |
| AS-80 | `stat` fails other than not-exist | Sub-row `unknown (<error>)`; exit code unchanged |
| AS-81 | Drive not mounted | Sub-row `drive not mounted`; no `stat` attempted |
| AS-82 | Container system stopped | Status still reported from disk; no `container` CLI call made |
| AS-83 | Sparse 512 GiB image with 268 MB allocated | Shows allocated size (`268 MB`), not 512 GiB |
| AS-84 | `status` with volumes | One info line per volume after the target's checks; summary and exit code unchanged, also when `missing` |
| AS-85 | `volumes` without `data_path`, invalid name, duplicate, or on another kind | Config error naming the target and key |
| AS-86 | Target without `volumes` | `list`/`status` output byte-identical to v0.6.0 |
| AS-87 | `start`/`stop`/`stop --all --eject`/dashboard with volumes configured | Behaviour and output identical to v0.6.0; a volume name is not a target |

## 7. Test plan

- Config tests for AS-85.
- Container target tests with an injected `stat` for present (with blocks), missing, error.
- App/output tests: exact `list` table for AS-78/79/80/81/86, `status` lines and exit codes for
  AS-84, and a fake `container` CLI asserting zero calls for AS-82.
- Regression: existing `start`/`stop`/`stop --all`/eject and dashboard tests pass unchanged (AS-87).
- Live check (maintainer): add `volumes: [fizzy-mysql]` to the real `fizzy` target; `list` with
  the container system running and stopped.

## 8. Decision requested

Approve as D-022:

1. persistent volumes shown as information on their container target, not as a separate target;
2. `volumes` config key resolved under `data_path`, status from disk only;
3. `list` sub-rows and `status` info lines; dashboard unchanged.
