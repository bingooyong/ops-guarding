package netx

import (
	"errors"
	"net"
	"testing"
)

type fakeConn struct {
	net.Conn
	local net.Addr
}

func (f fakeConn) LocalAddr() net.Addr { return f.local }
func (f fakeConn) Close() error        { return nil }

func dialOK(ip string) func(string, string) (net.Conn, error) {
	return func(network, addr string) (net.Conn, error) {
		if network != "udp4" {
			return nil, errors.New("want udp4")
		}
		return fakeConn{local: &net.UDPAddr{IP: net.ParseIP(ip), Port: 5000}}, nil
	}
}

func dialFail(string, string) (net.Conn, error) { return nil, errors.New("network is unreachable") }

func ipnet(s string) net.Addr {
	ip, n, _ := net.ParseCIDR(s)
	n.IP = ip
	return n
}

func ifaces(list ...Iface) func() ([]Iface, error) {
	return func() ([]Iface, error) { return list, nil }
}

var (
	up   = net.FlagUp
	lo   = Iface{Name: "lo", Flags: up | net.FlagLoopback, Addrs: []net.Addr{ipnet("127.0.0.1/8")}}
	dock = Iface{Name: "docker0", Flags: up, Addrs: []net.Addr{ipnet("172.17.0.1/16")}}
	down = Iface{Name: "eth9", Flags: 0, Addrs: []net.Addr{ipnet("10.9.9.9/24")}}
	ll   = Iface{Name: "eth1", Flags: up, Addrs: []net.Addr{ipnet("169.254.3.3/16"), ipnet("fe80::1/64")}}
	v6   = Iface{Name: "eth2", Flags: up, Addrs: []net.Addr{ipnet("2001:db8::5/64")}}
	eth0 = Iface{Name: "eth0", Flags: up, Addrs: []net.Addr{ipnet("fe80::2/64"), ipnet("10.0.0.5/24")}}
)

func TestLocalIP(t *testing.T) {
	cases := []struct {
		name    string
		opts    Options
		wantIP  string
		wantSrc string
		wantErr bool
	}{
		{"probe ok", Options{ProbeAddr: "1.2.3.4:80", Dial: dialOK("192.168.1.10"), Interfaces: ifaces(eth0)}, "192.168.1.10", SourceProbe, false},
		{"probe fails -> nic", Options{ProbeAddr: "1.2.3.4:80", Dial: dialFail, Interfaces: ifaces(lo, dock, down, ll, v6, eth0)}, "10.0.0.5", SourceNIC, false},
		{"probe loopback -> nic", Options{ProbeAddr: "127.0.0.1:20090", Dial: dialOK("127.0.0.1"), Interfaces: ifaces(lo, eth0)}, "10.0.0.5", SourceNIC, false},
		{"no probe -> nic", Options{Dial: dialFail, Interfaces: ifaces(eth0)}, "10.0.0.5", SourceNIC, false},
		{"interface override wins", Options{Interface: "docker0", ProbeAddr: "x:1", Dial: dialOK("192.168.1.10"), Interfaces: ifaces(eth0, dock)}, "172.17.0.1", SourceInterface, false},
		{"interface missing", Options{Interface: "bond0", Dial: dialOK("192.168.1.10"), Interfaces: ifaces(eth0)}, "", "", true},
		{"interface without ipv4", Options{Interface: "eth2", Interfaces: ifaces(v6)}, "", "", true},
		{"no candidates", Options{ProbeAddr: "1.2.3.4:80", Dial: dialFail, Interfaces: ifaces(lo, dock, down, ll, v6)}, "", "", true},
		{"interfaces error", Options{ProbeAddr: "1.2.3.4:80", Dial: dialFail, Interfaces: func() ([]Iface, error) { return nil, errors.New("boom") }}, "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ip, src, err := LocalIP(c.opts)
			if c.wantErr {
				if err == nil {
					t.Fatalf("want error, got %v %s", ip, src)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if ip.String() != c.wantIP || src != c.wantSrc {
				t.Fatalf("got %v/%s, want %s/%s", ip, src, c.wantIP, c.wantSrc)
			}
		})
	}
}

func TestLocalIPRealNetworkNoPanic(t *testing.T) {
	// Unroutable probe target: must not exit; may fall back to NIC scan or error.
	_, _, _ = LocalIP(Options{ProbeAddr: "192.0.2.1:9"})
}
