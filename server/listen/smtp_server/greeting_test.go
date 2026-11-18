package smtp_server

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/utils/netutil"
)

func TestSMTPGreetingDoesNotDiscloseConfiguredDomain(t *testing.T) {
	oldConfig := config.Instance
	config.Instance = &config.Config{Domain: "secret.example", ListenMode: "ipv4"}
	t.Cleanup(func() { config.Instance = oldConfig })

	server := newSMTPServer("127.0.0.1:0")
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() { server.Close(); <-done })

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(line, "secret.example") {
		t.Fatalf("greeting discloses configured domain: %q", line)
	}
	if line != "220 imail ESMTP Service Ready\r\n" {
		t.Fatalf("greeting = %q, want generic greeting", line)
	}
}

// TestSMTPServerListensOnConfiguredFamily is the ingress test at SMTP level:
// the server bound in ipv4/ipv6 mode answers a client of that family and does
// not answer on the other one.
func TestSMTPServerListensOnConfiguredFamily(t *testing.T) {
	oldConfig := config.Instance
	t.Cleanup(func() { config.Instance = oldConfig })

	cases := []struct {
		mode    string
		network string
	}{
		{mode: netutil.ModeIPv4, network: "tcp4"},
		{mode: netutil.ModeIPv6, network: "tcp6"},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			if tc.mode == netutil.ModeIPv4 && !netutil.SupportsIPv4() {
				t.Skip("no IPv4 loopback on this host")
			}
			if tc.mode == netutil.ModeIPv6 && !netutil.SupportsIPv6() {
				t.Skip("no IPv6 loopback on this host")
			}

			config.Instance = &config.Config{Domain: "secret.example", ListenMode: tc.mode}
			server := newSMTPServer(netutil.Address(config.Instance.ListenMode, 0))
			listener, err := net.Listen(netutil.Network(config.Instance.ListenMode), server.Addr)
			if err != nil {
				t.Fatalf("bind in %s mode: %v", tc.mode, err)
			}
			done := make(chan error, 1)
			go func() { done <- server.Serve(listener) }()
			t.Cleanup(func() { server.Close(); <-done })

			port := listener.Addr().(*net.TCPAddr).Port
			conn, err := net.DialTimeout(tc.network, netutil.LoopbackAddress(tc.mode, port), 2*time.Second)
			if err != nil {
				t.Fatalf("%s client could not reach the %s listener: %v", tc.network, tc.mode, err)
			}
			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			line, err := bufio.NewReader(conn).ReadString('\n')
			conn.Close()
			if err != nil {
				t.Fatalf("reading greeting over %s: %v", tc.network, err)
			}
			if !strings.HasPrefix(line, "220 ") {
				t.Fatalf("greeting over %s = %q", tc.network, line)
			}

			otherNetwork, otherMode := "tcp6", netutil.ModeIPv6
			if tc.mode == netutil.ModeIPv6 {
				otherNetwork, otherMode = "tcp4", netutil.ModeIPv4
			}
			if (otherMode == netutil.ModeIPv6 && netutil.SupportsIPv6()) ||
				(otherMode == netutil.ModeIPv4 && netutil.SupportsIPv4()) {
				if conn, err := net.DialTimeout(otherNetwork, netutil.LoopbackAddress(otherMode, port), time.Second); err == nil {
					conn.Close()
					t.Fatalf("%s listener accepted a %s connection", tc.mode, otherNetwork)
				}
			}
		})
	}
}
