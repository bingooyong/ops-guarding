// Package netx detects the local IPv4 address without requiring internet access.
package netx

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// DefaultProbeAddr is only used for the UDP route probe; no packet is sent.
const DefaultProbeAddr = "8.8.8.8:80"

// Sources reported by LocalIP.
const (
	SourceInterface = "interface"
	SourceProbe     = "probe"
	SourceNIC       = "nic-scan"
)

// Iface is the subset of net.Interface used for detection (test seam).
type Iface struct {
	Name  string
	Flags net.Flags
	Addrs []net.Addr
}

// Options controls LocalIP. Zero values use the real network.
type Options struct {
	// Interface, if set, must name a NIC; its first usable IPv4 is returned.
	Interface string
	// ProbeAddr is the host:port used for the UDP route probe ("" skips the probe).
	ProbeAddr string

	Dial       func(network, addr string) (net.Conn, error)
	Interfaces func() ([]Iface, error)
}

// virtualPrefixes are NIC name prefixes skipped by the NIC scan.
var virtualPrefixes = []string{"lo", "docker", "veth", "br-", "cni", "flannel", "cali", "virbr"}

// LocalIP returns the local IPv4 address and which source produced it.
// Order: Interface -> UDP probe to ProbeAddr -> first up, non-loopback,
// non-link-local IPv4 on a non-virtual NIC. It never exits the process.
func LocalIP(o Options) (net.IP, string, error) {
	if o.Dial == nil {
		o.Dial = func(network, addr string) (net.Conn, error) {
			return net.DialTimeout(network, addr, 2*time.Second)
		}
	}
	if o.Interfaces == nil {
		o.Interfaces = systemInterfaces
	}

	ifaces, ifErr := o.Interfaces()

	if o.Interface != "" {
		if ifErr != nil {
			return nil, "", fmt.Errorf("list interfaces: %w", ifErr)
		}
		for _, it := range ifaces {
			if it.Name == o.Interface {
				if ip := firstUsableIPv4(it); ip != nil {
					return ip, SourceInterface, nil
				}
				return nil, "", fmt.Errorf("interface %s has no usable IPv4 address", o.Interface)
			}
		}
		return nil, "", fmt.Errorf("interface %s not found", o.Interface)
	}

	var probeErr error
	if o.ProbeAddr != "" {
		var ip net.IP
		ip, probeErr = probe(o.Dial, o.ProbeAddr)
		if probeErr == nil {
			return ip, SourceProbe, nil
		}
	}

	if ifErr == nil {
		for _, it := range ifaces {
			if isVirtual(it.Name) {
				continue
			}
			if ip := firstUsableIPv4(it); ip != nil {
				return ip, SourceNIC, nil
			}
		}
	}

	return nil, "", fmt.Errorf("no usable local IPv4 found (probe %q: %v; interfaces: %v)", o.ProbeAddr, probeErr, ifErr)
}

func probe(dial func(string, string) (net.Conn, error), addr string) (net.IP, error) {
	conn, err := dial("udp4", addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	ua, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || ua.IP == nil {
		return nil, errors.New("probe returned no UDP local address")
	}
	ip := ua.IP.To4()
	if !usable(ip) {
		return nil, fmt.Errorf("probe returned unusable address %v", ua.IP)
	}
	return ip, nil
}

func firstUsableIPv4(it Iface) net.IP {
	if it.Flags&net.FlagUp == 0 || it.Flags&net.FlagLoopback != 0 {
		return nil
	}
	for _, a := range it.Addrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip4 := ip.To4(); usable(ip4) {
			return ip4
		}
	}
	return nil
}

func usable(ip net.IP) bool {
	return ip != nil && ip.To4() != nil && !ip.IsLoopback() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast()
}

func isVirtual(name string) bool {
	for _, p := range virtualPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func systemInterfaces() ([]Iface, error) {
	list, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	ret := make([]Iface, 0, len(list))
	for _, it := range list {
		addrs, err := it.Addrs()
		if err != nil {
			continue
		}
		ret = append(ret, Iface{Name: it.Name, Flags: it.Flags, Addrs: addrs})
	}
	return ret, nil
}
