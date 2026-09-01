package egress

import (
	"fmt"
	"net"
	"strings"
)

type Policy struct {
	Hosts []string
	Ports []int
}

func DenyAll() Policy { return Policy{} }

func (p Policy) Allows(host string, port int) bool {
	if blockedHost(host) {
		return false
	}
	if len(p.Hosts) == 0 || len(p.Ports) == 0 {
		return false
	}
	hostOK := false
	for _, allowed := range p.Hosts {
		if strings.EqualFold(allowed, host) {
			hostOK = true
			break
		}
	}
	if !hostOK {
		return false
	}
	for _, allowed := range p.Ports {
		if allowed == port {
			return true
		}
	}
	return false
}

func blockedHost(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip.To4() != nil {
		v4 := ip.To4()
		if v4[0] == 169 && v4[1] == 254 { // link-local / metadata
			return true
		}
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 { // CGNAT / Tailscale
			return true
		}
	}
	return false
}

type Adapter interface {
	Apply(Policy) error
}

type Systemd struct{}

func (Systemd) Apply(Policy) error {
	return fmt.Errorf("systemd egress adapter is unavailable until a root-owned network namespace or proxy proves deny-by-default")
}
