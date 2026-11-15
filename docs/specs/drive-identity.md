# Proposal — Registered Drive Identity and `dockguard mount`

**Status:** Proposed
**Target:** v0.7.0
**Owner:** One
**Builder:** Three
**Date:** 2026-11-15

## 1. Problem

DockGuard trusts a path, not a drive. Today "mounted" means only that `drive` (for example
`/Volumes/ExternalSSD`) exists as a directory (`checks.DriveMounted`). That is wrong in three
cases macOS produces on its own:

1. **Stray folder.** A leftover empty folder at the mount path counts as mounted. Targets then
   start and write to the internal disk, and Apple container or Xcode symlinks resolve into it.
2. **Same name, different volume.** Any volume with the same name mounts at the same path and is
   trusted, including a replacement or borrowed drive.
3. **Shifted mount point.** When the path is already taken, macOS mounts the real drive at
   `/Volumes/<name> 1`. DockGuard reports the drive as not mounted (or, with a stray folder, as
   mounted at the wrong place) and gives no hint why.

Separately, after DockGuard's eject the APFS volume is unmounted but the USB device often stays
attached. Remounting today means `diskutil list`, finding the right identifier, and
`diskutil mount diskXsY`, with an identifier that changes between connections.

## 2. Outcome

- The drive is registered once by its **volume UUID**. With a UUID registered, DockGuard treats
  the drive as mounted only when that exact volume is mounted at the configured path.
- `dockguard mount` (and the dashboard key `m`) mounts the registered drive in one step and confirms
  it landed at the configured path.
- Plugging in the drive behaves exactly as today. DockGuard does not change auto-mount, needs no
  `sudo`, and never writes the config file (D-015).

## 3. Product behaviour

### 3.1 Registration: `dockguard drive`

New read-only command. It inspects the volume mounted at the configured `drive` path with
`diskutil info -plist <path>` and prints its identity and the line to add:

```text
$ dockguard drive
Drive  /Volumes/ExternalSSD
Volume ExternalSSD (apfs, external)
UUID   0F3C2B1A-9E8D-4C7B-A6F5-1E2D3C4B5A69
Status not registered

To register this drive, add to dockguard.yaml:
drive_uuid: 0F3C2B1A-9E8D-4C7B-A6F5-1E2D3C4B5A69
```

- With a UUID already registered, it prints the identity status from §3.2 instead of the snippet,
  and the snippet only when the mounted volume differs (for a deliberate drive replacement).
- If the path is not a mount point, it says so and exits 1.

### 3.2 Identity status

New optional top-level key `drive_uuid` (a volume UUID, validated as
`XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX`, case-insensitive; invalid is a config error). The legacy
v0.1 format does not accept it.

With `drive_uuid` set, DockGuard derives one status from `diskutil info -plist <uuid>` and, when
needed, `diskutil info -plist <drive>`:

| Status | Condition | Meaning for DockGuard |
| --- | --- | --- |
| `mounted` | The registered volume's mount point is `drive` | Mounted, as today |
| `not mounted` | Registered volume attached, no mount point | Not mounted; offer `dockguard mount` |
| `not connected` | `diskutil` can't find the UUID | Not mounted |
| `mounted elsewhere` | Registered volume mounted at another path | Not mounted; name the actual path |
| `wrong volume` | Another volume is mounted at `drive` | Not mounted; name both UUIDs |
| `stray folder` | `drive` exists but is not a mount point | Not mounted; explain the folder |
| `unknown` | `diskutil` missing, failed, timed out (5 s) or returned unparsable output | Not mounted (fail closed); show the error |

Only `mounted` counts as mounted. Everywhere that uses "mounted" today (preflight, `list`,
`status`, the dashboard, the eject rule) uses this status when `drive_uuid` is set:

- Preflight adds one check after the existing drive checks: `registered drive at mount path`,
  failing with the status and its explanation. Target checks are skipped as they are today when the
  drive isn't mounted.
- `list` and `status` headers show the status; `stop --eject` and dashboard `E` refuse to eject
  unless the status is `mounted`, so DockGuard never ejects a volume it didn't verify.
- Without `drive_uuid`, behaviour is unchanged (directory check) and `list`/`status` print one hint
  line: `Drive not registered; run dockguard drive.`

### 3.3 `dockguard mount`

Requires `drive_uuid`. Acts on the status from §3.2:

| Status before | Action | Output, exit code |
| --- | --- | --- |
| `mounted` | none | `ExternalSSD is already mounted at /Volumes/ExternalSSD.`, 0 |
| `not mounted` | `diskutil mount <uuid>`, then re-read status | `ExternalSSD mounted at /Volumes/ExternalSSD.`, 0 |
| `not mounted`, then lands elsewhere | none further | `mounted at /Volumes/ExternalSSD 1, not /Volumes/ExternalSSD` with the reason, 1 |
| `not connected` | none | `ExternalSSD is not connected; plug it in.`, 1 |
| `mounted elsewhere`, `wrong volume`, `stray folder` | none; refuses before mounting | Explanation naming the paths, 1 |
| `unknown` | none | The inspection error, 1 |
| No `drive_uuid` | none | `No drive registered; run dockguard drive.`, 1 |

- It never deletes folders, renames volumes, unmounts anything or mounts any other volume.
- It does not start targets. After mounting, the user runs `start` as usual.

### 3.4 Dashboard

- The drive line shows the status. For everything but `mounted` it gives the reason in one line,
  e.g. `○ not mounted — press m to mount`, `○ mounted elsewhere: /Volumes/ExternalSSD 1`.
- Key `m` is active and shown in the key help **only** when `drive_uuid` is set and the status is
  `not mounted`. It runs the same code as `dockguard mount`, asks no confirmation (mounting is not
  destructive), shows the result in the action output area, and triggers a fresh snapshot. It
  follows the existing rule that probes and actions never overlap.
- The identity lookup joins the existing snapshot work. Measured cost: about 0.06 s per
  `diskutil info` call, well within the three-second refresh.

## 4. Architecture

- `internal/config`: `DriveUUID string` on `Config`; `drive_uuid` added to the new-format keys;
  validation as in §3.2.
- `internal/platform`: `VolumeInfo(target string) (Volume, error)` wrapping
  `diskutil info -plist` with a 5 s timeout (the context-aware runner from v0.6.0), parsing
  `VolumeUUID`, `VolumeName`, `MountPoint`, `FilesystemType`, `Internal`; and
  `MountVolume(uuid string) error` wrapping `diskutil mount`. Plist parsed with the standard
  library (`plutil -convert json` output or `encoding/xml`); no new module dependencies.
- `internal/checks` (or a small new `internal/drive`): `Status(cfg) DriveStatus` implementing the
  table in §3.2 from injected `VolumeInfo`, so every case is unit-testable without a disk.
- `internal/app`: replace direct `driveMounted(cfg.Drive)` calls with a function that uses the
  status when `drive_uuid` is set and the existing directory check otherwise; add `drive` and
  `mount` commands; the dashboard controller exposes a mount action.
- `internal/ui`: drive line per status; `m` key gated on status.

## 5. Out of scope

| Not proposed | Why |
| --- | --- |
| Preventing macOS auto-mount (`/etc/fstab` `noauto`) | Changes what users are used to when plugging in; needs `sudo` |
| More than one registered drive | Every target would need a drive; no current need |
| Writing `drive_uuid` into the config | Config is user-owned (D-015); DockGuard prints the line |
| Encrypted volumes (unlock) | Would need a password; the target drive is not encrypted |
| Removing stray folders or renaming volumes | Destructive; DockGuard explains, the user acts |
| `stop --unmount` without eject | Eject of an APFS volume often leaves the USB device attached already; revisit if live use shows otherwise |
| Tightening the unregistered directory check | Kept for compatibility; registration is the fix |

## 6. Acceptance scenarios

| ID | Scenario | Expected result |
| --- | --- | --- |
| AS-60 | `dockguard drive`, volume mounted, not registered | Identity and `drive_uuid:` snippet, exit 0 |
| AS-61 | `dockguard drive`, path missing or not a mount point | Says so, exit 1 |
| AS-62 | `drive_uuid` set, registered volume at path | Status `mounted`; preflight check passes; all existing behaviour unchanged |
| AS-63 | `drive_uuid` set, stray folder at path | Status `stray folder`; `start` refused; target checks skipped |
| AS-64 | `drive_uuid` set, other volume at path | Status `wrong volume` naming both UUIDs; `start` and eject refused |
| AS-65 | `drive_uuid` set, registered volume at another path | Status `mounted elsewhere` naming the path; `start` and eject refused |
| AS-66 | `diskutil` fails or times out | Status `unknown` with the error; treated as not mounted |
| AS-67 | `stop --all --eject` with status other than `mounted` | Targets stop as today; eject refused with the status |
| AS-68 | `mount`, status `not mounted` | Calls `diskutil mount <uuid>`; verifies path; exit 0 |
| AS-69 | `mount`, already mounted at path | No `diskutil mount` call; exit 0 |
| AS-70 | `mount`, not connected | `plug it in`, exit 1 |
| AS-71 | `mount`, path is a stray folder or another volume | Refuses before mounting, exit 1 |
| AS-72 | `mount`, volume lands at another path | Exit 1 naming both paths |
| AS-73 | `mount` without `drive_uuid` | Points to `dockguard drive`, exit 1 |
| AS-74 | No `drive_uuid` in config | Existing behaviour; one hint line in `list`/`status` only |
| AS-75 | Invalid `drive_uuid` | Config error naming the key |
| AS-76 | Dashboard, status `not mounted` | Drive line suggests `m`; `m` mounts and refreshes; no confirmation |
| AS-77 | Dashboard, any other status | `m` inactive and absent from key help; drive line explains the status |

## 7. Test plan

- Unit tests for every row of §3.2 and §3.3 with an injected `VolumeInfo` / `MountVolume`; no real
  disks, no real `diskutil` calls in the suite.
- Plist parsing tests from captured `diskutil info -plist` output (mounted, unmounted, not a mount
  point, unknown disk).
- App and UI tests for AS-62 to AS-77, including the `m` gate and the eject refusal.
- Live check on the T7 (maintainer): register; `list`; eject via `stop --all --eject`;
  `dockguard mount` remounts at the same path; dashboard `m` after a dashboard eject; a stray folder
  test is optional and needs the drive disconnected.

## 8. Release

Minor release `v0.7.0`: new optional key and two new commands; no change for configs without
`drive_uuid`. README: registration steps, the status table and `dockguard mount`.

## 9. Decision requested

Approve for `v0.7.0`, as separate items:

1. `drive_uuid` and identity status (§3.2), including the preflight check and eject refusal;
2. `dockguard drive` (§3.1), read-only, printing the snippet;
3. `dockguard mount` (§3.3);
4. dashboard drive-line status and the `m` key (§3.4).

If approved, record D-021.
