# lab-telemetry

## Purpose

A Viam generic service that, when added to a machine, installs a standard suite of users, keys, packages, and network configuration so lab admins can uniformly administer fleets of Viam machines — plus a Viam Application for browsing the resulting machine registry.

## User Profile

**Primary:** Lab admins managing fleets of Viam machines across multiple labs. They need a consistent baseline on every machine (shared SSH access, tailscale membership, standard tools) and a single place to see what machines exist, where they are, and whether they're healthy.

**Secondary:** The generic service itself, which runs unattended on every machine and must be quiet, idempotent, and self-healing on demand.

## Goals

**Goal:** One Viam generic service, added to a machine's config, brings that machine up to a known baseline (viam user + SSH key, tailscale joined with a lab-specific tag, hostname normalized, baseline packages installed) and registers it with a central registry.

**Goal:** A Viam Application gives lab admins a browsable list of registered machines across labs, with enough metadata to find and identify any machine.

**Goal:** Adding new housekeeping steps (future: unattended-upgrades, journald retention, etc.) is cheap — the reconcile framework is built for extension.

**Goal:** Logs are quiet by default. A no-op reconcile is a single info line. Only real work gets info-level logging.

**Non-Goal:** Secret rotation, secret storage, or any sophisticated credential management in v1. Secrets live in module config and are known tech debt.

**Non-Goal:** Real authentication on the Viam Application's Firestore reads in v1. Machine inventory is effectively semi-public; real auth is v2.

**Non-Goal:** Supporting OSes outside Ubuntu and Raspberry Pi OS in v1. Anything else errors loud at Reconfigure.

**Non-Goal:** Historical reconcile logs, bulk actions, UI-triggered reconciles. All v2.

## Features

### Required (v1)

**Generic service (Go module):**
- Pluggable reconcile framework with per-step structured status
- Root-check at startup; error loud if not running as root
- OS detection (Ubuntu + Raspberry Pi OS); error loud on unsupported
- Reconcile step: ensure `viam` user exists with `checkmate` password and authorized_keys entry
- Reconcile step: ensure tailscale is installed, up, and joined with the lab's auth key (which carries the lab tag)
- Reconcile step: normalize OS hostname to match Viam part name
- Reconcile step: install baseline package list
- Reconcile step: upsert this machine's registration doc in Firestore (soft — failure doesn't block other steps)
- `DoCommand("reconcile")` re-runs the full reconcile on demand
- One info line per no-op reconcile; info lines for work actually done; everything else debug

**Viam Application (static frontend):**
- Machine list table fed by Firestore `machines` collection
- Columns: part name, lab, OS, arch, tailscale IP, last reconcile time, per-step status
- Free-text search over part name and hostname
- Filter by lab
- Filter by "unhealthy on last reconcile"
- Machine detail panel with full Firestore doc + deep link to app.viam.com

**Data layer:**
- GCP project with Firestore (native mode)
- `machines` collection, document ID = Viam part ID
- `labs` collection (sparsely populated in v1; exists so v2 secret-refactor has a home)

### Milestones

Vertical-slice build order — prove each pipe before thickening:

1. ⏳ **GCP + Firestore bootstrap** — project, Firestore, service account for module writes, schema documented
2. ⏳ **Module scaffold** — Go module with reconcile framework, root check, OS detection, logging discipline, `DoCommand("reconcile")` — no reconcile steps implemented yet
3. ⏳ **First reconcile step: Firestore registration** — proves the data pipeline end-to-end on a real machine
4. ⏳ **Viam Application skeleton** — minimal machine-list table reading from Firestore, proves the UI pipeline
5. ⏳ **Remaining reconcile steps** — viam user → hostname → packages → tailscale, one at a time
6. ⏳ **UI thickening** — search, lab filter, unhealthy filter, detail panel

### Nice-to-Have (v2)

- Edit lab-level metadata (package list, tailscale tag, SSH keys) from the UI — the lab-level config refactor
- Trigger `reconcile` DoCommand from the UI via Viam SDK
- Live component health from the Viam SDK merged into the machine list
- Bulk actions (reconcile all in lab X)

### Bonus Round

- Historical reconcile log (requires storing more than `last_*` fields)
- Unattended upgrades, journald retention, SSH hardening, firewall, fail2ban — the "various housekeeping" pipeline, now cheap to add thanks to the pluggable reconcile framework
- Real Firestore auth: Cloud Function trading a FusionAuth cookie for a Firebase custom token
- Periodic reconcile loop (beyond the on-demand `DoCommand`)
- Additional OS support (Fedora, macOS, NixOS)

## Tech Stack

### Language(s)
- **Go** — generic service module (matches Viam SDK, matches lab-admin tooling conventions)
- **Svelte 5 + TypeScript** — Viam Application frontend. Viam Applications are static browser apps; Svelte 5's runes-based reactivity fits the "read from Firestore, render a table" shape cleanly with less ceremony than React.

### Frameworks/Libraries
- Viam Go SDK — module boilerplate, `generic.Service` interface, module context (part ID, machine ID, location, org)
- `cloud.google.com/go/firestore` — module-side writes to Firestore with a service account
- `@viamrobotics/sdk` — browser-side Viam auth and machine metadata
- Firebase JS SDK — browser-side Firestore reads
- Vite + Svelte 5 (TypeScript) — static-frontend build

### Platform/Deployment
- **Generic service:** distributed as a Viam module (registry or local). Runs as root under viam-server on Ubuntu + Raspberry Pi OS (amd64, arm64). Module metadata restricts supported architectures.
- **Viam Application:** static bundle deployed to Viam's application hosting; accessed at `<appname>_<namespace>.viamapplications.com` with FusionAuth-backed auth.
- **Firestore:** GCP native-mode Firestore in a dedicated project.

### Infrastructure
- GCP project (Firestore + one service account for module writes)
- Tailscale account with lab-tagged auth keys
- Viam org / Viam Application registration

## Technical Architecture

### Components

- **Generic service module** (Go, runs as root on each lab machine): implements Viam's `generic.Service` interface. Its `Reconfigure` and `DoCommand("reconcile")` entry points run a pluggable reconcile pipeline. Each step is a self-contained unit that reports a structured status. The module holds a Firestore client (constructed from the service-account key passed as a config attribute) for the registration step.
- **Firestore** (GCP): source of truth for the machine registry and (eventually) lab-level metadata. `machines` written by the module, read by the Viam Application. `labs` sparsely populated in v1.
- **Viam Application** (static Svelte 5 frontend): authenticates via FusionAuth, reads from Firestore using the Firebase JS SDK, uses the Viam SDK for deep links and future live-state integration. No server-side code.

### Integration points

- Module → Firestore: authenticated with a GCP service-account key passed via module config.
- Module → Tailscale: `tailscale up --authkey ...` with the lab's pre-tagged auth key.
- Module → System: direct `os/exec` to `useradd`, `apt-get`, `hostnamectl`, `tailscale`, etc. — all assume root.
- Browser → Firestore: Firebase JS SDK with public reads in v1.
- Browser → Viam: `@viamrobotics/sdk` with FusionAuth cookie, for deep links and future live-state enrichment.

### Data Schema

**Collection `machines`** — document ID is the Viam part ID (stable, unique, immutable).

| Field | Type | Source | Notes |
|---|---|---|---|
| `part_id` | string | Viam module context | Duplicated in doc ID for query convenience |
| `part_name` | string | Viam module context | Can change; kept in sync |
| `machine_id` | string | Viam module context | Parent machine |
| `location_id` | string | Viam module context | Viam location |
| `org_id` | string | Viam module context | Viam org |
| `hostname` | string | `os.Hostname()` | After normalization, should equal `part_name` |
| `os` | string | `/etc/os-release` ID + VERSION | e.g. `ubuntu 22.04` |
| `arch` | string | `runtime.GOARCH` | `amd64`, `arm64` |
| `tailscale_ip` | string | `tailscale ip -4` | 100.x address |
| `tailscale_hostname` | string | `tailscale status --json` | Tailnet name |
| `lab` | string | module config | Free-form lab identifier |
| `last_reconcile_at` | timestamp | `time.Now()` | Updated every reconcile |
| `last_reconcile_status` | map | reconcile result | `{user: "ok", tailscale: "ok", hostname: "ok", packages: "ok", firestore: "ok"}` — any step may be `"failed: <reason>"` |
| `module_version` | string | build-time constant | Tracks rollouts |

**Collection `labs`** — document ID is the lab identifier (matches `machines.lab`). Sparsely populated in v1; exists so the v2 secret-storage refactor has a home. Likely fields later: display name, tailscale tag, default package list, allowed SSH keys, default GCP service-account key reference.

### Configuration Variables

Module config attributes (per-machine in v1; migrate to lab-level Firestore in v2):

- `lab` — free-form lab identifier, written into every registration doc
- `tailscale_auth_key` — reusable, pre-tagged auth key for the lab's tailnet
- `authorized_ssh_key` — the public key to place in `/home/viam/.ssh/authorized_keys`
- `gcp_service_account_json` — JSON key for the Firestore-write service account
- `packages` — list of apt package names to ensure installed (default: `htop vim emacs tmux git curl wget rsync jq lsof build-essential`)
- `firestore_project` — GCP project ID hosting the Firestore database

## Development Process

**Testing approach:**
- **Unit tests:** reconcile-step logic in isolation — status reporting, error handling, the framework's aggregation of per-step results, OS detection, structured log-line emission. Filesystem and exec calls are abstracted behind an interface so steps can be tested without a real root shell.
- **Integration tests:** reconcile framework end-to-end with fake steps, verifying logging discipline (no-op = one line, work = summary lines, errors surface without stack traces).
- **Manual validation:** each milestone ends with deploying to a real Pi or Ubuntu VM and verifying the step runs, the registration appears in Firestore, and the UI reflects it.

**Deployment:**
- Module published via the Viam module registry (or local module during development)
- Viam Application built with Vite and deployed via the Viam Application upload flow
- Firestore schema managed in code (no migration tooling in v1; the schema is small and the module writes idempotently)
