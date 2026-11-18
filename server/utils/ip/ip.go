// Package ip discovers the public addresses of this host for DNS guidance.
package ip

import (
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	ipv4Once sync.Once
	ipv6Once sync.Once
	ipv4     string
	ipv6     string
)

var client = &http.Client{Timeout: 5 * time.Second}

// GetIPv4 returns the public IPv4 address or "" when none is available.
func GetIPv4() string {
	ipv4Once.Do(func() {
		ipv4 = detect("https://api.ipify.org", "https://ipv4.icanhazip.com")
		if parsed := net.ParseIP(ipv4); parsed == nil || parsed.To4() == nil {
			ipv4 = ""
		}
	})
	return ipv4
}

// GetIPv6 returns the public IPv6 address or "" when the host has no IPv6
// connectivity.
func GetIPv6() string {
	ipv6Once.Do(func() {
		ipv6 = detect("https://api6.ipify.org", "https://ipv6.icanhazip.com")
		parsed := net.ParseIP(ipv6)
		if parsed == nil || parsed.To4() != nil {
			ipv6 = ""
		}
	})
	return ipv6
}

// GetIp is kept for callers that only need a single address. It prefers
// IPv4 and falls back to IPv6.
func GetIp() string {
	if v4 := GetIPv4(); v4 != "" {
		return v4
	}
	return GetIPv6()
}

func detect(urls ...string) string {
	for _, url := range urls {
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 128))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || readErr != nil {
			continue
		}
		value := strings.TrimSpace(string(body))
		if net.ParseIP(value) != nil {
			return value
		}
	}
	return ""
}
