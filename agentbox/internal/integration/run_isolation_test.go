package integration

import (
	"testing"

	"devbox/agentbox/internal/config"
	"devbox/agentbox/internal/credential"
	"devbox/agentbox/internal/run"
)

func TestSandboxCannotReadSiblingRunWorkspaceOrHistory(t *testing.T) {
	driver := run.NewMemoryDriver()
	host := config.Host{RunMaxBytes: 1024, RunMemoryMax: "1G", RunCPUQuota: "50%", RunTasksMax: 8}
	profile := func() *credential.Profile {
		p, err := credential.New(credential.Profile{
			ID: "codex-primary", Provider: "codex", StoreDir: t.TempDir(),
			Adaptor: "systemd-credentials", MaxActiveRuns: 1,
			Verified: true, RevocationChecked: true, EgressChecked: true,
			Material: []byte("sk"),
		})
		if err != nil {
			t.Fatal(err)
		}
		return &p
	}
	a, err := run.Create(run.Input{Root: t.TempDir(), Host: host, Profile: profile(), Driver: driver})
	if err != nil {
		t.Fatal(err)
	}
	b, err := run.Create(run.Input{Root: t.TempDir(), Host: host, Profile: profile(), Driver: driver})
	if err != nil {
		t.Fatal(err)
	}
	if run.Allowed(a, b.Workspace) || run.Allowed(a, b.PaseoSocket) {
		t.Fatal("sibling isolation failed")
	}
}
