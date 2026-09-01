package daemon

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"devbox/agentbox/internal/config"
	"devbox/agentbox/internal/manifest"
	"devbox/agentbox/internal/packet"
	"devbox/agentbox/internal/protocol"
	"devbox/agentbox/internal/run"
	"devbox/agentbox/internal/store"
)

func TestDaemonEnrollActivateAndFakeRun(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	host := config.Host{
		Root: root, StagingMaxBytes: 1 << 20, Retention: config.Retention{Staging: time.Hour},
		RunMaxBytes: 1 << 20, RunMemoryMax: "1G", RunCPUQuota: "50%", RunTasksMax: 32,
		CredentialProfiles: []config.CredentialProfile{{
			ID: "fake", Provider: "fake", MaxActiveRuns: 1, CredentialInjectionAdaptor: "systemd-credentials",
		}},
	}
	sock := filepath.Join(os.TempDir(), "abx-d.sock")
	_ = os.Remove(sock)
	t.Cleanup(func() { os.Remove(sock) })
	srv := &Server{
		Host: host, DB: db, Socket: sock, Driver: run.NewMemoryDriver(),
		Launch: func(box run.Sandbox) error {
			return os.WriteFile(filepath.Join(box.MetadataRoot, "fake-marker"), []byte("started"), 0o600)
		},
	}
	ln, err := srv.Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go srv.Serve(ln)

	enroll := call(t, sock, protocol.Request{Version: 1, OperationID: "11111111-1111-4111-8111-111111111111", Operation: "enroll", ProjectID: "example-api", EnrollmentHash: "abc"})
	if !enroll.OK {
		t.Fatalf("enroll: %+v", enroll)
	}
	token := call(t, sock, protocol.Request{Version: 1, OperationID: "22222222-2222-4222-8222-222222222222", Operation: "issue_staging_token", ProjectID: "example-api"})
	if token.StagingToken == "" {
		t.Fatalf("token: %+v", token)
	}
	staging := filepath.Join(root, "staging", token.StagingToken)
	if err := os.WriteFile(filepath.Join(staging, "source", "hello.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "baseline", "hello.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceManifest, err := manifest.Build(filepath.Join(staging, "source"), manifest.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	baselineManifest, err := manifest.Build(filepath.Join(staging, "baseline"), manifest.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	sourceBytes, err := manifest.Encode(sourceManifest)
	if err != nil {
		t.Fatal(err)
	}
	baselineBytes, err := manifest.Encode(baselineManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "manifests", "source-manifest.json"), sourceBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "manifests", "baseline-manifest.json"), baselineBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	handoff, err := packet.Create(packet.CreateInput{
		ProjectID: "example-api", SourceManifestSHA256: sourceManifest.SHA256, BaselineManifestSHA256: &baselineManifest.SHA256,
		Task: "do the thing", CurrentState: "source is ready", NextAction: "edit hello.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	packetBytes, err := packet.Encode(handoff)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "packet.json"), packetBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	activated := call(t, sock, protocol.Request{Version: 1, OperationID: "33333333-3333-4333-8333-333333333333", Operation: "activate", ProjectID: "example-api", StagingToken: token.StagingToken})
	if !activated.OK {
		t.Fatalf("activate: %+v", activated)
	}
	started := call(t, sock, protocol.Request{Version: 1, OperationID: "44444444-4444-4444-8444-444444444444", Operation: "start_run", ProjectID: "example-api", Provider: "fake"})
	if !started.OK || started.RunID == "" {
		t.Fatalf("start_run: %+v", started)
	}
	if _, err := os.Stat(filepath.Join(root, "runs", started.RunID, "metadata", "fake-marker")); err != nil {
		t.Fatalf("marker missing: %v", err)
	}
}

func call(t *testing.T, sock string, req protocol.Request) protocol.Response {
	t.Helper()
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write(append(raw, '\n')); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := protocol.DecodeResponse(buf[:n])
	if err != nil {
		t.Fatal(err)
	}
	return resp
}
