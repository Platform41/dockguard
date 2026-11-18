# Proposal — Target dependencies (`depends_on`) and the container-system target

**Status:** Approved by Azrad, 2026-11-18; released in v0.8.0 (2026-11-19)
**Target:** v0.8.0
**Owner:** One
**Builder:** Three
**Date:** 2026-11-18
**Amends:** D-017 (`stop_system`)

## 1. Problem

One DockGuard command should act on one target. Today, shared prerequisites are hidden inside
scripts and special options:

- Every Apple `container` stack's `up` script runs `container system start` itself (for example
  Fizzy and the AWS Emulator recipe), so starting one stack quietly starts the shared container
  system.
- `stop_system: true` (D-017) makes stopping one stack also stop the whole container system, and
  `stop --all` needs a special two-pass rule (stacks first, then shared systems).

DockGuard can't say "fizzy needs the container system", so it can neither check that before starting
Fizzy nor protect running stacks when the system is stopped.

## 2. Outcome

- A target can declare the targets it needs: `depends_on: [container]`.
- The Apple container system becomes its own target kind, `container-system`.
- **One command acts on one target by default.** When a dependency is in the way, DockGuard **asks**
  (`[y/N]`, default No) before also starting or stopping the others; without a terminal it refuses
  with the exact command to run.
- `stop --all` and eject use the dependencies to order work: dependents stop before what they need.
- `stop_system` and the two-pass rule are replaced by the general mechanism, with one release of
  backward compatibility.

## 3. Product behaviour

### 3.1 Config

```yaml
targets:
  container:
    kind: container-system
    data_path: /Volumes/ExternalSSD/container-data   # optional, as for container stacks

  fizzy:
    kind: container
    depends_on: [container]
    up: ~/.config/fizzy/fizzy-up.sh      # no longer runs `container system start`
    down: ~/.config/fizzy/fizzy-down.sh
    containers: [fizzy-db, fizzy-app]
```

- `depends_on` is an optional list of target names, allowed on every kind.
- Config errors, each naming the target: unknown name; a target depending on itself; a cycle
  (named in order, e.g. `a → b → a`); duplicate entries.
- **`container-system` kind:**
  - **start:** `container system start`, then confirmed `running` from `container system status`;
  - **stop:** `container system stop`. It refuses while **unmanaged** containers are still running,
    naming them, because stopping the system stops every container (the D-017 safety rule, moved
    here);
  - **state:** `running` / `stopped` (`unregistered`) / `unknown`, exactly as the container kind reads
    the system today;
  - **checks:** the `container` CLI is on the PATH; the Apple container data folder is on the drive
    (and `data_path` checks if set, as for container stacks);
  - **eject:** a running container system blocks eject, as `stop_system` makes it do today.

### 3.2 Starting a target whose dependency isn't running

Dependencies are checked **before** the target's own preflight. "Not running" means any state other
than `running` (stopped, partial or unknown).

Interactive terminal:

```text
$ dockguard start fizzy
fizzy needs container, which is stopped.
Start container first? [y/N] y
container start requested.
…
fizzy start requested.
```

- Only `y` / `yes` (any case) proceeds. Enter, `n`, anything else or EOF prints
  `Not started. Start it first: dockguard start container` and exits 1.
- **On yes:** each missing dependency runs its own full preflight and start, in dependency order
  (deepest first), and must be confirmed `running` before the next. If any fails, DockGuard stops
  there, starts nothing further, and exits 1. Dependencies already running are left alone.
- **Chains** are one question listing the order: `Start docker, then container, first? [y/N]`.
- **Not a terminal:** no prompt; the same refusal message, exit 1.
- **Chain refusal (clarified 2026-11-19):** list only dependencies that are not running,
  deepest first. One missing dependency keeps the exact message above. For two or more:

  ```text
  Not started. Start these first, in order:
    dockguard start docker
    dockguard start container
  ```

  Each command is on its own line, ready to copy and paste. The chain question remains
  `Start docker, then container, first? [y/N]`.
- **Unknown dependency state:** never offered. It refuses, showing the probe error, because DockGuard
  can't confirm what it would be starting over.

### 3.3 Stopping a target that running targets depend on

```text
$ dockguard stop container
fizzy and aws-emulator depend on container and are running.
Stop them first? [y/N] y
aws-emulator stop requested.
fizzy stop requested.
container stop requested.
```

- Same answer rules and non-terminal refusal
  (`Not stopped. Stop these first: fizzy, aws-emulator`), exit 1.
- **On yes:** dependents stop in reverse dependency order (with their own `down`, ephemeral notices
  etc.); each must be confirmed stopped before the next; then the named target. Any failure stops
  the sequence; the named target is not stopped; exit 1.
- "Running" here means any state other than `stopped`, so partial and unknown dependents also count.
- `stop … --eject` prompts the same way before stopping; the eject rule itself is unchanged (D-019).

### 3.4 `stop --all` and eject

- `stop --all [--eject]` stops every target that isn't confirmed stopped, in **reverse dependency
  order**: a target is stopped only after every running target that depends on it. Independent
  targets keep reverse config order. No prompts (`--all` already names everything).
- If a target fails to stop, the targets it depends on are **not** stopped (they're still in use), and
  the run reports both; any failure still means no eject.
- This replaces the special two-pass rule for shared systems.
- **Legacy combination (clarified 2026-11-19):** target stops always honor dependency
  order, including configs that use `depends_on` without a `container-system` target.
  With no dependencies this is the same reverse config order as v0.7.x. After all
  target stops, only configs without a `container-system` target run the legacy
  `StopSystem` pass, with the same conditions, messages and failure handling.

### 3.5 Dashboard

- Each row shows its dependencies dimmed after the kind, e.g. the `fizzy` row shows
  `container  → container` (kind `container`, depends on the `container` target).
- `s` on a target with a dependency that isn't running opens a confirmation with the §3.2 question.
  `x` on a target with running dependents opens one with the §3.3 question. Both use the existing
  confirmation mechanism; a "no" changes nothing.

### 3.6 Backward compatibility (`stop_system`, D-017)

- Configs **without** a `container-system` target behave exactly as in v0.7.x, including
  `stop_system` and the two-pass `stop --all`. `list` and `status` print one deprecation line per
  target that uses it: `stop_system is deprecated; add a container-system target and depends_on
  (see README).`
- When such a config declares `depends_on`, the first pass follows dependency order
  and protects prerequisites of failed targets; the legacy system pass still runs
  after all targets. With no `depends_on`, target ordering and output are unchanged
  from v0.7.x, apart from the stated deprecation lines in `list`/`status`.
- A config that has a `container-system` target **and** `stop_system: true` anywhere is a config
  error naming the target: the two mechanisms would conflict.
- `stop_system` is removed in the release after v0.8.0.

### 3.7 Docs and the example config

README: `depends_on`, the `container-system` kind, the prompts, the ordering rule, and a migration
section: add `container: {kind: container-system}`, add `depends_on: [container]` to each container
stack, remove `container system start` from their `up` scripts and remove `stop_system`.
`examples/dockguard.yaml` is updated the same way.

## 4. Architecture

- `internal/config`: `DependsOn []string` on every target; `KindContainerSystem`; validation
  (unknown, self, cycle, duplicate); the `stop_system` + `container-system` conflict.
- `internal/target/containersystem` (new): start/stop/state/checks, reusing the container kind's
  system-status probe and the "other containers running" check.
- `internal/app`: a dependency graph built from the config; topological order for start chains and
  `stop --all`; prompt through the existing TTY check and an injectable reader (as `dockguard
  version` does); `runStart`/`runStop` gain the §3.2/§3.3 flows; the two-pass path kept only for
  legacy configs (§3.6).
- `internal/ui`: dependency column; confirmations reuse the existing `pending` mechanism.

## 5. Out of scope

| Not proposed | Why |
| --- | --- |
| Starting or stopping dependencies without asking (`--with-deps`, auto-start) | One target per command; the prompt covers it |
| Dependencies on things outside the config (processes, files, URLs) | Targets only |
| A `docker` dependency for Docker Desktop–based targets | Possible later with the same mechanism; no current need |
| Parallel starts or stops | Order and confirmation matter more than speed |

## 6. Acceptance scenarios

| ID | Scenario | Expected result |
| --- | --- | --- |
| AS-105 | `depends_on` with an unknown name, itself, a cycle or a duplicate | Config error naming the target (cycle shown in order) |
| AS-106 | `container-system` start/stop/state/checks | As §3.1; stop refuses while unmanaged containers run, naming them |
| AS-107 | Start a target whose dependency is running | No prompt; starts as today |
| AS-108 | Start with a stopped dependency, terminal, answer `y` | Dependency preflight + start, confirmed running, then the target |
| AS-109 | Same, answer Enter / `n` / other / EOF | `Not started. Start it first: …`; nothing started; exit 1 |
| AS-110 | Same, not a terminal | No prompt; refusal; exit 1 |
| AS-111 | Two-level chain stopped, answer `y` | One question listing the order; started deepest first |
| AS-112 | Dependency start fails after `y` | Sequence stops; target not started; exit 1 |
| AS-113 | Dependency state unknown | Refused with the probe error; no prompt |
| AS-114 | Stop a target with running dependents, answer `y` | Dependents stopped in reverse order, confirmed, then the target |
| AS-115 | Same, answer no or not a terminal | `Not stopped. Stop these first: …`; nothing stopped; exit 1 |
| AS-116 | A dependent fails to stop after `y` | Sequence stops; named target not stopped; exit 1 |
| AS-117 | `stop --all` with dependencies | Reverse dependency order; no prompts; a failed dependent keeps its dependencies running; no eject on failure |
| AS-118 | `stop --all --eject` with everything stoppable | All stopped in order; eject rule as D-019 |
| AS-119 | Legacy config with `stop_system`, no `container-system` target | v0.7.x behaviour and output, plus one deprecation line in `list`/`status` |
| AS-120 | `container-system` target plus `stop_system` | Config error naming the target |
| AS-121 | Dashboard `s` / `x` with a dependency in the way | §3.2 / §3.3 question as a confirmation; "no" changes nothing |
| AS-122 | Config without any `depends_on` | Output and behaviour identical to v0.7.x |

## 7. Test plan

Test-first with fakes (no real containers or drives): config validation; graph ordering, including
chains and independent targets; prompt answers through an injected reader and TTY check; failure
mid-sequence; `stop --all` ordering and failure propagation; legacy `stop_system` regression; the
dashboard confirmations. Live check (maintainer): migrate the real config (`container` target,
`depends_on` on each container stack; `container system start` removed from their
scripts), then `start fizzy` with the system stopped (answer y), `stop container` with stacks running
(answer y), and `stop --all --eject`.

## 8. Approved decision

Approved for v0.8.0 as **D-024** (amends D-017), all four items:

1. `depends_on` on every target and the `container-system` kind (§3.1);
2. interactive `[y/N]` (default No) to start missing dependencies and to stop running dependents;
   refusal with the command when not a terminal (§3.2, §3.3);
3. dependency-ordered `stop --all` replacing the two-pass rule (§3.4);
4. `stop_system` deprecated for one release, conflicting configs rejected (§3.6).
