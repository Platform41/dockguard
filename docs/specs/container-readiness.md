# Proposal — Container Readiness and Ephemeral Targets

**Status:** Approved by Azrad, 2026-11-15
**Target:** v0.6.0
**Owner:** One
**Builder:** Three
**Date:** 2026-11-15

## 1. Context

AWS released AWS Emulator on 2026-11-14: a local, account-free AWS API endpoint (S3, DynamoDB,
SQS and others) shipped as a Linux aarch64 RPM. On 2026-11-15 it was run on the T7 through Apple
container with no Docker involved, and wired into DockGuard as a configuration-only `container`
target:

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
  passes. Each attempt has its own bounded timeout (5 s).
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
- `stop` prints, before stopping: `aws-emulator is ephemeral: its data is discarded on stop.`
- `stop --all [--eject]` prints the same line for every ephemeral target it stops.
- No confirmation prompt and no new flag. DockGuard stays non-interactive; the line is
  information, and scripts keep working.
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
- `internal/target/container`: after a successful `up`, poll `ready` through the existing
  `runDetached` seam so tests fake it as they fake `up`/`down`. Return a typed not-ready error so
  `internal/app` can print the right message and exit code.
- `internal/app` / `internal/ui`: print the readiness result and the ephemeral notice; render the
  `(ephemeral)` marker. No new packages.
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
| AS-54 | `stop` / `stop --all` on ephemeral target | Discard notice printed before stopping; no prompt |
| AS-55 | `ephemeral` with eject | Eject readiness identical to D-019 |

## 7. Test plan

Three starts from failing tests for AS-46 to AS-55:

- config tests for the new keys, defaults, `~` expansion and the timeout range;
- container target tests using the fake `machine`: ready on attempt N, never ready, per-attempt
  timeout, `up` failure short-circuit, no-`ready` regression;
- app/ui tests for messages, exit codes and the `(ephemeral)` marker;
- live check on the T7 with the `aws-emulator` target: start reports ready; a `ready` script
  pointed at a wrong port times out; stop prints the discard notice; `stop --all --eject` still
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
