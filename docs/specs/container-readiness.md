# Proposal — Container Readiness and Ephemeral Targets

**Status:** Approved by Azrad, 2026-11-15
**Target:** v0.6.0
**Owner:** One
**Builder:** Three
**Date:** 2026-11-15

## 1. Context

AWS Emulator is a local, account-free AWS API endpoint (S3, DynamoDB, SQS and others) shipped as
a Linux aarch64 RPM. On 2026-11-15 the build then served as `LTS/latest` (`aws-emulator --version`:
release date 2026.11.14, commit `813af48`) was run on the T7 through Apple container with no Docker
involved, and wired into DockGuard as a configuration-only `container` target:

```yaml
aws-emulator:
  kind: container
  up: ~/.config/dockguard/aws-emulator-up.sh
  down: ~/.config/dockguard/aws-emulator-down.sh
  containers: [aws-emulator]
  data_path: /Volumes/T7SSD/container-data
```

What the live check established:

| Check | Result |
| --- | --- |
| `container build` of the AWS-documented Dockerfile | Built in ~60 s; image stored under `/Volumes/T7SSD/container-data` |
| `dockguard start` / `list` / `stop` | All work unchanged; 9/9 preflight checks pass |
| S3, DynamoDB, SQS round trips from macOS | Pass; unsigned requests get 403 |
| Health endpoint `https://127.0.0.1:9297/__admin/health` | `ok` (200) |
| Restart | Clean state: all resources are held in memory only |
| Graceful stop | The emulator deletes its per-run credential files |
| Idle memory | ~900 MiB of a 2 GiB limit |

**Conclusion: DockGuard needs no AWS-specific code.** The existing `container` kind (D-017)
already owns lifecycle, storage placement and eject safety (D-019). Two gaps showed up that are
not AWS-specific and would apply to any container target:

1. **Start is not readiness.** `Start()` runs the `up` script and reports `start requested`. A
   container can be `running` while the service inside is still booting, has crashed on start, or
   is unreachable from the host. Nothing tells the user "this is usable now".
2. **Stopping silently discards data for some targets.** For a database, stop is safe: data is on
   the drive. For an in-memory service such as AWS Emulator, stop destroys every bucket, table and
   queue. DockGuard presents both the same way.

## 2. Outcome

- A container target may declare a readiness check. `dockguard start` then reports the target as
  ready, or fails clearly when it doesn't become ready in time.
- A container target may declare itself ephemeral. DockGuard says so wherever the user stops it.
- A documented recipe makes AWS Emulator reproducible without anyone rediscovering today's steps.

## 3. Product behaviour

### 3.1 Readiness (`ready`, `ready_timeout_seconds`)

New optional keys on the `container` kind:

```yaml
aws-emulator:
  kind: container
  # ...existing keys...
  ready: ~/.config/dockguard/aws-emulator-ready.sh
  ready_timeout_seconds: 30        # default 30; allowed 1–300
```

- `ready` is a script, matching the `up`/`down` pattern. DockGuard does not learn HTTP, TLS or CA
  handling; the script does (`curl --cacert … /__admin/health`).
- After `up` succeeds, DockGuard runs `ready` once per second until it exits 0 or the timeout
  passes. Each attempt is bounded by 5 s or the time left before the overall deadline, whichever
  is shorter; the overall deadline always wins. An attempt that hits its bound is killed and
  counts as not ready. A stuck `ready` script can never block `start` past
  `ready_timeout_seconds`.
- Success: `aws-emulator started and ready (2.1 s).`
- Timeout: `aws-emulator started but not ready after 30 s.` and exit 1. DockGuard does **not**
  stop the container automatically; the user decides, and `stop` behaves as today.
- No `ready` key: behaviour and output are unchanged (`start requested`).
- Preflight adds `ready script executable` when `ready` is set.
- `list`, `status` and the dashboard's refresh cycle do **not** run `ready`. Readiness is checked
  at start only, so the three-second refresh stays cheap and the D-019 eject rule is untouched.
- Dashboard start actions reuse the CLI path, so they gain the same readiness result.

### 3.2 Ephemeral targets (`ephemeral`)

```yaml
aws-emulator:
  kind: container
  # ...existing keys...
  ephemeral: true                  # default false
```

- `list` and the dashboard mark the target: `aws-emulator  container  running (ephemeral)`.
- `stop` prints, before running the target's `down` script:
  `aws-emulator is ephemeral: its data is discarded on stop.`
- `stop --all [--eject]` prints the same line for every ephemeral target whose `down` script it
  runs.
- The notice is tied to `down` actually running, and is the only output change. Whenever `down`
  does not run, each command keeps its existing v0.5.1 output exactly and prints no notice:
  - fully stopped target (system down): `… is already stopped.`;
  - single-target `stop` where `stop_system` stops only the shared system (stack already stopped,
    `Stop()` returns true): `… stop requested.`;
  - `stop --all` where the stack is already stopped but the system is up: first pass prints
    `… is already stopped.`, second pass prints `…: container system stop requested.`.
- No additional CLI confirmation prompt and no new flag; the line is information, and scripts keep
  working. Existing dashboard stop/eject confirmations (`x`, `E`; multi-target spec AS-33) remain
  unchanged.
- `ephemeral` changes no state logic, eject readiness or stop ordering.

### 3.3 AWS Emulator recipe (docs only)

Add `docs/recipes/aws-emulator.md` with:

- the Dockerfile (base `amazonlinux:2023-minimal`, aarch64 RPM), with the emulator version and the
  RPM's SHA-256 recorded instead of an unpinned `LTS/latest`;
- the `up`, `down` and `ready` scripts used in the live check, with `--telemetry off` and the
  credential folder mounted at the same path in the container (`-e HOME="$HOME"
  -v "$HOME/.aws/emulator:$HOME/.aws/emulator"`), so the generated `config` works from macOS;
- the client environment (`AWS_CONFIG_FILE`, `AWS_SHARED_CREDENTIALS_FILE`);
- the known limits: in-memory state, ~900 MiB idle memory, API coverage varies by service.

## 4. Architecture

- `internal/config`: add `Ready string`, `ReadyTimeoutSeconds int`, `Ephemeral bool` to
  `config.Container`; validate the timeout range and expand `~` like `up`/`down`.
- `internal/platform`: add a context-aware variant of `RunDetached` (for example
  `RunDetachedContext(ctx, name, args...)`) built on `exec.CommandContext`. It keeps the
  temporary-file output handling, so a child that inherits stdout/stderr can't hold a pipe open,
  and reports a deadline as its own error, distinct from a non-zero exit. The existing
  `RunDetached` (no timeout, used by `up`/`down`/`pg_ctl`) is unchanged. Killing the attempt must
  not leave the poll loop waiting on the child's descendants.
- `internal/target/container`: after a successful `up`, poll `ready` through a new
  `runDetachedContext` seam, faked in tests the way `runDetached` is. Derive each attempt's context
  from the overall deadline context with the 5 s cap. Return a typed not-ready error so
  `internal/app` can print the right message and exit code.
- Discard notice ownership: `StopStack()` emits it, immediately before `runDetached(Down)` and
  after its own stopped-stack probe, so only a real `down` invocation produces it. The app can't do
  this: `runStop`/`runStopAll` only see the result after `Stop()`/`StopStack()` return. The
  container target gets an optional callback (for example `OnDiscard func(name string)`) that the
  app wires to a new `output.PrintEphemeralDiscard`. A callback, not a writer captured at
  construction, so the dashboard's redirection of `output.Out` still applies at call time. Nil
  callback or `ephemeral: false` means no notice. `internal/target/container` does not import
  `internal/output`.
- `internal/app` / `internal/ui`: print the readiness result and render the `(ephemeral)` marker.
  They do not print the discard notice. No new packages.
- Readiness stays in the `container` kind. If another kind needs it later, lift it then.

## 5. Out of scope

| Not proposed | Why |
| --- | --- |
| An AWS Emulator–specific target kind or adapter | The live check proved the generic kind is enough |
| Running readiness in `list`/`status`/dashboard refresh | Cost and noise; revisit only if live use asks for it |
| Persisting or snapshotting emulator state | The emulator doesn't support it; bootstrap scripts are the user's project concern |
| Compose files or a multi-service workspace manifest | Larger product question; the `up` script already composes containers |
| Removing the `docker-desktop` kind | Moving the user's own workloads off Docker is a config change, not a code change |
| Checking that the image exists before `up` | `up` already fails with the CLI's error; low value |

## 6. Acceptance scenarios

| ID | Scenario | Expected result |
| --- | --- | --- |
| AS-46 | `ready` set; service ready within timeout | `started and ready (N s)`, exit 0 |
| AS-47 | `ready` set; never ready | `started but not ready after N s`, exit 1, container left running |
| AS-48 | `ready` script not executable | Preflight fails; `up` is not run |
| AS-49 | `up` fails | Existing failure; `ready` is never run |
| AS-50 | No `ready` key | Output and exit code identical to v0.5.1 |
| AS-51 | `ready_timeout_seconds` outside 1–300 | Config error naming the target and key |
| AS-52 | `list` / dashboard with `ready` set | `ready` is not executed |
| AS-53 | `ephemeral: true` target listed | State shows `(ephemeral)` |
| AS-54 | `stop` / `stop --all` on a running ephemeral target | Discard notice printed before `down` runs; no additional CLI prompt; dashboard `x`/`E` confirmations unchanged |
| AS-55 | `ephemeral` with eject | Eject readiness identical to D-019 |
| AS-56 | `stop` on a fully stopped ephemeral target (system down) | `… is already stopped.` as in v0.5.1; no discard notice |
| AS-57 | `ready` script hangs past both the 5 s attempt cap and the overall deadline | Attempt is killed; `start` returns not-ready at `ready_timeout_seconds`, never later |
| AS-58 | Single-target `stop` on an ephemeral target with `stop_system`, stack already stopped, system up | `… stop requested.` as in v0.5.1; `down` not run; no discard notice |
| AS-59 | `stop --all` with an ephemeral target whose stack is already stopped and system is up | `… is already stopped.` then `…: container system stop requested.` as in v0.5.1; no discard notice |

## 7. Test plan

Three starts from failing tests for AS-46 to AS-59:

- config tests for the new keys, defaults, `~` expansion and the timeout range;
- platform tests for the context-aware detached runner: deadline kills a hanging command and is
  reported distinctly from a non-zero exit; output still captured through the temporary file;
- container target tests using the fake `machine`: ready on attempt N, never ready, per-attempt
  cap, overall deadline beating the cap, discard callback invoked only immediately before `down`
  (fully stopped, single-target system-only and `stop --all` system-only cases call it zero times), `up` failure short-circuit, no-`ready` regression;
- app/ui tests for messages, exit codes and the `(ephemeral)` marker;
- live check on the T7 with the `aws-emulator` target: start reports ready; a `ready` script
  pointed at a wrong port times out; a `ready` script that sleeps forever returns at the deadline; stop prints the discard notice; `stop --all --eject` still
  ejects.

## 8. Release

Minor release `v0.6.0`: new optional configuration keys, no change for existing configs. Update the
README's `container` kind section and link the recipe.

## 9. Approved decision

Approved for `v0.6.0`, all three items:

1. `ready` / `ready_timeout_seconds` on the `container` kind, checked at start only;
2. `ephemeral` on the `container` kind, informational only;
3. the AWS Emulator recipe in `docs/recipes/`.

Recorded as D-020.
