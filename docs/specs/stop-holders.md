# Proposal — Stopping processes and containers that block the drive

**Status:** Approved by Azrad, 2026-11-25; released in v0.9.0 (2026-11-26)
**Target:** v0.9.0
**Owner:** One
**Builder:** Three
**Date:** 2026-11-25
**Revised:** 2026-11-25 (maintainer answers: Finder stoppable, quit defaults to Yes, dashboard in scope; cross-review findings applied)
**Amends:** D-019 (holders are named, never closed by DockGuard), D-024 §3.4 (unmanaged containers
refuse the container-system stop)

## 1. Problem

DockGuard can tell the user *what* is holding the drive, but not do anything about it. Since v0.5.1
(D-019), `stop --eject` and the dashboard name every process holding the volume and then leave the
user to close each one. Since v0.8.0 (D-024), stopping the container system is refused while
containers DockGuard doesn't manage are running.

In practice a full "stop everything and eject" often takes several rounds of manual work:

- the iOS Simulator service keeps files inside an Xcode installed on the drive open;
- an Apple container build leaves its `buildkit` builder running, so the container-system stop is
  refused;
- a shell whose current directory is on the drive holds the volume.

Each round is: run DockGuard, read the names, find and stop each process or container by hand, run
DockGuard again.

## 2. Outcome

When processes or unmanaged containers are in the way, DockGuard offers to stop them, one at a
time, in the CLI and in the dashboard. Nothing is stopped without an answer from a person: the
polite quit and container stop default to Yes, the force-quit defaults to No. Processes DockGuard
must not touch are still named, with the reason.

Stopping a process can lose work (an editor with unsaved changes, a shell running a job). This is
the first DockGuard action with that risk, so the rules below err towards asking and refusing.

## 3. Product behaviour

### 3.1 Which processes can be stopped

DockGuard only ever considers processes the existing holder scan reports as holding the mounted
volume (D-019's `lsof` query). Each holder is classified:

| Class | Rule | Offered? |
| --- | --- | --- |
| **stoppable** | Owned by the current user and not in any class below | Yes |
| **other user** | Owned by root or another user | No — named, with "owned by <user>; DockGuard doesn't use sudo" |
| **self** | DockGuard's own PID and every ancestor (its shell, the terminal app) | No — named, with "relaunch DockGuard from outside the drive" for a shell holding it via its current directory |
| **protected** | Built-in list matched on the executable path, not the `lsof` command name: `launchd`, `kernel_task`, `WindowServer`, `loginwindow`, `Dock`, `SystemUIServer`, `mds`, `mds_stores`, `mdworker*` (prefix, covers `mdworker_shared`), `fseventsd`, and only when the executable lives under `/System/`, `/usr/` or `/sbin/` | No — named, with a fixed hint (for example "Spotlight is indexing the drive; wait or exclude it in System Settings") |
| **managed runtime** | Executable in Apple container's install (its CLI or apiserver, or a binary under `libexec/container-plugins/` or `libexec/container/plugins/`), under `/System/Library/Frameworks/Virtualization.framework/`, or inside `Docker.app/Contents/`. Match the executable path, including resolved symlinks, rather than the reported command name. | No — named, with "stop it through its DockGuard target or container stop" |

The protected list is fixed in code, not in config, for this release. Finder is deliberately not
on it: it's stoppable, and the question says that macOS relaunches it straight away, which is
usually enough to release a Finder window open on the drive. Ancestors are found by
walking parent PIDs from DockGuard's own process. Classification runs **after** DockGuard has
released its own current-directory hold (D-019 §3.3), in the CLI commands of §3.4 as well as the
dashboard; if that release can't be done safely, DockGuard's own PID stays a named self holder
with the relaunch hint, as D-019 already specifies. No sudo: DockGuard has never needed elevated privileges (D-021).

### 3.2 How a process is stopped

For each stoppable holder, in PID order:

```text
nvim (PID 48213) is using /Volumes/ExternalSSD. Quit it? [Y/n]
```

Enter alone means Yes for this question: the user has already asked to stop or eject, and a polite
quit lets the application save or ask about unsaved work.

On yes:

1. **Re-verify** that the PID still exists, still has the same command name and start time, and
   still holds the volume. If not, report "already gone" or "no longer using the drive" and move
   on. This prevents signalling a recycled PID.
2. Send `SIGTERM` (an ordinary quit request).
3. Wait up to 5 seconds. Check whether the process still exists about every 250 ms with
   `kill(pid, 0)`, which is cheap; run the holder scan at most once a second to see whether it has
   released the volume, and once more at the end of the wait.
4. If it still holds the volume, ask a second question, default No:

   ```text
   nvim (PID 48213) didn't quit. Force-quit it? Unsaved work will be lost. [y/N]
   ```

   Enter alone means No for this question, because a force-quit can lose work.

   On yes, re-verify as in step 1 again (the PID may have been recycled during the wait), then
   send `SIGKILL` and confirm the process is gone.

On no, the holder is left running and reported as still blocking. A failed signal (for example
`EPERM`) is reported with the error and treated as still blocking.

### 3.3 Unmanaged containers

When stopping the container system is refused because containers outside any DockGuard target are
running (D-024 §3.4), DockGuard asks about each one. This applies to **every** stop of a
`container-system` target: `stop <container-system target>`, the container-system step of
`stop --all`, with or without `--eject`, and the dashboard (clarified 2026-11-25). It does not
apply to the deprecated `stop_system` pass for configs without a `container-system` target, which
keeps v0.8.0's refusal until it's removed.

```text
Container buildkit is running and isn't managed by DockGuard. Stop it? [Y/n]
```

Enter alone means Yes: `container stop` is reversible.

On yes it runs `container stop <name>`, never `container delete` and never a signal to the
container runtime's PIDs, which could damage the virtual machine. After the answers, DockGuard
re-checks; if no unmanaged containers remain, the container-system stop proceeds. Containers stopped
this way can be started again with `container start <name>`, so this is printed after each stop.

### 3.4 Entry points

**`stop --all --eject` and `stop <target> --eject`.** Targets are stopped first, as today. If the
holder preflight then finds holders:

- in a terminal: name every holder with its class, then ask about each stoppable one (§3.2). After
  the last answer, re-scan. If nothing holds the volume, eject as today. Otherwise refuse, naming
  what's left, exit 1, and don't call `diskutil`;
- not in a terminal: refuse exactly as today, adding the command a user would run
  (`kill <pid>`) for each stoppable holder.

The same applies to the container-system step of `stop --all`, using §3.3.

This amends D-024 §3.4, which says `stop --all` asks no questions. Target stops in `stop --all`
still ask nothing. The only questions are the ones in this proposal: about holders in the eject
preflight, and about unmanaged containers at the container-system step. Without a terminal both
keep v0.8.0's refusal.

**`dockguard holders`** (new). Prints the holders of the mounted drive with their class, or "no
processes are using the drive". Exit 0 when there are no holders, 1 when there are, 2 when the scan
fails. It never stops anything.

**`dockguard holders --stop`** (new). The same list, then the §3.2 questions for each stoppable
holder, then a re-scan. Refuses without a terminal. Exit codes as `holders`, judged on the re-scan; a failed or timed-out
re-scan exits 2.

Holder questions (§3.2) are only asked in `--eject` flows and `holders --stop`; other commands
don't scan or stop holders. Container questions (§3.3) are asked wherever a container-system target
is stopped. `list` and `status` don't change.

### 3.5 Dashboard

The dashboard asks the same questions with its existing confirmation area (the one D-024 uses for
dependency questions). `y` answers yes, `n` answers no, Enter takes the question's default, and
`esc` cancels the remaining questions without stopping anything else.

- **`E` (stop all and eject, D-018):** after targets stop, holders and unmanaged containers in the
  way are asked about in turn (§3.2, §3.3), then the eject preflight runs again and ejects if
  clear. `x` keeps stopping the selected target.
- **`p` (new, processes):** asks about every stoppable holder without ejecting. Shown in the key
  help only while the drive is mounted and the last holder scan found holders.
- The Eject line keeps naming every holder with its class (D-019).
- A stop sequence is a dashboard action like any other: it runs as a command, the dashboard's
  existing one-action-at-a-time rule applies, and status refresh pauses until it ends (as for every
  action today). Input stays responsive, so `esc` can cancel the remaining questions during a
  5-second wait; the process already signalled is still waited for and reported.

### 3.6 Answer defaults and D-024

D-024's dependency questions default to No. These questions differ on purpose: a polite quit and a
container stop are what the user asked for and are low-risk, so they default to Yes; only the
force-quit, which can lose work, defaults to No. D-024's questions don't change.

## 4. Architecture

- `internal/platform`:
  - extend `Holder` with `UID`, `User` and `PPID`, read from the same `lsof` call by adding the
    `u`, `L` and `R` fields to `-F`. `lsof` emits fields in its own per-process order, not the
    requested order, so the parser starts a new holder on each `p` line and resets the other fields
    there. `L` can be missing when the login name can't be resolved; the user is then shown as the
    numeric UID;
  - add process inspection (exists, command name, start time) and `Signal(pid, sig)`, isolated so
    tests can replace them;
  - add `Ancestors(pid)` walking parent PIDs.
- New `internal/holders` (or within `internal/app`): classification (§3.1), the stop sequence
  (§3.2) with injected clock, signaller and prompt, and the result per holder
  (`stopped`, `force-stopped`, `declined`, `gone`, `failed`, `not offered`).
- `internal/target/containersystem`: return the unmanaged container names as structured data, not
  only an error string, so §3.3 can ask about them. The prompt itself stays in `internal/app`, next
  to D-024's `askDependency`, and reuses its terminal check and input handling (EOF is never
consent). The answer defaults come from §3.6, not from `askDependency`'s default No (clarified
2026-11-25).
- `internal/app`: wire `holders`, `holders --stop` and the eject and `stop --all` flows.
- `internal/ui`: the dashboard's `Controller` interface gains methods to list the stoppable holders
  and unmanaged containers in the way and to run one stop step with the user's answer; the model
  keeps a queue of questions for `E` and the new `p` key, reusing D-024's confirmation rendering
  with a per-question default.

## 5. Out of scope

- sudo, or stopping root-owned or other users' processes.
- A config-driven protected list or allow-list.
- Stopping holders without a terminal (no `--yes` flag in this release).
- Killing container runtime processes, `container delete`, or stopping containers that belong to a
  configured target outside its own `down` script.

## 6. Acceptance scenarios

| ID | Scenario | Expected |
| --- | --- | --- |
| AS-123 | `holders` with nothing on the drive | "no processes are using the drive", exit 0 |
| AS-124 | `holders` with holders of every class | Each named with its class and hint, exit 1; nothing signalled |
| AS-125 | `holders` when the scan fails or times out | Inspection error, exit 2 |
| AS-126 | `holders --stop`, user answers no | Nothing signalled; holder reported as still blocking; exit 1 |
| AS-127 | `holders --stop`, yes, process quits on SIGTERM | "quit"; re-scan clean; exit 0; no SIGKILL question |
| AS-128 | Yes, process ignores SIGTERM, user answers yes to force-quit | SIGKILL sent after re-verification; "force-quit"; exit 0 when clean |
| AS-129 | Yes, process ignores SIGTERM, user answers no to force-quit | No SIGKILL; reported as still blocking; exit 1 |
| AS-130 | PID exited or was reused between scan and signal | No signal sent; "already gone"; continues |
| AS-131 | Process releases the drive but keeps running after SIGTERM | Counted as released; no force-quit question |
| AS-132 | Root-owned or other user's holder | Not offered; named with owner and "doesn't use sudo" |
| AS-133 | DockGuard's own shell holds the drive (current directory) | Not offered; named with the relaunch hint |
| AS-134 | Holder on the protected list (for example `mds` under `/System/`) | Not offered; named with its fixed hint |
| AS-135 | Signal fails (EPERM) | Error reported; counted as still blocking |
| AS-136 | `holders --stop` without a terminal | Refused, with `kill <pid>` per stoppable holder; nothing signalled |
| AS-137 | `stop --all --eject` in a terminal, holders all stopped | Re-scan clean; `diskutil eject` called |
| AS-138 | `stop --all --eject` in a terminal, one holder declined | Refused, naming it; exit 1; `diskutil` not called |
| AS-139 | `stop --all --eject` without a terminal, holders present | Same refusal as v0.8.0 plus `kill <pid>` lines |
| AS-140 | Container-system stop refused by an unmanaged container, user answers yes | `container stop <name>` run; restart hint printed; system stop proceeds |
| AS-141 | Same, user answers no | Refusal as in v0.8.0; nothing stopped |
| AS-142 | Unmanaged container stop fails | Error reported; system stop refused |
| AS-143 | No holders and no unmanaged containers | Output and behaviour identical to v0.8.0 |
| AS-144 | Enter alone at the quit or container question | Treated as yes |
| AS-145 | Enter alone at the force-quit question | Treated as no; no SIGKILL |
| AS-146 | Finder holds the drive | Offered; question says macOS relaunches it |
| AS-147 | Dashboard `E` with stoppable holders, all answered yes | Holders stopped, re-scan clean, drive ejected |
| AS-148 | Dashboard `E`, `esc` at a question | Remaining questions dropped; nothing more stopped; no eject |
| AS-149 | Dashboard `p` with holders | Same questions as `holders --stop`; no eject; Eject line refreshed |
| AS-150 | Dashboard `p` with no holders, or drive unmounted | Key not shown; pressing it does nothing |
| AS-151 | Dashboard during a 5-second quit wait | Input stays responsive; `esc` cancels remaining questions; refresh resumes after the sequence |
| AS-152 | PID recycled during the 5-second wait, user answers yes to force-quit | No SIGKILL; "already gone" |
| AS-153 | Dashboard `E` with an unmanaged container blocking the container-system stop | Container question shown; yes runs `container stop`; system stop and eject proceed |
| AS-154 | A user binary named `mds` outside `/System/`, `/usr/`, `/sbin/` holds the drive | Classed stoppable, not protected |
| AS-155 | `lsof` output without an `L` field for a holder | Owner shown as the numeric UID; classification unchanged |
| AS-156 | `holders --stop` when the re-scan fails | Exit 2 with the inspection error |
| AS-157 | `stop <container-system target>` (no `--eject`) blocked by an unmanaged container, in a terminal | Container question shown; yes runs `container stop`; system stop proceeds |
| AS-158 | Legacy config (`stop_system`, no `container-system` target) with an unmanaged container | v0.8.0 refusal; no question |
| AS-159 | Apple container runtime at `/opt/homebrew/Cellar/container/1.7.1/libexec/container-plugins/container-runtime-linux/bin/container-runtime-linux` and macOS VM at `/System/Library/Frameworks/Virtualization.framework/Versions/A/XPCServices/com.apple.Virtualization.VirtualMachine.xpc/Contents/MacOS/com.apple.Virtualization.VirtualMachine` hold the drive | Both named with the managed-runtime class and hint. Neither is offered in `holders`, `holders --stop`, the eject flow, or dashboard `p`. |
| AS-160 | A Docker Desktop VM process under `Docker.app/Contents/` holds the drive | Named as managed runtime and never offered. |
| AS-161 | A user executable named `container-runtime-linux` outside the runtime paths holds the drive | Classed stoppable. |

## 7. Test plan

- Unit tests with injected scanner, process inspector, signaller, clock and prompt cover AS-123 to
  AS-146 and AS-152 to AS-161 without signalling real processes; dashboard model tests cover
  AS-147 to AS-151 and AS-153.
- One integration test starts a throwaway child process (`sleep` with its current directory in a
  temporary folder treated as the mount path), stops it through the real signaller, and covers a
  child that traps SIGTERM to exercise the force-quit path.
- Live check on a real drive: a `sleep` holding the volume, an editor with an unsaved file to
  confirm the force-quit warning, and a throwaway unmanaged container blocking the system stop.
  Never run the live eject from a worktree on the drive itself.

## 8. Approved decision

Approved for v0.9.0 as **D-025** (amends D-019 and D-024 §3.4):

1. classification of holders, with only the current user's non-protected, non-self processes
   offered (§3.1);
2. per-process questions: quit `[Y/n]` (SIGTERM), then after 5 seconds an optional force-quit
   `[y/N]` (SIGKILL), with re-verification before every signal (§3.2, §3.6);
3. per-container `[Y/n]` to `container stop` unmanaged containers blocking the container-system
   stop (§3.3);
4. offered inside `--eject` flows and at the container-system step of `stop --all` (amending
   D-024 §3.4's no-questions rule only for these questions), through new `holders` /
   `holders --stop` commands (refused without a terminal), and in the dashboard through `E` and a
   new `p` key (§3.4, §3.5).
