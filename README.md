<div align="center">

  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="logo-light.svg">
    <source media="(prefers-color-scheme: light)" srcset="logo-dark.svg">
    <img src="logo-dark.svg" alt="Keel" width="48" height="48">
  </picture>

  # Keel

  **One binary. Zero dependencies. Full Docker control.**

  <br/>

  [![Go](https://img.shields.io/badge/Go-1.24-00ADD8?style=flat-square&logo=go&logoColor=white)](https://golang.org)
  [![License](https://img.shields.io/badge/license-MIT-green?style=flat-square)](LICENSE)
  [![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macOS-lightgrey?style=flat-square)](https://github.com/getkaze/keel)

  <br/>

  [Prerequisites](#prerequisites) · [Install](#install) · [Usage](#usage) · [Features](#features) · [Seeders](#seeders) · [Dev Mode](#dev-mode) · [Remote Targets](#remote-targets) · [Service Config](#service-config) · [Stack](#stack) · [Build](#build) · [Data Directory](#data-directory)

</div>

---

## What is Keel

**Keel** (the keel of a ship — the hidden structure that keeps everything aligned) is a self-hosted web dashboard for managing Docker environments — local or remote via SSH — from a single Go binary (~10MB, no external dependencies).

```
keel
```

That's it. Open `http://localhost:60000` and you have a full dashboard with live status, logs, terminal, metrics, and container management.

---

## Prerequisites

- **Docker** — local install or remote host with Docker via SSH
- **SSH key pair** — required for remote targets
- **sudo** — only for `keel hosts setup` (modifies `/etc/hosts`)

---

## Install

```bash
curl -fsSL https://getkaze.dev/keel/install.sh | sudo bash
```

This installs the binary to `~/.local/bin/keel` and creates the data directory at `/var/lib/keel`. The binary is owned by your user, enabling self-update from the dashboard without sudo.

---

## Usage

```bash
# Start the dashboard (default: http://localhost:60000)
keel

# Container operations
keel start                     # start all services
keel start redis mysql         # start specific services
keel start infra               # start all services in a group
keel stop                      # stop all services
keel stop traefik              # stop specific service
keel stop tools                # stop all services in a group
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
| `-keel-dir` | `/var/lib/keel` (Linux) or `~/.keel` (macOS) | Data directory |
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
| `commands` | Ordered list of steps (see below) |

Each command entry supports:

| Field | Description |
|-------|-------------|
| `name` | Step identifier |
| `command` | Single command to execute via `docker exec` |
| `script` | Filename of a script in the seeders directory (alternative to `command`) |
| `interpreter` | Interpreter to pipe the script into — e.g. `bash`, `python3` (used with `script`) |

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

<!-- /var/lib/keel/data/targets.json -->
```json
{
  "targets": {
    "local": { "host": "127.0.0.1" },
    "ec2": {
      "host": "1.2.3.4",
      "ssh_user": "ubuntu",
      "ssh_key": "~/.ssh/id_ed25519",
      "ssh_jump": "ec2-user@bastion.example.com",
      "external_ip": "1.2.3.4",
      "port_bind": "0.0.0.0",
      "description": "AWS EC2 Ubuntu"
    }
  },
  "default": "local"
}
```

| Field | Description |
|-------|-------------|
| `host` | IP address or hostname |
| `ssh_user` | SSH user for remote targets (omit for local) |
| `ssh_key` | Path to SSH private key (supports `~/`) |
| `ssh_jump` | Bastion/jump host for multi-hop SSH |
| `external_ip` | External IP used by `keel hosts setup` |
| `port_bind` | Bind interface for ports — `127.0.0.1` (default) or `0.0.0.0` |
| `description` | Human-readable target label |
| `default` | Root-level field — default target name |

```bash
keel target ec2      # switch to remote
keel start           # commands now execute on ec2 via SSH
keel target local    # switch back
```

For remote targets, an SSH tunnel is opened automatically, forwarding the remote Docker socket to a local Unix socket (`/tmp/keel-docker-<target>.sock`). The tunnel is monitored with automatic reconnection and exponential backoff — a live status dot in the topbar shows the connection state via SSE.

---

## Service Config

Each service is a JSON file in `data/services/`. Full example:

```json
{
  "name": "redis",
  "group": "database",
  "hostname": "keel-redis",
  "image": "redis:7",
  "network": "keel-net",
  "ports": { "internal": 6379, "external": 6379 },
  "environment": { "REDIS_ARGS": "--maxmemory 256mb" },
  "volumes": ["keel-redis-data:/data"],
  "command": "redis-server --save 60 1",
  "files": ["data/config/redis.conf:/etc/redis/redis.conf"],
  "start_order": 1,
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

| Field | Description |
|-------|-------------|
| `name` | Unique service identifier |
| `group` | Logical grouping — `infra` starts first, then seeders, then the rest |
| `hostname` | Docker container hostname |
| `image` | Docker image `name:tag` |
| `registry` | `ghcr` — auto-login with stored credentials; `local` — skip pull for locally built images (omit for public images) |
| `network` | Docker network (defaults to `keel-net`) |
| `ports` | `{ internal, external }` port mapping |
| `environment` | Environment variables passed to the container |
| `volumes` | Volume mounts — named volumes, bind mounts, or config files |
| `command` | Override container CMD |
| `files` | Config files mounted read-only into the container; synced via `scp` on remote targets (`local:container`) |
| `start_order` | Startup priority (lower = earlier, 0 = last) |
| `ram_estimate_mb` | Display hint for the dashboard |
| `dashboard_url` | External URL — shows an **OPEN** button in the UI |
| `health_check` | HTTP or command-based health check config |
| `logs` | Log sources — `docker` or `file` with optional `host_path` |
| `dev` | Development mode config — `dockerfile`, `command`, `cap_add` |

---

## Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.24, stdlib `net/http`, `gorilla/websocket`, `creack/pty` |
| Frontend | HTMX 2.0, Alpine.js 3.x, xterm.js |
| Design | Kaze design system — Recursive variable font |
| Assets | `go:embed` — single binary, ~10MB |
| Icons | Lucide v0.469.0 (self-hosted SVG sprite) |
| Metrics | gopsutil v4, `docker stats`, remote cache (10s) |

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

# Run tests (155 unit tests)
go test ./...

# Run with race detection
go test -race ./...
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
