# Changelog

All notable changes to keel will be documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added

- **Restart command** — `keel restart [service|group]` stops and starts services in one step (@mateusmetzker)
- **Extra ports** — new `extra_ports` field in service config for mapping additional port pairs beyond the primary port (@mateusmetzker)
- **Network aliases** — new `network_aliases` field in service config adds Docker network aliases to containers, enabling virtual hostname resolution within the Docker network (@mateusmetzker)
- **File log volume mounts** — log sources with `type: file` and `host_path` are now automatically bind-mounted into the container at boot (@mateusmetzker)
- **HTTP seeder commands** — seeders now support an `http` field for executing HTTP requests (via curl) inside the target container as initialization steps (@mateusmetzker)
- **Log viewer groups** — logs sidebar now shows services grouped by their group tag, ordered by boot priority, with a search field to filter by name (@mateusmetzker)
- **Dashboard Unknown filter** — new "Unknown" filter chip for containers with missing/unknown status (@mateusmetzker)
- **Auto PATH setup** — installer now detects the user's shell (zsh, bash, fish) and automatically appends the install directory to the appropriate profile file (`~/.zshrc`, `~/.bash_profile`, `~/.bashrc`, or `config.fish`); falls back to a manual instruction for unsupported shells (@mateusmetzker)
- **Platform override** — new `platform` field in service config passes `--platform` to `docker run`, allowing cross-architecture images (e.g. `linux/amd64` on ARM hosts) (@mateusmetzker)

### Changed

- **Branding refresh** — replace SVG logos with PNG banners and icons; remove PWA manifest and apple-mobile-web-app meta tags; navbar now uses a raster logo image (@mateusmetzker)

### Fixed

- **Dashboard filter persistence** — filter selection now survives HTMX polling refreshes via sessionStorage; container count updates reactively with the active filter (@mateusmetzker)
- **Dashboard stopped filter** — "Stopped" filter no longer shows running or unknown containers (@mateusmetzker)
- **Seeder status** — server-rendered seeder status is now authoritative; sessionStorage only takes priority while a run is actively in progress (@mateusmetzker)
- **Log host path expansion** — `~/` in log source host paths is now correctly expanded via `ssh.ExpandHome` (@mateusmetzker)
- **Reset clears runtime logs** — `keel reset` now removes the `runtime/<service>/` directory (@mateusmetzker)
- **Metrics** — metrics page now correctly shows remote host data when target is not local; previously always displayed local machine metrics after hot-reload (@mateusmetzker)
- **Seeder interpreter** — seeder scripts now execute via `sh -c <interpreter>` instead of splitting the interpreter string on whitespace, fixing commands that contain arguments with spaces (@mateusmetzker)
- **Operation banner stuck** — the operation banner (loading spinner) no longer gets stuck when the card polling swap aborts the in-flight SSE request; aborted requests now resolve the banner as success (@mateusmetzker)
- **View Logs button** — "View Logs" in the operation banner now navigates to the logs page with the service pre-selected instead of opening the operation progress panel (@mateusmetzker)

## [0.5] — 2026-04-01

### Changed

- **Branding** — replace old wind/purple icon with Keel cross logo; favicon adapts to light/dark system theme; tab title and navbar wordmark capitalized to "Keel" (@mateusmetzker)
- **Kaze design system** — replace dark-first Recursive theme with light-only Inter-based design system; new color tokens, layered shadows, rounded cards with hover lift, and Kaze-aligned palette (@mateusmetzker)
- **Remove dark theme** — single light theme only; theme toggle button and JS removed (@mateusmetzker)

### Added

- `registry: "local"` — skip `docker pull` for locally built images; allows using images like `myapp:local` without a remote registry (@mateusmetzker)
- **Local registry in UI** — "New container" form now includes a "Local (pre-built image)" option in the registry dropdown (@mateusmetzker)
- **Docs icon in update modal** — changelog sections titled "Docs" now render with a document icon instead of being blank (@mateusmetzker)
- **Operation banner** — persistent progress indicator during container operations (start, stop, restart, update); shows spinner + service name + action label, stays visible until completion, then resolves to success/error state with auto-dismiss; replaces the old auto-dismissing toast that left users blind during long operations (@mateusmetzker)

### Fixed

- Dashboard: page header action buttons no longer overflow off-screen on narrow viewports (@mateusmetzker)
- Dashboard: sidebar is now accessible on mobile via a hamburger menu instead of being hidden with no way to reopen (@mateusmetzker)
- `keel reset` now pulls the latest image before recreating the container, matching the dashboard behavior — previously it reused the local cache, so updates were silently ignored (@mateusmetzker)
- Health checks are now applied to containers at boot — `--health-cmd`, `--health-interval`, `--health-retries`, and `--health-start-period` flags were missing from `docker run` despite being defined in service config (@mateusmetzker)
- HTTP health check now uses `wget` with `curl` as fallback instead of requiring `curl` — covers Alpine-based images (wget) and Debian/Ubuntu (curl) without changes to the image (@mateusmetzker)
- CLI runner (`keel reset`, `keel start`) now splits `command` into args instead of wrapping with `sh -c` — fixes crash on minimal images (scratch, distroless) that have no `/bin/sh` (@mateusmetzker)
- CLI runner now applies health check flags at boot, matching the dashboard executor behavior (@mateusmetzker)

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

[Unreleased]: https://github.com/getkaze/keel/compare/v0.5...HEAD
[0.5]: https://github.com/getkaze/keel/compare/v0.4...v0.5
[0.4]: https://github.com/getkaze/keel/compare/v0.3...v0.4
[0.3]: https://github.com/getkaze/keel/compare/v0.2...v0.3
[0.2]: https://github.com/getkaze/keel/compare/v0.1.1...v0.2
[0.1.1]: https://github.com/getkaze/keel/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/getkaze/keel/releases/tag/v0.1.0
