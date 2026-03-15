<div align="center">

  <img src="logo.svg" alt="keel" width="48" height="48"/>

  # keel

  **One binary. Zero dependencies. Full Docker control.**

  <br/>

  [![Go](https://img.shields.io/badge/Go-1.24-00ADD8?style=flat-square&logo=go&logoColor=white)](https://golang.org)
  [![License](https://img.shields.io/badge/license-MIT-green?style=flat-square)](LICENSE)
  [![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macOS-lightgrey?style=flat-square)](https://github.com/getkaze/keel)

  <br/>

  [Install](#install) · [Usage](#usage) · [Features](#features) · [Seeders](#seeders) · [Dev Mode](#dev-mode) · [Remote Targets](#remote-targets) · [Stack](#stack) · [Build](#build)

</div>

---

## What is Keel

**Keel** (the keel of a ship — the hidden structure that keeps everything aligned) is a self-hosted web dashboard for managing Docker environments — local or remote via SSH — from a single Go binary (~10MB, no external dependencies)..

```
keel
```

That's it. Open `http://localhost:60000` and you have a full dashboard with live status, logs, terminal, metrics, and container management.

---

## Install

```bash
curl -fsSL https://getkaze.dev/keel/install.sh | sudo bash
```

This installs the binary to `/usr/local/bin/keel` and creates the data directory at `/var/lib/keel`.

---

## Usage

```bash
# Start the dashboard (default: http://localhost:60000)
keel

# Container operations
keel start                     # start all services
keel start redis mysql         # start specific services
keel stop                      # stop all services
keel stop traefik              # stop specific service
keel reset --all               # destroy and recreate all containers
keel reset redis               # recreate a single service

# Remote targets
keel target                    # show active target
keel target ec2                # switch to remote target

# Dev mode — mount local code into a container with hot reload
keel dev api ~/projects/api

# Seeders — run data seeding scripts inside containers
keel seed                      # run all seeders
keel seed mysql-init           # run a single seeder

# Updates
keel update                    # check for updates and install latest

# Hosts — manage /etc/hosts entries from Traefik config
keel hosts setup               # add service domains to /etc/hosts
keel hosts setup --ip 10.0.0.5 # use custom IP
keel hosts remove              # remove keel entries

# Maintenance
keel purge                     # remove all containers + network + data directory
keel version
keel help
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | `60000` | HTTP port |
| `-bind` | `127.0.0.1` | Bind address |
| `-keel-dir` | `/var/lib/keel` | Data directory |
| `-dev` | `false` | Serve web assets from filesystem |

Override the data directory with `KEEL_DIR` environment variable.

---

## Features

### Dashboard

Live grid view of all containers with real-time status polling. Group services by category. Start, stop, restart, and update containers directly from the UI.

### Logs

SSE-based streaming from `docker logs` or container files. Supports **host-path logs** — read log files directly from the host filesystem even when the container is crashed or stopped.

### Terminal

Full interactive terminal in the browser via WebSocket + PTY. Ctrl+\` to toggle. Multi-tab support — a fixed **Local** tab is always available, and each `docker exec` connection opens in its own tab. Click **CONNECT** on any running container to open a shell session.

### Metrics

Real-time CPU, memory, disk, load average, uptime, and per-container Docker stats (CPU%, RAM, network I/O, block I/O).

### Config Editor

Inline JSON editor for each service. Edit, save, and the config is written atomically with backup.

### Dashboard URL

Services can define a `dashboard_url` in their JSON config. When set, an **OPEN** button appears on both the overview card and detail page, linking to the service's web UI (e.g., RedisInsight, DBGate, pgAdmin).

```json
{
  "name": "redisinsight",
  "dashboard_url": "http://localhost:5540"
}
```

### Health Checks

HTTP or command-based health checks with configurable interval, retries, and start period.

---

## Seeders

Run data seeding scripts inside running containers — database migrations, fixture data, initial configs.

Each seeder is a JSON file in `data/seeders/`:

```json
{
  "name": "mysql-init",
  "target": "db-seeder",
  "description": "Create localdev databases, users, and seed data",
  "order": 1,
  "commands": [
    { "name": "Install dependencies", "command": "pip install mysql-connector-python" },
    { "name": "Seed data",            "command": "python3 seed.py --env localdev" }
  ]
}
```

| Field | Description |
|-------|-------------|
| `target` | Container name to exec into |
| `order` | Execution order (lower = first) |
| `commands` | Ordered list of `{ name, command }` steps |

Seeders can be run from the UI (Seeders page) or via CLI:

```bash
keel seed                      # run all seeders in order
keel seed mysql-init           # run a single seeder
```

---

## Dev Mode

Run a service locally with your source code mounted and hot reload enabled.

```bash
keel dev <service> <local-path>
```

How it works:
1. Reads `dev.dockerfile` from the service JSON and builds a dev image
2. Stops the existing container
3. Runs the dev container in foreground with your local code mounted as a volume
4. Streams stdout/stderr to your terminal — Ctrl+C to stop

**Requirement:** local target only (not supported over SSH).

Example service config:

```json
{
  "dev": {
    "dockerfile": [
      "FROM golang:1.24",
      "RUN go install github.com/air-verse/air@latest",
      "WORKDIR /app",
      "COPY go.mod go.sum ./",
      "RUN go mod download"
    ],
    "command": ["air"],
    "cap_add": ["NET_BIND_SERVICE"]
  }
}
```

---

## Remote Targets

Keel supports multiple Docker targets — local or remote via SSH tunnel.

```json
// /var/lib/keel/data/targets.json
{
  "targets": {
    "local": { "host": "127.0.0.1" },
    "ec2":   { "host": "user@1.2.3.4", "ssh_key": "~/.ssh/id_ed25519", "external_ip": "1.2.3.4" }
  }
}
```

```bash
keel target ec2      # switch to remote
keel start           # commands now execute on ec2 via SSH
keel target local    # switch back
```

For remote targets, an SSH tunnel is opened automatically, forwarding the remote Docker socket to a local Unix socket (`/tmp/keel-docker-<target>.sock`).

---

## Service Config

Each service is a JSON file in `data/services/`. Full example:

```json
{
  "name": "redis",
  "group": "database",
  "hostname": "keel-redis",
  "image": "redis:7",
  "registry": "dockerhub",
  "network": "keel-net",
  "ports": { "internal": 6379, "external": 6379 },
  "environment": { "REDIS_ARGS": "--maxmemory 256mb" },
  "volumes": ["keel-redis-data:/data"],
  "ram_estimate_mb": 256,
  "dashboard_url": "http://localhost:8001",
  "health_check": {
    "type": "command",
    "command": "redis-cli ping",
    "interval": 5,
    "retries": 10,
    "start_period": 5
  },
  "logs": [
    { "name": "redis", "type": "docker" },
    { "name": "slow", "type": "file", "path": "/var/log/redis-slow.log", "host_path": "/applog/redis/slow.log" }
  ],
  "dev": {
    "dockerfile": ["FROM redis:7", "WORKDIR /data"],
    "command": ["redis-server", "--loglevel", "debug"]
  }
}
```

---

## Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.24, stdlib `net/http`, `gorilla/websocket`, `creack/pty` |
| Frontend | HTMX 2.0, Alpine.js 3.x, xterm.js |
| Design | Kaze design system — Recursive variable font |
| Assets | `go:embed` — single binary, ~10MB |
| Icons | Lucide v0.469.0 (self-hosted SVG sprite) |
| Metrics | `/proc/stat`, `/proc/meminfo`, `syscall.Statfs`, `docker stats` |

---

## Build

```bash
make build              # current platform
make build-linux        # Linux amd64
make build-linux-arm64  # Linux arm64
make build-all          # all targets
```

Binaries are output to `bin/`.

### Development

```bash
# Build and install locally
make build
sudo bash install-dev.sh

# Run with live asset reloading
keel -dev

# Run tests
go test ./...
```

---

## Data Directory

| Platform | Default Path | Override |
|----------|-------------|----------|
| Linux | `/var/lib/keel/` | `KEEL_DIR` or `-keel-dir` |
| macOS | `~/.keel/` | `KEEL_DIR` or `-keel-dir` |

```
/var/lib/keel/      # Linux (or ~/.keel/ on macOS)
  data/
    config.json           # global config (network, subnet)
    targets.json          # Docker targets (local + SSH)
    services/
      redis.json          # one file per container
      traefik.json
    seeders/
      mysql-init.json
    config/
      traefik/
        dynamic.yml       # Traefik routing rules (used by "keel hosts")
  state/
    target                # active target name
    ghcr-user             # GHCR credentials (chmod 600)
    ghcr-pat
```

---

## Star History

[![Star History Chart](https://api.star-history.com/image?repos=getkaze/keel&type=date&legend=top-left)](https://www.star-history.com/?repos=getkaze%2Fkeel&type=date&legend=top-left)

---

## License

MIT — see [LICENSE](LICENSE).
