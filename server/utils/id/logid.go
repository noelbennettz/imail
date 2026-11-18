package id

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net"
	"os"
	"time"
)

var ip_instance string

func getLocalIP() string {
	if ip_instance != "" {
		return ip_instance
	}

	fallback := "127.0.0.1"
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		ip_instance = fallback
		return ip_instance
	}
	var v6 string
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() {
			continue
		}
		if ipnet.IP.To4() != nil {
			ip_instance = ipnet.IP.String()
			return ip_instance
		}
		// IPv6-only hosts have no non-loopback IPv4 address; keep a global
		// unicast IPv6 as the fallback instead of reporting loopback.
		if v6 == "" && ipnet.IP.IsGlobalUnicast() {
			v6 = ipnet.IP.String()
		}
	}
	if v6 != "" {
		ip_instance = v6
		return ip_instance
	}
	ip_instance = fallback
	return ip_instance
}

// addressPrefix returns four bytes derived from the local address. It works
// for both 4-byte and 16-byte addresses, so IPv6-only hosts keep a stable,
// non-empty log-id prefix.
func addressPrefix(address string) []byte {
	ip := net.ParseIP(address)
	if ip == nil {
		return []byte{127, 0, 0, 1}
	}
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return ip.To16()[:4]
}

func GenLogID() string {
	r := rand.New(rand.NewSource(time.Now().UnixMicro()))

	ip := getLocalIP()

	now := time.Now()
	timestamp := uint32(now.Unix())
	timeNano := now.UnixNano()
	pid := os.Getpid()
	b := bytes.Buffer{}

	b.WriteString(hex.EncodeToString(addressPrefix(ip)))
	b.WriteString(fmt.Sprintf("%x", timestamp&0xffffffff))
	b.WriteString(fmt.Sprintf("%04x", timeNano&0xffff))
	b.WriteString(fmt.Sprintf("%04x", pid&0xffff))
	b.WriteString(fmt.Sprintf("%06x", r.Int31n(1<<24)))
	b.WriteString("b0")

	return b.String()
}
