package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validHostTOML() string {
	return strings.TrimSpace(`
version = 1
root = "/var/lib/devbox"
tailscale_interface = "tailscale0"
ssh_port = 22
staging_max_bytes = 1073741824
staging_quota_backend = "project-quota"
run_max_bytes = 1073741824
run_quota_backend = "project-quota"
run_memory_max = "4G"
run_cpu_quota = "200%"
run_tasks_max = 256
monitor_interval = "30s"

[retention]
receipts = "720h"
raw_output = "168h"
archives = "2160h"
staging = "24h"
database_backups = "720h"

[[credential_profiles]]
id = "codex-primary"
provider = "codex"
max_active_runs = 1
credential_injection_adaptor = "systemd-credentials"
egress_policy_id = "codex"
`) + "\n"
}

func writeHost(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "host.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write host config: %v", err)
	}
	return path
}

func TestHostConfigRejectsMissingRetention(t *testing.T) {
	contents := strings.Replace(validHostTOML(), "[retention]\nreceipts = \"720h\"\nraw_output = \"168h\"\narchives = \"2160h\"\nstaging = \"24h\"\ndatabase_backups = \"720h\"\n\n", "", 1)
	_, err := LoadHost(writeHost(t, contents))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "retention") {
		t.Fatalf("error = %v, want retention", err)
	}
}

func TestHostConfigRejectsMissingStagingQuota(t *testing.T) {
	contents := strings.Replace(validHostTOML(), "staging_max_bytes = 1073741824\n", "", 1)
	_, err := LoadHost(writeHost(t, contents))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "staging") {
		t.Fatalf("error = %v, want staging quota", err)
	}
}

func TestHostConfigRejectsUnsupportedQuotaBackend(t *testing.T) {
	contents := strings.Replace(validHostTOML(), "staging_quota_backend = \"project-quota\"", "staging_quota_backend = \"xfs-prjquota\"", 1)
	_, err := LoadHost(writeHost(t, contents))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "quota") {
		t.Fatalf("error = %v, want unsupported quota backend", err)
	}
}

func TestHostConfigRejectsMissingCredentialProfiles(t *testing.T) {
	contents := strings.Split(validHostTOML(), "[[credential_profiles]]")[0]
	_, err := LoadHost(writeHost(t, contents))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "credential") {
		t.Fatalf("error = %v, want credential profiles", err)
	}
}

func TestHostConfigRejectsInvalidCredentialProfileAdaptor(t *testing.T) {
	contents := strings.Replace(validHostTOML(), "credential_injection_adaptor = \"systemd-credentials\"", "credential_injection_adaptor = \"env-file\"", 1)
	_, err := LoadHost(writeHost(t, contents))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "adaptor") {
		t.Fatalf("error = %v, want invalid adaptor", err)
	}
}

func TestHostConfigRejectsMissingMonitorInterval(t *testing.T) {
	contents := strings.Replace(validHostTOML(), "monitor_interval = \"30s\"\n", "", 1)
	_, err := LoadHost(writeHost(t, contents))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "monitor") {
		t.Fatalf("error = %v, want monitor interval", err)
	}
}

func TestHostConfigRejectsInvalidTailscaleInterface(t *testing.T) {
	contents := strings.Replace(validHostTOML(), "tailscale_interface = \"tailscale0\"", "tailscale_interface = \"../eth0\"", 1)
	_, err := LoadHost(writeHost(t, contents))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "tailscale") {
		t.Fatalf("error = %v, want invalid tailscale interface", err)
	}
}

func TestHostConfigRejectsMissingRunQuotaAndCgroupLimits(t *testing.T) {
	contents := validHostTOML()
	for _, line := range []string{
		"run_max_bytes = 1073741824\n",
		"run_memory_max = \"4G\"\n",
		"run_cpu_quota = \"200%\"\n",
		"run_tasks_max = 256\n",
	} {
		contents = strings.Replace(contents, line, "", 1)
	}
	_, err := LoadHost(writeHost(t, contents))
	if err == nil {
		t.Fatal("LoadHost accepted a host config with no run quota or cgroup limits")
	}
	lower := strings.ToLower(err.Error())
	if !strings.Contains(lower, "run") && !strings.Contains(lower, "cgroup") && !strings.Contains(lower, "memory") {
		t.Fatalf("error = %v, want run quota or cgroup limits", err)
	}
}

func TestHostConfigLoadsValidFile(t *testing.T) {
	host, err := LoadHost(writeHost(t, validHostTOML()))
	if err != nil {
		t.Fatalf("LoadHost: %v", err)
	}
	if host.StagingQuotaBackend != "project-quota" || len(host.CredentialProfiles) != 1 {
		t.Fatalf("host = %+v", host)
	}
}
