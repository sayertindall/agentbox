package egress

import "testing"

func TestSandboxStartsWithEgressDenied(t *testing.T) {
	p := DenyAll()
	if p.Allows("api.openai.com", 443) {
		t.Fatal("deny-all policy allowed a destination")
	}
}

func TestSandboxAllowsOnlyRecordedProviderDestination(t *testing.T) {
	p := Policy{Hosts: []string{"api.openai.com"}, Ports: []int{443}}
	if !p.Allows("api.openai.com", 443) {
		t.Fatal("recorded provider destination was denied")
	}
	if p.Allows("example.com", 443) {
		t.Fatal("unrecorded destination was allowed")
	}
}

func TestSandboxDeniesPrivateMetadataHostAndSiblingRanges(t *testing.T) {
	p := Policy{Hosts: []string{"169.254.169.254", "10.0.0.1", "127.0.0.1", "100.64.0.1", "192.168.1.1"}}
	for _, host := range []string{"169.254.169.254", "10.0.0.1", "127.0.0.1", "100.64.0.1", "192.168.1.1"} {
		if p.Allows(host, 443) {
			t.Fatalf("blocked range %s was allowed", host)
		}
	}
}

func TestSystemdAdapterUnavailable(t *testing.T) {
	err := (Systemd{}).Apply(Policy{Hosts: []string{"api.openai.com"}, Ports: []int{443}})
	if err == nil {
		t.Fatal("systemd egress adapter is available without a proven netns or proxy")
	}
}
