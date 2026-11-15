# Recipe — AWS Emulator as a `container` target

Run [AWS Emulator](https://github.com/aws/aws-emulator) (a local, account-free AWS API endpoint
for S3, DynamoDB, SQS and other services) on Apple `container`, with its image stored on the
external drive and its lifecycle managed by DockGuard. No Docker is involved.

Tested on 2026-11-15 with Apple `container` 1.7.1 on Apple silicon, AWS Emulator **1.0.385**
(release date 2026.11.14, commit `813af48`).

## What to expect

- **State is in memory only.** Every bucket, table and queue is gone when the emulator stops; every
  start is a clean environment. The target is marked `ephemeral` so DockGuard says so on stop.
  Recreate test resources with your own bootstrap scripts after `start`.
- **Requests must be signed** with the per-run keys the emulator generates; unsigned requests get
  `403`.
- About **900 MiB** of memory while idle (2 GiB limit below).
- API coverage varies by service. Treat it as a fast local loop, not a replacement for testing
  against AWS.

## Prerequisites

- Apple `container` whose data folder is on the drive (see [Container checks](../../README.md#container-checks)).
- A DockGuard build with `ready` and `ephemeral` support (v0.6.0 or later).
- The AWS CLI or an AWS SDK on macOS, for clients.

## 1. Build the image

Save as `~/.config/dockguard/aws-emulator/Dockerfile`:

```dockerfile
FROM public.ecr.aws/amazonlinux/amazonlinux:2023-minimal

# AWS publishes only an LTS/latest URL. The checksum pins the tested package (1.0.385): if latest
# moves on, the build fails with "digest mismatch" instead of silently changing version.
ADD --checksum=sha256:e2f01620e9f4e8c37c0ce7f32381fde3ffb7e8270331c8c23de2ec877b6a9043 \
    https://downloads.aws.com/aws-emulator/packages/aws-emulator/LTS/latest/aws-emulator-aarch64.rpm \
    /tmp/aws-emulator.rpm

RUN rpm -i /tmp/aws-emulator.rpm \
    && rm /tmp/aws-emulator.rpm \
    && mkdir -p /home/emulator \
    && chown 1000 /home/emulator

USER 1000
ENV HOME=/home/emulator
EXPOSE 9297
ENTRYPOINT ["aws-emulator", "-b", "0.0.0.0"]
```

Build it (about a minute; the image is stored in Apple container's data folder on the drive):

```sh
container system start
container build --platform linux/arm64 -t aws-emulator:local ~/.config/dockguard/aws-emulator
```

`rpm` prints `Header V4 RSA/SHA512 Signature, key ID d940f624: NOKEY`: the package signature is
not verified inside the image. The checksum is what guarantees you get the tested file.

## 2. Scripts

`~/.config/dockguard/aws-emulator-up.sh`:

```sh
#!/bin/sh
# Start AWS Emulator. Readiness is checked by the ready script, not here.
set -eu
PATH=/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin
PORT=9297

container system start
container delete aws-emulator >/dev/null 2>&1 || true
# Per-run credentials and CA left behind by an unclean stop.
rm -rf "$HOME/.aws/emulator/$PORT"
mkdir -p "$HOME/.aws/emulator"
exec container run -d --name aws-emulator --cpus 2 --memory 2G \
  -p "127.0.0.1:$PORT:$PORT" \
  -e HOME="$HOME" -v "$HOME/.aws/emulator:$HOME/.aws/emulator" \
  aws-emulator:local --telemetry off
```

`~/.config/dockguard/aws-emulator-ready.sh`:

```sh
#!/bin/sh
# Exit 0 once the health endpoint answers "ok". DockGuard polls this after up.
PATH=/usr/bin:/bin
STATE="$HOME/.aws/emulator/9297"
[ -f "$STATE/ca.pem" ] || exit 1
[ "$(curl -s --max-time 2 --cacert "$STATE/ca.pem" https://127.0.0.1:9297/__admin/health)" = ok ]
```

`~/.config/dockguard/aws-emulator-down.sh`:

```sh
#!/bin/sh
# Graceful stop (SIGTERM): the emulator removes its per-run credential files. Resources are discarded.
set -eu
PATH=/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin
exec container stop --time 30 aws-emulator
```

```sh
chmod +x ~/.config/dockguard/aws-emulator-*.sh
```

Why these flags:

- `-e HOME="$HOME"` with the folder mounted at the **same path** in the container: the emulator
  writes `config` (including its `ca_bundle` path) and `emulator_credentials` there, so the files
  work from macOS unchanged.
- `-b 0.0.0.0` (in the image) is needed inside the container's VM; `-p 127.0.0.1:…` keeps the port
  reachable from this Mac only. The emulator logs a warning about `0.0.0.0`; that is expected.
- `--telemetry off` disables the emulator's usage telemetry.

## 3. DockGuard target

Add to `~/.config/dockguard/dockguard.yaml`:

```yaml
targets:
  aws-emulator:
    kind: container
    up: ~/.config/dockguard/aws-emulator-up.sh
    down: ~/.config/dockguard/aws-emulator-down.sh
    containers: [aws-emulator]
    data_path: /Volumes/ExternalSSD/container-data
    ready: ~/.config/dockguard/aws-emulator-ready.sh
    ready_timeout_seconds: 30
    ephemeral: true
```

```text
$ dockguard start aws-emulator
...
aws-emulator started and ready (1.2 s).

$ dockguard list
NAME          KIND       STATE                PATH
aws-emulator  container  running (ephemeral)  /Volumes/ExternalSSD/container-data

$ dockguard stop aws-emulator
aws-emulator is ephemeral: its data is discarded on stop.
aws-emulator stop requested.
```

Add `stop_system: true` if nothing else uses Apple container and you want `stop` to stop its system
service too.

## 4. Clients

Point the AWS CLI or an SDK at the files the emulator generates on each start:

```sh
export AWS_CONFIG_FILE=~/.aws/emulator/9297/config
export AWS_SHARED_CREDENTIALS_FILE=~/.aws/emulator/9297/emulator_credentials

aws s3 mb s3://demo
aws dynamodb create-table --table-name demo \
  --attribute-definitions AttributeName=id,AttributeType=S \
  --key-schema AttributeName=id,KeyType=HASH --billing-mode PAY_PER_REQUEST
aws dynamodb wait table-exists --table-name demo   # tables start as CREATING, as on AWS
aws sqs create-queue --queue-name demo
```

These replace your normal AWS configuration only in that shell. The keys change on every start,
so re-read the files rather than copying them.

## Updating the emulator

1. Download the current package and compute its checksum:

   ```sh
   curl -sO https://downloads.aws.com/aws-emulator/packages/aws-emulator/LTS/latest/aws-emulator-aarch64.rpm
   shasum -a 256 aws-emulator-aarch64.rpm
   ```

2. Replace the `sha256:` value in the Dockerfile, rebuild with `container build`, and check
   `container run --rm aws-emulator:local --version`.
3. Update the tested version at the top of this recipe.
