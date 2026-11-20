package gopop

import (
	"bufio"
	"net"
	"testing"
	"time"
)

func TestRequiresTransaction(t *testing.T) {
	for _, cmd := range []string{"STAT", "LIST", "UIDL", "TOP", "RETR", "DELE", "REST"} {
		if !requiresTransaction(cmd) {
			t.Errorf("%s should require a transaction", cmd)
		}
	}
	for _, cmd := range []string{"USER", "PASS", "CAPA", "NOOP", "QUIT", "STLS"} {
		if requiresTransaction(cmd) {
			t.Errorf("%s should not require a transaction", cmd)
		}
	}
}

func TestWritePOP3DataDotStuffsMultiline(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(2 * time.Second))

	go writePOP3Data(server, "line one\r\n.leading\r\n\r\nlast")
	reader := bufio.NewReader(client)
	want := []string{"line one\r\n", "..leading\r\n", "\r\n", "last\r\n", ".\r\n"}
	for _, line := range want {
		got, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if got != line {
			t.Fatalf("line = %q, want %q", got, line)
		}
	}
}
