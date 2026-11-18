package smtp

import (
	"errors"
	"fmt"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/utils/netutil"
	"io"
	"net"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

func TestSendMailUnsafeUsesFinalDataResponseAsDeliveryResult(t *testing.T) {
	tests := []struct {
		name                 string
		dataResponse         string
		holdWithoutQuitReply bool
		wantCode             int
		maxDuration          time.Duration
	}{
		{
			name:                 "accepted message returns while server withholds QUIT reply",
			dataResponse:         "250 2.0.0 queued",
			holdWithoutQuitReply: true,
			maxDuration:          time.Second,
		},
		{
			name:         "DATA rejection remains a delivery failure",
			dataResponse: "550 5.0.0 rejected",
			wantCode:     550,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, serverDone := startSMTPTransactionServer(t, tt.dataResponse, tt.holdWithoutQuitReply)

			start := time.Now()
			err := SendMailUnsafe(
				"",
				addr,
				nil,
				"sender@example.com",
				"example.com",
				[]string{"recipient@example.net"},
				[]byte("From: sender@example.com\r\nTo: recipient@example.net\r\nSubject: test\r\n\r\nbody\r\n"),
			)
			duration := time.Since(start)
			if tt.maxDuration > 0 && duration > tt.maxDuration {
				t.Fatalf("SendMailUnsafe() took %v, want at most %v", duration, tt.maxDuration)
			}

			if tt.wantCode == 0 {
				if err != nil {
					t.Fatalf("SendMailUnsafe() = %v, want nil", err)
				}
			} else {
				var protocolErr *textproto.Error
				if !errors.As(err, &protocolErr) {
					t.Fatalf("SendMailUnsafe() error type = %T, want *textproto.Error", err)
				}
				if protocolErr.Code != tt.wantCode {
					t.Fatalf("SMTP code = %d, want %d", protocolErr.Code, tt.wantCode)
				}
			}

			select {
			case serverErr := <-serverDone:
				if serverErr != nil {
					t.Fatal(serverErr)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("fake SMTP server did not finish")
			}
		})
	}
}

func startSMTPTransactionServer(t *testing.T, dataResponse string, holdWithoutQuitReply bool) (string, <-chan error) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() failed: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		defer listener.Close()

		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

		text := textproto.NewConn(conn)
		defer text.Close()

		if err := text.PrintfLine("220 test ESMTP ready"); err != nil {
			done <- err
			return
		}
		if err := expectSMTPCommand(text, "EHLO "); err != nil {
			done <- err
			return
		}
		if err := text.PrintfLine("250 test"); err != nil {
			done <- err
			return
		}
		if err := expectSMTPCommand(text, "MAIL FROM:"); err != nil {
			done <- err
			return
		}
		if err := text.PrintfLine("250 2.1.0 sender accepted"); err != nil {
			done <- err
			return
		}
		if err := expectSMTPCommand(text, "RCPT TO:"); err != nil {
			done <- err
			return
		}
		if err := text.PrintfLine("250 2.1.5 recipient accepted"); err != nil {
			done <- err
			return
		}
		if err := expectSMTPCommand(text, "DATA"); err != nil {
			done <- err
			return
		}
		if err := text.PrintfLine("354 end data with <CR><LF>.<CR><LF>"); err != nil {
			done <- err
			return
		}
		body, err := io.ReadAll(text.DotReader())
		if err != nil {
			done <- err
			return
		}
		if !strings.Contains(string(body), "Subject: test") {
			done <- fmt.Errorf("message body was not received: %q", body)
			return
		}
		if err := text.PrintfLine("%s", dataResponse); err != nil {
			done <- err
			return
		}

		if holdWithoutQuitReply {
			if err := expectSMTPCommand(text, "QUIT"); err != nil {
				done <- err
				return
			}

			// Keep the server side open without sending 221. The client must close
			// the accepted transaction without waiting for a reply.
			buf := make([]byte, 1)
			if _, err := conn.Read(buf); err != io.EOF {
				done <- fmt.Errorf("waiting for client close after QUIT: %w", err)
				return
			}
		}
		done <- nil
	}()

	return listener.Addr().String(), done
}

func expectSMTPCommand(conn *textproto.Conn, prefix string) error {
	line, err := conn.ReadLine()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(line, prefix) {
		return fmt.Errorf("SMTP command = %q, want prefix %q", line, prefix)
	}
	return nil
}

func TestAddress(t *testing.T) {
	cases := []struct {
		host string
		port int
		want string
	}{
		{host: "mx.example.test", port: 25, want: "mx.example.test:25"},
		{host: "mx.example.test.", port: 587, want: "mx.example.test:587"},
		{host: " 2001:db8::1 ", port: 465, want: "[2001:db8::1]:465"},
	}
	for _, tc := range cases {
		if got := Address(tc.host, tc.port); got != tc.want {
			t.Fatalf("Address(%q, %d) = %q, want %q", tc.host, tc.port, got, tc.want)
		}
	}
}

// startGreetingServer accepts one connection and writes the SMTP banner, which
// is all Dial needs before handing the connection to the caller.
func startGreetingServer(t *testing.T, network string) (string, <-chan error) {
	t.Helper()
	listener, err := net.Listen(network, map[string]string{
		"tcp4": "127.0.0.1:0",
		"tcp6": "[::1]:0",
	}[network])
	if err != nil {
		t.Skipf("cannot listen on %s here: %v", network, err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	done := make(chan error, 1)
	go func() {
		defer listener.Close()
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		if _, err := conn.Write([]byte("220 test ESMTP ready\r\n")); err != nil {
			done <- err
			return
		}
		buf := make([]byte, 1)
		for {
			if _, err := conn.Read(buf); err != nil {
				done <- nil
				return
			}
		}
	}()
	return listener.Addr().String(), done
}

// TestDialForcesConfiguredEgressFamily covers requirement 2: outbound mail
// must follow egressMode, so an IPv6-only listener is reachable over IPv6 and
// not over IPv4.
func TestDialForcesConfiguredEgressFamily(t *testing.T) {
	if !netutil.SupportsIPv6() {
		t.Skip("no IPv6 loopback on this host")
	}

	oldConfig := config.Instance
	t.Cleanup(func() { config.Instance = oldConfig })

	// IPv6 egress reaches an IPv6-only listener.
	addr, done := startGreetingServer(t, "tcp6")
	config.Instance = &config.Config{EgressMode: netutil.ModeIPv6}
	client, err := Dial(addr, "example.com")
	if err != nil {
		t.Fatalf("IPv6 egress dial failed: %v", err)
	}
	_ = client.Close()
	if serverErr := <-done; serverErr != nil {
		t.Fatalf("greeting server: %v", serverErr)
	}

	// IPv4 egress must not fall back to the IPv6 address.
	addr, _ = startGreetingServer(t, "tcp6")
	config.Instance = &config.Config{EgressMode: netutil.ModeIPv4}
	if client, err := Dial(addr, "example.com"); err == nil {
		_ = client.Close()
		t.Fatal("IPv4 egress dial reached an IPv6-only listener")
	}
}

func TestDialUsesBothFamiliesByDefault(t *testing.T) {
	if !netutil.SupportsIPv4() {
		t.Skip("no IPv4 loopback on this host")
	}
	oldConfig := config.Instance
	t.Cleanup(func() { config.Instance = oldConfig })
	config.Instance = &config.Config{}

	addr, done := startGreetingServer(t, "tcp4")
	client, err := Dial(addr, "example.com")
	if err != nil {
		t.Fatalf("dual-stack dial failed: %v", err)
	}
	_ = client.Close()
	if serverErr := <-done; serverErr != nil {
		t.Fatalf("greeting server: %v", serverErr)
	}
}
