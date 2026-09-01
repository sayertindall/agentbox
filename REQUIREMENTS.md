# DevBox requirements

**Status:** DRAFT FOR IMPLEMENTATION REVIEW  
**Created:** 2026-08-29  
**Authoritative location:** `/Users/sayertindall/Dev/DevBox/REQUIREMENTS.md`  
**Requirement owner:** Unassigned. Assign one person before implementation starts.

## Purpose and success definition

DevBox moves a dirty local project and its task context to a VPS. It starts a new coding-agent run there. After the VPS acknowledges the run, loss of Mac Internet access must not stop that run.

The project succeeds when a developer can prepare a dirty project, transfer a sanitized source snapshot, start Claude Code, Codex, or OMP on the VPS, disconnect the Mac, reconnect later, inspect the result, and reclaim the changes without a Git push.

DevBox can coordinate source writes. It cannot stop a developer from manually editing the local project while a remote run exists. It detects that local divergence at reclaim and refuses automatic overwrite.

## Scope

DevBox includes:

- a local client and a root-owned VPS controller;
- separate forced SSH control and restricted rsync transfer identities;
- source manifests, sanitized baseline trees, task packets, enrollment identifiers, and staging quotas;
- server-side receipts, operations, credential-profile locks, source leases, and reconciliation;
- one disposable Unix user and one systemd sandbox per remote run;
- one private Paseo Unix socket per run;
- host-enforced run storage quotas and systemd resource limits for every run sandbox;
- return candidates, rollback journals, explicit conflict resolution, control backups, retention, and host checks.

## Non-goals

DevBox does not include:

- migration of a live Claude Code, Codex, or OMP process between hosts;
- direct SSH shells, PTYs, port forwarding, or raw Paseo access for ordinary DevBox keys;
- public DevBox, Paseo, provider, or control endpoints;
- copying local OAuth stores, SSH keys, `.env` files, session stores, or provider secrets to the VPS;
- Mutagen, Syncthing, Unison, Git LFS, Git submodules, nested repositories, or case-colliding source trees in version 1;
- Git hosting, pushes, pull requests, deployment credentials, cloud-console access, or multi-user tenancy.

## Actors, terms, and ownership

| Term | Meaning | Owner of durable truth |
|---|---|---|
| Operator | The human who owns the Mac and VPS. | Approves enrollment, credentials, retention, and break-glass use. |
| Local client | `agentbox` on the Mac. | Local packet copy, return candidate, and rollback journal. |
| Control key | An SSH key that can run only `devbox-gateway`. | It carries one receipt-bound control request. |
| Transfer key | An SSH key that can run only the staging rsync wrapper. | It writes a bounded untrusted staging tree. |
| `agentboxd` | Root-owned VPS controller on a Unix socket. | Project registry, source state, operations, credential-profile locks, run creation, and receipts. |
| Credential profile | Root-owned provider authentication material and policy. | One provider secret and its writer-capacity lock. |
| Run sandbox | One temporary Unix user, systemd cgroup, workspace, `PASEO_HOME`, raw history, and Paseo socket for one operation. | The selected run while active. |
| Workspace generation | The immutable source activation from one handoff. | `agentboxd` after promotion. |
| Source lease | The protocol right to make a local or remote source result authoritative. | `agentboxd` database. |
| Operation | A request with a durable unique identifier and digest. | `agentboxd` database. |
| Receipt | The user-readable view of handoff, source generation, run, and return state. | `agentboxd` database. |
| Source projection | Only the files named by the source manifest. | Source manifest. |
| Return projection | All allowed source paths present after the stopped remote run. | Return manifest. |
| Runtime metadata | Packet, synthetic Git database, raw history, and service state outside the source projection. | `agentboxd` and root. |

`project_id` matches `[a-z0-9][a-z0-9-]{0,63}`. `enrollment_id` is a random UUID generated during `agentbox init`. The server stores only its hash. A caller cannot supply a workspace path, run user, credential profile path, socket path, or daemon port.

## Scenario traceability

| Scenario | Source | Requirements |
|---|---|---|
| Developer hands off uncommitted local code. | User request | FR-001 through FR-005, INV-001 through INV-005 |
| Developer closes the laptop after a remote run starts. | User clarification | FR-006, FR-007, NFR-REL-001 |
| Developer reconnects and returns remote changes. | User request | FR-008, FR-009, INV-001 |
| Control or network fails during transfer or start. | Derived safety case | FR-005, FR-007, FR-010, NFR-REL-002 |
| Runtime crashes, leaks a child process, or becomes unobservable. | Paseo lifecycle evidence in [RESEARCH.md](RESEARCH.md) | FR-010, NFR-REL-001 |
| One project attempts to read another project's source or secret. | Derived security case | FR-011, NFR-SEC-002 |
| Developer edits local source while remote owns the source lease. | Derived concurrency case | FR-009, INV-001 |

## Functional requirements

### FR-001: Initialize and enroll a project

`agentbox init` shall create a local configuration with a generated enrollment identifier. `agentbox enroll` shall register the project through `agentboxd`.

**Acceptance criteria**

- `init` validates project ID grammar and writes `.agentbox/project.toml` plus an enrollment identifier.
- `enroll` sends the enrollment identifier hash through the control gateway.
- The server accepts an existing project ID only when its stored enrollment hash matches.
- The server derives every project path from its own registry row.
- The server rejects caller-supplied workspace, staging, profile, socket, and daemon paths.

### FR-002: Prepare one immutable handoff

`agentbox prepare` shall create one handoff identifier and one prepare operation identifier before it starts a transfer.

**Acceptance criteria**

- The command writes the packet, source manifest, baseline manifest, and local prepare record before it opens the transfer connection.
- The packet binds both manifest digests, the enrollment project ID, task, current state, next action, constraints, and base revision label.
- Repeating the same operation identifier returns its saved state only when the request digest matches.
- Reusing the operation identifier with a different packet or manifest digest fails.
- `handoff` and `reclaim` write a fsynced local workflow record before every external request and after every known response.
- The workflow record contains each child operation identifier, expected receipt revision, local stage, and whether the outcome is known or unknown.
- A restarted local client reconciles an unknown workflow with `agentboxd` before it retries any child operation.

### FR-003: Build sanitized source and baseline trees

The local client shall build a positive current source manifest and a positive baseline manifest. It shall never transfer raw Git storage.

**Acceptance criteria**

- The current source manifest includes permitted dirty and untracked regular files.
- Mandatory exclusion paths are absent from both current and baseline manifests even when Git tracks them.
- For Git projects, the baseline materializes allowed `HEAD` paths only.
- A current path absent from baseline is an addition. A baseline path absent from current source is a deletion.
- The client rejects Git LFS, submodules, nested repositories, unsafe symlinks, special files, and case collisions.
- The client transfers no `.git` directory, Git object, ref, or raw bundle.

### FR-004: Transfer bounded untrusted staging data

The transfer key shall copy current and baseline trees only below a server-issued staging token. Staging data is untrusted until `agentboxd` validates it.

**Acceptance criteria**

- The server issues one one-time staging token with a byte quota derived from manifests and host configuration.
- The host assigns an enforced filesystem project quota to the token root before rsync starts.
- The local rsync command uses `--files-from` generated from the positive manifest.
- The transfer wrapper permits rsync server mode only below the token's staging child. It permits no shell, forwarding, or path above staging.
- Extra, missing, or unsafe staging paths cannot activate a source generation.
- An over-quota or expired staging token fails and becomes eligible for cleanup.

### FR-005: Activate an exact source projection

`agentboxd` shall validate staging, build an exact source projection, and promote one workspace generation only after validation.

**Acceptance criteria**

- The server rewalks both staging trees and compares each to its manifest.
- The server rejects undeclared, missing, mismatched, or unsafe paths.
- The source workspace contains only current source-manifest paths.
- Synthetic Git metadata lives outside the source workspace. It uses a separate `GIT_DIR` and `GIT_WORK_TREE` environment.
- The handoff packet lives outside the source workspace and is read-only to the run sandbox.
- A failed activation leaves no active workspace generation or remote source lease.

### FR-006: Start a receipt-bound sandboxed run

`agentboxd` shall start a provider in a fresh run sandbox. It shall persist a start intent before it calls Paseo.

**Acceptance criteria**

- The control gateway accepts receipt ID, provider name, and operation ID. It accepts no workspace path or credential-profile identifier.
- `agentboxd` resolves the generation, selected credential profile, run user, packet, environment wrapper, and Paseo socket from server state.
- Before it starts the run service, the server assigns a host-enforced storage quota to the entire run tree and applies host-defined memory, CPU, and task limits to the run cgroup.
- The server creates a fresh run user and systemd cgroup for the run.
- The sandbox receives only the selected credential profile material, source projection, runtime metadata, and required toolchain paths.
- The server records `remote_starting` before it starts the run service.
- The started agent has labels for the DevBox operation and receipt.
- A known response records the agent ID and `remote_running` before the server replies.
- An ambiguous response creates `unknown_remote_run`, which blocks source mutation until reconciliation finishes.

### FR-007: Continue after local disconnect

After `agentboxd` records `remote_running`, the provider runtime shall not depend on an open local terminal, SSH connection, local client, or local file-sync process.

**Acceptance criteria**

- A test closes the control connection after the receipt records `remote_running`.
- The run cgroup, source projection, run `PASEO_HOME`, credentials, and packet remain on the VPS.
- A new control-gateway request reads the same receipt after reconnect.
- Local source edits after disconnect do not grant a second source lease. DevBox detects them during reclaim.

### FR-008: Observe a receipt-bound run

The operator shall inspect structured state, remote diff, live output, and historical raw output through receipt-bound server operations.

**Acceptance criteria**

- `status` reads the server receipt and reconciliation result.
- `diff` runs against the run's synthetic Git state. It works for Git and non-Git projects.
- `attach` and `logs` require a receipt-bound known agent and do not create a replacement agent.
- DevBox stores no raw provider output in the control database or generic daemon log.
- Raw output stays in root-controlled run archives and streams only through a receipt-bound operation or logged break-glass path.

### FR-009: Return, reclaim, resolve, and recover

DevBox shall verify a complete remote candidate before it changes the local project. It shall use an explicit conflict and recovery protocol.

**Acceptance criteria**

- `agentboxd` stops the entire run cgroup before it freezes a return candidate.
- The server refuses return when it cannot stop or reconcile the run cgroup.
- After it stops the run cgroup, the server builds a return manifest from every allowed source path that exists in the remote workspace. The manifest may include agent-created allowed files.
- The server copies only return-manifest paths into a root-owned return candidate and verifies its manifest.
- The local client verifies the downloaded candidate before it changes any local path.
- If the local current source manifest differs from the original handoff manifest, DevBox records `conflicted` and changes neither source tree.
- If the manifests match, DevBox applies the candidate through a durable rollback journal that preserves excluded local paths.
- `resolve` requires confirmation, verifies no run cgroup remains, records the chosen local manifest, and returns the source lease to local ownership.
- `recover` restores the original local tree or finishes a previously verified candidate before a new handoff can begin.

### FR-010: Reconcile uncertainty and terminate sandboxes

`agentboxd` shall reconcile start, runtime, daemon, and service uncertainty before it permits another writer-affecting operation.

**Acceptance criteria**

- The receipt records credential profile, run ID, systemd unit, agent ID when known, last observed runtime state, and terminal reason when known.
- A monitor queries only DevBox-labeled agents in the run sandbox.
- A service restart, unreachable socket, or ambiguous agent result enters `unknown_remote_run` until a server-side check resolves it.
- If run cgroup archive cleanup fails, the receipt enters `archived_cleanup_pending` and the credential profile remains locked until repair completes.
- Return, reclaim, close, resolve, a second activation, and a second run reject `unknown_remote_run`.
- After terminal reconciliation, `agentboxd` stops the run cgroup, archives its runtime metadata under root ownership, and removes the run user's access before a new run may use that credential profile.
- `resume` creates a fresh run sandbox from the saved source generation and packet. It does not claim provider-native session continuation.

### FR-011: Isolate every run sandbox

A run sandbox shall not read another run's workspace, credential, raw output, `PASEO_HOME`, control state, or Paseo socket.

**Acceptance criteria**

- Every run uses a distinct Unix user, `PASEO_HOME`, source workspace, raw-output directory, and mode-0600 Paseo Unix socket.
- A run user receives one selected credential profile only through a root-controlled launch path.
- A run user cannot traverse another run directory, control database, credential profile store, or archive.
- A run user has no sudo, Docker socket, administrator SSH key, deployment credential, or public listener.
- An integration test proves cross-run filesystem, credential, raw-output, control-state, and Unix-socket denial.

### FR-012: Back up and retain control data

`agentboxd` shall back up its control database and clean expired artifacts only under a declared host policy.

**Acceptance criteria**

- Host configuration requires receipt, raw-output, workspace, staging, and database-backup retention values before a real run starts.
- A root-owned backup job creates a verified SQLite backup outside run sandboxes.
- A restore drill loads the newest verified backup into an isolated control database and reconciles recorded active runs against live systemd units before it can replace production control state.
- A root-controlled restore operation stops new writer requests, restores a verified backup into a temporary control database, reconciles every active run against live systemd units and run sockets, and atomically replaces production control state only after reconciliation succeeds.
- A root-owned cleanup job deletes only eligible closed artifacts.
- Cleanup never deletes active, starting, unknown, returning, conflicted, archived-for-reclaim, or journal-recovery data.
- Every backup, restore drill, and deletion adds a structured control event.

## Quality requirements

### NFR-REL-001: Remote durability

The VPS shall own the source generation, run sandbox, provider process, packet, runtime metadata, credential injection, and control state before it acknowledges a remote run.

**Acceptance criteria**

- A real-VPS fake-provider test proves that a run continues after the Mac disconnects.
- Separate provider smoke tests prove each selected provider can authenticate, start, use the receipt workspace, and report a terminal state.
- Systemd restarts `agentboxd`. DevBox reconciles every active run before it releases a source lease.

### NFR-REL-002: Fail closed on ambiguity

DevBox shall not activate, start, return, reclaim, close, resolve, or reuse a credential profile when a manifest, source lease, operation, run service, or provider state is ambiguous.

**Acceptance criteria**

- Every writer-affecting request has a durable idempotency key and expected receipt revision.
- A persistence failure, external-call timeout, cgroup-stop failure, or reconciliation gap creates a blocking state.
- The server emits a control event for every transition, recovery decision, archive, backup, and break-glass action.
- Only a logged break-glass operation can abandon an unknown run.

### NFR-SEC-001: Restrict network and command access

The VPS shall expose no public DevBox, Paseo, provider, or control endpoint.

**Acceptance criteria**

- Tailscale policy permits the selected Mac identity to reach only VPS SSH.
- The host firewall accepts SSH only on the Tailscale interface and rejects public TCP port 22.
- An off-tailnet probe fails to establish SSH.
- Control and transfer keys use forced commands with `no-port-forwarding`, `no-agent-forwarding`, `no-X11-forwarding`, `no-pty`, and `no-user-rc`.
- The selected Paseo daemon and client pass an isolated Unix-socket listen and query test before a real run starts.
- Version 1 has no TCP fallback for Paseo. Each run has only a mode-0600 Unix socket.

### NFR-REL-003: Contain run resource use

Each run sandbox shall have host-enforced storage, memory, CPU, and task limits that cannot consume control-state or sibling-run resources.

**Acceptance criteria**

- Host configuration requires run storage quota, memory limit, CPU quota, and task limit values.
- The server assigns the run storage quota before the run user can write workspace, `PASEO_HOME`, raw history, or socket metadata.
- The systemd unit applies `MemoryMax`, `CPUQuota`, and `TasksMax` from host configuration.
- Provisioning stops when the host cannot enforce run storage quota or cgroup resource limits.
- An integration test proves that a quota-exceeding run fails without changing control state or a sibling run.

### NFR-SEC-002: Enforce least privilege and run isolation

The control, transfer, credential-profile, and run roles shall have separate permissions. An unknown role shall have no authority.

**Acceptance criteria**

- `agentboxd` is the only root-owned component that creates a run user, injects a selected credential, starts or stops a run cgroup, promotes a workspace, and reads control state.
- The gateway cannot execute arbitrary commands or choose a path, profile, or socket.
- The transfer user cannot call control operations or read a generation, candidate, archive, or credential store.
- A run user can read only its source projection, packet, toolchain, selected secret material, and its own socket.
- An integration test proves cross-run denial for every runtime path and endpoint.

### NFR-SEC-003: Keep secrets and excluded paths out of snapshots

DevBox shall treat source and return manifests as positive allowlists. It shall not activate, return, or apply a path that the relevant manifest does not declare. Staging may contain bounded untrusted extra data until validation rejects it.

**Acceptance criteria**

- Mandatory exclusions include `.env`, `.env.*`, `.ssh`, `.aws`, `.config`, `.claude`, `.codex`, `.omp`, `.agentbox`, `.git`, dependency directories, and configured build output.
- A tracked canary below an excluded path appears in neither baseline nor source tree.
- The host-enforced staging quota bounds untrusted extra upload. The activation validator rejects every extra path.
- The source manifest governs activation. The return manifest governs allowed agent-created and modified source paths after a run stops.
- Runtime metadata remains outside source and return projections.
- Return does not change an excluded local path.

### NFR-SEC-004: Restrict per-run outbound network access

Each run sandbox shall start with outbound network access denied. The selected runtime adapter shall allow only server-recorded model-provider, DNS, and package-registry destinations required by that run.

**Acceptance criteria**

- A project or credential profile declares the exact provider hosts, ports, DNS resolver, and optional package registries required for a run.
- The runtime adapter denies all egress by default.
- The adapter denies loopback, private RFC1918 ranges, link-local ranges, cloud metadata addresses, Tailscale ranges, host services, and run-to-run traffic.
- A run cannot add or broaden its own egress rules.
- A Microsandbox adapter uses its documented domain, DNS pinning, TLS, and secret-host policy.
- A systemd adapter remains unavailable until a root-owned network namespace, proxy, or equivalent host policy proves the same deny-by-default behavior.
- An integration test proves that a run can reach an allowed model endpoint and cannot reach a disallowed public endpoint, host service, private address, metadata address, Tailscale address, or sibling run.

### NFR-MAINT-001: Reproduce project environments

Each project shall declare a pinned Nix flake and devenv configuration, or an approved explicit opt-out reason.

**Acceptance criteria**

- Enrollment verifies the flake lock and devenv configuration, or records the opt-out reason.
- The server records lock digest, Linux architecture, wrapper version, and synthetic Git mode in every receipt.
- Every run starts through the server-resolved environment wrapper.
- A clean run sandbox builds the declared project environment without copying a local dependency directory.

### NFR-OBS-001: Preserve bounded operational evidence

DevBox shall store structured control evidence separately from sensitive raw provider output.

**Acceptance criteria**

- The control database stores time, actor, project, receipt, operation, run ID, source-lease revision, and result.
- The control database never stores source content, provider authorization values, or raw provider output.
- Raw output is root-controlled run metadata with separate retention. Ordinary status never returns it.
- `logs` streams a receipt-bound archive or active run. It never appends streamed bytes to the control database.
- Host policy declares all retention values before the first real run.

### NFR-PORT-001: Support the stated host pair

DevBox shall support a macOS arm64 local client and a selected Linux VPS controller and run sandbox.

**Acceptance criteria**

- The client builds and runs on macOS arm64.
- `agentboxd` builds and runs on the selected Linux architecture.
- The host audit records Linux release, architecture, systemd support, Unix-socket capability, firewall manager, and provider binary support.
- The test matrix runs source-manifest, synthetic Git, return, and journal fixtures on macOS arm64 and the selected Linux architecture.
- The test matrix covers line endings, executable bits, case collisions, symlinks, source deletions, non-Git diff, path traversal, and socket denial.

## Constraints

| ID | Constraint | Source |
|---|---|---|
| CON-001 | A handoff must work without a Git push or hosted Git service. | User request |
| CON-002 | The VPS, not the Mac, owns active remote execution. | User clarification |
| CON-003 | The design uses Paseo for provider lifecycle. | User tool choice |
| CON-004 | The VPS installs and validates Claude Code, Codex, and OMP. | User request |
| CON-005 | Normal DevBox control does not use raw Paseo port forwarding. | Security design decision |
| CON-006 | Normal source transfer uses restricted rsync staging. | Security design decision |
| CON-007 | Tailscale and OpenSSH provide private operator access. | Accepted design decision |
| CON-008 | The selected Paseo daemon and client must pass the Unix-socket capability test. | Security design decision |
| CON-009 | One credential profile may have only one active run in version 1. | Credential-refresh safety decision |
| CON-010 | Every run sandbox must have enforced storage and cgroup resource limits. | Security review decision |
| CON-011 | Every run sandbox must use a deny-by-default outbound network policy. | Security research decision |

## Invariants

- **INV-001:** DevBox permits one remote source writer operation for one project at one time.
- **INV-002:** `agentboxd` is the authority for project registration, server path resolution, source leases, operations, credential-profile locks, run services, and receipts.
- **INV-003:** The source manifest is the complete activation projection. The return manifest is the complete return projection. Runtime metadata is separate and never enters either projection.
- **INV-004:** A run starts only after a validated generation, a selected credential profile lock, and a durable start intent exist.
- **INV-010:** A run sandbox cannot consume storage, memory, CPU, or task capacity beyond its host-defined resource limits.
- **INV-005:** A local disconnect never grants a second source lease.
- **INV-006:** A normal control request cannot directly invoke Paseo, choose a run user, or choose a credential profile.
- **INV-007:** A run sandbox cannot read another run's source, secret, raw output, control state, or socket.
- **INV-008:** DevBox does not build a return candidate until it stops the run cgroup.
- **INV-009:** A local apply begins only after candidate verification and journal fsync.
- **INV-011:** A run sandbox can send network traffic only through its server-recorded egress policy.

## Failure and recovery behavior

| Condition | Required behavior |
|---|---|
| Transfer disconnects before acknowledgement | The local client records an unknown operation. The server reconciles by operation ID. No automatic second transfer starts. |
| Control disconnects during run start | The server retains `remote_starting` or `unknown_remote_run`, reconciles the run socket label and systemd unit, and blocks source mutation until it knows the outcome. |
| Mac disconnects after `remote_running` | The VPS continues the run. A reconnect starts read-only. |
| Staging differs from either manifest | The server rejects activation. Staging remains untrusted and is later cleaned by policy. |
| Run service cannot stop | Return remains blocked. The receipt becomes unknown or requires logged break-glass. |
| Run archive cleanup fails | The receipt becomes `archived_cleanup_pending`. The credential profile remains locked until root completes cleanup and reconciliation. |
| Run exceeds a resource limit | The run cgroup receives the host-defined limit behavior. `agentboxd` records the result, preserves control state, and blocks return until it reconciles the run. |
| Provider runtime or run socket cannot reconcile | The receipt becomes `unknown_remote_run`. Only reconciliation or break-glass can leave it. |
| `agentboxd` restarts | The service reconciles active run units and labels before it releases any source or credential-profile lock. |
| VPS reboots | DevBox records known terminal state or unknown state. It does not claim provider-session continuation. |
| Local source changes during a remote source lease | Reclaim records `conflicted`. Both source trees remain unchanged. |
| Local apply stops midway | `agentbox recover` restores original paths or completes the verified candidate from the fsynced journal. |
| Provider credential expires | The run fails or becomes unknown. The operator renews only the selected credential profile on the VPS. |
| Run attempts denied egress | The runtime blocks the request, records a structured egress-policy event, and keeps the credential profile unavailable for reuse until run reconciliation completes. |

## Assumptions, dependencies, risks, and open decisions

### Assumptions

- The VPS has persistent storage, systemd, and outbound Internet access to model providers.
- The operator can create restricted SSH identities, Tailscale device policy, root-owned services, and per-run Unix users.
- The source project can build without excluded local secret files.
- The host must enforce per-run project quota and cgroup resource limits.

### Dependencies

- Paseo, Claude Code, Codex, and OMP support the selected Linux architecture.
- The selected Paseo daemon and client support Unix-socket listen and query.
- Tailscale, OpenSSH, rsync, Nix, devenv, mise, SQLite, and systemd are available or provisionable.
- The host firewall has a controllable rule set for Tailscale-only SSH.
- The selected runtime adapter must enforce the recorded per-run outbound network policy.

### Risks

- Paseo documents provider-runtime loss after crash, OOM kill, or host suspend. DevBox blocks on uncertainty but cannot promise native-session survival.
- Provider login forms differ. Credential injection into a disposable run sandbox needs a provider-specific verification test.
- A compromised local transfer key can fill only its bounded staging quota. It cannot activate source or access credentials.
- A project may need unsupported Git features or excluded secret files. Preflight must reject it rather than invent behavior.
- Per-run user creation adds host operations complexity. It is necessary to keep a later run from reading an earlier run's profile state.
- An allowed provider endpoint can still receive its selected credential. The allowlist must stay minimal and be reviewed when provider endpoints change.

### Open decisions

| ID | Decision needed | Why it blocks implementation |
|---|---|---|
| OD-001 | VPS hostname, Linux distribution, architecture, disk, memory, CPU, systemd, and firewall manager | Bootstrap and service templates must match the host. |
| OD-002 | Expected credential-profile count and run concurrency | It determines provider account setup, user lifecycle, and capacity. |
| OD-003 | Project inventory and environment declarations | Each project needs source-policy, Git feature, and Nix or devenv review. |
| OD-004 | Credential injection method for Claude Code, Codex, and OMP | The run sandbox needs a vendor-approved path without Mac session copying. |
| OD-005 | Receipt, raw-output, archive, staging, and database-backup retention values | The host policy must exist before a real run. |
| OD-006 | Per-provider egress destination set and systemd fallback enforcement adapter | A run cannot receive a credential until its deny-by-default policy is verified. |

## Next-stage recommendation

**Selected lane: formal implementation plan.** The protocol is specified. The remaining uncertainty is host provisioning, provider credential injection, and per-run egress enforcement. [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md) begins with a read-only host audit and stops for review before it changes a VPS.

## Handoff manifest

Implementation must cover FR-001 through FR-012; NFR-REL-001, NFR-REL-002, NFR-REL-003, NFR-SEC-001 through NFR-SEC-004, NFR-MAINT-001, NFR-OBS-001, and NFR-PORT-001; CON-001 through CON-011; and INV-001 through INV-011. State-changing VPS work cannot begin until OD-001, OD-004, OD-005, and OD-006 are known.