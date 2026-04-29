# viam-fleet-tools

## Purpose

A small Viam module for fleet operators — measure machines, enforce baseline state on them, and surface both through Viam's existing alerting and data infrastructure. Distributed as a single Viam module (`avery:fleet-tools`) declaring two sensor models (`workload`, `baseline`); no external services to host; everything flows through the Viam fabric.

## User Profile

**Primary:** A fleet operator running a set of Viam machines — could be Viam's internal lab admins, a robotics company managing deployed bots, an integrator with customer fleets, or a hobbyist with a home cluster. They want fleet-wide visibility and a way to keep their machines uniformly configured without per-machine SSH sessions.

**Secondary:** Downstream enterprise systems that already consume Viam data — BI tools pointing at Viam's MongoDB backend, alert routing services receiving Viam webhook notifications, etc. These integrate with the project transparently because everything it produces is just Viam sensor data.

## Goals

**Goal:** Operators get a per-machine snapshot of system load (CPU, memory, disk, network, power, thermals, top processes) rich enough to characterize any workload against any target machine — and Viam's conditional telemetry alerts can fire on any of those signals out of the box.

**Goal:** Operators add one Viam module to a machine's config and get a baseline applied automatically — `viam` user with their SSH key, Tailscale joined and tagged, hostname normalized, packages installed — with each piece independently opt-in via configuration.

**Goal:** State drift gets corrected on demand and reported through standard Viam channels. A failed reconcile step shows up in Viam's alert engine within one poll cycle.

**Goal:** Adding a new baseline-enforcement step (future: unattended-upgrades, journald retention, fail2ban, etc.) is cheap — the reconcile framework is built for extension.

**Goal:** Logs are quiet by default. A no-op reconcile is one info line. Only real work gets info-level logging.

**Non-Goal:** Hosting an external state store, writing a separate UI, or maintaining a separate auth/integration surface. Viam already has all of those; this project produces data and signals that flow through them.

**Non-Goal:** Fleet-wide aggregation alerts (e.g., "alert if 30% of bots in group X are unhealthy"). Viam's alert engine is per-machine. Cross-machine aggregation is left to whatever downstream system the operator routes alerts into.

**Non-Goal:** Supporting OSes outside Linux (Ubuntu, Debian, Raspberry Pi OS, similar) in v1. Anything else errors loud at Reconfigure. Architectures: amd64 and arm64.

**Non-Goal:** Secret rotation, credential management, or anything beyond passing secrets via module config attributes in v1.

## Features

### Required (v1)

**`workload` model (Go sensor; subpackage `workload/`)**
- Per-poll Readings dict capturing system metrics rich enough for workload characterization (see Data Schema below)
- Cross-platform Linux: works on amd64 and arm64; Intel RAPL, AMD k10temp/zenpower, generic hwmon all detected at runtime; degrades silently when a metric source is unavailable
- Optional config: `top_n_processes` (default 3), `extra_disk_devices` (default empty)
- Polling interval governed by Viam data capture; no internal scheduling
- All polls captured (legitimate time series data)

**`baseline` model (Go sensor + DoCommand; subpackage `baseline/`)**
- Pluggable reconcile framework with per-step structured status
- Root-check at startup; error loud if not running as root
- OS detection (Linux distros listed above); error loud on unsupported
- Built-in reconcile steps, each independently enabled by presence of its config block:
  - `viam_user`: ensure `viam` user exists with the configured authorized_keys entry
  - `tailscale`: ensure Tailscale is installed, up, and joined with the configured auth key
  - `hostname`: normalize OS hostname to match the Viam part name
  - `packages`: ensure the configured apt packages are installed
- `Readings` returns current state + per-step reconcile status; uses `data.ErrNoCaptureToStore` to skip capture on no-change polls (heartbeat threshold forces periodic capture even on no change)
- `DoCommand("reconcile")` re-runs the full reconcile on demand
- Single info line per no-op reconcile; tight info-level summary for actual work; everything else debug
- Single error log on any failed step (drives Viam log-based alerts as backup signal)

**Documentation**
- README.md: what this is, who it's for, how to install each module, link to per-module docs
- Per-module config reference covering every config block
- Setup guide showing the typical "install on a fleet" flow
- Architecture note explaining the no-external-store design and how Viam data + alerts cover the integration story

### Milestones

Vertical-slice build order:

1. ⏳ **`workload` sensor v1** — repo scaffold + Go module + Readings dict + Viam data capture verified on a real machine + Viam conditional telemetry alert fires on a threshold
2. ⏳ **`baseline` sensor scaffold** — pluggable reconcile framework + root check + OS detection + DoCommand + Readings emitting `ErrNoCaptureToStore` correctly + single-line logging discipline — no enforcement steps implemented yet
3. ⏳ **`baseline` reconcile steps** — `viam_user` → `hostname` → `packages` → `tailscale`, one at a time
4. ⏳ **Alerting cookbook** — documented webhook + Slack + log-based alert recipes deployers can copy-paste

### Nice-to-Have (v2)

- Viam Application for browsing fleet state via Viam SDK
- Reconcile-trigger button in the app (calls `DoCommand("reconcile")`)
- Group-level config refactor — secrets and shared state move out of per-machine module config (probably leveraging Viam's org-level secrets API)
- Additional baseline steps: unattended-upgrades, journald retention, SSH hardening, firewall, fail2ban
- Additional workload metrics: context switches, interrupts, fd counts, network errors

### Bonus Round

- Additional OS support (Fedora/RHEL, NixOS, macOS)
- Fleet-wide aggregation alerting via an external evaluator service
- Custom user-supplied reconcile steps loaded as Go plugins or sub-modules

## Tech Stack

### Language(s)
- **Go** — the entire `avery:fleet-tools` module. Cross-compiles to a single static binary per architecture that registers both models; matches the dominant Viam community module pattern; one toolchain across the repo.

### Frameworks/Libraries
- Viam Go SDK (`go.viam.com/rdk/...`) — sensor API, module boilerplate, data manager integration, `data.ErrNoCaptureToStore` sentinel
- `gopsutil` — cross-platform system metrics (CPU, memory, disk, network, processes); covers ~80% of the workload sensor's needs
- Direct `/sys` reads — Intel RAPL, AMD k10temp/zenpower, generic hwmon temperature scanning (filling the gaps gopsutil doesn't cover)

### Platform/Deployment
- Modules distributed via the Viam module registry (or local module during development)
- Runs as root under viam-server on Linux (amd64, arm64)
- Module metadata (`meta.json`) restricts to supported architectures

### Infrastructure
- None hosted by this project. Viam's MongoDB backend serves as the data store; Viam's alert engine handles alerting; Viam's auth handles access control.
- Operator-supplied credentials (Tailscale auth key, SSH public keys) flow through module config attributes.

## Technical Architecture

### Components

- **`workload` sensor** (Go, runs as the regular module user): each `Readings` call samples `/proc`, `/sys`, and `gopsutil` and returns a structured dict. Stateless apart from the small amount of delta tracking needed for rate-style metrics (CPU%, disk I/O, network throughput, RAPL energy).

- **`baseline` sensor + service** (Go, runs as root): on `Reconfigure` and on `DoCommand("reconcile")`, runs each enabled reconcile step idempotently and aggregates results. `Readings` returns the current snapshot of state plus the most recent per-step status. Tracks last-emitted snapshot in memory; returns `data.ErrNoCaptureToStore` when current snapshot equals the last emitted one (with a periodic heartbeat override so "still alive" remains visible in captured data).

- **Viam data manager** (provided by viam-server): persists captured readings to Viam's MongoDB backend. Operators point BI tools, ETL jobs, or compliance tooling at the same backend.

- **Viam alert engine** (provided by Viam): conditional telemetry alerts fire on workload thresholds and on `baseline` status field transitions; log-based alerts catch reconcile errors via the module's structured error logs; part-online/offline alerts cover machine connectivity.

### Integration points

- Module → System: direct `os/exec` to `useradd`, `apt-get`, `hostnamectl`, `tailscale`, etc. — all assume root in `baseline`. `workload` reads `/proc` and `/sys` only; no privilege beyond what gopsutil needs.
- Module → Tailscale: `tailscale up --authkey ...` with the operator's pre-tagged auth key.
- Module → Viam fabric: standard SDK integration (sensor Readings, DoCommand, reconfigure).
- Operator's enterprise systems → Viam: BI tools query MongoDB; alert routing receives webhook payloads.

### Data Schema

#### `workload` Readings (per poll)

| Group | Field | Notes |
|---|---|---|
| **CPU** | `cpu_pct_avg` | Overall utilization |
| | `cpu_pct_max_core` | Single-thread bottleneck signal |
| | `cpu_pct_per_core` | Per-core array |
| | `cpu_freq_mhz_avg` | Turbo/throttle visibility |
| | `cpu_freq_mhz_max` | Peak among cores |
| | `load_avg_1`, `load_avg_5`, `load_avg_15` | Classic Linux signals |
| **Memory** | `mem_used_mb` | Excluding cache (real working set) |
| | `mem_available_mb`, `mem_total_mb` | Headroom context |
| | `swap_used_mb` | Absolute |
| | `swap_in_mb_per_sec`, `swap_out_mb_per_sec` | Activity (more diagnostic than usage) |
| **Disk** (per device) | `disk_read_mbps`, `disk_write_mbps` | Throughput |
| | `disk_read_iops`, `disk_write_iops` | Operations |
| | `disk_util_pct` | iostat-style utilization |
| **Network** (per interface) | `net_rx_mbps`, `net_tx_mbps` | Throughput |
| | `net_rx_pps`, `net_tx_pps` | Packets |
| **Power** | `pkg_watts` | Intel RAPL or AMD energy interfaces |
| **Thermal** | `cpu_temp_max_c` | Hottest core (throttling-proximity signal) |
| | `cpu_temp_avg_c` | Mean across cores (cooling-adequacy / trend signal) |
| | `cpu_temp_per_core_c` | Per-core array (hotspot detection, multi-package visibility) |
| **GPU** (when present) | `gpu_util_pct`, `gpu_mem_mb`, `gpu_watts` | Via `nvidia-smi` |
| **Processes** | `top_procs_by_cpu` | List of `{pid, name, cpu_pct, mem_mb}` (configurable N) |
| | `top_procs_by_mem` | Same shape, top by memory |
| **System** | `uptime_sec` | |
| | `timestamp` | ISO8601, redundant with capture timestamp but useful for standalone analysis |

Thermal source resolution: at startup, the sensor scans `/sys/class/hwmon/*/name` and `/sys/class/thermal/*/type` and selects the first match in priority order — `coretemp` → `zenpower` → `k10temp` → `thermal_zone(type=cpu_thermal)` → `thermal_zone(type=x86_pkg_temp)` — then reads only from that source on every poll. Per-core values come from `temp*_label` entries matching `Core N` (coretemp) or `Tdie`/`Tccd*` (k10temp/zenpower). When no recognized source is present, the `cpu_temp_*` keys are omitted entirely. Multi-socket aggregation is out of scope for v1.

#### `baseline` Readings

| Field | Notes |
|---|---|
| `group` | Free-form string from config; the operator's grouping label |
| `hostname` | Current OS hostname (after normalization, should equal Viam part name) |
| `os` | `/etc/os-release` ID + VERSION |
| `arch` | `runtime.GOARCH` |
| `tailscale_ip` | When tailscale step is enabled and joined |
| `last_reconcile_at` | Timestamp of most recent reconcile attempt |
| `reconcile_status` | Map: `{viam_user: "ok", tailscale: "failed: <reason>", hostname: "ok", packages: "ok"}` — only includes steps that are enabled |

The baseline sensor returns `data.ErrNoCaptureToStore` from `Readings` when the current snapshot matches the last-emitted snapshot, except every Nth poll which forces emission as a heartbeat.

### Configuration

Each reconcile step has its own config block; presence enables the step, absence disables it.

```json
{
  "group": "warehouse-bots",
  "heartbeat_polls": 120,

  "viam_user": {
    "ssh_authorized_key": "ssh-ed25519 AAAA..."
  },

  "tailscale": {
    "auth_key": "tskey-..."
  },

  "hostname": {},

  "packages": {
    "list": ["htop", "vim", "emacs", "tmux", "git", "curl", "wget", "rsync", "jq", "lsof", "build-essential"]
  }
}
```

Top-level keys:
- `group` — opaque label included in baseline Readings; used by operators for filtering / aggregation downstream
- `heartbeat_polls` — force a baseline capture every N polls even when state hasn't changed (default 120)

Step-block keys are step-specific. An empty object means "enable with defaults" (e.g., `hostname: {}` enables hostname normalization, which has no other configuration).

## Development Process

**Testing approach:**
- **Unit tests:** reconcile-step logic in isolation; status aggregation; OS detection; the `ErrNoCaptureToStore` decision for the baseline sensor; structured log-line emission. Filesystem and exec calls are abstracted behind an interface so steps can be tested without a real root shell.
- **Integration tests:** reconcile framework end-to-end with fake steps, verifying logging discipline (no-op = one line, work = summary lines, errors surface without stack traces).
- **Manual validation:** each milestone ends with deploying to a real Linux machine and verifying readings appear in Viam data, and at least one Viam alert fires on a synthetic condition.

**Deployment:**
- Modules built locally or via `viam module build` cloud runners
- Published to the Viam module registry per release (`viam module upload`)
- `viam module reload` for fast iteration on a target machine during development
