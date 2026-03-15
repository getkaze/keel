# Changelog

All notable changes to keel will be documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added

- Maximize/restore button on the terminal panel (@mateusmetzker)
- Sortable columns on the metrics container resources table (container, cpu, memory, mem %, net i/o, block i/o) (@mateusmetzker)
- Mobile responsive layout with breakpoints at 768px and 480px (@mateusmetzker)

### Changed

- Replaced Inter + JetBrains Mono with Recursive variable font, aligned with Kaze design system (@mateusmetzker)

### Removed

- Terminal drag-to-resize (replaced by maximize/restore toggle) (@mateusmetzker)

---

## [0.1.1] — 2026-03-13

### Changed

- Translated all Portuguese strings to English in the GitHub release workflow, dev installer, and GHCR setup prompts (@mateusmetzker)

---

## [0.1.0] — 2026-03-13

Initial public release (@mateusmetzker).

### Dashboard

- Service grid with real-time status polling, grouped by category
- Start, stop, restart, and update containers directly from the UI
- Inline JSON editor for service config with atomic backup
- Service creation and deletion
- SSE-based log streaming (container stdout/stderr and host-path files)
- Real-time metrics: CPU, memory, disk I/O, load average, uptime
- Per-container Docker stats (CPU %, RAM, network I/O, block I/O)
- Full browser terminal via WebSocket + PTY with multi-tab support
- Per-container shell sessions via CONNECT button
- Health check status display
- Dashboard URL button (OPEN) for linking to service web UIs
- Seeders page with grouped layout and live SSE execution streaming

### CLI

- `keel` — start the web dashboard (default port 60000)
- `keel start [service|group …]` — start services or groups
- `keel stop [service|group …]` — stop services or groups
- `keel reset [--all | service …]` — recreate containers
- `keel dev <service> <path>` — run a service with a local code mount (hot reload)
- `keel seed [name]` — run data seeders
- `keel target [name]` — show or switch the active deployment target
- `keel hosts setup / remove` — manage `/etc/hosts` entries from Traefik config
- `keel purge` — remove all containers, network, and data directory
- `keel update` — check for and install the latest version
- `keel version` — print version

### Deployment targets

- Local target (direct Docker socket, `127.0.0.1`)
- Remote targets via SSH tunnel (remote Docker socket forwarded to a local Unix socket)
- Per-target SSH key, jump host, port binding, and external IP configuration
- Remote file sync for volume mounts via scp

### Service configuration

- JSON-based service definitions (`data/services/*.json`)
- Fields: name, group, image, hostname, ports, environment, volumes, command
- `start_order` — control boot sequence across services and groups
- `registry: "ghcr"` — pull images from GitHub Container Registry
- `dashboard_url` — link to a service's web UI from the dashboard
- `health_check` — HTTP or command-based, with interval, retries, and start period
- Multiple log sources per service (container logs, container paths, host paths)
- Development mode config (Dockerfile, command, cap_add, local volume)

### Seeders

- JSON-based seeder scripts (`data/seeders/*.json`)
- Multi-step commands with named steps
- `order` field for execution sequencing
- Triggered by `keel seed` or automatically after `keel start`

### Registry

- GitHub Container Registry (ghcr.io) support
- Username and PAT stored in `state/` with `chmod 600`
- Automatic `docker login` when pulling GHCR images, for both local and remote targets

### Distribution

- Single static binary, no runtime dependencies
- Targets: Linux amd64, Linux arm64, macOS amd64, macOS arm64
- Data directory: `/var/lib/keel` (Linux) or `~/.keel` (macOS)
- Install script: `curl -fsSL https://getkaze.dev/keel/install.sh | sudo bash`

[Unreleased]: https://github.com/getkaze/keel/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/getkaze/keel/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/getkaze/keel/releases/tag/v0.1.0
