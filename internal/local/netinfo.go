package local

import (
	"fmt"
	"net"
	"strings"
)

// LANIP picks the IPv4 address other devices on the network can reach.
// Docker, VPN, VM and link-local interfaces are skipped; force wins.
func LANIP(force string) (string, error) {
	if force != "" {
		if net.ParseIP(force) == nil {
			return "", fmt.Errorf("--ip %q is not a valid IP address", force)
		}
		return force, nil
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	var fallback string
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 || skipInterface(ifc.Name) {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipn.IP.To4()
			if ip4 == nil || ip4.IsLinkLocalUnicast() || ip4.IsLoopback() {
				continue
			}
			if ip4.IsPrivate() {
				return ip4.String(), nil
			}
			if fallback == "" {
				fallback = ip4.String()
			}
		}
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("no usable network interface found; use --ip to choose one")
}

// skipInterface filters the usual virtual interfaces by name.
func skipInterface(name string) bool {
	for _, p := range []string{"docker", "br-", "veth", "utun", "tun", "tap", "wg", "vboxnet", "vmnet", "virbr", "zt", "tailscale", "awdl", "llw", "bridge", "anpi"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// FreePort returns preferred if it is free, otherwise the next free port
// after it (up to 50 tries), otherwise a random free port.
func FreePort(host string, preferred int) (int, error) {
	for p := preferred; preferred > 0 && p < preferred+50; p++ {
		l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, p))
		if err == nil {
			_ = l.Close()
			return p, nil
		}
	}
	l, err := net.Listen("tcp", host+":0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}
