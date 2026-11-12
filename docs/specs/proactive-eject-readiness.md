# Proposal — Proactive Eject Readiness

**Status:** Approved by Azrad, 2026-11-12
**Target:** v0.5.1
**Owner:** One
**Builder:** Three
**Date:** 2026-11-12

## 1. Problem

DockGuard currently uses two different meanings of eject readiness:

- the dashboard says `safe to eject` when every configured target is confirmed stopped;
- the eject command discovers other processes only after `diskutil eject` fails as busy three times.

That makes the dashboard claim more than it has verified. A shell whose current directory is on
the drive, Finder/Quick Look, an editor, or an unconfigured process can still prevent eject while
the dashboard says the drive is safe.

The live case was a `zsh` process whose current directory was
`/Volumes/ExternalSSD/Projects/...`. DockGuard's existing `lsof +f -- <mount>` query found it, so
the gap is when the query runs and how its result participates in readiness. It is not a missing
macOS permission or a failure of the existing parser.

## 2. Outcome

DockGuard reports a drive as ready to eject only when both conditions are freshly confirmed:

1. every configured target is `stopped`; and
2. no other process is holding the mounted volume open.

If DockGuard cannot inspect volume holders, readiness is unknown. It must not display the positive
`safe to eject` state.

This proposal does not turn arbitrary processes into managed targets. DockGuard names them and
leaves the user in control of closing them.

## 3. Product behaviour

### 3.1 Dashboard

Each mounted-drive snapshot includes a holder scan using the existing non-recursive command:

```text
lsof +c 0 -Fpc +f -- /Volumes/ExternalSSD
```

The scan runs concurrently with target probes and is refreshed on the dashboard's existing
three-second cycle. It does not use `lsof +D`; recursive traversal is too expensive and is not
needed for mount-level open-file detection.

The Eject line has four states:

| State | Display |
| --- | --- |
| Drive unmounted | `nothing to eject` |
| Targets or holders present | `blocked by:` followed by every target blocker and process holder |
| Holder scan failed | `readiness unknown: couldn't inspect processes using the drive` |
| All targets stopped and holder scan succeeded with no holders | `safe to eject` |

Examples:

```text
Eject  ✗ blocked by: xcode (running); zsh (PID 23721, using drive)
Eject  ? readiness unknown: lsof unavailable
Eject  ✓ safe to eject
```

Target blockers and process holders remain visibly different. Some duplication is acceptable—for
example, Xcode may appear as a running target and a process holder—because DockGuard must not guess
which process belongs to which configured target.

Long holder lists use the dashboard's existing wrapping behaviour. Holders are ordered by PID,
matching the existing deterministic parser output.

### 3.2 Stop and eject

After all configured targets are freshly confirmed stopped, `stop ... --eject` performs a holder
preflight before calling `diskutil`:

- holders present: refuse immediately, name them, exit 1, and do not call `diskutil`;
- holder inspection failed: refuse with the inspection error and exit 1;
- no holders: call `diskutil eject` as today.

The existing retry and post-failure holder scan remain. A process can open the drive between the
preflight and `diskutil`; the second scan diagnoses that race.

Commands without `--eject` do not gain a holder scan. `list` keeps its current fast, target-only
contract. `status` keeps its current readiness/check contract; this change concerns eject
readiness, not whether applications are ready to run.

### 3.3 DockGuard's own process

An interactive dashboard inherits the shell's current directory. If that directory is on the
external drive, both the dashboard process and its parent shell can hold the volume.

Before starting the dashboard refresh loop, DockGuard should release only its own current-directory
hold when the current directory is inside the configured drive. The implementation must first
finish config loading and preserve the initial working directory for any already-supported
relative configuration semantics, then change DockGuard's working directory to `/`.

The parent shell remains a genuine blocker and is shown. The current DockGuard PID may be excluded
from holder output only after DockGuard has released its own hold. Other DockGuard processes are
not excluded.

If preserving relative-path behaviour cannot be demonstrated, do not change directory silently.
Instead, show DockGuard itself as a blocker and add a clear instruction to relaunch it from outside
the drive. This is the implementation gate for the self-holder case.

## 4. Architecture

Keep process discovery in `internal/platform` and presentation types in `internal/ui`:

- extend the dashboard snapshot with process holders and an optional holder-scan error;
- map `platform.Holder` into a small UI holder value rather than importing platform concerns into
  the renderer;
- run the holder scan alongside target probes only when the drive is mounted;
- centralize filtering by PID in `internal/platform`, with the ignored PID passed explicitly;
- reuse the same preflight function from the dashboard snapshot and eject command.

The positive readiness predicate becomes:

```text
mounted
AND all configured targets stopped
AND holder scan succeeded
AND no process holders
```

Do not cache a positive result across snapshots or use it to skip the final eject preflight.

## 5. Failure and performance rules

- Missing `lsof`, execution failure, or unparseable inspection failure produces unknown readiness.
- Normal `lsof` exit 1 with empty output continues to mean no holders.
- Permission warnings may coexist with useful field output. Preserve the parsed holders, but expose
  an inspection warning if completeness cannot be guaranteed.
- Holder scanning gets a bounded timeout. Proposed default: two seconds, below the three-second UI
  refresh interval. Timeout means unknown readiness.
- A slow holder scan must not freeze input or overlap a later snapshot; existing snapshot
  serialization remains authoritative.
- No process is terminated automatically and no force-eject option is introduced.

## 6. Acceptance scenarios

| ID | Scenario | Expected result |
| --- | --- | --- |
| AS-36 | All targets stopped; no holders | Dashboard shows `safe to eject` |
| AS-37 | All targets stopped; shell cwd is on drive | Dashboard names `zsh (PID ...)`; no safe claim |
| AS-38 | Target running and unrelated holder present | Dashboard shows both blocker categories |
| AS-39 | Holder scan fails or times out | Dashboard says readiness unknown; eject is refused |
| AS-40 | Drive unmounted | No holder scan; dashboard says nothing to eject |
| AS-41 | `stop --all --eject` finds a holder | Names holder, exits 1, does not invoke `diskutil` |
| AS-42 | Holder appears after clean preflight | `diskutil` failure triggers the retained rescan and names it |
| AS-43 | Dashboard launched from a directory on drive | DockGuard releases its own cwd safely or identifies itself with relaunch guidance; parent shell remains visible |
| AS-44 | `list`, `status`, and stop without eject | Existing command semantics and output remain unchanged |
| AS-45 | Holder list exceeds terminal width | Every holder remains visible through deterministic wrapping |

## 7. Test plan

Three starts from failing tests for AS-36 to AS-45:

- platform tests for timeout, scan errors, PID filtering, useful output with warnings, and the
  clean-preflight/race-rescan sequence;
- app controller tests proving holder scans run only for mounted dashboard snapshots and run
  concurrently with target probes;
- UI tests for safe, blocked, mixed, unknown, unmounted, and wrapped states;
- command tests proving a known holder prevents the `diskutil` call;
- a macOS live check with a terminal intentionally left inside a test volume, followed by `cd` out
  and successful readiness/eject.

The live check must record scan duration and confirm that the dashboard remains responsive at the
three-second refresh interval.

## 8. Documentation and release

If approved, record a new decision: eject readiness includes both configured target state and a
fresh process-holder scan. Update the README's eject rule and dashboard example, then ship as a
patch release because this corrects an unsafe/misleading readiness claim without changing target
configuration or command syntax.

## 9. Approved decision

Approved for `v0.5.1`:

1. proactive holder inspection in dashboard snapshots and immediately before eject;
2. fail-closed readiness when holder inspection is unavailable;
3. no holder scans for `list`, `status`, or stop without `--eject`;
4. the guarded self-holder behaviour in §3.3.
