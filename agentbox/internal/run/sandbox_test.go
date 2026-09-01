package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devbox/agentbox/internal/config"
	"devbox/agentbox/internal/credential"
	"devbox/agentbox/internal/egress"
)
func testProfile(t *testing.T) *credential.Profile {
	t.Helper()
	p, err := credential.New(credential.Profile{
		ID: "codex-primary", Provider: "codex", StoreDir: t.TempDir(),
		Adaptor: "systemd-credentials", EgressPolicyID: "codex", MaxActiveRuns: 1,
		Verified: true, RevocationChecked: true, EgressChecked: true,
		Material: []byte("sk-test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &p
}

func testHost() config.Host {
	return config.Host{RunMaxBytes: 1024, RunMemoryMax: "1G", RunCPUQuota: "50%", RunTasksMax: 32}
}

func TestSandboxGetsFreshUserAndPaseoHome(t *testing.T) {
	driver := NewMemoryDriver()
	a, err := Create(Input{Root: t.TempDir(), Host: testHost(), Profile: testProfile(t), Driver: driver})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	b, err := Create(Input{Root: t.TempDir(), Host: testHost(), Profile: testProfile(t), Driver: driver})
	if err != nil {
		t.Fatalf("Create sibling: %v", err)
	}
	if a.UnixUser == b.UnixUser || a.PaseoHome == b.PaseoHome || a.RunID == b.RunID {
		t.Fatalf("sandboxes reused identity: %+v vs %+v", a, b)
	}
	if !strings.HasPrefix(a.UnixUser, "devbox-run-") {
		t.Fatalf("unix user = %q", a.UnixUser)
	}
}

func TestSandboxGetsOnlySelectedCredentialMaterial(t *testing.T) {
	box, err := Create(Input{Root: t.TempDir(), Host: testHost(), Profile: testProfile(t), Driver: NewMemoryDriver()})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(box.CredentialPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "sk-test" {
		t.Fatalf("injected = %q", data)
	}
}

func TestSandboxCannotReadCredentialProfileStore(t *testing.T) {
	profile := testProfile(t)
	box, err := Create(Input{Root: t.TempDir(), Host: testHost(), Profile: profile, Driver: NewMemoryDriver()})
	if err != nil {
		t.Fatal(err)
	}
	if Allowed(box, profile.StorePath()) {
		t.Fatal("run user can read the credential profile store")
	}
}

func TestSandboxCannotReadSiblingRunWorkspaceOrHistory(t *testing.T) {
	driver := NewMemoryDriver()
	a, err := Create(Input{Root: t.TempDir(), Host: testHost(), Profile: testProfile(t), Driver: driver})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Create(Input{Root: t.TempDir(), Host: testHost(), Profile: testProfile(t), Driver: driver})
	if err != nil {
		t.Fatal(err)
	}
	if Allowed(a, b.Workspace) || Allowed(a, filepath.Join(b.MetadataRoot, "raw-history")) {
		t.Fatal("run user can read sibling workspace or history")
	}
}

func TestSandboxCannotConnectSiblingRunSocket(t *testing.T) {
	driver := NewMemoryDriver()
	a, err := Create(Input{Root: t.TempDir(), Host: testHost(), Profile: testProfile(t), Driver: driver})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Create(Input{Root: t.TempDir(), Host: testHost(), Profile: testProfile(t), Driver: driver})
	if err != nil {
		t.Fatal(err)
	}
	if Allowed(a, b.PaseoSocket) {
		t.Fatal("run user can connect to a sibling Paseo socket")
	}
}

func TestStopAndArchiveStopsEntireCgroup(t *testing.T) {
	driver := NewMemoryDriver()
	box, err := Create(Input{Root: t.TempDir(), Host: testHost(), Profile: testProfile(t), Driver: driver})
	if err != nil {
		t.Fatal(err)
	}
	if err := StopAndArchive(box, t.TempDir(), driver); err != nil {
		t.Fatal(err)
	}
	if !driver.CgroupEmpty(box.SystemdUnit) {
		t.Fatal("cgroup still has processes")
	}
}

func TestStopAndArchiveRemovesUnitSocketUserAndGroup(t *testing.T) {
	driver := NewMemoryDriver()
	box, err := Create(Input{Root: t.TempDir(), Host: testHost(), Profile: testProfile(t), Driver: driver})
	if err != nil {
		t.Fatal(err)
	}
	if err := StopAndArchive(box, t.TempDir(), driver); err != nil {
		t.Fatal(err)
	}
	if driver.HasUser(box.UnixUser) || driver.HasUnit(box.SystemdUnit) {
		t.Fatal("unit or user remained after archive")
	}
	if _, err := os.Lstat(box.PaseoSocket); err == nil {
		t.Fatal("socket remained after archive")
	}
}

func TestCleanupFailureRetainsCredentialProfileLock(t *testing.T) {
	driver := NewMemoryDriver()
	driver.FailDelete = true
	profile := testProfile(t)
	if err := profile.Lock("op-1"); err != nil {
		t.Fatal(err)
	}
	box, err := Create(Input{Root: t.TempDir(), Host: testHost(), Profile: profile, Driver: driver})
	if err != nil {
		t.Fatal(err)
	}
	err = StopAndArchive(box, t.TempDir(), driver)
	if err == nil {
		t.Fatal("cleanup failure was ignored")
	}
	if !profile.Locked() {
		t.Fatal("credential lock was released after failed cleanup")
	}
}

func TestSandboxRunQuotaContainsWorkspaceAndMetadata(t *testing.T) {
	box, err := Create(Input{Root: t.TempDir(), Host: testHost(), Profile: testProfile(t), Driver: NewMemoryDriver()})
	if err != nil {
		t.Fatal(err)
	}
	if box.QuotaBytes != 1024 {
		t.Fatalf("quota = %d", box.QuotaBytes)
	}
	if !strings.HasPrefix(box.Workspace, box.Root) || !strings.HasPrefix(box.MetadataRoot, box.Root) {
		t.Fatal("workspace or metadata is outside the quota root")
	}
}

func TestSandboxResourceLimitsProtectSiblingAndControlState(t *testing.T) {
	box, err := Create(Input{Root: t.TempDir(), Host: testHost(), Profile: testProfile(t), Driver: NewMemoryDriver(), ControlRoot: "/var/lib/devbox"})
	if err != nil {
		t.Fatal(err)
	}
	if box.MemoryMax != "1G" || box.CPUQuota != "50%" || box.TasksMax != 32 {
		t.Fatalf("limits = %+v", box)
	}
	if Allowed(box, "/var/lib/devbox/control/devbox.db") {
		t.Fatal("run user can read control state")
	}
}

func TestSandboxStartsWithEgressDenied(t *testing.T) {
	box, err := Create(Input{Root: t.TempDir(), Host: testHost(), Profile: testProfile(t), Driver: NewMemoryDriver()})
	if err != nil {
		t.Fatal(err)
	}
	if box.Egress.Allows("api.openai.com", 443) {
		t.Fatal("sandbox started with egress allowed")
	}
}

func TestSandboxAllowsOnlyRecordedProviderDestination(t *testing.T) {
	p := egress.Policy{Hosts: []string{"api.openai.com"}, Ports: []int{443}}
	if !p.Allows("api.openai.com", 443) || p.Allows("evil.example", 443) {
		t.Fatal("provider allowlist is wrong")
	}
}

func TestSandboxDeniesPrivateMetadataHostAndSiblingRanges(t *testing.T) {
	p := egress.Policy{Hosts: []string{"10.0.0.1", "169.254.169.254"}, Ports: []int{443}}
	if p.Allows("10.0.0.1", 443) || p.Allows("169.254.169.254", 443) {
		t.Fatal("private or metadata destination was allowed")
	}
}
