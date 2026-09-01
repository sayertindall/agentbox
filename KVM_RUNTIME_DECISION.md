# Hostinger KVM runtime decision

**Status:** Provider limitation confirmed  
**Target:** `vps-bastion`  
**Confirmed:** 2026-08-29  

## Decision

Do not request KVM enablement for this Hostinger VPS.

Hostinger states that nested virtualization is not supported on its VPS plans. Microsandbox and Incus VMs require nested virtualization inside this QEMU guest. The host audit confirms that the guest has no `/dev/kvm` and no visible VMX or SVM CPU flag.

## What remains possible on Hostinger

Hostinger can still run the hardened DevBox systemd-per-run fallback after the existing host gates pass:

- Tailscale-only SSH and public SSH denial;
- enforced staging and run project quotas;
- cgroup resource limits;
- private provider credentials;
- per-run deny-by-default egress;
- Paseo, Claude Code, Codex, and OMP installation;
- restricted control and transfer identities.

This path does not provide a guest-kernel boundary around hostile agent code.

## What requires another host

Microsandbox microVMs and Incus VMs require a host that exposes KVM or nested virtualization. Hostinger's published policy directs workloads that need a hypervisor or nested VMs to a local environment or dedicated bare-metal infrastructure.

## Runtime choices

1. Keep `vps-bastion` and use the hardened systemd DevBox fallback.
2. Add a separate KVM-enabled or bare-metal VPS for Microsandbox runs while retaining `vps-bastion` as the control and archive host.
3. Move the complete DevBox deployment to a KVM-enabled or bare-metal host.

## Evidence

- [Hostinger nested-virtualization policy](https://www.hostinger.com/support/10429687-is-nested-virtualization-supported-in-hostinger/)
- [VPS audit](VPS_AUDIT.md)
- [Incus and Microsandbox comparison](INCUS_MICROSANDBOX_RESEARCH.md)