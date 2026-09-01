package gateway

import (
	"bufio"
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGatewayRejectsUnknownFieldAndOperation(t *testing.T) {
	var out bytes.Buffer
	err := Serve(strings.NewReader("{\"version\":1,\"operation_id\":\"11111111-1111-4111-8111-111111111111\",\"operation\":\"rm\",\"project_id\":\"example-api\"}\n"), &out, filepath.Join(t.TempDir(), "unused.sock"))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "operation") {
		t.Fatalf("error = %v, want unknown operation", err)
	}
	err = Serve(strings.NewReader("{\"version\":1,\"operation_id\":\"11111111-1111-4111-8111-111111111111\",\"operation\":\"start_run\",\"project_id\":\"example-api\",\"shell\":true}\n"), &out, filepath.Join(t.TempDir(), "unused.sock"))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "unknown") {
		t.Fatalf("error = %v, want unknown field", err)
	}
}

func TestGatewayForwardsOnlyToAgentboxdSocket(t *testing.T) {
	sock := filepath.Join(os.TempDir(), "abx-gw.sock")
	os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer os.Remove(sock)
	defer ln.Close()
	got := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			got <- err.Error()
			return
		}
		defer conn.Close()
		line, err := bufio.NewReader(conn).ReadBytes('\n')
		if err != nil {
			got <- err.Error()
			return
		}
		got <- string(line)
		conn.Write([]byte("ok\n"))
	}()

	raw := "{\"version\":1,\"operation_id\":\"11111111-1111-4111-8111-111111111111\",\"operation\":\"status\",\"project_id\":\"example-api\"}\n"
	var out bytes.Buffer
	if err := Serve(strings.NewReader(raw), &out, sock); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if forwarded := <-got; forwarded != raw {
		t.Fatalf("forwarded %q, want original request", forwarded)
	}
	if out.String() != "ok\n" {
		t.Fatalf("response = %q", out.String())
	}
}

func TestGatewayRejectsWorkspaceAndProfilePath(t *testing.T) {
	var out bytes.Buffer
	err := Serve(strings.NewReader("{\"version\":1,\"operation_id\":\"11111111-1111-4111-8111-111111111111\",\"operation\":\"start_run\",\"project_id\":\"example-api\",\"workspace\":\"/tmp/proj\"}\n"), &out, filepath.Join(t.TempDir(), "unused.sock"))
	if err == nil {
		t.Fatal("accepted workspace path")
	}
	err = Serve(strings.NewReader("{\"version\":1,\"operation_id\":\"11111111-1111-4111-8111-111111111111\",\"operation\":\"start_run\",\"project_id\":\"example-api\",\"provider\":\"../../credentials/codex\"}\n"), &out, filepath.Join(t.TempDir(), "unused.sock"))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "path") {
		t.Fatalf("error = %v, want profile path rejection", err)
	}
}

func TestControlAuthorizedKeyDisablesAllForwardingAndUserRC(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "deploy", "ssh", "authorized_keys.control.example"))
	if err != nil {
		t.Fatalf("read control key: %v", err)
	}
	text := string(data)
	for _, flag := range []string{"no-port-forwarding", "no-agent-forwarding", "no-X11-forwarding", "no-pty", "no-user-rc"} {
		if !strings.Contains(text, flag) {
			t.Fatalf("control authorized_keys missing %s", flag)
		}
	}
	if !strings.Contains(text, "command=\"/usr/local/libexec/devbox-gateway\"") && !strings.Contains(text, "command=\"/usr/local/bin/devbox-gateway\"") {
		t.Fatalf("control key does not force devbox-gateway: %s", text)
	}
}
