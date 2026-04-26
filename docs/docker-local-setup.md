# Docker Desktop Local Setup (External SSD)

This guide standardizes host-side Docker Desktop setup when Docker storage is placed on an external SSD.

Use this for machine setup and recovery. Use DockGuard for project preflight checks before starting Docker Desktop.

## Quick Path (Recommended)

1. Install DockGuard binary with `go install ./cmd/dockguard`.
2. Store config at `~/.config/dockguard/dockguard.yaml`.
3. Set:
   - `external_mount_path: /Volumes/DockerSSD`
   - `docker_storage_path: /Volumes/DockerSSD/docker-data/DockerDesktop`
4. Run with explicit config:
   - `dockguard status --config ~/.config/dockguard/dockguard.yaml`
   - `dockguard start --config ~/.config/dockguard/dockguard.yaml`
   - `dockguard stop --config ~/.config/dockguard/dockguard.yaml --eject`

## Scope and Responsibility

- Docker Desktop app settings, disk format, and storage location are host-level operations.
- DockGuard validates runtime readiness for a project based on configured paths and thresholds.

## Recommended Layout

- External mount: `/Volumes/DockerSSD`
- Active Docker Desktop storage root: `/Volumes/DockerSSD/docker-data`
- Docker Desktop managed path (example): `/Volumes/DockerSSD/docker-data/DockerDesktop`
- Rollback backups: `/Volumes/DockerSSD/docker-rollback/<date>/`

Keep rollback backups separate from active Docker data.

## Preconditions

- macOS with Docker Desktop installed
- External SSD connected over stable USB/Thunderbolt
- External SSD formatted as APFS with GUID partition map (preferred)

Notes:

- `Mac OS Extended (Journaled)` can work, but APFS is preferred for Docker workloads.
- Reformatting erases the disk.

## 1) Optional: Reformat SSD to APFS

Identify the external disk:

```bash
diskutil list
```

Erase the whole device as APFS + GUID (replace `disk4` with your actual disk id):

```bash
diskutil unmountDisk /dev/disk4
diskutil eraseDisk APFS DockerSSD GPT /dev/disk4
```

Verify:

```bash
diskutil info /Volumes/DockerSSD
```

## 2) Create Active and Rollback Folders

```bash
mkdir -p /Volumes/DockerSSD/docker-data
mkdir -p /Volumes/DockerSSD/docker-rollback
```

## 2.5) Install DockGuard as a Native Command

From repository root:

```bash
go install ./cmd/dockguard
```

Ensure Go bin is on your PATH (zsh):

```bash
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

Verify:

```bash
which dockguard
dockguard --help
```

If `dockguard` is still not found, check install paths:

```bash
go env GOBIN
go env GOPATH
ls "$(go env GOPATH)/bin" | grep dockguard
```

When `GOBIN` is empty, binaries are installed to `$(go env GOPATH)/bin`.

## 2.6) Create a User-Level DockGuard Config

Create a reusable config directory:

```bash
mkdir -p ~/.config/dockguard
```

Create `~/.config/dockguard/dockguard.yaml`:

```yaml
external_mount_path: /Volumes/DockerSSD
docker_storage_path: /Volumes/DockerSSD/docker-data/DockerDesktop
minimum_free_space_gb: 100

docker_desktop:
  settings_path: ~/Library/Group Containers/group.com.docker/settings-store.json
  require_cli_start_support: true
  fail_if_already_running: false
```

Run with explicit config:

```bash
dockguard status --config ~/.config/dockguard/dockguard.yaml
dockguard start --config ~/.config/dockguard/dockguard.yaml
dockguard stop --config ~/.config/dockguard/dockguard.yaml --eject
```

Optional shell alias:

```bash
alias dg='dockguard --config ~/.config/dockguard/dockguard.yaml'
```

Then use:

```bash
dg status
dg start
dg stop --eject
```

## 3) Backup Current Docker Desktop Disk Image (Rollback)

Fully quit Docker Desktop first, then back up the VM data directory:

```bash
mkdir -p /Volumes/DockerSSD/docker-rollback/$(date +%F)
cp -av \
  /Users/$USER/Library/Containers/com.docker.docker/Data/vms/0/data \
  /Volumes/DockerSSD/docker-rollback/$(date +%F)/
```

Optional integrity check:

```bash
shasum -a 256 /Volumes/DockerSSD/docker-rollback/$(date +%F)/data/Docker.raw
```

## 4) Move Docker Desktop Disk Image Location

In Docker Desktop:

1. Open Settings.
2. Go to Resources (or the current storage section in your Docker Desktop version).
3. Set disk image location to `/Volumes/DockerSSD/docker-data`.
4. Apply and restart Docker Desktop.

Docker Desktop may create a managed subfolder such as `DockerDesktop`. This is expected.

## 5) Validation Checklist

Run:

```bash
docker info
docker run --rm hello-world
docker system df
```

Confirm:

- Docker daemon is reachable and healthy.
- Test container runs successfully.
- `Docker.raw` exists under `/Volumes/DockerSSD/docker-data/...`.
- Old default path no longer contains the active `Docker.raw`.

## 6) Rollback Procedure

If startup or data behavior becomes unstable:

1. Fully quit Docker Desktop.
2. Restore backed-up `data` directory to:
   `/Users/$USER/Library/Containers/com.docker.docker/Data/vms/0/`
3. Repoint Docker Desktop disk image location to internal/default path.
4. Start Docker Desktop and re-run validation commands.

## 7) Daily Operations Rules

- Keep the external SSD mounted before starting Docker Desktop.
- Do not unplug the SSD while Docker Desktop is running.
- Keep free space above your DockGuard threshold (recommended 50 GB or higher).
- Use DockGuard (`dockguard status`, `dockguard check`, `dockguard start`) before development sessions.
- Before disconnecting the SSD, run `dockguard stop --eject`.

## Troubleshooting

- APFS not shown in Disk Utility:
  - Use View -> Show All Devices.
  - Erase the top-level physical disk with GUID scheme.
  - If needed, use `diskutil eraseDisk APFS <name> GPT /dev/<disk>`.
- Docker starts but pulls/builds fail unexpectedly:
  - Check proxy fields in `docker info` and local network policy.
- Disk image path appears as `.../DockerDesktop` under your selected folder:
  - This is normal Docker Desktop behavior.
