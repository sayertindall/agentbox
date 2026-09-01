package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	QuotaBackendProject = "project-quota"

	AdaptorSystemdCredentials = "systemd-credentials"
	AdaptorBindMount          = "bind-mount"
)

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
	EgressPolicies      []EgressPolicy
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
	ID                string
	AllowedHosts      []string
	AllowedPorts      []int
	DNSResolvers      []string
	PackageRegistries []string
	Adapter           string
}

func LoadHost(path string) (Host, error) {
	file, err := os.Open(path)
	if err != nil {
		return Host{}, fmt.Errorf("open host configuration: %w", err)
	}
	defer file.Close()

	host := Host{SSHPort: 22}
	section := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]") {
			section = strings.TrimSpace(line[2 : len(line)-2])
			switch section {
			case "credential_profiles":
				host.CredentialProfiles = append(host.CredentialProfiles, CredentialProfile{})
			case "egress_policies":
				host.EgressPolicies = append(host.EgressPolicies, EgressPolicy{})
			default:
				return Host{}, fmt.Errorf("unknown host configuration table %q", section)
			}
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			if section != "retention" {
				return Host{}, fmt.Errorf("unknown host configuration section %q", section)
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Host{}, fmt.Errorf("invalid host configuration line")
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if err := assignHost(&host, section, key, value); err != nil {
			return Host{}, err
		}
	}
	if err := scanner.Err(); err != nil {
		return Host{}, fmt.Errorf("read host configuration: %w", err)
	}
	if err := host.Validate(); err != nil {
		return Host{}, err
	}
	return host, nil
}

func (h Host) Validate() error {
	if h.Version != 1 {
		return fmt.Errorf("unsupported host configuration version %d", h.Version)
	}
	if h.Root == "" || !strings.HasPrefix(h.Root, "/") {
		return fmt.Errorf("host root must be an absolute path")
	}
	if err := validateTailscaleInterface(h.TailscaleInterface); err != nil {
		return err
	}
	if h.SSHPort <= 0 || h.SSHPort > 65535 {
		return fmt.Errorf("invalid SSH port")
	}
	if h.StagingMaxBytes <= 0 {
		return fmt.Errorf("staging quota is required")
	}
	if h.StagingQuotaBackend != QuotaBackendProject {
		return fmt.Errorf("unsupported staging quota backend %q", h.StagingQuotaBackend)
	}
	if h.RunMaxBytes <= 0 || h.RunMemoryMax == "" || h.RunCPUQuota == "" || h.RunTasksMax <= 0 {
		return fmt.Errorf("run quota and cgroup limits are required")
	}
	if h.RunQuotaBackend != QuotaBackendProject {
		return fmt.Errorf("unsupported run quota backend %q", h.RunQuotaBackend)
	}
	if h.MonitorInterval <= 0 {
		return fmt.Errorf("monitor interval is required")
	}
	if h.Retention.Receipts <= 0 || h.Retention.RawOutput <= 0 || h.Retention.Archives <= 0 || h.Retention.Staging <= 0 || h.Retention.DatabaseBackups <= 0 {
		return fmt.Errorf("retention values are required")
	}
	if len(h.CredentialProfiles) == 0 {
		return fmt.Errorf("credential profiles are required")
	}
	seen := map[string]bool{}
	for _, profile := range h.CredentialProfiles {
		if profile.ID == "" || seen[profile.ID] {
			return fmt.Errorf("invalid credential profile id")
		}
		seen[profile.ID] = true
		if profile.Provider == "" {
			return fmt.Errorf("credential profile %s has no provider", profile.ID)
		}
		if profile.MaxActiveRuns <= 0 {
			return fmt.Errorf("credential profile %s has no active-run capacity", profile.ID)
		}
		if profile.CredentialInjectionAdaptor != AdaptorSystemdCredentials && profile.CredentialInjectionAdaptor != AdaptorBindMount {
			return fmt.Errorf("invalid credential profile adaptor %q", profile.CredentialInjectionAdaptor)
		}
	}
	return nil
}

func validateTailscaleInterface(name string) error {
	if name == "" {
		return fmt.Errorf("invalid tailscale interface")
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			continue
		}
		return fmt.Errorf("invalid tailscale interface")
	}
	return nil
}

func assignHost(host *Host, section, key, value string) error {
	switch section {
	case "":
		return assignRoot(host, key, value)
	case "retention":
		return assignRetention(&host.Retention, key, value)
	case "credential_profiles":
		if len(host.CredentialProfiles) == 0 {
			return fmt.Errorf("credential profile key %q has no table", key)
		}
		return assignProfile(&host.CredentialProfiles[len(host.CredentialProfiles)-1], key, value)
	case "egress_policies":
		if len(host.EgressPolicies) == 0 {
			return fmt.Errorf("egress policy key %q has no table", key)
		}
		return assignEgress(&host.EgressPolicies[len(host.EgressPolicies)-1], key, value)
	default:
		return fmt.Errorf("unknown host configuration section %q", section)
	}
}

func assignRoot(host *Host, key, value string) error {
	switch key {
	case "version":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid version")
		}
		host.Version = n
	case "root":
		s, err := unquote(value)
		if err != nil {
			return fmt.Errorf("invalid root")
		}
		host.Root = s
	case "tailscale_interface":
		s, err := unquote(value)
		if err != nil {
			return fmt.Errorf("invalid tailscale interface")
		}
		host.TailscaleInterface = s
	case "ssh_port":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid SSH port")
		}
		host.SSHPort = n
	case "staging_max_bytes":
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid staging quota")
		}
		host.StagingMaxBytes = n
	case "staging_quota_backend":
		s, err := unquote(value)
		if err != nil {
			return fmt.Errorf("invalid staging quota backend")
		}
		host.StagingQuotaBackend = s
	case "run_max_bytes":
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid run quota")
		}
		host.RunMaxBytes = n
	case "run_quota_backend":
		s, err := unquote(value)
		if err != nil {
			return fmt.Errorf("invalid run quota backend")
		}
		host.RunQuotaBackend = s
	case "run_memory_max":
		s, err := unquote(value)
		if err != nil {
			return fmt.Errorf("invalid run memory limit")
		}
		host.RunMemoryMax = s
	case "run_cpu_quota":
		s, err := unquote(value)
		if err != nil {
			return fmt.Errorf("invalid run CPU quota")
		}
		host.RunCPUQuota = s
	case "run_tasks_max":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid run task limit")
		}
		host.RunTasksMax = n
	case "monitor_interval":
		d, err := parseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid monitor interval")
		}
		host.MonitorInterval = d
	default:
		return fmt.Errorf("unknown host configuration key %q", key)
	}
	return nil
}

func assignRetention(retention *Retention, key, value string) error {
	d, err := parseDuration(value)
	if err != nil {
		return fmt.Errorf("invalid retention %s", key)
	}
	switch key {
	case "receipts":
		retention.Receipts = d
	case "raw_output":
		retention.RawOutput = d
	case "archives":
		retention.Archives = d
	case "staging":
		retention.Staging = d
	case "database_backups":
		retention.DatabaseBackups = d
	default:
		return fmt.Errorf("unknown retention key %q", key)
	}
	return nil
}

func assignProfile(profile *CredentialProfile, key, value string) error {
	switch key {
	case "id":
		s, err := unquote(value)
		if err != nil {
			return fmt.Errorf("invalid credential profile id")
		}
		profile.ID = s
	case "provider":
		s, err := unquote(value)
		if err != nil {
			return fmt.Errorf("invalid credential profile provider")
		}
		profile.Provider = s
	case "max_active_runs":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid max_active_runs")
		}
		profile.MaxActiveRuns = n
	case "credential_injection_adaptor":
		s, err := unquote(value)
		if err != nil {
			return fmt.Errorf("invalid credential profile adaptor")
		}
		profile.CredentialInjectionAdaptor = s
	case "egress_policy_id", "egress_policy":
		s, err := unquote(value)
		if err != nil {
			return fmt.Errorf("invalid egress policy id")
		}
		profile.EgressPolicyID = s
	default:
		return fmt.Errorf("unknown credential profile key %q", key)
	}
	return nil
}

func assignEgress(policy *EgressPolicy, key, value string) error {
	switch key {
	case "id":
		s, err := unquote(value)
		if err != nil {
			return fmt.Errorf("invalid egress policy id")
		}
		policy.ID = s
	case "adapter":
		s, err := unquote(value)
		if err != nil {
			return fmt.Errorf("invalid egress adapter")
		}
		policy.Adapter = s
	case "allowed_hosts":
		items, err := parseArray(value)
		if err != nil {
			return fmt.Errorf("invalid allowed_hosts")
		}
		policy.AllowedHosts = items
	case "dns_resolvers":
		items, err := parseArray(value)
		if err != nil {
			return fmt.Errorf("invalid dns_resolvers")
		}
		policy.DNSResolvers = items
	case "package_registries":
		items, err := parseArray(value)
		if err != nil {
			return fmt.Errorf("invalid package_registries")
		}
		policy.PackageRegistries = items
	case "allowed_ports":
		ports, err := parseIntArray(value)
		if err != nil {
			return fmt.Errorf("invalid allowed_ports")
		}
		policy.AllowedPorts = ports
	default:
		return fmt.Errorf("unknown egress policy key %q", key)
	}
	return nil
}

func parseDuration(value string) (time.Duration, error) {
	s, err := unquote(value)
	if err != nil {
		return 0, err
	}
	return time.ParseDuration(s)
}

func unquote(value string) (string, error) {
	if strings.HasPrefix(value, "\"") {
		return strconv.Unquote(value)
	}
	return value, nil
}

func parseIntArray(value string) ([]int, error) {
	value = strings.TrimSpace(value)
	if len(value) < 2 || value[0] != '[' || value[len(value)-1] != ']' {
		return nil, fmt.Errorf("expected array")
	}
	value = strings.TrimSpace(value[1 : len(value)-1])
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}
