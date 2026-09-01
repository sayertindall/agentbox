# Research status

This file preserves the primary-source research collected on 2026-08-29. It is evidence, not the current design authority. [SPEC.md](SPEC.md) and [REQUIREMENTS.md](REQUIREMENTS.md) supersede it where they differ. In particular, the final design uses restricted rsync staging in version 1 instead of Mutagen after the security review.

## Primary-source evidence report: local-to-VPS agent workspaces without Git pushes

**Research date:** 2026-08-29  
**Scope:** first-party documentation, source repositories, release metadata, and current support for Tailscale/WireGuard, rsync/Mutagen/Syncthing/Unison, Dev Containers/Nix/devenv/Devbox, tmux/mosh, and Claude Code/Codex/OMP/Paseo.

## Recommendation in one paragraph

Use **Tailscale → verified file handoff → VPS-local environment and files → Paseo daemon → tmux for direct long jobs → VPS-only secrets**. For a Mac↔VPS pair, Mutagen v0.18.1 is the best optional online transfer/return channel; rsync v3.5.0 is the simplest one-way seed. After handoff, pause laptop-owned sync: the VPS copy is canonical, so an indefinitely offline laptop cannot stop the remote agent. Use Syncthing v2.1.3 for several always-on devices and Unison v2.54.0 when ownership/permission fidelity matters. Use pinned Nix/devenv v2.2.2 or a pinned Dev Container. Current Devbox v0.18.0 removed Jetify Cloud cache, secrets, and auth; do not build on stale Cloud Secrets pages.

## Durability and remote/local state separation

The laptop and VPS must not be peers in the **execution** lifecycle. They can be peers in an optional data-plane sync while the laptop is online, but only the VPS owns the long-running agent turn.

Paseo's first-party [agent-lifecycle document](https://raw.githubusercontent.com/getpaseo/paseo/main/docs/agent-lifecycle.md) says state transitions persist to disk and stream to subscribed clients over WebSocket. An unarchived agent can close without deletion or archive while retaining identity, timeline, workspace, labels, usage, and parent relationship; opening or prompting resumes it under the same agent ID. Exact evidence: “Idle agents remain resident indefinitely.” Runtime closure is caused by explicit lifecycle action, reload/replacement, workspace teardown, or daemon shutdown—not a client tab disconnect. The same document warns that a provider runtime can die from crash, OOM kill, or host suspend; background shells/workflows in that process die too, and non-Claude agents can appear idle until a later turn. Run VPS process/host monitoring in addition to client reconnect.

Mutagen's [getting-started guide](https://mutagen.io/documentation/introduction/getting-started/) says its daemon is a per-user background process “hosting and managing synchronization sessions.” If the daemon is on the laptop, it cannot watch/reconcile the local endpoint while the laptop is off or disconnected. That is acceptable only after the source is materialized on the VPS. A live mount or bidirectional watcher is unsuitable as canonical execution because it couples agent correctness to laptop power, network reachability, watcher state, and cross-OS filesystem semantics.

**Durable handoff:** freeze local writes → Mutagen flush or rsync seed → verify manifest/tool-native state → activate pinned VPS environment → start Paseo/agent on VPS path → pause laptop-owned sync → reconnect later and explicitly inspect/pull/merge. Do not make agent correctness depend on the local sync process.

## Recommendation matrix: data plane

| Tool | Bidirectional/conflict model | Ownership/permission semantics | Fit and risks |
|---|---|---|---|
| **Mutagen v0.18.1** | Yes. `two-way-safe` is default; `two-way-resolved` alpha wins; `one-way-safe` and `one-way-replica` exist. Three-way merge tracks last agreed state and records unresolved conflicts. | Does not propagate raw ownership or permission bits. POSIX executability only. New files default to endpoint owner/group and `0600` files/`0700` directories; explicit owner/group/mode options exist. | Best active Mac↔VPS source transfer. Use online while laptop is available, then pause after handoff for indefinite-offline durability. Not for exact UID/GID/mode mirroring. |
| **Syncthing v2.1.3** | Yes. `sendreceive` replicates creation/modification/deletion. Versioning defaults to no old copies; enabled versioning archives remote replacements/deletions, not local edits. | `ignorePerms=true` disables permission bits. `syncOwnership` is opt-in and only POSIX↔POSIX or Windows↔Windows. Names precede numeric UID/GID; ownership changes require root or CHOWN/FOWNER capability. Xattrs are separate settings. | Best multi-device always-on peer. Discovery/relay can leak device IDs/metadata; protect config/TLS keys. Do not make a laptop watcher the only live path for a durable remote agent. |
| **Unison v2.54.0** | Yes. Two replicas can change independently; non-conflicting updates propagate and conflicts are detected/displayed. | Meaningful permission bits propagate across OSes; Unix `setuid`/`setgid` are excluded. Owner/group IDs can propagate by names or numeric IDs. ACL sync needs compatible support and process authority. | Best metadata-fidelity option for compatible POSIX endpoints. UID/GID mismatch, cross-platform properties, profiles, and version interoperability need explicit operation. |
| **rsync v3.5.0** | **No.** Local/remote copy with push/pull forms; no persistent bidirectional or three-way conflict system. | `-a` means `-rlptgoD`, not ACLs/xattrs/hard links. `-o` owner preservation is super-user-only; `-g` needs group authority; `-A`/`-X` are explicit. `--fake-super` stores privileged metadata in xattrs. | Simplest one-way durable seed/promotion/backup. SSH/remote-shell is safe default; direct daemon is not encrypted. `--delete` removes destination-only files; review dry run/manifest first. |

**Acceptance answer:** Mutagen, Syncthing, and Unison are bidirectional. Rsync is not. Unison and rsync can preserve ownership/permissions with documented flags and authority/compatibility constraints. Syncthing can synchronize ownership only when explicitly enabled with matching OS/privileges. Mutagen does not preserve raw modes/owners by design. Tailscale/WireGuard do not synchronize files or define filesystem permissions.

## Recommendation matrix: environment reproducibility

| Option | Evidence and recommendation | Caveat |
|---|---|---|
| **Dev Containers specification** | [`devcontainer.json`](https://raw.githubusercontent.com/devcontainers/spec/main/README.md) is JSON-with-comments metadata for containerized coding. The reference CLI supports a single development container and Docker Compose. Use a pinned OCI image, feature versions/digests, and runtime on VPS. | A spec/container is not whole-host identity. Mac and Linux differ outside the container. |
| **Nix flakes, manual 2.30.6** | [Flake manual](https://nix.dev/manual/nix/2.30/command-ref/new-cli/nix3-flake) defines inputs/outputs; [`nix flake lock`](https://nix.dev/manual/nix/2.30/command-ref/new-cli/nix3-flake-lock) records input revisions. Commit/use the lock. | Manual marks `nix flake` experimental and subject to change. Build for VPS system (`aarch64-linux` or `x86_64-linux`), not Mac system. |
| **devenv v2.2.2** | [README](https://raw.githubusercontent.com/cachix/devenv/main/README.md) documents Nix-backed packages/services/processes/tasks/containers/MCP. [SecretSpec](https://devenv.sh/integrations/secretspec/) “separates secret declaration from secret provisioning” and recommends runtime loading only for needed processes. | Pin inputs/channels; keep resolved secrets out of sync tree. |
| **Devbox v0.18.0** | [README](https://raw.githubusercontent.com/jetify-com/devbox/main/README.md) documents Nix-backed isolated shells. [Quickstart](https://www.jetify.com/docs/devbox/quickstart/index.md) says commit `devbox.json` and `devbox.lock` for the same environment. | Current release is breaking: Jetify Cloud cache, `devbox secrets`/Envsec, and `devbox auth` were removed; legacy `env_from: jetify-cloud` is skipped with a warning. Website old Cloud Secrets pages are stale for v0.18.0. |

## Recommendation matrix: control plane / agent execution

| Product | Execution/reconnect semantics | No-Git assessment |
|---|---|---|
| **Paseo v0.6.1** | Daemon runs on laptop/VM/remote server and manages agents; README documents remote `paseo run --host ... --cwd ...`. Persisted agents survive client disconnect; idle agents remain resident indefinitely. | **Primary control plane.** Run daemon on VPS and source at VPS path. It lists Claude Code, Codex, and OMP integrations, but is not a documented file synchronizer. |
| **Claude Code** | Desktop SSH sessions run Claude Code on remote Linux/macOS and install it on first connection. Remote Control keeps execution/filesystem local and is only a remote view/control surface; outbound HTTPS only and transcript is stored on Anthropic servers. Local-to-web is separate and requires a clean tree. | SSH execution is useful; Remote Control is not Mac-to-VPS transfer or remote execution. |
| **Codex CLI/app** | Official ChatGPT Desktop remote projects read/write remote filesystem/shell; remote app-server starts through SSH. Handoff transfers chat/Git state only for a matching repository; cloud connects GitHub/GitLab environments. | SSH execution works. No arbitrary no-Git handoff is documented; use filesystem transfer for uncommitted/untracked source. Do not expose app-server transports publicly. |
| **OMP v18.0.10** | `/collab` host runs agent/all tools; guests render/control host. Frames use client-side AES-256-GCM; full links carry write token; production relay is hosted and not distributed for self-hosting. `read` supports remote `ssh://` paths, not workspace synchronization. | Session UI only, not Mac-to-VPS data plane. Run OMP on VPS under Paseo or directly; join links are bearer secrets. |
| **tmux 3.7c** | Detached process continues in background and can be reattached. | Process-lifetime safety net under direct VPS jobs; no source/environment contract. |
| **mosh 1.4.0** | Roaming/intermittent connectivity/local echo; login still via SSH. Official site says “No privileged code. No daemon”; client/server last only for connection. | Optional terminal transport, not persistence. Combine SSH/Tailscale plus tmux; firewall UDP. |

## Recommendation matrix: networking and secrets

| Plane | Recommendation | Risks/controls |
|---|---|---|
| **Tailscale v1.102.3** | Default. Tailnet connections are denied unless policy allows; Grants are recommended. Tailscale SSH uses tailnet identity and encrypts over WireGuard. Permit only Mac→VPS sync/SSH/Paseo paths; avoid broad root rules. | Hosted control-plane dependency; policy errors can broaden access. Use least-privilege grants; revoke devices/keys promptly. |
| **Raw WireGuard** | Small self-managed VPN. Encrypts/authenticates UDP IP packets; AllowedIPs provide cryptokey routing. | Key distribution and pushed config are explicitly out of scope. Maintain peer inventory, rotation/revocation, routes, firewall, and application authorization separately. |
| **VPS runtime secrets** | SecretSpec (devenv) or independent host/OS secret manager; load at process start and expose only to needed process. Provider credentials stay on VPS. | Never sync `.env`, OAuth/session files, private keys, or agent transcript stores. `.gitignore` is not a network boundary. |
| **Control/session links** | Treat OMP full-control links, Tailscale auth keys, WireGuard private keys, SSH keys, and Paseo/agent credentials as bearer secrets. | OMP link possession grants control; Syncthing config/TLS keys can impersonate a device; Claude Remote Control stores transcripts server-side. |

## Reference architecture

    Mac editor
      │
      ├─ Tailscale policy: Mac → VPS sync / SSH / Paseo only
      ├─ rsync seed (simplest) or Mutagen flush (online two-way option)
      │      └─ verify, then pause laptop-owned sync
      │
      ╳ Laptop/Internet may disappear indefinitely
      │
    VPS: /workspace/source (canonical execution tree)
      ├─ pinned devenv/Nix or Dev Container
      ├─ Paseo daemon → Claude/Codex/OMP agent
      ├─ tmux for direct jobs
      └─ VPS-only secret provider / OS key store

    Reconnect: inspect remote diff → deliberate pull/merge → resume optional sync.

## Risk register

- **Laptop dependency:** a laptop-owned watcher/daemon stops when laptop or link disappears. Materialize the VPS tree before starting; remote agent must use local VPS files.
- **Provider runtime death:** Paseo documents crash/OOM/suspend loss of provider background work. Monitor VPS and turn runtime death into an explicit failure state.
- **Data loss:** rsync `--delete` removes destination-only files; Syncthing has no old-copy versioning by default; simultaneous edits can conflict. Use one writer, Mutagen safe mode, versioning, and reviewed dry runs.
- **Metadata drift:** Mac/Linux IDs, case sensitivity, symlinks, xattrs, and ACLs differ. Choose portability (Mutagen) or fidelity (Unison/rsync with authority); do not grant root merely to force a portable model.
- **Network exposure:** rsync daemon is unencrypted; WireGuard does not define application authorization; Codex says not to expose app-server transports. Bind services to Tailscale/WireGuard and firewall public interfaces.
- **Relay/account retention:** OMP production relay is not self-hostable; Claude Remote Control stores transcripts on Anthropic servers; Syncthing discovery/relay reveals device IDs/metadata. Prefer direct Tailscale paths for code and classify prompts/transcripts as sensitive.
- **Stale Devbox docs:** current 0.18.0 removed Cloud features. Pin CLI and use an independent current secret provider.

## Release snapshot and primary citations

| Component | Current source snapshot | Primary source |
|---|---|---|
| Tailscale | v1.102.3, published 2026-08-20 | [release API](https://api.github.com/repos/tailscale/tailscale/releases/latest) |
| WireGuard | No single latest version stated by official pages | [overview](https://www.wireguard.com/) · [quick start](https://www.wireguard.com/quickstart/) |
| rsync | v3.5.0, published 2026-08-13; release calls it a major security release | [release API](https://api.github.com/repos/RsyncProject/rsync/releases/latest) · [manual](https://download.samba.org/pub/rsync/rsync.1) |
| Mutagen | v0.18.1, published 2025-02-24 | [release API](https://api.github.com/repos/mutagen-io/mutagen/releases/latest) · [sync](https://mutagen.io/documentation/synchronization/) · [permissions](https://mutagen.io/documentation/synchronization/permissions/) |
| Syncthing | v2.1.3, published 2026-08-05 | [release API](https://api.github.com/repos/syncthing/syncthing/releases/latest) · [user model](https://docs.syncthing.net/users/syncthing.html) · [ownership](https://docs.syncthing.net/advanced/folder-sync-ownership.html) · [security](https://docs.syncthing.net/users/security.html) |
| Unison | v2.54.0, published 2026-05-01; macOS arm64 binary published | [release API](https://api.github.com/repos/bcpierce00/unison/releases/latest) · [official overview](https://www.cis.upenn.edu/~bcpierce/unison/) · [manual](https://raw.githubusercontent.com/bcpierce00/unison/master/doc/unison-manual.tex) |
| Nix | Manual 2.30.6 | [flake](https://nix.dev/manual/nix/2.30/command-ref/new-cli/nix3-flake) · [lock](https://nix.dev/manual/nix/2.30/command-ref/new-cli/nix3-flake-lock) |
| devenv | v2.2.2, published 2026-08-13 | [release API](https://api.github.com/repos/cachix/devenv/releases/latest) · [README](https://raw.githubusercontent.com/cachix/devenv/main/README.md) |
| Devbox | v0.18.0, published 2026-08-16; Cloud cache/secrets/auth removed | [release API](https://api.github.com/repos/jetify-com/devbox/releases/latest) |
| tmux | 3.7c, published 2026-08-17 | [release API](https://api.github.com/repos/tmux/tmux/releases/latest) · [README](https://raw.githubusercontent.com/tmux/tmux/master/README) |
| mosh | 1.4.0, official release news 2022-10-31 | [official site](https://mosh.org/) |
| Paseo | v0.6.1, published 2026-08-25 | [release API](https://api.github.com/repos/getpaseo/paseo/releases/latest) · [README](https://raw.githubusercontent.com/getpaseo/paseo/main/README.md) · [lifecycle](https://raw.githubusercontent.com/getpaseo/paseo/main/docs/agent-lifecycle.md) |
| OMP | v18.0.10, published 2026-08-28 | [release API](https://api.github.com/repos/can1357/oh-my-pi/releases/latest) · [collab docs](https://raw.githubusercontent.com/can1357/oh-my-pi/main/docs/collab.md) |
| Codex | rust-v0.151.0, published 2026-08-29 | [release API](https://api.github.com/repos/openai/codex/releases/latest) · [remote connections](https://learn.chatgpt.com/docs/remote-connections.md) · [cloud](https://learn.chatgpt.com/docs/cloud.md) |
| Claude Code | Current docs; version is installation/channel-managed | [overview](https://code.claude.com/docs/en/overview.md) · [Remote Control](https://code.claude.com/docs/en/remote-control.md) · [Desktop SSH](https://code.claude.com/docs/en/desktop.md#ssh-sessions) |

## Final recommendation

Use **Tailscale → rsync verified seed (simplest) or Mutagen flush → VPS-local pinned devenv/Nix or Dev Container → Paseo daemon on the VPS → tmux for direct long jobs → VPS-only secret injection**. Mutagen's online two-way-safe mode is the preferred optional return channel for one Mac↔VPS, but must not be the canonical execution dependency. This design continues after an indefinitely offline laptop because the remote agent, files, environment, and credentials are all owned by the VPS runtime.
