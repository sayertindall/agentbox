# DevBox

**Status:** DRAFT FOR IMPLEMENTATION REVIEW  
**Created:** 2026-08-29  
**Authoritative location:** `/Users/sayertindall/Dev/DevBox`

## Objective

DevBox moves a dirty local project and task packet to a private VPS. Claude Code, Codex, or OMP then runs in a disposable VPS sandbox after the Mac loses Internet access.

## Documents

- [REQUIREMENTS.md](REQUIREMENTS.md) defines behavior, constraints, invariants, and acceptance criteria.
- [SPEC.md](SPEC.md) defines the architecture, privilege boundaries, transfer protocol, recovery rules, and operating model.
- [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md) defines the build order and verification gates. It does not authorize implementation.
- [RESEARCH.md](RESEARCH.md) records source research behind the technology choices.
- [INCUS_MICROSANDBOX_RESEARCH.md](INCUS_MICROSANDBOX_RESEARCH.md) compares microVM and Incus runtime adapters. It recommends a gated Microsandbox adapter and keeps the current systemd runtime as fallback.
- [VPS_AUDIT.md](VPS_AUDIT.md) records the read-only audit of `vps-bastion`. It blocks MicroSandbox and Incus VM adoption until KVM is available.
- [KVM_RUNTIME_DECISION.md](KVM_RUNTIME_DECISION.md) records Hostinger's nested-virtualization limitation and the resulting runtime choices.

## Decision summary

DevBox uses a root-owned VPS controller, separate restricted SSH control and transfer identities, positive source manifests, bounded rsync staging, and a server-side source lease. The normal control path is a forced SSH gateway. The normal transfer path can write only to a quota-bound staging token.

Each provider run gets a new Unix user, systemd cgroup, source workspace, `PASEO_HOME`, raw-history directory, and mode-0600 Paseo Unix socket. Root injects only one selected credential profile into that sandbox. After the run ends, DevBox stops the complete cgroup and archives runtime metadata under root ownership before another run can reuse that credential profile.

Nix flakes and devenv define project environments. Mise pins tools where a project needs it. Mutagen is not part of version 1 because it needs a separate restricted-identity design.

DevBox does not move a live provider process from the Mac to the VPS. It transfers a sanitized source projection and structured task packet. It detects local edits made outside DevBox during remote ownership and refuses automatic overwrite at reclaim.

## Current state

The local computer has Paseo 0.6.1, Claude Code, Codex, and OMP installed. Paseo reports all three providers as available locally. No VPS endpoint or operating-system evidence was supplied during design. DevBox is not provisioned.

## Implementation start condition

Supply a VPS SSH alias or `user@host`, Linux distribution and version, CPU architecture, firewall manager, a first project root, provider credential injection choices, profile count, retention values, staging quota, and monitor interval. The first implementation action is a read-only VPS audit.