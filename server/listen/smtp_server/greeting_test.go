package smtp_server

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/noelbennettz/imail/config"
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
