# Siftd Runbook

## Purpose

This runbook defines the first operator-ready deployment shape for `siftd`.

Target model:

- one Linux host;
- one `systemd` service;
- one Postgres database;
- one Zitadel application for bearer-token auth;
- one reverse proxy in front of `127.0.0.1:8080` if public HTTPS is required.

This is intentionally a single-node operating model.
The repository also publishes a container image for GitOps and cluster deployment, but
this runbook remains the host-first operating baseline.

## Repository Assets

- systemd unit: `ops/systemd/siftd.service`
- environment template: `ops/env/siftd.env.example`
- binary entrypoint: `cmd/siftd/main.go`
- container image build: `Dockerfile`
- CI image publish workflow: `.github/workflows/publish-image.yml`

## Container Artifact

The repository publishes:

- image: `ghcr.io/heurema/sift`
- mutable convenience tag: `latest`
- rollout tag format: `dev-<sha7>-<timestamp>`

The hosted GitOps path should pin one of the `dev-*` tags. `latest` is for manual
inspection and ad-hoc runs only.

## Required Inputs

Before deployment, have all of the following:

- a Linux host with `systemd`;
- Go `1.25.5`;
- reachable Postgres DSN for the shared Sift database;
- reachable Zitadel issuer URL;
- Zitadel audience for the API application;
- one allowed browser origin if WebSocket is used from a browser UI;
- repo checkout path `/srv/sift/current`.

## Expected Host Layout

Use this fixed layout:

- repo checkout: `/srv/sift/current`
- service env file: `/etc/sift/siftd.env`
- systemd unit: `/etc/systemd/system/siftd.service`
- derived output and runtime data: `/var/lib/sift`

The service writes digest projections to:

- `/var/lib/sift/output/digests/crypto/24h.{json,md}`
- `/var/lib/sift/output/digests/crypto/7d.{json,md}`

## Bootstrap

Create the service user and directories:

```bash
sudo useradd --system --home /var/lib/sift --shell /usr/sbin/nologin sift
sudo install -d -o sift -g sift /etc/sift /var/lib/sift /srv/sift
```

Deploy the repository and binary:

```bash
git clone https://github.com/heurema/sift.git /srv/sift/current
cd /srv/sift/current
git checkout main
go build -o /srv/sift/current/siftd ./cmd/siftd
sudo chown -R sift:sift /var/lib/sift
```

Install the service assets:

```bash
sudo install -m 0644 ops/systemd/siftd.service /etc/systemd/system/siftd.service
sudo install -m 0640 ops/env/siftd.env.example /etc/sift/siftd.env
sudo systemctl daemon-reload
```

Edit `/etc/sift/siftd.env` before first start.

## Environment Contract

Required variables:

- `SIFTD_POSTGRES_DSN`
- `SIFTD_ZITADEL_ISSUER`
- `SIFTD_ZITADEL_AUDIENCE`

Recommended variables:

- `SIFTD_ADDR=127.0.0.1:8080`
- `SIFTD_REGISTRY=/srv/sift/current/docs/contracts/source-registry.seed.json`
- `SIFTD_OUTPUT_DIR=/var/lib/sift/output`
- `SIFTD_SYNC_INTERVAL=5m`
- `SIFTD_SYNC_TIMEOUT=4m`
- `SIFTD_RETENTION=720h`
- `SIFTD_SYNC_ON_START=true`
- `SIFTD_WS_ALLOWED_ORIGINS=https://console.example.com`

Operational notes:

- `SIFTD_RETENTION=720h` means `30d`.
- `SIFTD_SYNC_TIMEOUT` must stay below `SIFTD_SYNC_INTERVAL`.
- if `SIFTD_WS_ALLOWED_ORIGINS` is set, browser clients must send one of those origins;
- keep `SIFTD_ADDR` bound to localhost unless TLS termination is handled directly in-process.

## Start and Verify

Enable and start the service:

```bash
sudo systemctl enable --now siftd
sudo systemctl status siftd
```

Watch logs:

```bash
journalctl -u siftd -f
```

Health verification:

```bash
curl -fsS http://127.0.0.1:8080/healthz
curl -i http://127.0.0.1:8080/readyz
```

Expected behavior:

- `/healthz` returns `200` once the process is alive;
- `/readyz` returns `503` until the first successful sync;
- after the first successful sync, `/readyz` returns `200` with `last_run_id`.

Authenticated API verification:

```bash
ACCESS_TOKEN=replace-me
curl -fsS \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  "http://127.0.0.1:8080/v1/events?limit=5"
```

## Upgrade Procedure

For a normal deploy:

```bash
cd /srv/sift/current
git fetch --all
git checkout main
git pull --ff-only
go build -o /srv/sift/current/siftd ./cmd/siftd
sudo systemctl restart siftd
```

Post-upgrade checks:

```bash
sudo systemctl status siftd
curl -fsS http://127.0.0.1:8080/healthz
curl -i http://127.0.0.1:8080/readyz
```

## Rollback

Rollback is binary-and-restart:

```bash
cd /srv/sift/current
git checkout <known-good-commit>
go build -o /srv/sift/current/siftd ./cmd/siftd
sudo systemctl restart siftd
```

If the issue is configuration-only, revert `/etc/sift/siftd.env` and restart.

## Failure Cases

If startup fails immediately:

- inspect `journalctl -u siftd -n 200`;
- confirm `/etc/sift/siftd.env` exists and is readable by root/systemd;
- confirm the binary path `/srv/sift/current/siftd` exists;
- confirm Postgres DSN and Zitadel issuer/audience are present.

If `/readyz` stays degraded:

- inspect logs for sync failures;
- confirm source registry path exists;
- confirm Postgres migrations succeeded;
- confirm remote sources are reachable from the host.

If browser WebSocket connection fails:

- confirm the client uses `Authorization: Bearer <token>`;
- confirm request `Origin` matches `SIFTD_WS_ALLOWED_ORIGINS`;
- confirm the reverse proxy forwards the WebSocket upgrade headers unchanged.
