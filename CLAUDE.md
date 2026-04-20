# CLAUDE.md

This file provides project-specific guidance to Claude Code (claude.ai/code) for this repository.

## Current Status

Project phase: **Setup** — `project_spec.md` written, no code yet. Next: Milestone 1 (GCP + Firestore bootstrap).

## About This Project

*Brief description of what this project does and its key components.*
a viam generic service that, when added to a machine, installs on that machine a standard suite of packages, keys, and services to make it easy to administer. it has to:
  - make sure there's a 'viam' user with a 'checkmate' password and a specific .ssh/authorized_keys entry
  - make sure tailscale is installed and up and registered with a specific auth key
  - register the machine's viam part name and OS hostname with a remote service (probably with a rest call)
  - implmement the viam app that will handle those registrations and provide a UI for browsing those machines
  - do various other housekeeping that makes it easy for lab admins to keep track of thier teams' various viam machines.

See `project_spec.md` for technical architecture, milestones, and implementation decisions.

## Project-Specific Conventions

*Override or extend global defaults here. Only include conventions that differ from or add to the global CLAUDE.md.*

### Testing

*Describe how to run tests and any project-specific testing conventions:*
```bash
make test
```

### Build / Run

*How to build and run the project:*
```bash
# Build
make build

# Run
make run
```
