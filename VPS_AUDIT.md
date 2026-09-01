# VPS capability audit

**Status:** READ-ONLY AUDIT  
**Target:** `vps-bastion`  
**Audit date:** 2026-08-29  
**Changes made:** None

## Decision

Do not install Microsandbox or Incus on this VPS yet.

Microsandbox and Incus VMs are blocked because the guest has no `/dev/kvm` and no visible VMX or SVM CPU flag. The kernel has a `kvm_intel` module, but Hostinger does not expose nested virtualization on VPS plans. Incus system containers are not an acceptable hostile-agent boundary because they share the host kernel. The current DevBox systemd-per-run fallback is technically viable after host hardening, but the host is not ready for agent execution now.

## Verified host facts

| Area | Evidence | Result |
|---|---|---|
| Remote access | `id` | The audit connected as `root`. |
| Operating system | `/etc/os-release` | Ubuntu 26.04 LTS. |
| Architecture | `uname -m` | `x86_64`. |
| Kernel | `uname -r` | `7.0.0-27-generic`. |
| Service manager | `systemctl --version` | systemd 259. |
| cgroups | `/sys/fs/cgroup/cgroup.controllers` | cgroup v2 with `cpu`, `io`, `memory`, and `pids` controllers. |
| Root filesystem | `findmnt` | ext4. The active mount options do not show project quota support. |
| Memory | `free -b` | About 8.3 GB total and 7.8 GB available at audit time. |
| Root disk | `df -B1 /` | About 100.6 GB available at audit time. |
| glibc | `ldd --version` | glibc 2.43. Microsandbox's documented glibc floor is satisfied. |
| Virtualization | DMI, `lscpu`, `modinfo`, `/dev/kvm`, and CPU flags | The VPS is a QEMU guest on a KVM hypervisor. The Intel KVM module exists, but `/dev/kvm` and VMX or SVM flags are absent. The provider must expose nested virtualization before microVMs can run. |
| Tailscale | `tailscale status --json` | Running. |
| Microsandbox | `msb --version` | Not installed. |
| Incus | `incus version` | Not installed. |
| Paseo | `paseo daemon status` | Not installed. |
| Agent CLIs | `which claude codex omp` | Claude Code, Codex, and OMP are not installed. |

## Security findings

### Public SSH is not restricted to Tailscale

The host SSH service listens on public IPv4 and IPv6 addresses. UFW is inactive. The active nftables input chains have an accept policy and do not add a public SSH deny rule.

This does not meet the DevBox requirement that SSH accept only through the Tailscale interface. Do not install an agent control plane before the firewall and SSH policy are corrected and verified from an off-tailnet probe.

### Root is the current SSH identity

The audit used a root SSH session. DevBox must not run agents as root. The final design needs separate root-controlled services, restricted control and transfer identities, and disposable run users.

### Project quota is not active

The root filesystem is ext4, but the active mount options do not show a project quota mount option. The DevBox design requires enforced project quotas for both staging tokens and complete run trees. A future provisioning step must prove the filesystem supports the selected quota backend before it starts a run.

## Runtime assessment

| Runtime | Current result | Reason |
|---|---|---|
| Microsandbox microVM | Blocked | It requires KVM. The guest has no `/dev/kvm` or nested virtualization flag. The binary is also not installed. |
| Incus system container | Rejected as outer boundary | It would share the host kernel. It does not meet the hostile-agent isolation target. |
| Incus VM | Blocked | It requires KVM. The guest has no nested virtualization. Incus is not installed. |
| DevBox systemd sandbox | Conditional fallback | cgroup v2 and systemd are present. It still needs project quotas, SSH hardening, egress controls, tool installation, and credential isolation. |

## Required gates before provisioning

1. Configure the host firewall so SSH accepts only on `tailscale0` and rejects public TCP port 22. Verify from a non-tailnet source.
2. Choose and enable an enforced ext4 or XFS project-quota backend for DevBox staging and run trees. Prove both a staging quota and a run quota reject over-limit writes.
3. Install Paseo, Claude Code, Codex, and OMP under the DevBox provisioning model. Do not install provider credentials in the root account.
4. Create restricted DevBox control and transfer identities. Keep the Incus socket, Microsandbox home, and provider credentials inaccessible to agent runs.
5. Add a per-run deny-by-default egress policy. Allow only required model endpoints, required DNS, and explicitly required package registries. Deny private ranges, cloud metadata, host services, Tailscale ranges, and run-to-run traffic.
6. Use a separate KVM-enabled or bare-metal host if Microsandbox remains the target. Hostinger's published VPS policy does not support nested virtualization.

## Current recommendation

Use the DevBox systemd-per-run implementation as the fallback for this exact Hostinger VPS only after the first five gates above pass. Do not use Incus system containers as the outer agent boundary.

Use Microsandbox microVMs only on a separate host that exposes KVM and passes a capability spike for detached lifecycle, source copy and return, per-run provider authentication, deny-by-default egress, archive, and reconnect behavior.

Use Incus VMs only if Microsandbox fails the capability spike on that separate KVM-enabled host. Incus VM adoption also needs a separate QEMU resource-control proof and a DNS-aware egress design.

## Evidence

- [Incus and Microsandbox comparison](INCUS_MICROSANDBOX_RESEARCH.md)
- [DevBox requirements](REQUIREMENTS.md)
- [DevBox system design](SPEC.md)
- [DevBox implementation plan](IMPLEMENTATION_PLAN.md)
- [Hostinger nested-virtualization policy](https://www.hostinger.com/support/10429687-is-nested-virtualization-supported-in-hostinger/)
- [Hostinger KVM runtime decision](KVM_RUNTIME_DECISION.md)