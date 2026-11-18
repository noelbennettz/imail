package pop3_server

import (
	"bufio"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/third_party/gopop"
)

func TestBuildServerUsesGenericGreeting(t *testing.T) {
	oldConfig := config.Instance
	config.Instance = &config.Config{Domain: "secret.example", Domains: []string{"secret.example"}}
	t.Cleanup(func() { config.Instance = oldConfig })

	server := buildServer(110, false)
	if server.Domain != serverGreetingName {
		t.Fatalf("POP3 greeting name = %q, want %q", server.Domain, serverGreetingName)
	}
	if strings.Contains(server.Domain, "secret.example") {
		t.Fatal("POP3 greeting name discloses configured domain")
	}
}

func TestPOP3STLSCompletesAndDisablesSTLSAdvertisement(t *testing.T) {
	certPEM := testPOP3Certificate(t)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()

	server := gopop.NewPop3Server(0, serverGreetingName, false, &tls.Config{
		Certificates: []tls.Certificate{certPEM},
	}, action{})
	server.Addr = addr
	done := make(chan error, 1)
	go func() { done <- server.Start() }()
	t.Cleanup(func() {
		server.Stop()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("POP3 server did not stop")
		}
	})

	conn := dialPOP3(t, addr)
	defer conn.Close()
	reader := bufio.NewReader(conn)

	if banner, err := reader.ReadString('\n'); err != nil || !strings.Contains(banner, serverGreetingName) {
		t.Fatalf("banner = %q, err = %v", banner, err)
	}

	if _, err := conn.Write([]byte("STLS\r\n")); err != nil {
		t.Fatal(err)
	}
	stlsReply, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if stlsReply != "+OK Begin TLS negotiation\r\n" {
		t.Fatalf("STLS reply = %q", stlsReply)
	}

	tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true}) // test certificate only
	if err := tlsConn.Handshake(); err != nil {
		t.Fatal(err)
	}
	tlsReader := bufio.NewReader(tlsConn)
	if _, err := tlsConn.Write([]byte("CAPA\r\n")); err != nil {
		t.Fatal(err)
	}
	var caps strings.Builder
	for {
		line, err := tlsReader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		caps.WriteString(line)
		if line == ".\r\n" {
			break
		}
	}
	if strings.Contains(caps.String(), "STLS") {
		t.Fatalf("STLS advertised after TLS: %q", caps.String())
	}
}

func testPOP3Certificate(t *testing.T) tls.Certificate {
	t.Helper()
	ts := httptest.NewTLSServer(http.NotFoundHandler())
	defer ts.Close()
	return ts.TLS.Certificates[0]
}

func dialPOP3(t *testing.T, addr string) net.Conn {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.SetDeadline(time.Now().Add(3 * time.Second))
			return conn
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
