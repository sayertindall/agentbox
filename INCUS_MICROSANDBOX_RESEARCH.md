# Incus and Microsandbox for DevBox

**Research date:** 2026-08-29  
**Scope:** Incus system containers, Incus virtual machines, Superrad Microsandbox microVMs, and the current DevBox systemd-per-run design for one Linux VPS.

## How to read this report

- **Verified** means the statement comes from an official project document, repository, source file, or release record. The link is next to the statement.
- **Assessment** means a direct mapping of verified behavior to a DevBox requirement.
- **Assumption** means a fact that DevBox must confirm on the selected VPS or during an adapter test.
- `Yes` means the primitive exists and can satisfy the criterion with a small DevBox adapter.
- `Partial` means a required part exists, but another part needs host policy, extra code, or a proof test.
- `No` means the documented model conflicts with the criterion.
- `Design only` means the current DevBox documents specify the behavior, but the repository says DevBox is not provisioned yet.

## Decision in one page

**Assessment:** Make Microsandbox local microVMs the first alternative runtime adapter behind `agentboxd`, after a host and provider proof gate. Keep the current systemd-per-run runtime as the fallback for a VPS without KVM or a supported Microsandbox build. Do not use an Incus system container as the security boundary for untrusted agent work. Use an Incus VM only as a second-choice adapter when Incus operational maturity matters more than host-only credentials and hostname-level egress policy.

Microsandbox is the only evaluated runtime with all of these documented in one local runtime: a dedicated guest kernel, a separate writable root, destination-aware egress rules, host-side secret substitution that never sends the real value into the guest, detached persistence, stable identity reconnect, and disk-only snapshots. Sources: [Microsandbox security model](https://docs.microsandbox.dev/security/overview.md), [network policy](https://docs.microsandbox.dev/networking/overview.md), [secret handling](https://docs.microsandbox.dev/security/secrets.md), and [lifecycle](https://docs.microsandbox.dev/sandboxes/lifecycle.md).

Incus has the strongest general-purpose host lifecycle and API story, and its container driver applies memory, CPU, I/O, and process limits with cgroups. Its system containers share the host kernel. Its network ACLs match IP and CIDR destinations and ports, not a provider hostname or TLS identity. Its documented credential mechanisms place credentials in the guest, rather than doing destination-bound host substitution. Sources: [Incus containers and VMs](https://linuxcontainers.org/incus/docs/main/explanation/containers_and_vms/), [Incus security](https://linuxcontainers.org/incus/docs/main/explanation/security/), [Incus ACLs](https://linuxcontainers.org/incus/docs/main/howto/network_acls/), and [Incus instance options](https://linuxcontainers.org/incus/docs/main/reference/instance_options/).

The current systemd design already has the needed receipt, source-lease, archive, and per-run user model. It specifies `MemoryMax`, `CPUQuota`, `TasksMax`, and a project-quota tree. It does not specify a per-run outbound model-API allowlist. It is a design, not a deployed runtime. Sources: [DevBox requirements](REQUIREMENTS.md), [DevBox system specification](SPEC.md), and [DevBox README](README.md).

## Version and project evidence

| Project | Version evidence retrieved | License and maturity evidence |
|---|---|---|
| Incus | The official GitHub release record reports Incus 7.4.0, published 2026-08-27: [release API](https://api.github.com/repos/lxc/incus/releases/latest), [release page](https://github.com/lxc/incus/releases/tag/v7.4.0). | The official repository reports Apache-2.0 and is not archived: [repository metadata](https://api.github.com/repos/lxc/incus). The security guide distinguishes feature and LTS releases and says to use only supported versions: [supported versions](https://linuxcontainers.org/incus/docs/main/explanation/security/#supported-versions). |
| Microsandbox | The official GitHub release record reports v0.6.16, published 2026-08-29: [release API](https://api.github.com/repos/superradcompany/microsandbox/releases/latest), [release page](https://github.com/superradcompany/microsandbox/releases/tag/v0.6.16). | The official repository reports Apache-2.0: [repository](https://github.com/superradcompany/microsandbox). Its README explicitly says: “Microsandbox is still beta software. Expect breaking changes, missing features, and rough edges.” The v0.6.16 notes include the convergent lifecycle API as a newly released feature: [release notes](https://github.com/superradcompany/microsandbox/releases/tag/v0.6.16). |
| Current DevBox design | The repository documents Paseo 0.6.1 and a draft implementation plan. The README states “DevBox is not provisioned”: [README](README.md), [implementation plan](IMPLEMENTATION_PLAN.md). | This is an internal design, not a released runtime. The requirements file is marked “DRAFT FOR IMPLEMENTATION REVIEW”: [requirements](REQUIREMENTS.md). |

**Assessment:** Incus has a longer-established general-purpose management model, but that is an assessment, not a security guarantee. Microsandbox has the security primitives DevBox needs, but its own documentation calls the release beta. Pin the exact release and test the selected provider images before production use.

## DevBox requirements used for comparison

Verified from the current DevBox files:

- The VPS must own source data, task context, process supervision, provider credentials, and recovery state before a run starts. A laptop mount, raw client port forward, or live sync process cannot be a runtime dependency. See [SPEC background](SPEC.md#background).
- A run gets a new Unix user, a systemd cgroup, a workspace, `PASEO_HOME`, raw history, and a private Paseo Unix socket. See [requirements scope](REQUIREMENTS.md#scope) and [SPEC deployment shape](SPEC.md#deployment-shape).
- Host-enforced storage quotas and memory, CPU, and task limits are required. See [FR-006](REQUIREMENTS.md#fr-006-start-a-receipt-bound-sandboxed-run) and [NFR-REL-003](REQUIREMENTS.md#nfr-rel-003).
- The run must continue when the Mac disconnects. The client reconnects through receipt-bound server operations. See [FR-007](REQUIREMENTS.md#fr-007-continue-after-local-disconnect) and [FR-008](REQUIREMENTS.md#fr-008-observe-a-receipt-bound-run).
- The return candidate is created only after the entire run cgroup stops. Runtime metadata is archived under root ownership. See [FR-009](REQUIREMENTS.md#fr-009-return-reclaim-resolve-and-recover) and [FR-010](REQUIREMENTS.md#fr-010-reconcile-uncertainty-and-terminate-sandboxes).
- The selected provider profile is injected into one run only. The run must not read another run's workspace, credential, raw history, control state, or socket. See [FR-006](REQUIREMENTS.md#fr-006-start-a-receipt-bound-sandboxed-run) and [FR-011](REQUIREMENTS.md#fr-011-isolate-every-run-sandbox).
- DevBox requires private endpoints and a Tailscale-only SSH path. It currently requires outbound model-provider Internet access, but does not define a per-run model-host allowlist. See [SPEC security invariants](SPEC.md#security-invariants) and [REQUIREMENTS assumptions](REQUIREMENTS.md#assumptions).

## Host prerequisites and adoption blockers

### Incus system containers and VMs

**Verified:** The Incus daemon works only on Linux. The official requirements list a minimum supported kernel version of 6.12, cgroup controllers (`blkio`, `cpuset`, `devices`, `freezer`, `memory`, and `pids`), namespaces (`cgroup`, `ipc`, `pid`, `mount`, `net`, `user`, and `uts`), seccomp, and native Linux AIO. Incus requires LXC 6.0.0 or newer. VMs require QEMU 8.2 or newer. Network tooling minimums are nftables 1.0.0 and dnsmasq 2.90. The storage-driver minimums depend on the selected driver. See the [Incus requirements](https://linuxcontainers.org/incus/docs/main/requirements/).

**Verified:** Incus unprivileged containers need subordinate UID and GID ranges. The installation guide shows a contiguous root range in `/etc/subuid` and `/etc/subgid`. Local access to the Unix socket is controlled by root and `incus-admin`; members of that group have full Incus control. See [installation](https://linuxcontainers.org/incus/docs/main/installing/#machine-setup) and [daemon access](https://linuxcontainers.org/incus/docs/main/explanation/security/#security-daemon-access).

**VM-specific blocker:** VMs need hardware virtualization. The Incus model uses a dedicated guest kernel and hardware virtualization. A VPS provider that hides KVM cannot run Incus VMs. See [containers and VMs](https://linuxcontainers.org/incus/docs/main/explanation/containers_and_vms/) and [Incus QEMU requirements](https://linuxcontainers.org/incus/docs/main/requirements/#qemu).

**Assumption:** The selected VPS has kernel 6.12 or newer, the listed cgroup and namespace features, the required LXC and QEMU versions, working subordinate-ID configuration, and KVM access for VM mode. DevBox has no VPS identity, distribution, kernel, architecture, storage, or firewall evidence yet. The repository requires a read-only host audit before VPS mutation. See [DevBox implementation start condition](README.md#implementation-start-condition).

### Microsandbox local microVMs

**Verified:** Microsandbox Linux releases require glibc 2.28 or newer on x64 and ARM64 hosts. The local runtime needs KVM through `/dev/kvm`. The current Linux guide says the user must be able to read and write `/dev/kvm`; nested virtualization must be exposed when the VPS is itself a VM. Musl-only hosts such as bare Alpine are not supported for the local runtime. See [Linux troubleshooting](https://docs.microsandbox.dev/troubleshooting/linux.md).

**Verified:** The README lists Linux with KVM enabled as a requirement, and it warns that Microsandbox is beta. See the [official README](https://raw.githubusercontent.com/superradcompany/microsandbox/main/README.md).

**Assumption:** The VPS has a supported glibc-based distribution, KVM access, and a host policy that permits the root-owned DevBox controller to access `/dev/kvm`. DevBox must test this with `msb doctor`, a real VM boot, a stop, a reconnect, and an image-cache check.

### Current systemd-per-run design

**Verified:** The current specification requires Linux systemd, project quotas for staging and run trees, cgroup resource limits, root-owned services and paths, restricted SSH identities, Tailscale, and a controllable firewall. See [host configuration](SPEC.md#host-configuration) and [operating assumptions](REQUIREMENTS.md#assumptions).

**Assessment:** The design has fewer runtime dependencies than Incus or Microsandbox, but it cannot claim a working deployment until the host audit and VPS smoke tests pass.

## Detailed evidence by runtime

### Incus system container

#### Isolation and security

**Verified:** Incus system containers use Linux namespaces and cgroups and share the host kernel. They are software-only isolation. By default they are unprivileged and use a user namespace. Incus documents `security.idmap.isolated` for non-overlapping UID and GID maps. Incus warns that privileged containers are not root-safe and can allow a container root to deny service to or escape the host. See [containers and VMs](https://linuxcontainers.org/incus/docs/main/explanation/containers_and_vms/) and [container security](https://linuxcontainers.org/incus/docs/main/explanation/security/#container-security).

**Assessment:** A fresh unprivileged container and a fresh isolated ID map are useful run boundaries, but they do not provide a separate kernel. Do not treat an Incus container as equivalent to a microVM for hostile coding-agent code.

#### Resource and storage controls

**Verified:** Incus documents `limits.cpu` and `limits.cpu.allowance`. A time-form allowance is a hard limit for containers. The CPU limits use the cpuset and CPU cgroup controllers. Incus also documents memory limits, process limits, and disk I/O priority. See [resource limits](https://linuxcontainers.org/incus/docs/main/reference/instance_options/#instance-options-limits).

**Verified source behavior:** The Incus LXC driver applies memory limits, CPU shares or CFS quotas, I/O priority, and the pids limit to the instance cgroup when the corresponding controllers exist. See [`driver_lxc.go` limit application](https://github.com/lxc/incus/blob/main/internal/server/instance/drivers/driver_lxc.go#L1173-L1305).

**Verified:** Storage quotas depend on the storage driver. The `dir` driver supports quotas only on ext4 or XFS with project quotas enabled. See [`dir` storage quotas](https://linuxcontainers.org/incus/docs/main/reference/storage_dir/#quotas).

**Assessment:** Incus containers can satisfy whole-run CPU, memory, process-count, and storage controls when the host exposes the required cgroup controllers and quota backend. The host audit must prove those conditions. The limits apply to the container's process tree, which is the scope DevBox needs.

#### Network policy

**Verified:** A managed bridge creates an L2 segment, provides DHCP and DNS, and performs NAT by default. Incus warns that an untrusted instance on the default bridge can transmit arbitrary L2 traffic and spoof MAC or IP addresses. NIC filtering uses nftables. See [bridge networking](https://linuxcontainers.org/incus/docs/main/_sources/reference/network_bridge.md.txt) and [network security](https://linuxcontainers.org/incus/docs/main/explanation/security/#bridged-nic-security).

**Verified:** Network ACLs have ingress and egress rule lists. Egress rules match destination CIDR or IP ranges, protocol, and destination ports. An ACL adds a default reject for unmatched traffic unless changed. Bridge ACLs apply at the bridge-to-host boundary and cannot create intra-bridge firewalls unless applied directly to the NIC. See [network ACLs](https://linuxcontainers.org/incus/docs/main/_sources/howto/network_acls.md.txt).

**Assessment:** Incus can provide a private managed network and IP-and-port egress rules. Its documented ACL format does not match a model hostname, TLS SNI, or HTTP authority. A provider API that uses changing IPs needs an external DNS-aware proxy, maintained IP sets, or a separate host firewall design. Incus ACLs alone do not satisfy a hostname-level outbound model API allowlist.

#### Credentials

**Verified:** Incus instance options include `environment.*`, `systemd.credential.*`, and `systemd.credential-binary.*`. The docs say systemd credentials are passed as a read-only bind mount in containers. See [instance options](https://linuxcontainers.org/incus/docs/main/reference/instance_options/#instance-miscellaneous).

**Assessment:** DevBox can select one credential and inject it into one container, but the documented mechanisms make the credential guest-visible. Incus does not document destination-bound host-side substitution that keeps the real value out of the guest. Use a separate root-controlled adaptor and treat the credential as exposed inside that run. Do not place it in a shared profile directory.

#### Workspace, lifecycle, and archives

**Verified:** `incus launch` creates and starts a container. `incus exec` runs a command in the instance. The `ephemeral` instance property deletes an instance when it is stopped. See [create instances](https://linuxcontainers.org/incus/docs/main/howto/instances_create/) and [instance properties](https://linuxcontainers.org/incus/docs/main/reference/instance_properties/).

**Verified source behavior:** The LXC driver deletes an instance after a non-reboot stop when the ephemeral flag is set. See [`driver_lxc.go` ephemeral stop path](https://github.com/lxc/incus/blob/main/internal/server/instance/drivers/driver_lxc.go#L3825-L3831).

**Verified:** Instance snapshots are point-in-time snapshots. They are stored in the same pool. By default they have no expiry unless configured. Custom storage volumes attached to an instance are not included in the instance backup and need separate backup. Export creates a standalone file. `incus copy` duplicates an instance or a snapshot. See [instance backup](https://linuxcontainers.org/incus/docs/main/howto/instances_backup/) and [move and copy](https://linuxcontainers.org/incus/docs/main/_sources/howto/move_instances.md.txt).

**Assessment:** An Incus container provides a persistent workspace until the instance is deleted. A DevBox adapter can stop it, verify stopped state, export or copy the workspace, archive runtime metadata outside Incus, and then delete it. Do not use an attached custom volume without a separate archive step. Use `--ephemeral` only when the adapter has already copied the return candidate and archive.

#### Host control and reconnect

**Verified:** Incus clients use a REST API over a local Unix socket or a remote TLS socket. Long operations return background operation IDs that can be polled or observed through notifications. The local socket grants full Incus control. See [REST API](https://linuxcontainers.org/incus/docs/main/rest-api/) and [daemon access](https://linuxcontainers.org/incus/docs/main/explanation/security/#security-daemon-access).

**Assessment:** `agentboxd` can keep all Incus control on the VPS and expose only receipt-bound operations through the existing forced SSH gateway. Never give a DevBox control key direct Incus socket or remote API access. The `incus-admin` group is equivalent to full host control for this purpose.

### Incus VM

#### Isolation and security

**Verified:** Incus VMs use hardware virtualization and a dedicated guest kernel. Incus documents the VM boundary as hardware-enforced, unlike system containers. See [containers and VMs](https://linuxcontainers.org/incus/docs/main/explanation/containers_and_vms/).

**Assessment:** Incus VM isolation is closer to the DevBox security goal than an Incus container. Incus still trusts its root daemon, its host kernel, QEMU, and the image supply chain. A run must not receive host paths or the Incus socket.

#### Resource and storage controls

**Verified:** Incus VM `limits.cpu` controls vCPU count, CPU topology, or pinning. VM CPU allowance is documented as container-only. VM memory is assigned and can be hotplugged or reduced with a balloon device. The Incus QEMU driver reports `CGroup()` as not implemented. See [CPU and memory limits](https://linuxcontainers.org/incus/docs/main/reference/instance_options/#instance-options-limits) and [`driver_qemu.go`](https://github.com/lxc/incus/blob/main/internal/server/instance/drivers/driver_qemu.go#L9722-L9725).

**Assessment:** Incus VM limits provide a guest vCPU count, guest memory allocation, and storage-volume limit. They do not, by the documented API, provide the same host cgroup CPU-quota and process-tree controls that an Incus container provides. To meet DevBox's whole-run limit contract, the adapter needs a separate host cgroup or an independently managed QEMU service slice, and it must prove that the QEMU process and all helper processes enter that slice. Mark this criterion `Partial` until that proof exists.

#### Network policy and credentials

**Verified:** VM NICs can use Incus managed networks and ACLs. The ACL rule format still uses destination CIDR or IP ranges, protocol, and ports. See [network ACL rule properties](https://linuxcontainers.org/incus/docs/main/_sources/howto/network_acls.md.txt#network-acls-rules-properties).

**Assessment:** Incus VMs have the same hostname-allowlist gap as containers. Incus's `systemd.credential.*` option passes a credential to a VM through SMBIOS Type 11 data, according to the [instance option documentation](https://linuxcontainers.org/incus/docs/main/reference/instance_options/#instance-miscellaneous). The credential is therefore available to the guest, not held only by a host network proxy.

#### Workspace, lifecycle, and reconnect

**Verified:** Incus supports the same instance create, start, stop, snapshot, export, copy, and REST control model for VMs. VM snapshots can be stateful, which captures running state, but custom attached storage volumes remain separate. `incus exec`, file operations, and detailed VM metrics need an Incus Agent in the guest. Official VM images from the `images` remote are preconfigured for the agent. See [VM agent setup](https://linuxcontainers.org/incus/docs/main/howto/instances_create/#install-the-incus-agent-into-virtual-machine-instances) and [instance backup](https://linuxcontainers.org/incus/docs/main/howto/instances_backup/).

**Assessment:** Incus VMs provide persistent workspace and robust host reconnect after the Incus daemon has the instance. They have more boot, image, guest-agent, and storage setup than containers. Deterministic archiving is possible with stop, export, and separate volume handling, but must be implemented by `agentboxd`.

### Superrad Microsandbox microVM

#### Isolation and security

**Verified:** Microsandbox describes each sandbox as a VM with its own Linux kernel, filesystem, and network boundary. The guest cannot make host syscalls, read host memory, or open host connections directly. The host is trusted, and the microVM and hypervisor enforce the guest boundary. See [security overview](https://docs.microsandbox.dev/security/overview.md) and [isolation boundary](https://docs.microsandbox.dev/security/isolation.md).

**Verified:** Each sandbox has a private writable layer. Writes do not change the shared image cache or another sandbox. The guest sees only its image and explicit mounts. See [filesystem and images](https://docs.microsandbox.dev/security/filesystem.md).

**Assessment:** This is the strongest evaluated boundary for an untrusted coding agent. It does not protect against a compromised VPS, hypervisor, CPU, or host launcher. The host and image registry remain part of the trusted base.

#### Resource and storage controls

**Verified:** Microsandbox defaults to one vCPU and 512 MiB of guest memory. The SDK exposes CPU and memory limits, maximum hotpluggable CPU and memory, root-disk configuration, and named or disk volumes. See [sandbox overview](https://docs.microsandbox.dev/sandboxes/overview.md), [Go options source](https://github.com/superradcompany/microsandbox/blob/main/sdk/go/options.go#L600-L805), and [global configuration](https://docs.microsandbox.dev/configuration.md#sandbox_defaults).

**Verified:** The Go source exposes guest baseline rlimits. The in-guest agent applies those limits to PID 1 before later guest processes start, so descendant processes inherit them. See [`crates/agentd/lib/rlimit.rs`](https://github.com/superradcompany/microsandbox/blob/main/crates/agentd/lib/rlimit.rs#L30-L71).

**Assessment:** A microVM's vCPU, guest memory, private root-disk size, volume quotas, and inherited guest rlimits cover whole-run limits inside the VM. They are VM limits, not Linux cgroup settings. DevBox still needs host capacity accounting and a test that root-disk, volume, and process limits fail closed. Network rate limits are separate from CPU, memory, disk, and API-request limits. See [network rate limits](https://docs.microsandbox.dev/networking/overview.md#rate-limits).

#### Outbound network policy

**Verified:** Microsandbox routes sandbox traffic through a host-controlled user-space network stack. The default allows public Internet access and denies private ranges, loopback, link-local, cloud metadata, and the host. Custom rules can use a default action and first-match rules with public, private, host, IP, CIDR, domain, and port targets. See [network overview](https://docs.microsandbox.dev/networking/overview.md) and [network defenses](https://docs.microsandbox.dev/security/network.md).

**Verified source behavior:** The Go SDK's `NetworkConfig` has default egress and ingress actions, domain deny lists, DNS rebind protection, TLS settings, and rules. A `PolicyRule` destination may be an IP, CIDR, domain, domain suffix, or named group. See [`sdk/go/options.go` network and secret types](https://github.com/superradcompany/microsandbox/blob/main/sdk/go/options.go#L1043-L1170) and [network policy factory](https://github.com/superradcompany/microsandbox/blob/main/sdk/go/options.go#L1243-L1310).

**Verified:** Domain rules use DNS answers pinned by the interceptor. TLS rules require matching SNI and the pinned destination IP. HTTP interception also checks the request authority against SNI. The network guide documents DNS-over-HTTPS and tunneled DNS as bypass surfaces for DNS-based defenses. See [DNS-to-IP binding and bypasses](https://docs.microsandbox.dev/security/network.md#dns-to-ip-binding-toctou).

**Assessment:** Microsandbox can express a per-run allowlist such as `api.openai.com:443` or an exact provider domain and keep private VPS networks denied. To make this fail closed, set default egress to deny, allow only the required model domains and DNS gateway, and deny or constrain tunnels and alternate DNS transports. Treat the allowed provider as trusted because it receives the real credential when secret substitution succeeds.

#### Credential isolation

**Verified:** Microsandbox puts a placeholder in the guest. A real value stays in host memory and is substituted only at the network boundary when the allowed host, DNS pin, TLS identity, and authority checks pass. The real value is never written to the guest environment, guest disk, or snapshot. See [secret handling](https://docs.microsandbox.dev/security/secrets.md).

**Verified source behavior:** `SecretEntry.Value` is the actual secret, while the source comment says it never crosses the FFI into the guest. `AllowHosts`, `AllowHostPatterns`, `RequireTLS`, and `OnViolation` are part of the Go option. See [`sdk/go/options.go` SecretEntry](https://github.com/superradcompany/microsandbox/blob/main/sdk/go/options.go#L1313-L1381).

**Caveat:** The secrets guide warns that raw SDK values are saved as-is in the sandbox config until rotated to a reference. The CLI `ENV@HOST` form stores a host-side environment reference instead of an inline value. See [secret configuration](https://docs.microsandbox.dev/sandboxes/secrets.md#add-at-create-time).

**Assessment:** Microsandbox supplies the right credential primitive for DevBox. `agentboxd` still needs the DevBox-level profile lock, provider selection, root-owned storage, rotation, and revocation checks. Prefer a CLI reference or a root-owned environment handoff over a raw SDK value when at-rest exposure matters.

#### Workspace, lifecycle, snapshots, and archive

**Verified:** A sandbox's state is persisted. A stopped sandbox can be restarted with its configuration. A detached sandbox continues after the creating process exits and can be reattached by name or stable ID. The Go SDK exposes `ConnectOrCreateSandbox`, `GetSandbox`, `StartSandbox`, and `StartSandboxDetached`. See [lifecycle](https://docs.microsandbox.dev/sandboxes/lifecycle.md) and [Go sandbox API](https://docs.microsandbox.dev/sdk/go/sandbox.md).

**Verified source behavior:** `StartSandboxDetached` starts the persisted sandbox in detached mode. `Stop` waits for stopped state, `Kill` force-kills and waits for stopped state, `RequestStop` and `RequestKill` are asynchronous requests, `WaitUntilStopped` observes terminal state, and `Detach` releases a detached handle without stopping the VM. See [`sdk/go/sandbox.go`](https://github.com/superradcompany/microsandbox/blob/main/sdk/go/sandbox.go#L495-L508) and [lifecycle methods](https://github.com/superradcompany/microsandbox/blob/main/sdk/go/sandbox.go#L989-L1041).

**Verified:** Local snapshots are disk-only and require a stopped or crashed sandbox. They capture writable filesystem changes, image identity, labels, and an optional integrity hash. They do not capture memory, running processes, or network state. The snapshot directory is portable and can be saved as `.tar.zst`; including the image cache makes a target fully offline. See [snapshots](https://docs.microsandbox.dev/sandboxes/snapshots.md) and [`sdk/go/snapshot.go`](https://github.com/superradcompany/microsandbox/blob/main/sdk/go/snapshot.go#L12-L52).

**Verified:** `WithEphemeral(true)` removes the sandbox database row, on-disk state, logs, and captured output after terminal status. See [`sdk/go/options.go`](https://github.com/superradcompany/microsandbox/blob/main/sdk/go/options.go#L791-L800).

**Assessment:** Use a non-ephemeral detached sandbox for the run. On terminal reconciliation, call `Stop` with a bounded timeout, escalate to `Kill`, wait for stopped state, copy the allowed workspace and logs into the root-owned DevBox archive, optionally create and verify a disk snapshot, and only then remove the sandbox. Do not use an ephemeral sandbox before the archive is durable.

#### Host control and reconnect

**Verified:** The local runtime is child-process based and does not require a long-running daemon. A local sandbox can be created by the CLI or SDK, and the SDK can list, inspect, reconnect, stop, kill, snapshot, and remove sandboxes. Global paths, including the home, database, sandboxes, volumes, snapshots, logs, and secrets directories, are configurable under `~/.microsandbox/config.json`. See [lifecycle](https://docs.microsandbox.dev/sandboxes/lifecycle.md), [global configuration](https://docs.microsandbox.dev/configuration.md), and [Go sandbox API](https://docs.microsandbox.dev/sdk/go/sandbox.md).

**Assessment:** Run Microsandbox under the root-owned `agentboxd` service with a dedicated `MSB_HOME` equivalent and receipt-derived names. Keep direct `msb` and SDK access off ordinary DevBox SSH keys. The daemonless local model is an adoption blocker for crash recovery: DevBox must reconcile persisted sandbox state and detached runtime processes after `agentboxd` restarts. The docs guarantee survival after the creator process exits, but they do not promise that a running VM survives a VPS reboot or that it auto-restarts. Mark host-reboot recovery as an assumption until tested.

#### Provider tooling fit

**Verified:** Official Microsandbox examples run pinned Claude Code, Codex CLI, and Pi coding-agent versions inside a microVM. The examples set vCPU, memory, root-disk, workdir, and either a host directory mount or an isolated copy. See [Claude Code](https://docs.microsandbox.dev/examples/agents/claude-code.md), [Codex CLI](https://docs.microsandbox.dev/examples/agents/codex.md), and [Pi](https://docs.microsandbox.dev/examples/agents/pi.md).

**Assessment:** The examples prove that the runtime can host the provider binaries DevBox cares about, but their writable host-directory mode is not DevBox's canonical source boundary. DevBox should use its verified source projection and copy it into the sandbox or a private volume. The official docs index contains provider examples for Claude Code, Codex CLI, and Pi, but no Paseo or OMP example. [Observation] Paseo and OMP integration remains DevBox adapter work.

**Verified supply-chain caveat:** Microsandbox verifies image layers against manifest SHA-256 digests but does not verify signatures or attestations. The registry is part of the trusted base. See [filesystem and image supply chain](https://docs.microsandbox.dev/security/filesystem.md#image-supply-chain).

### Current systemd-per-run design

#### Isolation, credentials, and resources

**Verified:** The DevBox specification assigns each run a new Unix user, a private workspace, a private `PASEO_HOME`, a raw-history directory, a mode-0600 Paseo socket, and a selected credential profile. It requires `NoNewPrivileges`, `PrivateTmp`, `PrivateDevices`, `ProtectHome`, `ProtectSystem=strict`, `ReadWritePaths`, `MemoryMax`, `CPUQuota`, and `TasksMax`. See [run service](SPEC.md#run-service).

**Verified:** The design assigns the entire run tree to a host-enforced project quota before creating the run user. See [host configuration](SPEC.md#host-configuration) and [FR-006](REQUIREMENTS.md#fr-006-start-a-receipt-bound-sandboxed-run).

**Assessment:** If implemented exactly and verified on the VPS, this design provides a strong host-level process, filesystem, and resource boundary. It shares the host kernel. The current design does not claim hardware isolation.

#### Network policy

**Verified:** The current design's network invariants limit ordinary access to Tailscale SSH and reject public TCP 22. Its assumptions require outbound Internet access to model providers. See [SPEC security invariants](SPEC.md#security-invariants) and [requirements assumptions](REQUIREMENTS.md#assumptions).

**Assessment:** The current design does not define a per-run outbound destination or port allowlist. It therefore fails the strict model-API allowlist criterion as written. Add a host-side egress policy keyed to a run identity, cgroup, network namespace, or proxy before claiming this criterion. The policy must also protect DNS and prevent the run from reaching private VPS ranges.

#### Workspace, stop, archive, and reconnect

**Verified:** The design keeps source and runtime metadata separate. After terminal reconciliation it stops the entire run cgroup, moves workspace and metadata into root-owned archives, removes the run user and socket, removes the unit, and releases the credential lock only after cleanup succeeds. See [terminal archive flow](IMPLEMENTATION_PLAN.md#task-6-implement-credential-profiles-disposable-run-sandboxes-and-environment-wrappers) and [FR-009/FR-010](REQUIREMENTS.md#fr-009-return-reclaim-resolve-and-recover).

**Assessment:** The current systemd design is the most direct match for DevBox's receipt-bound archive and source-lease protocol. It remains `Design only` until the host audit, implementation, and integration checks pass.

## Decision matrix

The matrix rates the runtime primitive, not the full DevBox product. Every option still needs the DevBox source projection, receipt, source lease, provider start, archive, retention, and reconciliation adapter.

| Criterion | Incus system container | Incus VM | Microsandbox microVM | Current systemd-per-run design |
|---|---|---|---|---|
| Offline VPS-owned operation after Mac disconnect | **Yes, with adapter**. Incus persists instance state under the daemon and exposes start and stop as daemon API operations. | **Yes, with adapter**. Same daemon model, with VM guest-agent setup. | **Yes, with adapter**. Detached sandboxes survive creator-process exit; VPS-reboot behavior is an unverified assumption. | **Design only: Yes**. FR-007 explicitly removes local client and sync dependencies after `remote_running`. |
| Per-run credential isolation | **Partial**. Separate instance and Unix/user namespaces isolate runs, but documented environment/systemd credentials are guest-visible. | **Partial**. Separate VM isolates runs, but SMBIOS credential injection remains guest-visible. | **Yes** for the isolation primitive. Real values stay in host memory and are substituted only for allowed destinations. DevBox profile locks and at-rest policy remain required. | **Design only: Yes**. Fresh run user and selected profile are specified; the adaptor is not implemented or verified. |
| Whole-run resource limits | **Yes, host-dependent**. LXC applies memory, CPU, I/O, pids, and kernel limits with cgroups; storage quota needs a supported filesystem/backend. | **Partial**. vCPU, guest memory, and storage limits exist, but the QEMU driver does not implement the Incus cgroup API; add and prove a host cgroup. | **Yes** as VM limits. vCPU, guest memory, root disk, volume quota, and guest rlimits exist. | **Design only: Yes**. Project quota plus `MemoryMax`, `CPUQuota`, and `TasksMax` are specified. |
| Outbound model API network allowlists | **Partial**. Egress ACLs can allow IP/CIDR and port, but not a provider hostname or TLS identity. Bridge defaults need hardening. | **Partial**. Same ACL and hostname gap. | **Yes, with fail-closed policy**. Domain/IP/CIDR/port policy, private-range denial, DNS pinning, SNI checks, and secret destination checks are documented. Close DNS tunnels and use deny-by-default. | **No in current design**. Tailscale-only SSH is specified, but no per-run outbound model allowlist is specified. |
| Persistent workspace during a run | **Yes**. Instance root and attached disks persist until deletion. | **Yes**. VM root disk and attached disks persist until deletion. | **Yes**. Per-sandbox writable root, named volumes, and stopped-sandbox restart persist. | **Design only: Yes**. Workspace and generation paths are persistent until terminal archive. |
| Deterministic stop and archive | **Yes, with adapter**. `incus stop --force`, export, snapshot, copy, and ephemeral deletion exist. Custom volumes need a separate archive. | **Yes, with adapter**. Force stop, export, and stateful snapshots exist. VM agent and custom volumes need explicit handling. | **Yes, with adapter**. `Stop`, `Kill`, `WaitUntilStopped`, disk snapshot, copy, and remove exist. Archive before `Ephemeral`. | **Design only: Yes**. Stop the full cgroup, verify no process remains, archive under root, then remove unit/user/socket. |
| Local reconnect and host lifecycle control | **Yes**. Local Unix socket or remote TLS REST API, `incus exec`, operation IDs, and notifications. Protect the full-control socket. | **Yes**. Same API, but `incus-agent` is needed for VM exec/file/metrics. | **Yes, with adapter**. Stable names/IDs, `GetSandbox`, `ConnectOrCreateSandbox`, `ConnectOrStart`, logs, metrics, stop, kill, and snapshot. Reconcile after controller crash. | **Design only: Yes**. Receipt-bound forced gateway and root-owned `agentboxd` are specified. |

## Practical adoption blockers

### Blockers shared by all options

1. **Unknown VPS capability.** DevBox has no selected Linux distribution, kernel, architecture, KVM state, storage backend, or firewall manager. Run the planned read-only host audit first. This is a repository fact, not a guess. See [README](README.md#implementation-start-condition).
2. **Provider state is not portable.** The DevBox design starts a new provider process from a sanitized source projection. None provides a DevBox-supported migration of a live provider process from Mac to VPS. See [SPEC background](SPEC.md#background).
3. **Source and runtime metadata need separate handling.** A runtime snapshot is not the DevBox return candidate. Keep packet, synthetic Git state, provider logs, and receipts outside the source projection.
4. **Images and tools must be pinned.** Incus images and Microsandbox OCI images have their own supply-chain policies. Use immutable digests or an equivalent verified image record, and keep the image cache on the VPS.

### Incus-specific blockers

1. **System-container kernel sharing.** A hostile agent can attack the common Linux kernel. Use an Incus VM if the threat model requires a hardware boundary.
2. **Credential visibility.** Incus's documented credential options deliver data into the guest. Build and review a root-only injection adaptor, or do not use Incus for credentials that must remain outside the guest.
3. **Hostname allowlists.** Incus ACLs are L3 or L4 rules. Add a DNS-aware proxy, maintained destination IP sets, or another host policy if an exact model hostname allowlist is required.
4. **VM resource gap.** The QEMU driver does not implement the Incus cgroup API. Prove a separate QEMU cgroup or accept a weaker VM resource contract.
5. **VM agent and KVM.** Incus VM command execution, file transfer, and metrics need an agent. The VPS must expose KVM and supply compatible QEMU and guest images.
6. **Attached-volume backup.** Incus instance snapshots and exports do not include attached custom storage volumes. Archive every volume explicitly.

### Microsandbox-specific blockers

1. **Beta release.** v0.6.16 is current by the official release endpoint and the README still warns of breaking changes, missing features, and rough edges. Pin the binary, SDK, kernel, image digests, and config format.
2. **Daemonless local runtime.** Detached VMs survive creator-process exit, but DevBox must own the persisted database, process reconciliation, and post-reboot policy. The docs do not promise host-reboot continuation or automatic restart.
3. **KVM and glibc.** A nested or restricted VPS may lack `/dev/kvm`; a musl-only VPS cannot run the local runtime. Keep the systemd-per-run fallback until the audit and smoke tests pass.
4. **Raw SDK secret persistence.** The SDK can save raw secret values in the host sandbox config. Prefer a CLI environment reference or an equivalent root-owned indirection, and verify permissions and cleanup.
5. **Allowed endpoint trust.** The allowed provider receives the real credential. The protection is against unintended hosts, not against a provider that is explicitly allowed.
6. **No first-party Paseo or OMP adapter.** Official examples cover Claude Code, Codex CLI, and Pi. DevBox must supply the command wrapper, source copy, receipt labels, socket or control path, and terminal reconciliation.
7. **DNS tunnel bypasses.** DNS-based policy cannot inspect every DoH or tunneled DNS flow. Restrict egress to the exact provider destinations and the required DNS gateway, and test alternate transports.

### Current systemd-specific blockers

1. **Not implemented.** The design is detailed, but the repository says DevBox is not provisioned.
2. **No per-run egress allowlist.** Add and test one before treating model credentials as protected by network policy.
3. **Credential adaptor unknown.** Each provider needs a vendor-approved headless authentication method and a test that a run cannot read another profile or archived state.
4. **Host enforcement depends on the audit.** Project quotas and cgroup limits must be proven on the selected filesystem and kernel.

## Recommended staged path

### Stage 1: Keep `agentboxd` as the owner

Keep the existing DevBox protocol independent from the runtime choice. `agentboxd` should continue to own project paths, positive source manifests, source leases, operation IDs, credential-profile locks, receipt state, return candidates, archives, retention, and reconciliation. This is directly aligned with the current [SPEC data model](SPEC.md#durable-data-model).

### Stage 2: Add a Microsandbox adapter behind a capability gate

Use one root-owned local Microsandbox home per VPS. Start one detached sandbox per DevBox run with a receipt-derived name and a pinned image. Copy the verified source projection into the sandbox instead of binding the Mac tree. Configure:

- vCPU, memory, root-disk, volume, and guest rlimit limits;
- a deny-by-default egress policy with only the required model hostnames and ports;
- DNS rebind protection and no public inbound ports;
- one selected secret with exact allowed hosts and TLS-required substitution;
- an environment reference rather than a raw SDK secret where possible;
- a detached lifecycle and a root-owned archive path outside the Microsandbox home.

The capability gate must fail closed if `/dev/kvm`, glibc, the image cache, network policy, secret substitution, detached reconnect, stop escalation, or archive copy fails. The first provider proof should cover one Claude Code, Codex, or Pi command. OMP and Paseo need separate DevBox smoke tests.

### Stage 3: Keep systemd-per-run as the fallback

On a host without KVM or supported Microsandbox binaries, use the current systemd design. Keep the same source and archive protocol. Add a per-run egress policy before enabling real provider credentials. This fallback has weaker isolation because it shares the host kernel, but it already models the required run ownership and stop/archive sequence.

### Stage 4: Evaluate Incus only for a specific operational need

Choose Incus when DevBox needs general instance inventory, storage-driver snapshots, an established remote REST control plane, or a guest VM that must be managed outside Microsandbox. Prefer an Incus VM over a system container for hostile code. Before adoption, prove all of the following:

- KVM, QEMU, guest agent, and image requirements;
- a host cgroup for every QEMU process and helper;
- a DNS-aware egress allowlist or maintained IP policy;
- a credential design that meets the accepted guest-visibility threat model;
- complete custom-volume archive and deletion behavior;
- receipt-bound access that never grants the Incus socket to ordinary DevBox keys.

## Source index

### Incus official sources

- [Repository and Apache-2.0 metadata](https://github.com/lxc/incus)
- [Latest release API](https://api.github.com/repos/lxc/incus/releases/latest)
- [Incus 7.4 release](https://github.com/lxc/incus/releases/tag/v7.4.0)
- [Host requirements](https://linuxcontainers.org/incus/docs/main/requirements/)
- [Install and access](https://linuxcontainers.org/incus/docs/main/installing/)
- [Containers versus VMs](https://linuxcontainers.org/incus/docs/main/explanation/containers_and_vms/)
- [Security model](https://linuxcontainers.org/incus/docs/main/explanation/security/)
- [Resource limits](https://linuxcontainers.org/incus/docs/main/reference/instance_options/)
- [Network ACLs](https://linuxcontainers.org/incus/docs/main/howto/network_acls/)
- [Managed bridge networking](https://linuxcontainers.org/incus/docs/main/explanation/networks/)
- [Create container or VM examples](https://linuxcontainers.org/incus/docs/main/howto/instances_create/)
- [Exec](https://linuxcontainers.org/incus/docs/main/reference/manpages/incus/exec/)
- [Stop](https://linuxcontainers.org/incus/docs/main/reference/manpages/incus/stop/)
- [Snapshots and export](https://linuxcontainers.org/incus/docs/main/howto/instances_backup/)
- [Move and copy](https://linuxcontainers.org/incus/docs/main/howto/move_instances/)
- [Ephemeral property](https://linuxcontainers.org/incus/docs/main/reference/instance_properties/)
- [LXC cgroup source](https://github.com/lxc/incus/blob/main/internal/server/instance/drivers/driver_lxc.go)
- [QEMU cgroup source](https://github.com/lxc/incus/blob/main/internal/server/instance/drivers/driver_qemu.go)
- [REST API specification](https://github.com/lxc/incus/blob/main/doc/rest-api.yaml)

### Microsandbox official sources

- [Repository and README](https://github.com/superradcompany/microsandbox)
- [Latest release API](https://api.github.com/repos/superradcompany/microsandbox/releases/latest)
- [Microsandbox v0.6.16 release](https://github.com/superradcompany/microsandbox/releases/tag/v0.6.16)
- [Documentation index](https://docs.microsandbox.dev/llms.txt)
- [Introduction](https://docs.microsandbox.dev/getting-started/introduction.md)
- [Linux prerequisites](https://docs.microsandbox.dev/troubleshooting/linux.md)
- [Security overview](https://docs.microsandbox.dev/security/overview.md)
- [Isolation boundary](https://docs.microsandbox.dev/security/isolation.md)
- [Filesystem and image supply chain](https://docs.microsandbox.dev/security/filesystem.md)
- [Network overview](https://docs.microsandbox.dev/networking/overview.md)
- [Network defenses](https://docs.microsandbox.dev/security/network.md)
- [Secret handling](https://docs.microsandbox.dev/security/secrets.md)
- [Sandbox overview](https://docs.microsandbox.dev/sandboxes/overview.md)
- [Lifecycle](https://docs.microsandbox.dev/sandboxes/lifecycle.md)
- [Volumes](https://docs.microsandbox.dev/sandboxes/volumes.md)
- [Snapshots](https://docs.microsandbox.dev/sandboxes/snapshots.md)
- [Global configuration](https://docs.microsandbox.dev/configuration.md)
- [Go SDK sandbox API](https://docs.microsandbox.dev/sdk/go/sandbox.md)
- [Go SDK options source](https://github.com/superradcompany/microsandbox/blob/main/sdk/go/options.go)
- [Go SDK lifecycle source](https://github.com/superradcompany/microsandbox/blob/main/sdk/go/sandbox.go)
- [Go SDK snapshot source](https://github.com/superradcompany/microsandbox/blob/main/sdk/go/snapshot.go)
- [Guest rlimit source](https://github.com/superradcompany/microsandbox/blob/main/crates/agentd/lib/rlimit.rs)
- [Claude Code example](https://docs.microsandbox.dev/examples/agents/claude-code.md)
- [Codex CLI example](https://docs.microsandbox.dev/examples/agents/codex.md)
- [Pi example](https://docs.microsandbox.dev/examples/agents/pi.md)

