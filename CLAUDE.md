# CLAUDE.md

This file provides project-specific guidance to Claude Code (claude.ai/code) for this repository.

## Current Status

Project phase: **Setup** — `project_spec.md` rewritten for the new architecture, no code yet. Next: Milestone 1 (the `workload` sensor).

Note: the local directory is still named `lab-telemetry` for historical reasons; the project is now `viam-fleet-tools` and will be renamed locally on the next session restart.

## About This Project

A toolkit of Viam modules for fleet operators. Two modules in v1:

- **`workload`** — a sensor that emits a per-poll snapshot of system load (CPU, memory, disk, network, power, thermals, top processes), rich enough to characterize any workload against any target Linux machine.
- **`baseline`** — a sensor + DoCommand service that enforces and reports machine baseline state: ensures the `viam` user exists with a configured SSH key, Tailscale is installed and joined with a configured tag, OS hostname matches the Viam part name, and a configured set of packages is installed. Each step is opt-in via the presence of its config block.

No external services. Everything flows through the Viam fabric — Viam data manager persists captured readings to MongoDB, Viam's alert engine fires on conditional telemetry / log-based / part-offline alerts, downstream BI and integration systems consume from Viam directly.

See `project_spec.md` for the full architecture, milestones, and design rationale.

## Project-Specific Conventions

### Reconcile logging discipline

The `baseline` module follows a strict logging policy (also captured in memory at `feedback_log_verbosity.md`):

- A no-op reconfigure emits exactly one info line.
- A reconfigure that actually did work emits a tight info-level summary of what changed — never per-step chatter.
- A reconfigure with a failed step emits a single info-level summary line; details go to debug.
- Everything else (intermediate state, "checking X", "X is fine") is debug, off by default.

When adding a new reconcile step, preserve this invariant.

### Sensor data discipline

The `baseline` sensor uses `data.ErrNoCaptureToStore` to skip capture cycles when its state snapshot hasn't changed. A configurable `heartbeat_polls` interval forces periodic capture even on no change so "still alive" remains visible in captured data. The `workload` sensor captures every poll — its data is legitimate time series.

### Testing

```bash
make test
```

### Build / Run

The repo is a single Viam module (`avery:fleet-tools`) declaring multiple models. `Makefile`, `meta.json`, `go.mod`, `cmd/module/main.go`, and the deploy workflow live at the repo root. Each model's implementation lives in its own subpackage: `workload/` for the workload sensor, `baseline/` for the baseline sensor + service.

```bash
make build          # builds bin/fleet-tools (one binary registers all models)
make test           # ./... — tests every subpackage
make module.tar.gz  # tarball for registry upload
```

**Convention:** Viam registry expects one git repo to map to one module. Multiple semantic concepts in this project (workload + baseline) are expressed as *models* within the single `avery:fleet-tools` module, not as separate modules.
