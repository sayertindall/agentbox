# DevBox implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `subagent-driven-development` or `executing-plans` to implement this plan task by task. Steps use checkbox syntax for tracking.

**Goal:** Build a private local-to-VPS handoff system that starts a disposable Paseo run sandbox and continues after the Mac loses Internet access.

**Architecture:** `agentbox` runs on the Mac. `agentboxd` runs as root on Linux and owns all server state. Restricted SSH identities separate control from staging transfer. Each provider run uses a newly created Unix user, systemd cgroup, `PASEO_HOME`, workspace, raw-history directory, and mode-0600 Unix socket. Root injects only one selected credential profile into that sandbox. Source manifests, not rsync, define what may activate or return.

**Tech stack:** Go, SQLite through `modernc.org/sqlite`, OpenSSH, rsync, Tailscale, systemd, Paseo, Nix flakes, devenv, and mise.

**Spec:** [SPEC.md](SPEC.md)

## Global constraints

- Do not copy a Mac credential store, provider session directory, private key, `.env` file, `.git` directory, or raw Git object.
- Do not require a Git push or hosted Git service.
- Do not expose a raw Paseo endpoint, TCP control port, public SSH path, public provider port, or public DevBox endpoint.
- Do not give ordinary control or transfer keys a shell, PTY, forwarding, or user rc.
- Do not trust a caller-supplied workspace, run, socket, profile, staging, or daemon path.
- Do not support Mutagen, Syncthing, Unison, Git LFS, submodules, nested repositories, or case-colliding source trees in version 1.
- Do not reuse a run Unix user, `PASEO_HOME`, raw history, workspace, or socket for a later run.
- Do not create a return candidate until the run cgroup has stopped.
- Do not start state-changing VPS work until the host audit, credential injection design, and retention policy are reviewed.
- Do not commit or push as part of this plan.

---

## Current-state evidence

- The local Mac has Paseo 0.6.1, Claude Code 2.1.238, Codex CLI 0.149.1, OMP 18.0.4, and mise 2026.7.13.
- Paseo reports Claude, Codex, pi, and OMP providers as available locally.
- No VPS identity, Linux distribution, architecture, storage, firewall manager, profile count, credential injection method, retention policy, or source project was supplied.
- [RESEARCH.md](RESEARCH.md) records source evidence and tool limitations.

## Planned file structure

```text
DevBox/
  README.md
  REQUIREMENTS.md
  SPEC.md
  IMPLEMENTATION_PLAN.md
  RESEARCH.md
  agentbox/
    go.mod
    mise.toml
    cmd/agentbox/main.go
    cmd/agentboxd/main.go
    cmd/devbox-gateway/main.go
    internal/id/project_id.go
    internal/id/project_id_test.go
    internal/config/project.go
    internal/config/project_test.go
    internal/config/host.go
    internal/config/host_test.go
    internal/enrollment/enrollment.go
    internal/enrollment/enrollment_test.go
    internal/manifest/manifest.go
    internal/manifest/manifest_test.go
    internal/baseline/baseline.go
    internal/baseline/baseline_test.go
    internal/packet/packet.go
    internal/workflow/workflow.go
    internal/workflow/workflow_test.go
    internal/packet/packet_test.go
    internal/store/store.go
    internal/store/store_test.go
    internal/store/migrations.go
    internal/backup/backup.go
    internal/backup/backup_test.go
    internal/protocol/request.go
    internal/protocol/request_test.go
    internal/gateway/gateway.go
    internal/gateway/gateway_test.go
    internal/transfer/rsync.go
    internal/transfer/rsync_test.go
    internal/activation/activation.go
    internal/activation/activation_test.go
    internal/quota/quota.go
    internal/quota/quota_test.go
    internal/run/sandbox.go
    internal/run/sandbox_test.go
    internal/credential/profile.go
    internal/credential/profile_test.go
    internal/environment/wrapper.go
    internal/environment/wrapper_test.go
    internal/egress/policy.go
    internal/egress/policy_test.go
    internal/paseo/client.go
    internal/paseo/client_test.go
    internal/reconcile/reconcile.go
    internal/reconcile/reconcile_test.go
    internal/returning/candidate.go
    internal/returning/candidate_test.go
    internal/returning/journal.go
    internal/returning/journal_test.go
    internal/retention/retention.go
    internal/retention/retention_test.go
    internal/doctor/doctor.go
    internal/doctor/doctor_test.go
    internal/integration/concurrent_operation_test.go
    internal/integration/staging_quota_test.go
    internal/integration/unknown_start_test.go
    internal/integration/run_isolation_test.go
    internal/integration/offline_fake_provider_test.go
    fixtures/
      basic-project/
      tracked-secret-project/
      deleted-source-project/
      non-git-project/
      case-collision-project/
      outside-symlink-project/
      special-file-project/
      run-isolation/
    deploy/
      host-config.example.toml
      systemd/agentboxd.service
      systemd/devbox-run@.service.tmpl
      systemd/devbox-retention.timer
      systemd/devbox-backup.timer
      ssh/authorized_keys.control.example
      ssh/authorized_keys.transfer.example
      ssh/rsync-staging-wrapper
      quota/project-quota.sh
      firewall/nftables-devbox.conf
      firewall/ufw-devbox.sh
      tailscale/policy.example.hujson
      bootstrap/host-audit.sh
      bootstrap/install-host.sh
      bootstrap/install-provider-tools.sh
      bootstrap/provision-credential-profile.sh
      bootstrap/verify-host.sh
    docs/
      operations.md
      recovery.md
      credentials.md
      transfer-boundary.md
```

## Shared interfaces

```go
package id

type ProjectID string

func ParseProjectID(value string) (ProjectID, error)
```

`ParseProjectID` accepts only `[a-z0-9][a-z0-9-]{0,63}`.

```go
package enrollment

type Record struct {
    EnrollmentID string
    Hash         string
}

func Create() (Record, error)
func Load(path string) (Record, error)
```

`EnrollmentID` is a random UUID. The server stores `Hash`, not the raw value.

```go
package manifest

type Entry struct {
    Path       string
    Kind       string
    Executable bool
    Size       int64
    SHA256     string
    Target     string
}

type Manifest struct {
    Version int
    Entries []Entry
    SHA256  string
    Bytes   int64
}

func Build(root string, policy Policy) (Manifest, error)
func Compare(expected, actual Manifest) error
func Materialize(sourceRoot string, m Manifest, destination *os.Root) error
```

`Materialize` writes only declared entries through a caller-provided, pre-opened trusted destination root. It never accepts a destination path string or creates a caller-controlled destination ancestor.

```go
package packet

type Handoff struct {
    Version                int
    HandoffID              string
    PrepareOperationID     string
    ProjectID              string
    SourceManifestSHA256   string
    BaselineManifestSHA256 *string
    BaseRevision           *string
    Task                   string
    CurrentState           string
    NextAction             string
    Constraints            []string
    CreatedAt              time.Time
}

func Create(input CreateInput) (Handoff, error)
func (h Handoff) Validate() error
```

```go
package config

type Host struct {
    Version             int
    Root                string
    TailscaleInterface  string
    SSHPort             int
    StagingMaxBytes     int64
    StagingQuotaBackend string
    RunMaxBytes         int64
    RunQuotaBackend     string
    RunMemoryMax        string
    RunCPUQuota         string
    RunTasksMax         int
    MonitorInterval     time.Duration
    EgressPolicies     []EgressPolicy
    Retention           Retention
    CredentialProfiles  []CredentialProfile
}

type CredentialProfile struct {
    ID                         string
    Provider                   string
    MaxActiveRuns              int
    CredentialInjectionAdaptor string
    EgressPolicyID             string
}

type Retention struct {
    Receipts        time.Duration
    RawOutput       time.Duration
    Archives        time.Duration
    Staging         time.Duration
    DatabaseBackups time.Duration
}
type EgressPolicy struct {
    ID               string
    AllowedHosts     []string
    AllowedPorts     []int
    DNSResolvers     []string
    PackageRegistries []string
    Adapter          string
}

func LoadHost(path string) (Host, error)
func (h Host) Validate() error
```

```go
package store

type ReceiptState string

const (
    LocalOwned       ReceiptState = "local_owned"
    Staging          ReceiptState = "staging"
    RemoteOwned      ReceiptState = "remote_owned"
    RemoteStarting   ReceiptState = "remote_starting"
    RemoteRunning    ReceiptState = "remote_running"
    UnknownRemoteRun ReceiptState = "unknown_remote_run"
    Failed           ReceiptState = "failed"
    Returning        ReceiptState = "returning"
    Conflicted       ReceiptState = "conflicted"
    Closed           ReceiptState = "closed"
    ArchivedCleanupPending ReceiptState = "archived_cleanup_pending"
)

type Store interface {
    WithProjectAndProfileWrite(
        ctx context.Context,
        projectID id.ProjectID,
        credentialProfileID string,
        fn func(Tx) error,
    ) error
}
```

The transaction locks project source state and credential-profile capacity. It validates receipt revision and operation digest before it performs an external side effect.

```go
package run

type Sandbox struct {
    RunID          string
    UnixUser       string
    SystemdUnit    string
    Workspace      string
    MetadataRoot   string
    PaseoSocket    string
    CredentialID   string
}

func Create(ctx context.Context, input CreateInput) (Sandbox, error)
func StopAndArchive(ctx context.Context, sandbox Sandbox) error
```

`StopAndArchive` stops the full systemd cgroup, verifies that no process remains, moves source and metadata into root-owned archive storage, removes the socket and unit, reloads systemd, deletes the run user and group, and verifies cleanup. It retains the credential-profile lock and records `archived_cleanup_pending` when any cleanup step fails.

## Task 1: Create strict local configuration, enrollment, and command wiring

**Files:**

- Create: `agentbox/go.mod`
- Create: `agentbox/mise.toml`
- Create: `agentbox/cmd/agentbox/main.go`
- Create: `agentbox/internal/id/project_id.go`
- Create: `agentbox/internal/id/project_id_test.go`
- Create: `agentbox/internal/config/project.go`
- Create: `agentbox/internal/config/project_test.go`
- Create: `agentbox/internal/enrollment/enrollment.go`
- Create: `agentbox/internal/enrollment/enrollment_test.go`
- Create: `agentbox/internal/workflow/workflow.go`
- Create: `agentbox/internal/workflow/workflow_test.go`

**Produces:** local `init`, `enroll`, `prepare`, `handoff`, `run`, `status`, `attach`, `logs`, `diff`, `cancel`, `reclaim`, `resolve`, `recover`, `resume`, and `doctor` command parsing.

- [ ] **Step 1: Create the Go module and tool file.**

Create `go.mod` with `modernc.org/sqlite`. Create `mise.toml` with a selected Go version, `test`, and `race` tasks.

```toml
[tools]
go = "<selected-version>"

[tasks.test]
run = "go test ./..."

[tasks.race]
run = "go test -race ./..."
```

Select the Go version from the implementation machine. Do not guess it.

- [ ] **Step 2: Write identifier and enrollment tests.**

```go
func TestParseProjectIDAcceptsCanonicalValue(t *testing.T)
func TestParseProjectIDRejectsTraversal(t *testing.T)
func TestParseProjectIDRejectsUnicode(t *testing.T)
func TestEnrollmentCreatesRandomUUIDAndHash(t *testing.T)
func TestEnrollmentLoadRejectsMalformedRecord(t *testing.T)
```

- [ ] **Step 3: Write configuration and command tests.**

```go
func TestInitWritesProjectAndEnrollmentRecord(t *testing.T)
func TestEnrollRequiresControlTransport(t *testing.T)
func TestPrepareRequiresEnrollment(t *testing.T)
func TestHandoffWorkflowFsyncsBeforeExternalRequest(t *testing.T)
func TestHandoffWorkflowReconcilesUnknownChildOperation(t *testing.T)
func TestReclaimWorkflowFsyncsBeforeLocalApply(t *testing.T)
func TestRunRequiresReceiptAndProvider(t *testing.T)
func TestResumeRequiresReceipt(t *testing.T)
func TestCommandNeverAcceptsWorkspacePath(t *testing.T)
```

- [ ] **Step 4: Run focused tests and confirm failure.**

Run:

```text
mise run test -- ./internal/id ./internal/config ./internal/enrollment
```

Expected result: tests fail because the parsing and enrollment APIs do not exist.

- [ ] **Step 5: Implement local files and command dispatch.**

`agentbox init` creates `.agentbox/project.toml` and `.agentbox/enrollment.json`. It validates project ID, local root, SSH aliases, source policy, and provider allow-list. It does not contact the VPS.

`agentbox enroll` sends the enrollment hash to the control gateway. `handoff` requests a staging token, transfers two trees, then sends separate `stage` and `activate` operations. `reclaim` requests `prepare_return`, verifies and applies locally, then sends `reclaim_complete` or `resolve`. `resume` creates a fresh start request from a terminal or explicitly abandoned receipt.

Every multi-step command owns a local workflow record in `.agentbox/workflows/<workflow-id>.json`. Before each external request and each local apply phase, the client fsyncs the child operation ID, expected receipt revision, stage, and pending outcome. It fsyncs the known response after it arrives. A restarted client reconciles an unknown workflow through server status and operation lookup before it retries a child operation. No command accepts an arbitrary remote path.

- [ ] **Step 6: Run tests and formatting.**

Run:

```text
gofmt -w cmd internal
mise run test -- ./internal/id ./internal/config ./internal/enrollment
```

**Verification gate:** local command wiring has one explicit entry point for every specified lifecycle action. Project IDs and enrollment records cannot act as path authority.

## Task 2: Build positive manifests, sanitized baseline trees, and prepare records

**Files:**

- Create: `agentbox/internal/manifest/manifest.go`
- Create: `agentbox/internal/manifest/manifest_test.go`
- Create: `agentbox/internal/baseline/baseline.go`
- Create: `agentbox/internal/baseline/baseline_test.go`
- Create: `agentbox/internal/packet/packet.go`
- Create: `agentbox/internal/packet/packet_test.go`
- Create fixture directories under `agentbox/fixtures/`.

**Produces:** current source manifest, sanitized baseline manifest, materialized baseline tree, and immutable packet.

### Execution split

Task 2 has three independent contracts. Execute them as three serial review units:

1. **Manifest unit:** `internal/manifest` and source fixtures through Step 3.
2. **Baseline unit:** `internal/baseline` and Git or non-Git fixtures through Step 4. It consumes the reviewed manifest API.
3. **Packet unit:** `internal/packet` and local prepare record through Step 5. It consumes the reviewed manifest and baseline APIs.

Run the focused test for each unit before the next unit begins. Review each unit separately. The task-level verification gate remains unchanged after all three units pass.

- [ ] **Step 1: Write source-manifest tests.**

```go
func TestBuildIncludesDirtyAndUntrackedAllowedFiles(t *testing.T)
func TestBuildExcludesTrackedSecretPath(t *testing.T)
func TestBuildRejectsCaseCollision(t *testing.T)
func TestBuildRejectsAbsoluteAndOutsideSymlink(t *testing.T)
func TestBuildRejectsFIFOOrSocket(t *testing.T)
func TestMaterializeRejectsUndeclaredPath(t *testing.T)
```

- [ ] **Step 2: Write baseline and non-Git tests.**

```go
func TestBaselineIncludesOnlyAllowedHEADPaths(t *testing.T)
func TestBaselineExcludesTrackedSecretAtHEAD(t *testing.T)
func TestSourceDeletionSurvivesBaselineOverlay(t *testing.T)
func TestBaselineRejectsLFSSubmoduleAndNestedRepository(t *testing.T)
func TestNonGitProjectGetsSyntheticDiffBaseline(t *testing.T)
```

- [ ] **Step 3: Implement canonical manifests.**

Walk without following symlinks. Normalize paths. Hash regular files. Allow only relative symlinks that resolve beneath the materialized root. Canonically encode and hash the manifest. Calculate total byte count for staging quota.

- [ ] **Step 4: Implement sanitized baseline materialization.**

For Git projects, create a temporary detached `HEAD` worktree, build the sanitized baseline through the reviewed manifest API, materialize it into the caller-provided trusted destination root, then remove and prune the temporary worktree. For non-Git projects, materialize the current source projection as the synthetic baseline. Never copy `.git`, refs, objects, or raw bundle data.

- [ ] **Step 5: Implement `agentbox prepare`.**

Prepare creates handoff and operation UUIDs, source and baseline manifests, materialized trees, packet JSON, and a fsynced local record before networking begins. It uses the same source policy for baseline and current trees.

- [ ] **Step 6: Run focused tests.**

Run:

```text
mise run test -- ./internal/manifest ./internal/baseline ./internal/packet
```

**Verification gate:** a tracked excluded canary never enters either tree. A source deletion survives. A non-Git project receives a deterministic synthetic diff baseline.

## Task 3: Build host configuration, control database, serialization, and backups

**Files:**

- Create: `agentbox/internal/config/host.go`
- Create: `agentbox/internal/config/host_test.go`
- Create: `agentbox/cmd/agentboxd/main.go`
- Create: `agentbox/internal/store/store.go`
- Create: `agentbox/internal/store/store_test.go`
- Create: `agentbox/internal/store/migrations.go`
- Create: `agentbox/internal/backup/backup.go`
- Create: `agentbox/internal/backup/backup_test.go`
- Create: `agentbox/internal/integration/concurrent_operation_test.go`
- Create: `agentbox/deploy/host-config.example.toml`

**Produces:** required host policy, durable project registration, receipt revisions, credential locks, operation idempotency, events, and backups.

- [ ] **Step 1: Write host configuration tests.**

```go
func TestHostConfigRejectsMissingRetention(t *testing.T)
func TestHostConfigRejectsMissingStagingQuota(t *testing.T)
func TestHostConfigRejectsUnsupportedQuotaBackend(t *testing.T)
func TestHostConfigRejectsMissingCredentialProfiles(t *testing.T)
func TestHostConfigRejectsInvalidCredentialProfileAdaptor(t *testing.T)
func TestHostConfigRejectsMissingMonitorInterval(t *testing.T)
func TestHostConfigRejectsInvalidTailscaleInterface(t *testing.T)
func TestHostConfigRejectsMissingRunQuotaAndCgroupLimits(t *testing.T)
```
- [ ] **Step 2: Write database transaction tests.**

```go
func TestEnrollRejectsMismatchedEnrollmentHash(t *testing.T)
func TestDuplicateOperationSameDigestReturnsSavedResult(t *testing.T)
func TestDuplicateOperationDifferentDigestFails(t *testing.T)
func TestConcurrentActivationAllowsExactlyOneWinner(t *testing.T)
func TestConcurrentRunLocksCredentialProfile(t *testing.T)
func TestRevisionMismatchBlocksMutation(t *testing.T)
func TestUnknownOperationBlocksWriterMutation(t *testing.T)
```

Use two independent SQLite connections and a barrier. Assert exactly one committed generation and one active credential-profile lock.

- [ ] **Step 3: Write backup tests.**

```go
func TestBackupCreatesVerifiedDatabaseCopy(t *testing.T)
func TestBackupUsesTemporaryFileAndAtomicRename(t *testing.T)
func TestBackupRetentionSkipsNewestVerifiedCopy(t *testing.T)
func TestRestoreDrillReconcilesActiveRunsBeforeReplacement(t *testing.T)
func TestRestoreControlBlocksWritersUntilLiveRunReconciliation(t *testing.T)
```

- [ ] **Step 4: Implement host config and schema.**

Create tables for projects, handoffs, manifests, staging tokens, generations, source leases, credential profiles, credential locks, runs, operations, receipts, events, retention policies, and backups. Enable foreign keys and WAL. The schema stores enrollment hash, source policy digest, environment identity, and expected receipt revision.

- [ ] **Step 5: Implement serialized mutation.**

`WithProjectAndProfileWrite` starts `BEGIN IMMEDIATE`, validates project and credential locks, checks operation digest and receipt revision, writes durable intent, appends an event, and commits. A lock or commit ambiguity returns a blocking error. It does not guess an outcome.

- [ ] **Step 6: Implement backup, restore drill, and restore-control.**

Use SQLite's backup API. Write the result to a temporary root-owned file, verify it opens and passes integrity check, fsync it, and rename it into backup storage. Record the backup event. Implement a restore drill that loads the newest verified backup into an isolated database and reconciles active receipts with test run units.

Implement `restore-control` as a root-only break-glass operation. It blocks writer requests, restores a selected verified backup into a temporary database, reconciles every active receipt with live units and run sockets, then atomically replaces production control state only when reconciliation succeeds. It records each phase. It keeps unresolved runs unknown and blocks replacement.

- [ ] **Step 7: Run focused tests.**

Run:

```text
mise run test -- ./internal/config ./internal/store ./internal/backup ./internal/integration
mise run race -- ./internal/store ./internal/backup
```

**Verification gate:** the server has one authoritative transaction boundary. Concurrent mutations and credential-profile reuse fail closed. Host policy and backup state exist before a real run.

## Task 4: Implement restricted control and staging transport

**Files:**

- Create: `agentbox/cmd/devbox-gateway/main.go`
- Create: `agentbox/internal/protocol/request.go`
- Create: `agentbox/internal/protocol/request_test.go`
- Create: `agentbox/internal/gateway/gateway.go`
- Create: `agentbox/internal/gateway/gateway_test.go`
- Create: `agentbox/internal/transfer/rsync.go`
- Create: `agentbox/internal/transfer/rsync_test.go`
- Create SSH deployment files.
- Create: `agentbox/internal/integration/staging_quota_test.go`
- Create: `agentbox/internal/quota/quota.go`
- Create: `agentbox/internal/quota/quota_test.go`

**Produces:** a forced control gateway, a one-time quota-bound staging token, and a restricted rsync wrapper.

- [ ] **Step 1: Write gateway tests.**

```go
func TestGatewayRejectsUnknownFieldAndOperation(t *testing.T)
func TestGatewayForwardsOnlyToAgentboxdSocket(t *testing.T)
func TestGatewayRejectsWorkspaceAndProfilePath(t *testing.T)
func TestControlAuthorizedKeyDisablesAllForwardingAndUserRC(t *testing.T)
```

- [ ] **Step 2: Implement `devbox-gateway`.**

Read one bounded newline-delimited JSON request. Forward it unchanged to the root-owned Unix socket. Do not invoke a shell, open a network socket, or interpret source paths.

- [ ] **Step 3: Write staging tests.**

```go
func TestTokenHasQuotaAndExpiry(t *testing.T)
func TestRsyncUsesManifestFilesFromList(t *testing.T)
func TestWrapperRejectsShellAndParentPath(t *testing.T)
func TestProjectQuotaRejectsOverQuotaWrite(t *testing.T)
func TestExtraStagingPathNeverActivates(t *testing.T)
func TestOverQuotaStagingNeverActivates(t *testing.T)
```

The server issues a one-time token after prepare. It computes quota from source and baseline manifest bytes plus host-defined overhead. Before rsync starts, the root quota adapter assigns the token root an enforced filesystem project quota using the host-approved `project-quota` backend. The local client uses `--files-from` generated from each positive manifest. The transfer wrapper accepts rsync server mode only under that token root.

Treat staging as untrusted. The quota backend bounds extra upload. `agentboxd` enforces manifest authority after upload. Extra bytes may consume only the token quota and cannot become source projection.

- [ ] **Step 5: Write SSH restrictions.**

Both authorized-key examples include:

```text
no-port-forwarding,no-agent-forwarding,no-X11-forwarding,no-pty,no-user-rc
```

The control key forces `devbox-gateway`. The transfer key forces the staging rsync wrapper. Neither key can open an interactive shell.

- [ ] **Step 6: Run focused tests.**

Run:

```text
mise run test -- ./internal/protocol ./internal/gateway ./internal/transfer ./internal/integration
```

**Verification gate:** normal control is receipt-bound. Transfer writes only bounded untrusted staging. Neither key has raw host authority.

## Task 5: Activate exact source projection and synthetic Git metadata

**Files:**

- Create: `agentbox/internal/activation/activation.go`
- Create: `agentbox/internal/activation/activation_test.go`
- Modify: `agentbox/internal/store/store.go`
- Create: `agentbox/internal/integration/activation_test.go`

**Produces:** immutable source generations with separate runtime metadata.

- [ ] **Step 1: Write activation tests.**

```go
func TestStageRejectsExtraMissingOrMismatchedPath(t *testing.T)
func TestActivationSourceWorkspaceHasOnlyManifestPaths(t *testing.T)
func TestActivationPlacesPacketOutsideWorkspace(t *testing.T)
func TestActivationPlacesGitDirectoryOutsideWorkspace(t *testing.T)
func TestActivationRemovesDeletedBaselinePath(t *testing.T)
func TestActivationExcludesTrackedSecret(t *testing.T)
func TestNonGitActivationCreatesSyntheticDiff(t *testing.T)
```

- [ ] **Step 2: Implement staging validation.**

Rewalk source and baseline staging trees. Compare each to its manifest. Reject every extra, missing, mismatched, unsafe, expired, or over-quota tree before generation allocation.

- [ ] **Step 3: Implement exact candidate build.**

Create a root-owned generation candidate. Materialize only source-manifest entries into `source/`. For Git projects, build synthetic Git metadata from the sanitized baseline outside `source/`, then overlay and delete paths until `source/` matches current manifest. For non-Git projects, create a synthetic baseline from current source before provider work begins.

- [ ] **Step 4: Promote generation.**

Verify source candidate manifest. Rename it into `generations/<project>/<generation>`. Store packet and Git metadata outside source. Commit `remote_owned` only after all verification succeeds.

- [ ] **Step 5: Run focused tests.**

Run:

```text
mise run test -- ./internal/activation ./internal/integration
```

**Verification gate:** source workspace contains no `.git`, `.agentbox`, packet, raw output, service file, or undeclared source. The synthetic Git diff works for Git and non-Git projects.

## Task 6: Implement credential profiles, disposable run sandboxes, and environment wrappers

**Files:**

- Create: `agentbox/internal/credential/profile.go`
- Create: `agentbox/internal/credential/profile_test.go`
- Create: `agentbox/internal/run/sandbox.go`
- Create: `agentbox/internal/run/sandbox_test.go`
- Create: `agentbox/internal/environment/wrapper.go`
- Create: `agentbox/internal/environment/wrapper_test.go`
- Create systemd and credential bootstrap templates.
- Create: `agentbox/internal/egress/policy.go`
- Create: `agentbox/internal/egress/policy_test.go`
- Create: `agentbox/internal/integration/run_isolation_test.go`

**Produces:** a server-controlled credential-to-run injection path, a fresh Unix user and cgroup per run, a source-safe environment wrapper, and a deny-by-default egress policy adapter.

- [ ] **Step 1: Write credential and sandbox tests.**

```go
func TestCredentialProfileAllowsOneActiveRun(t *testing.T)
func TestSandboxGetsFreshUserAndPaseoHome(t *testing.T)
func TestSandboxGetsOnlySelectedCredentialMaterial(t *testing.T)
func TestSandboxCannotReadCredentialProfileStore(t *testing.T)
func TestSandboxCannotReadSiblingRunWorkspaceOrHistory(t *testing.T)
func TestSandboxCannotConnectSiblingRunSocket(t *testing.T)
func TestStopAndArchiveStopsEntireCgroup(t *testing.T)
func TestStopAndArchiveRemovesUnitSocketUserAndGroup(t *testing.T)
func TestCleanupFailureRetainsCredentialProfileLock(t *testing.T)
func TestSandboxRunQuotaContainsWorkspaceAndMetadata(t *testing.T)
func TestSandboxResourceLimitsProtectSiblingAndControlState(t *testing.T)
func TestSandboxStartsWithEgressDenied(t *testing.T)
func TestSandboxAllowsOnlyRecordedProviderDestination(t *testing.T)
func TestSandboxDeniesPrivateMetadataHostAndSiblingRanges(t *testing.T)
```

- [ ] **Step 2: Write wrapper and capability tests.**

```go
func TestWrapperUsesServerResolvedWorkspace(t *testing.T)
func TestWrapperSetsExternalGitMetadataPaths(t *testing.T)
func TestWrapperSetsHandoffFileOutsideWorkspace(t *testing.T)
func TestWrapperRejectsWrongEnvironmentIdentity(t *testing.T)
func TestPaseoUnixSocketListenAndQuery(t *testing.T)
func TestNoTCPFallbackPathExists(t *testing.T)
```

- [ ] **Step 3: Implement credential profiles.**

Store provider credential material under a root-owned profile directory. Record provider, verification time, expiry when known, revocation-test result, injection adaptor, and egress policy identifier. A profile cannot receive a run until credential verification, revocation, and egress-policy tests pass.

- [ ] **Step 4: Implement run sandbox creation.**

Generate a server-safe run ID and a new `devbox-run-<id>` user. Before the run user can write, assign the whole run tree a host-enforced project quota. Create unique workspace and metadata paths. Create a systemd unit that runs as that user with restrictive service properties, `MemoryMax`, `CPUQuota`, and `TasksMax` from host configuration. Inject only selected profile material through the root-controlled adaptor. Attach the server-recorded deny-by-default egress adapter before the run receives a credential. Start Paseo on a mode-0600 Unix socket in run metadata.

The wrapper verifies the server-recorded Nix and devenv identity. It sets `GIT_DIR`, `GIT_WORK_TREE`, and `DEVBOX_HANDOFF_FILE` to runtime metadata paths. Before it executes the provider binary, it writes the run-specific wrapper nonce into the root-created invocation-marker path. The real smoke test checks that marker. The wrapper does not accept a network-provided path.

Run an isolated daemon and client query with the selected installed Paseo binary:

```text
<PASEO_BIN> daemon start --foreground --listen <temporary-socket> --no-web-ui --no-relay
<PASEO_BIN> ls --host <temporary-socket> --json
```

If listen or query fails, stop. Do not add a TCP fallback.

Stop the full systemd run cgroup. Verify no process remains. Move source workspace and metadata into root-owned archive storage. Remove the socket, unit, and drop-in. Reload systemd. Delete the run user and group. Verify that the UID owns no active path. Release the credential profile lock only after every cleanup step succeeds. If cleanup fails, record `archived_cleanup_pending`, retain the lock, and require repair before reuse.

- [ ] **Step 7: Implement environment wrapper.**

The wrapper verifies the server-recorded Nix and devenv identity. It sets `GIT_DIR`, `GIT_WORK_TREE`, and `DEVBOX_HANDOFF_FILE` to runtime metadata paths. It then executes `devenv shell --` and the provider binary. It does not accept a network-provided path.

- [ ] **Step 8: Run focused tests.**

Run:

```text
mise run test -- ./internal/credential ./internal/run ./internal/environment ./internal/integration
```

**Verification gate:** a later run cannot read an earlier run's source, raw history, `PASEO_HOME`, credential, or socket. A run cannot start without Unix-socket capability, credential verification, and environment identity.

**Egress gate:** a MicroSandbox adapter must configure a default-deny policy with provider-domain, DNS pinning, TLS, and secret-host rules. A systemd adapter is unavailable until a root-owned network namespace, egress proxy, or equivalent policy proves the same allowed and denied destination behavior.

## Task 7: Implement server-side Paseo lifecycle and client command behavior

**Files:**

- Create: `agentbox/internal/paseo/client.go`
- Create: `agentbox/internal/paseo/client_test.go`
- Create: `agentbox/internal/reconcile/reconcile.go`
- Create: `agentbox/internal/reconcile/reconcile_test.go`
- Create: `agentbox/internal/integration/unknown_start_test.go`
- Create: `agentbox/internal/integration/offline_fake_provider_test.go`
- Modify: `agentbox/cmd/agentbox/main.go`
- Modify: `agentbox/cmd/agentboxd/main.go`

**Produces:** server-side start, status, attach, logs, diff, cancel, reconciliation, and resume operations.

- [ ] **Step 1: Write start and command tests.**

```go
func TestStartCommitsIntentBeforeExternalCall(t *testing.T)
func TestStartRecordsAgentIDBeforeReply(t *testing.T)
func TestUnknownStartBlocksSecondRunAndReturn(t *testing.T)
func TestReconcileFindsAgentByOperationLabel(t *testing.T)
func TestRealPaseoRoundTripsOperationAndReceiptLabels(t *testing.T)
func TestRealPaseoInvokesEnvironmentWrapper(t *testing.T)
func TestStatusUsesServerReceipt(t *testing.T)
func TestAttachAndLogsDoNotCreateReplacementAgent(t *testing.T)
func TestDiffUsesSyntheticGitForNonGitProject(t *testing.T)
func TestResumeCreatesFreshSandboxFromTerminalReceipt(t *testing.T)
```

- [ ] **Step 2: Implement run-local Paseo client.**

`agentboxd` invokes the selected run's Paseo client through its mode-0600 socket. It runs provider diagnostic, start, list, attach, logs, diff, cancel, and status only after receipt authorization.

The initial prompt tells the provider to read `DEVBOX_HANDOFF_FILE`. The packet contains task, current state, next action, constraints, source digest, and base revision. Labels include operation and receipt IDs.

- [ ] **Step 3: Implement durable start and reconciliation.**

Commit `remote_starting` before creating or using a sandbox. After known success, record run ID, user, unit, socket, and agent ID before reply. On ambiguity, set `unknown_remote_run`. The reconciliation worker checks run unit state and queries only the matching operation label. An unavailable socket never proves a terminal run.

- [ ] **Step 4: Implement terminal lifecycle.**

On known terminal success or failure, record the result. Stop and archive the cgroup before return or credential-profile reuse. On unknown state, preserve the sandbox and block source mutation until reconciliation or logged break-glass.

- [ ] **Step 5: Implement output and local commands.**

`status` returns structured receipt state. `attach` and `logs` stream run or archive output but never write stream bytes to control events. `diff` uses external synthetic Git metadata. `cancel` has an idempotent operation. `resume` creates a new sandbox only from a terminal or explicitly abandoned receipt.

- [ ] **Step 6: Run deterministic local Linux fixture.**

Use a fake provider that writes a known marker after a controlled delay. Start it, close the control request, and reconcile from a new request. The test proves process ownership without relying on model behavior.

- [ ] **Step 7: Run focused tests.**

Run:

```text
mise run test -- ./internal/paseo ./internal/reconcile ./internal/integration
mise run race -- ./internal/paseo ./internal/reconcile
```

**Verification gate:** an acknowledged run is server-owned. An ambiguous run blocks source mutation. The full packet reaches the provider through a file outside source workspace.

## Task 8: Implement return candidates, rollback journals, conflict resolution, and recovery

**Files:**

- Create: `agentbox/internal/returning/candidate.go`
- Create: `agentbox/internal/returning/candidate_test.go`
- Create: `agentbox/internal/returning/journal.go`
- Create: `agentbox/internal/returning/journal_test.go`
- Modify: `agentbox/cmd/agentbox/main.go`
- Create: `agentbox/docs/recovery.md`

**Produces:** cgroup-frozen return candidates, local journaled apply, explicit conflict resolution, and recovery.

- [ ] **Step 1: Write candidate and journal tests.**

```go
func TestPrepareReturnRejectsActiveStartingAndUnknownRun(t *testing.T)
func TestPrepareReturnStopsCgroupBeforeCandidate(t *testing.T)
func TestReturnManifestIncludesAllowedAgentCreatedFile(t *testing.T)
func TestCandidateContainsOnlyReturnManifestPaths(t *testing.T)
func TestCandidateVerifiesBeforeLocalMutation(t *testing.T)
func TestConflictLeavesBothTreesUntouched(t *testing.T)
func TestJournalRestoresOriginalAfterApplyFailure(t *testing.T)
func TestResolveReturnsLeaseAfterConfirmation(t *testing.T)
```

- [ ] **Step 2: Implement server-side candidate preparation.**

Reconcile run state. Stop full cgroup. Reject if stop cannot be proven. Build a return manifest from every allowed source path present after the stopped run. Materialize only return-manifest paths into a root-owned candidate. Verify candidate manifest. Set receipt to `returning`.

- [ ] **Step 3: Implement local verification and journal.**

Download to `.agentbox/returns/<receipt-id>/candidate`. Verify before local mutation. Compare current local source to original handoff source. On a mismatch, record `conflicted` and stop.

On a match, fsync journal metadata and per-path backups. Apply only return-manifest additions, replacements, and deletions. Preserve excluded local paths. Verify final return manifest before local lease returns.

- [ ] **Step 4: Implement resolve and recover.**

`resolve` requires explicit confirmation, no active or unknown run, a selected local manifest, and a server event. `recover` restores original paths or finishes a candidate from an incomplete journal. A new handoff refuses while recovery is pending.

- [ ] **Step 5: Run focused tests.**

Run:

```text
mise run test -- ./internal/returning ./internal/integration
```

**Verification gate:** DevBox never builds a return candidate while an agent cgroup can still write. It never changes local source before candidate verification and journal fsync.

## Task 9: Audit and provision the real VPS

**Files:**

- Create: `agentbox/deploy/bootstrap/host-audit.sh`
- Create: `agentbox/deploy/bootstrap/install-host.sh`
- Create: `agentbox/deploy/bootstrap/install-provider-tools.sh`
- Create: `agentbox/deploy/bootstrap/provision-credential-profile.sh`
- Create: `agentbox/deploy/bootstrap/verify-host.sh`
- Create systemd, SSH, firewall, Tailscale, timer, and host-config deployment files.
- Create: `agentbox/docs/operations.md`
- Create: `agentbox/docs/credentials.md`
- Create: `agentbox/docs/transfer-boundary.md`

**Produces:** a read-only audit, then an idempotent host bootstrap using code and templates from Tasks 1 through 8.

- [ ] **Step 1: Run the read-only host audit.**

The audit reports operating-system release, kernel, architecture, systemd, disk, memory, CPU, cgroup v2 support, listeners, SSH listener addresses, public firewall rules, firewall manager, Tailscale status, OpenSSH effective settings, package managers, provider binary support, Unix-socket capability, filesystem project-quota support for staging and run trees, and available per-run egress-enforcement backends. It redacts environment values and never reads credential content.

- [ ] **Step 2: Review the audit before mutation.**

Stop when any condition is true:

- Public-interface SSH accepts TCP port 22.
- A DevBox, Paseo, or provider endpoint is public.
- Tailscale policy is broad.
- The host lacks user lifecycle, systemd, cgroup resource limits, Unix-socket, firewall, project-quota, or provider support.
- Capacity cannot hold active run quotas, candidate, journal, enforced staging quota, archive, and backup retention needs.
- Credential injection method or deny-by-default egress adapter is not approved for all requested providers.

- [ ] **Step 3: Apply idempotent host bootstrap.**

Bootstrap reads valid root-owned host config. It creates `agentboxd`, control and transfer users, root-only state paths, source generation paths, run parent paths, archive paths, SSH forced commands, project-quota support for staging and run trees, cgroup resource-limit templates, deny-by-default egress policy infrastructure, Tailscale prerequisites, backup and retention timers, and firewall rules.

The firewall accepts SSH only through the configured Tailscale interface and drops public TCP port 22. It uses the detected nftables or UFW manager. Re-running bootstrap converges on the same users, permissions, units, rules, quota configuration, cgroup templates, config, and database migrations.

- [ ] **Step 4: Install and capability-check tools.**

Install Paseo, Claude Code, Codex, OMP, rsync, Nix, devenv, mise, and project dependencies from approved sources. Run the Unix-socket capability test before profile provisioning. Do not create a TCP fallback.

- [ ] **Step 5: Provision credential profiles.**

For each requested provider, perform an operator-owned VPS-local login or scoped API-key setup. Store secret material root-owned. Test injection into a disposable test sandbox and run a revocation check. Store verification metadata only.

- [ ] **Step 6: Verify the host.**

`verify-host.sh` must prove:

```text
agentboxd has no TCP listener.
Normal control SSH has no shell, port forwarding, or user rc.
Transfer SSH cannot leave token staging.
The host firewall permits SSH through Tailscale and rejects public TCP port 22.
An off-tailnet probe cannot establish SSH.
Paseo daemon and client pass Unix-socket listen and query.
A run socket has mode 0600 and no TCP listener.
A run user cannot read another run's source, raw history, credential, control state, or socket.
No run user has sudo, Docker socket access, deploy credentials, or cloud-console access.
Host config includes all retention values, staging and run quotas, quota backends, memory, CPU, task, and monitor limits, plus credential profile definitions.
The staging token and a run sandbox both reject writes that exceed their assigned filesystem project quotas. The run unit shows configured `MemoryMax`, `CPUQuota`, and `TasksMax`.
The run sandbox reaches only its recorded provider, DNS, and package destinations. It cannot reach private, metadata, host, Tailscale, or sibling-run ranges.
Control database backup passes verification and restore drill reconciliation.
```

**Verification gate:** the selected VPS has every required boundary before a real source handoff.

## Task 10: Prove real VPS behavior and update operations evidence

**Files:**

- Create: `agentbox/internal/integration/offline_fake_provider_test.go`
- Create: `agentbox/internal/integration/run_isolation_test.go`
- Modify: `agentbox/docs/operations.md`
- Modify: `README.md`

**Produces:** real VPS proof for offline execution, source boundaries, run isolation, recovery, and provider availability.

- [ ] **Step 1: Create deterministic test fixtures.**

Use a test project with tracked source, untracked source, tracked excluded canary, source deletion, no unsupported Git feature, and a non-Git variant. Use a fake provider that writes a known marker after a controlled delay and exits with a known result.

- [ ] **Step 2: Prove control, transfer, and firewall boundaries.**

Attempt a control shell, control port forward, transfer control request, rsync path escape, staging quota overrun, public SSH connection from off-tailnet, cross-run socket connection, cross-run credential read, and cross-run workspace read. Attempt a run workspace, `PASEO_HOME`, and raw-history write that exceeds the assigned run quota. Attempt memory, CPU, and task pressure through the fake provider. Attempt an allowed provider endpoint and disallowed public, private, metadata, host, Tailscale, and sibling-run destinations.

Expected result: every unauthorized access attempt fails. The run reaches only its allowed provider destination. The run hits its configured resource limit without modifying control state or another run sandbox. Control events contain no raw output or canary secret.

- [ ] **Step 3: Prove offline execution.**

Enroll, prepare, hand off, and start the fake provider. After `remote_running`, close the Mac control connection and remove Mac network access. Wait for the marker. Reconnect, reconcile, diff, and reclaim.

Expected result:

```text
The run cgroup remains active after Mac disconnect.
The marker exists in the verified candidate and returned source.
The excluded canary never enters VPS source, baseline, workspace, or return candidate.
The deleted source remains deleted.
The source workspace has no runtime metadata path.
```

- [ ] **Step 4: Prove start ambiguity and cgroup freeze.**

Stop `agentboxd` after fake Paseo accepts start and before agent ID persistence. Restart it. Verify `unknown_remote_run`, label-based reconciliation, and blocked return. Then verify that candidate preparation stops the complete run cgroup before it copies a source path.

- [ ] **Step 5: Prove local conflict and journal recovery.**

Edit local source outside DevBox while remote source lease is active. Reclaim must record conflict and leave both trees untouched. Then interrupt a clean journaled apply. `agentbox recover` must restore original or complete verified candidate.

- [ ] **Step 6: Prove backup restore and active-run reconciliation.**

Create a verified control backup while a fake run record exists. Restore that backup into an isolated database. Reconcile the restored active record against the live fake run unit and its label. Prove that the drill does not replace production control state until reconciliation passes.

- [ ] **Step 7: Run provider smoke tests.**

For Claude Code, Codex, and OMP, create one disposable sandbox with approved credentials. Acceptance means authenticated start, receipt-bound workspace use, Unix-socket endpoint, and visible terminal state. Do not use model output as proof of offline protocol correctness.

For each real provider smoke test, verify that `paseo ls --json` returns the DevBox operation and receipt labels. Verify the environment wrapper emitted its root-owned structured invocation marker before the provider process started.

- [ ] **Step 8: Record operations evidence.**

Write host architecture, Linux release, firewall manager, quota backend, service names, socket test result, credential profile verification metadata, retention values, backup and restore-drill results, label round-trip evidence, wrapper evidence, and test results to `operations.md`. Exclude IP addresses, private hostnames, credentials, source content, and raw provider output.

**Verification gate:** the actual VPS proves the user-critical behavior. A run continues after Mac Internet loss. Every ordinary access path remains constrained.

## Validation matrix

| Requirement group | Proof |
|---|---|
| FR-001, FR-002 | Local config, enrollment, prepare, idempotency, and packet tests |
| FR-003 through FR-005 | Manifest, baseline, staging quota, exact projection, and synthetic Git tests |
| FR-006, FR-007, FR-010 | Run sandbox, server-side start, reconciliation, and fake-provider offline tests |
| FR-008 | Receipt-bound status, logs, attach, synthetic diff, and archive authorization tests |
| FR-009 | Cgroup-stop, candidate, journal, conflict, resolve, and recovery tests |
| FR-011 | Cross-run filesystem, credential, raw-history, control-state, and socket denial tests |
| FR-012 | Host config, enforced project quota, SQLite backup, isolated restore drill, retention, and cleanup tests |
| NFR-SEC-001 | Forced-key, staging-wrapper, firewall, off-tailnet, and Unix-socket tests |
| NFR-SEC-002 | Root-only run creation and cross-run isolation tests |
| NFR-REL-003 | Run project-quota, `MemoryMax`, `CPUQuota`, `TasksMax`, and sibling-control protection tests |
| NFR-SEC-003 | Tracked-secret, source projection, return candidate, and staging-extra tests |
| NFR-MAINT-001 | Environment wrapper and clean sandbox build tests |
| NFR-PORT-001 | macOS arm64 build and fixture tests plus selected Linux build and fixture tests |

## Risks and rollback

| Risk | Mitigation | Rollback |
|---|---|---|
| Bad transfer content | Positive manifests, staging quota, and inactive validation | Expire token and delete untrusted staging |
| Ambiguous run start | Durable start intent, labels, run unit, and blocking unknown state | Reconcile or use logged break-glass |
| Credential exposure | Root-owned profile plus selected run injection | Revoke profile, stop affected runs, rotate access |
| Run data leak into later project | Per-run user, metadata, socket, and root archive | Stop sandbox, archive root-only, remove run access |
| Database or host failure | Transactions, backups, and reconciliation | Restore control backup, reconcile run units, then decide receipts |
| Local apply interruption | Verified candidate and fsynced journal | Run `agentbox recover` before another handoff |
| Capacity shortfall | Audit and explicit host configuration | Refuse new run and add capacity or profile later |

## Plan self-review

- Normal Mac control has no raw Paseo or shell path.
- Staging is untrusted and quota-bound until positive manifest validation.
- Source projection has no packet, `.git`, raw history, or service metadata.
- Every run has a fresh Unix user, cgroup, `PASEO_HOME`, source workspace, socket, and selected credential injection.
- The full cgroup stops before candidate creation and credential-profile reuse.
- Local command wiring includes enrollment, resume, logs, resolve, and recover.
- The host configuration defines quota, monitor, retention, backup, firewall, and profile policy before real work.
- A fake provider proves offline behavior. Real provider smoke tests prove only authenticated start and terminal visibility.

## Implementation start condition

Do not begin Task 9 or Task 10 until the user supplies VPS SSH identity, Linux distribution and version, CPU architecture, firewall manager, capacity, first project root, credential injection choices, profile count, retention values, staging quota, and monitor interval. Tasks 1 through 8 produce local code, tests, templates, and fixtures only. Task 9 begins with a read-only audit and stops for review before it changes the VPS.