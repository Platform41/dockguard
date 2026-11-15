# Proposal — `dockguard version` and Self-Update

**Status:** Proposed
**Target:** v0.7.0
**Owner:** One
**Builder:** Three
**Date:** 2026-11-15

## 1. Problem

DockGuard can't say which version it is, and users only learn about a new release by checking
GitHub. Updating means remembering the `go install …@version` line from the README.

## 2. Outcome

- `dockguard version` prints the running version, checks for a newer release, and, in an
  interactive terminal, offers to update: `Update to v0.7.0? [y/N]`. Answering `y` installs it.
- No new download channel: the update is the same `go install` the README documents, and the
  module is verified under the user's configured Go module verification policy. With Go's defaults
  for a public module that means the public checksum database (`sum.golang.org`); `GOSUMDB`,
  `GONOSUMDB` or `GOPRIVATE` can change or bypass it, and DockGuard does not override them or
  claim a verification the user's settings didn't perform. DockGuard never contacts the network except when `version`
  runs.

## 3. Product behaviour

### 3.1 Current version (offline)

From the binary's build info (`runtime/debug.ReadBuildInfo`), with no ldflags or build-time
injection:

```text
$ dockguard version
DockGuard v0.6.0 (go1.26.1, darwin/arm64)
```

- A release install shows its tag (`v0.6.0`).
- A build from a checkout shows Go's pseudo-version and the commit, labelled as a development build:
  `DockGuard v0.6.1-0.20261116…-abc1234 (development build, commit abc1234)`. A checkout with
  uncommitted changes adds `, modified`.
- No version information (`(devel)`, e.g. very old toolchains): `DockGuard (version unknown)`.

### 3.2 Latest release check

Immediately after the version line, DockGuard asks the Go toolchain:

```text
go list -m -json github.com/platform41/dockguard@latest
```

This respects the user's `GOPROXY`, `GOPRIVATE`, `GONOSUMDB` and `GOFLAGS`, including
`GOPROXY=off`. It runs from a neutral working directory (the user's home) so a `go.mod` in the
current directory can't affect it, with a 10-second timeout. Measured cost: about 0.1–0.25 s.

| Result | Output | Exit |
| --- | --- | --- |
| Running version ≥ latest (semver, pseudo-versions included) | `You're on the latest release (v0.6.0).` | 0 |
| Latest is newer | `A newer release is available: v0.7.0.` then §3.3 | 0, or per §3.3 |
| `go` not on the PATH | `Can't check for updates: the go command isn't installed.` | 0 |
| Check failed or timed out (offline, `GOPROXY=off`, proxy error) | `Can't check for updates: <reason>.` | 0 |
| Version unknown | Prints latest, offers nothing | 0 |

A failed check never fails the command: `version` is informational.

### 3.3 Update prompt

Shown only when a newer release exists **and** both standard input and output are terminals.

```text
A newer release is available: v0.7.0.
This runs: go install github.com/platform41/dockguard/cmd/dockguard@v0.7.0
and installs to /Users/me/go/bin/dockguard.
Update to v0.7.0? [y/N]
```

- Default is **No**. Only `y` or `yes` (any case) updates; anything else, including Enter and EOF,
  prints `Not updated.` and exits 0.
- On yes: runs exactly that command with the **pinned version** (never `@latest`, so the installed
  version is the one shown) and streams its output.
- **Verification reads the installed file, not the running process.** `runtime/debug.ReadBuildInfo`
  only describes the binary that is running, which is still the old one. After `go install`
  succeeds, DockGuard reads the build info of the file at the install path with
  `debug/buildinfo.ReadFile` and claims success only when that file's module version equals the
  pinned version:

  | Installed file | Output | Exit |
  | --- | --- | --- |
  | Reports the pinned version | `Updated to v0.7.0.` | 0 |
  | Missing | `go install finished, but <path> doesn't exist; not updated.` | 1 |
  | Unreadable or has no build info | `go install finished, but <path> can't be verified: <error>.` | 1 |
  | Reports another version | `go install finished, but <path> reports vX, not v0.7.0.` | 1 |

  If `go install` itself fails, its output is shown and the exit code is 1; nothing is verified.
  In no case does DockGuard say `Updated` without the file reporting the pinned version.
- **Install location.** Go installs to `GOBIN`, or `$(go env GOPATH)/bin`. If the running binary
  (`os.Executable`, symlinks resolved) is a different file, the prompt says so before asking:
  `You're running /usr/local/bin/dockguard; the update goes to /Users/me/go/bin/dockguard and
  won't replace it.` After a successful update the same note is repeated.
- **Not a terminal** (scripts, CI, pipes): no prompt. It prints the command instead:
  `To update, run: go install github.com/platform41/dockguard/cmd/dockguard@v0.7.0` and exits 0.
- No `--yes` flag, no automatic updates, no background checks, no update check in any other
  command or in the dashboard.

## 4. Architecture

- `internal/version` (new, small): `Current() Info` from `debug.ReadBuildInfo`; `Latest(ctx)` via
  an injectable runner for `go list -m -json`; `Newer(current, latest) bool` using semver
  comparison (`golang.org/x/mod/semver`, the Go team's module, is acceptable as the only new
  dependency; a small in-tree comparer is also acceptable); `InstallPath()` from `go env GOBIN
  GOPATH`; `Installed(path) (Info, error)` via `debug/buildinfo.ReadFile` for post-update
  verification (standard library), behind an injectable seam so tests cover missing, unreadable,
  no-build-info and wrong-version files.
- `internal/app`: `version` command; prompt through an injectable reader/TTY check so tests drive
  yes/no/EOF without a terminal; `go install` through the context-aware runner.
- Uses only the `go` command for network access; DockGuard opens no sockets itself.

## 5. Out of scope

| Not proposed | Why |
| --- | --- |
| Prebuilt binary downloads, Homebrew, GitHub Releases | No such channel exists; would need signing and notarization |
| Automatic or background update checks | No network use without an explicit command |
| Update checks in other commands or the dashboard | Keep them offline and fast |
| `--yes` / non-interactive update | Not requested; non-terminal prints the command |
| Updating a copy outside `GOBIN` | DockGuard doesn't overwrite binaries it didn't install; it explains instead |
| Downgrades or choosing a version | `go install …@<version>` already does this |

## 6. Acceptance scenarios

| ID | Scenario | Expected result |
| --- | --- | --- |
| AS-88 | Release build, latest equals current | Version line; `You're on the latest release`; no prompt; exit 0 |
| AS-89 | Development build (pseudo-version, clean and modified) | Version line labelled development build with commit and `, modified` when dirty |
| AS-90 | Newer release, terminal, answer `y`, installed file reports the pinned version | Runs `go install …@<pinned version>`; reads the installed file's build info; `Updated to vX`; exit 0 |
| AS-91 | Newer release, terminal, answer Enter, `n`, other text or EOF | `Not updated.`; no `go install`; exit 0 |
| AS-92 | Newer release, not a terminal | No prompt; prints the `go install` command; exit 0 |
| AS-93 | `go` not on the PATH | Version line and `Can't check for updates…`; exit 0 |
| AS-94 | Check fails or times out (`GOPROXY=off`, offline) | Version line and the reason; exit 0 |
| AS-95 | Running binary differs from the install path | Note shown before the prompt and after the update |
| AS-96 | `go install` fails | Its output shown; exit 1; running binary unchanged |
| AS-97 | Running build newer than latest release (pseudo-version after the tag) | Treated as up to date; no prompt |
| AS-98 | Any other command, the dashboard | No `go` invocation, no network |
| AS-99 | `go install` succeeds but the installed file is missing, unreadable, has no build info, or reports another version | Matching message from §3.3; exit 1; never `Updated` |

## 7. Test plan

- Unit tests for version formatting from fake build info (release, pseudo clean/modified,
  `(devel)`), semver comparison including pseudo-versions (AS-97), and `go list` JSON parsing.
- App tests with injected runner, TTY check, input and installed-file reader for AS-88 to AS-96
  and AS-99; a fake runner asserting
  zero `go` calls from other commands (AS-98).
- Live check (maintainer), right after tagging `v0.7.0`: run a build from the release candidate
  commit (pseudo-version `v0.6.1-0.…`, which sorts below `v0.7.0`); `dockguard version` offers
  `v0.7.0`; answer `n` (nothing changes), then `y`; `dockguard version` then reports `v0.7.0` and
  `You're on the latest release`. Before tagging, the same build must report itself as up to date
  against `v0.6.0` (AS-97).

## 8. Release

Ships in `v0.7.0` with the drive-identity work. README: `dockguard version` in the command table.

## 9. Decision requested

Approve as D-023, as separate items:

1. `dockguard version` with offline version info (§3.1);
2. the latest-release check through the `go` command, only in `version` (§3.2);
3. the interactive `[y/N]` update via pinned `go install` (§3.3).
