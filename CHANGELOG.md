# Changelog

All notable changes to keel will be documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

## [0.4] — 2026-03-27

### Changed

- Command field in service definitions is now split into args instead of wrapped with `sh -c`, fixing containers with custom entrypoints like cloudflared (@mateusmetzker)
- Install script always uses `~/.local/bin` to enable self-update without sudo (@mateusmetzker)

### Fixed

- Containers with ENTRYPOINT (e.g. cloudflared) no longer fail when command contains spaces — previously `sh -c` wrapper conflicted with the entrypoint (@mateusmetzker)

### Docs

- README documents keel-dir defaults per OS (Linux vs macOS) (@mateusmetzker)

## [0.3] — 2026-03-23

### Added

- CmdRunner abstraction — local and remote Docker execution behind a unified interface (@mateusmetzker)
- LocalRunner and ReloadableRunner with hot-swap support for target config changes (@mateusmetzker)
- SSH utilities extracted into `internal/ssh` package, shared across all SSH consumers (@mateusmetzker)
- TunnelMonitor with automatic reconnection, exponential backoff, and health checks (@mateusmetzker)
- SSE endpoint for tunnel status (`GET /api/tunnel/status`) with live status dot in the topbar (@mateusmetzker)
- Label-based container detection (`keel.managed=true`) with network fallback for backward compatibility (@mateusmetzker)
- Semver comparison in updater — correctly handles `0.10.0 > 0.9.0` (@mateusmetzker)
- Cross-device-safe updater — temp file created in same directory as binary (@mateusmetzker)
- IP validation for `keel hosts setup` — rejects invalid addresses before modifying `/etc/hosts` (@mateusmetzker)
- Body size limits: 64 KB for service creation, 1 MB for config save (@mateusmetzker)
- CI workflow with `go test -race` on push/PR; test job added as prerequisite in release workflow (@mateusmetzker)
- 155 unit tests across all packages (@mateusmetzker)
- Dual donate options: Stripe for Brazilian supporters, Buy Me a Coffee for international (@mateusmetzker)

### Changed

- Migrated local metrics (CPU, memory, disk, load average, uptime) from manual `/proc` parsing to gopsutil v4 (@mateusmetzker)
- `start-all`, `stop-all`, and seeder run endpoints changed from GET to POST (@mateusmetzker)
- SSE error events renamed from `error` to `app-error` to avoid conflicts with `EventSource.onerror` (@mateusmetzker)
- SSE streams now support POST via `fetch + ReadableStream` for mutation endpoints (@mateusmetzker)
- Template rendering buffered — errors return clean HTTP 500 instead of partial HTML (@mateusmetzker)
- Health check handler reuses a shared `http.Client` instead of creating one per request (@mateusmetzker)
- Remote metrics cached for 10 seconds with background refresh — no more blocking SSH calls per HTTP request (@mateusmetzker)
- Log navigation uses `htmx:afterSettle` instead of `setTimeout` for reliable service pre-selection (@mateusmetzker)
- GHCR login now pipes PAT over stdin instead of shell interpolation (@mateusmetzker)
- SSH options hardened: `StrictHostKeyChecking=accept-new` replaces `StrictHostKeyChecking=no` (@mateusmetzker)
- WebSocket origin check validates same-host/localhost instead of accepting all origins (@mateusmetzker)

### Fixed

- Terminal deadlock: `Session.Close` is now idempotent via `sync.Once`; close called before `wg.Wait` (@mateusmetzker)
- Terminal ANSI clear race condition: moved from server-side PTY write to client-side `term.clear()` on WebSocket open (@mateusmetzker)
- Update toast now shows error details when pull fails, instead of always showing "UPDATE COMPLETE" (@mateusmetzker)
- Log viewer path traversal: file paths validated against configured log source directories (@mateusmetzker)
- Config editor: `saveServiceConfig` uses `io.ReadAll` + `json.Valid` instead of broken `fmt.Fscan` (@mateusmetzker)
- Destructive update prevention: failed `docker pull` no longer removes the running container (@mateusmetzker)
- Seeder card CSS selector corrected from `.seeder-card` to `.seeder-item` (@mateusmetzker)
- Executor `dockerStream` gains idle timeout and non-blocking channel sends with log-on-drop (@mateusmetzker)

---

## [0.2] — 2026-03-15

### Added

- Maximize/restore button on the terminal panel (@mateusmetzker)
- Sortable columns on the metrics container resources table (container, cpu, memory, mem %, net i/o, block i/o) (@mateusmetzker)
- Mobile responsive layout with breakpoints at 768px and 480px (@mateusmetzker)
- Update modal with changelog and one-click update from the dashboard (@mateusmetzker)

### Changed

- Replaced Inter + JetBrains Mono with Recursive variable font, aligned with Kaze design system (@mateusmetzker)
- Release workflow now includes "What's new" section extracted from CHANGELOG.md (@mateusmetzker)

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

[Unreleased]: https://github.com/getkaze/keel/compare/v0.4...HEAD
[0.4]: https://github.com/getkaze/keel/compare/v0.3...v0.4
[0.3]: https://github.com/getkaze/keel/compare/v0.2...v0.3
[0.2]: https://github.com/getkaze/keel/compare/v0.1.1...v0.2
[0.1.1]: https://github.com/getkaze/keel/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/getkaze/keel/releases/tag/v0.1.0
