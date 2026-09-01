package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"

	"devbox/agentbox/internal/activation"
	"devbox/agentbox/internal/config"
	"devbox/agentbox/internal/credential"
	"devbox/agentbox/internal/manifest"
	"devbox/agentbox/internal/protocol"
	"devbox/agentbox/internal/run"
	"devbox/agentbox/internal/store"
	"devbox/agentbox/internal/transfer"
)

const DefaultSocket = "/run/agentboxd/agentboxd.sock"

type Server struct {
	Host          config.Host
	DB            *store.DB
	Socket        string
	Driver        run.Driver
	Launch        func(run.Sandbox) error
	FakeProvider  string
	ControlGroup  string
}

func (s *Server) Listen() (net.Listener, error) {
	if s.Socket == "" {
		s.Socket = DefaultSocket
	}
	if s.Driver == nil {
		s.Driver = run.NewMemoryDriver()
	}
	if err := os.MkdirAll(filepath.Dir(s.Socket), 0o750); err != nil {
		return nil, err
	}
	_ = os.Remove(s.Socket)
	ln, err := net.Listen("unix", s.Socket)
	if err != nil {
		return nil, fmt.Errorf("listen agentboxd: %w", err)
	}
	if err := os.Chmod(s.Socket, 0o660); err != nil {
		ln.Close()
		return nil, err
	}
	if err := chownControl(s.Socket, s.ControlGroup); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

func (s *Server) Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	line, err := readLine(conn, protocol.MaxRequestBytes)
	if err != nil {
		writeErr(conn, err)
		return
	}
	req, err := protocol.Decode(line)
	if err != nil {
		writeErr(conn, err)
		return
	}
	resp := s.dispatch(req)
	encoded, err := protocol.EncodeResponse(resp)
	if err != nil {
		writeErr(conn, err)
		return
	}
	_, _ = conn.Write(encoded)
}

func (s *Server) dispatch(req protocol.Request) protocol.Response {
	switch req.Operation {
	case "enroll":
		return s.enroll(req)
	case "issue_staging_token":
		return s.issueToken(req)
	case "activate":
		return s.activate(req)
	case "start_run":
		return s.startRun(req)
	case "status":
		return s.status(req)
	case "cancel":
		return s.cancel(req)
	default:
		return fail(fmt.Errorf("operation %s is not implemented", req.Operation))
	}
}

func (s *Server) enroll(req protocol.Request) protocol.Response {
	if err := s.DB.Enroll(req.ProjectID, req.EnrollmentHash, "allowlist"); err != nil {
		return fail(err)
	}
	for _, profile := range s.Host.CredentialProfiles {
		if err := s.DB.PutProfile(profile.ID, profile.MaxActiveRuns); err != nil {
			return fail(err)
		}
	}
	view, err := s.DB.Receipt(req.ProjectID)
	if err != nil {
		return fail(err)
	}
	return protocol.Response{OK: true, Operation: req.Operation, OperationID: req.OperationID, Revision: view.Revision, State: string(view.State)}
}

func (s *Server) issueToken(req protocol.Request) protocol.Response {
	token, err := transfer.Issue(0, 0, s.Host.StagingMaxBytes, s.Host.Retention.Staging)
	if err != nil {
		return fail(err)
	}
	root := filepath.Join(s.Host.Root, "staging", token.ID)
	for _, dir := range []string{"source", "baseline", "manifests"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o770); err != nil {
			return fail(err)
		}
	}
	if err := chownGroupTree(root, "devbox-transfer"); err != nil {
		return fail(err)
	}
	return protocol.Response{OK: true, Operation: req.Operation, OperationID: req.OperationID, StagingToken: token.ID, State: string(store.Staging)}
}

func (s *Server) activate(req protocol.Request) protocol.Response {
	staging := filepath.Join(s.Host.Root, "staging", req.StagingToken)
	sourceManifest, err := loadManifest(filepath.Join(staging, "manifests", "source-manifest.json"))
	if err != nil {
		return fail(fmt.Errorf("source manifest: %w", err))
	}
	baselineManifest, err := loadManifest(filepath.Join(staging, "manifests", "baseline-manifest.json"))
	if err != nil {
		return fail(fmt.Errorf("baseline manifest: %w", err))
	}
	packetBytes, err := os.ReadFile(filepath.Join(staging, "packet.json"))
	if err != nil {
		return fail(fmt.Errorf("packet: %w", err))
	}
	result, err := activation.Activate(activation.Input{
		ProjectID:        req.ProjectID,
		SourceStaging:    filepath.Join(staging, "source"),
		BaselineStaging:  filepath.Join(staging, "baseline"),
		SourceManifest:   sourceManifest,
		BaselineManifest: baselineManifest,
		Packet:           packetBytes,
		GenerationsRoot:  filepath.Join(s.Host.Root, "generations"),
	})
	if err != nil {
		return fail(err)
	}
	view, err := s.DB.Receipt(req.ProjectID)
	if err != nil {
		return fail(err)
	}
	_, err = s.DB.WithProjectAndProfileWrite(context.Background(), store.WriteRequest{
		ProjectID:        req.ProjectID,
		OperationID:      req.OperationID,
		Digest:           digest(req),
		ExpectedRevision: revision(req, view.Revision),
		Type:             "activate",
	}, func(tx store.Tx) error {
		return tx.Activate(result.Generation)
	})
	if err != nil {
		_ = os.RemoveAll(filepath.Join(s.Host.Root, "generations", req.ProjectID, result.Generation))
		return fail(err)
	}
	view, err = s.DB.Receipt(req.ProjectID)
	if err != nil {
		return fail(err)
	}
	return protocol.Response{OK: true, Operation: req.Operation, OperationID: req.OperationID, Revision: view.Revision, State: string(view.State), Generation: result.Generation}
}

func (s *Server) startRun(req protocol.Request) protocol.Response {
	view, err := s.DB.Receipt(req.ProjectID)
	if err != nil {
		return fail(err)
	}
	if view.Generation == "" {
		return fail(fmt.Errorf("no activated generation"))
	}
	profile, err := s.profile(req.Provider)
	if err != nil {
		return fail(err)
	}
	_, err = s.DB.WithProjectAndProfileWrite(context.Background(), store.WriteRequest{
		ProjectID:        req.ProjectID,
		ProfileID:        profile.ID,
		OperationID:      req.OperationID,
		Digest:           digest(req),
		ExpectedRevision: revision(req, view.Revision),
		Type:             "start_run",
	}, func(tx store.Tx) error {
		return tx.LockProfile()
	})
	if err != nil {
		return fail(err)
	}
	box, err := run.Create(run.Input{
		Root:    filepath.Join(s.Host.Root, "runs"),
		Host:    s.Host,
		Profile: profile,
		Driver:  s.Driver,
	})
	if err != nil {
		return fail(err)
	}
	genRoot := filepath.Join(s.Host.Root, "generations", req.ProjectID, view.Generation)
	if err := copyTree(filepath.Join(genRoot, "source"), box.Workspace); err != nil {
		return fail(err)
	}
	if err := copyTree(filepath.Join(genRoot, "git"), filepath.Join(box.MetadataRoot, "git")); err != nil {
		return fail(err)
	}
	if err := copyFile(filepath.Join(genRoot, "packet.json"), filepath.Join(box.MetadataRoot, "handoff.json")); err != nil {
		return fail(err)
	}
	if err := chownTree(box.Root, box.UnixUser); err != nil {
		return fail(err)
	}
	if s.Launch != nil {
		if err := s.Launch(box); err != nil {
			return fail(err)
		}
	} else if err := s.launchSystemd(box); err != nil {
		return fail(err)
	}
	if err := s.DB.PutActiveRun(req.ProjectID, box.SystemdUnit); err != nil {
		return fail(err)
	}
	view, err = s.DB.Receipt(req.ProjectID)
	if err != nil {
		return fail(err)
	}
	return protocol.Response{OK: true, Operation: req.Operation, OperationID: req.OperationID, Revision: view.Revision, State: string(view.State), RunID: box.RunID, AgentID: box.UnixUser}
}

func (s *Server) status(req protocol.Request) protocol.Response {
	view, err := s.DB.Receipt(req.ProjectID)
	if err != nil {
		return fail(err)
	}
	return protocol.Response{OK: true, Operation: req.Operation, OperationID: req.OperationID, Revision: view.Revision, State: string(view.State), Generation: view.Generation}
}

func (s *Server) cancel(req protocol.Request) protocol.Response {
	units, err := s.DB.ActiveUnits()
	if err != nil {
		return fail(err)
	}
	for _, unit := range units {
		_ = s.Driver.StopCgroup(unit)
	}
	return protocol.Response{OK: true, Operation: req.Operation, OperationID: req.OperationID, State: string(store.Failed), Detail: "stop requested"}
}

func (s *Server) profile(provider string) (*credential.Profile, error) {
	if provider == "" {
		provider = "fake"
	}
	for _, declared := range s.Host.CredentialProfiles {
		if declared.Provider != provider && declared.ID != provider {
			continue
		}
		material := []byte("fake")
		path := filepath.Join(s.Host.Root, "credentials", declared.ID, "material")
		if data, err := os.ReadFile(path); err == nil {
			material = data
		}
		p, err := credential.New(credential.Profile{
			ID: declared.ID, Provider: declared.Provider, StoreDir: filepath.Join(s.Host.Root, "credentials", declared.ID),
			Adaptor: declared.CredentialInjectionAdaptor, EgressPolicyID: declared.EgressPolicyID,
			MaxActiveRuns: declared.MaxActiveRuns, Verified: true, RevocationChecked: true, EgressChecked: true,
			Material: material,
		})
		if err != nil {
			return nil, err
		}
		return &p, nil
	}
	return nil, fmt.Errorf("unknown provider %q", provider)
}

func (s *Server) launchSystemd(box run.Sandbox) error {
	execPath := s.FakeProvider
	if execPath == "" {
		execPath = "/usr/local/libexec/devbox-fake-provider"
	}
	body := fmt.Sprintf(`[Unit]
Description=DevBox run %s
[Service]
Type=simple
User=%s
WorkingDirectory=%s
NoNewPrivileges=yes
PrivateTmp=yes
ProtectHome=yes
ProtectSystem=strict
ReadWritePaths=%s
MemoryMax=%s
CPUQuota=%s
TasksMax=%d
Environment=DEVBOX_WORKSPACE=%s
Environment=DEVBOX_HANDOFF_FILE=%s
Environment=DEVBOX_METADATA=%s
ExecStart=%s
`, box.RunID, box.UnixUser, box.Workspace, box.Root, box.MemoryMax, box.CPUQuota, box.TasksMax, box.Workspace, filepath.Join(box.MetadataRoot, "handoff.json"), box.MetadataRoot, execPath)
	if err := s.Driver.WriteUnit(box.SystemdUnit, body); err != nil {
		return err
	}
	cmd := exec.Command("systemctl", "daemon-reload")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("daemon-reload: %s: %w", out, err)
	}
	cmd = exec.Command("systemctl", "start", box.SystemdUnit)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("start %s: %s: %w", box.SystemdUnit, out, err)
	}
	return nil
}

func loadManifest(path string) (manifest.Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest.Manifest{}, err
	}
	var m manifest.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return manifest.Manifest{}, err
	}
	if m.Entries == nil {
		m.Entries = []manifest.Entry{}
	}
	return m, m.Validate()
}

func digest(req protocol.Request) string {
	encoded, _ := json.Marshal(req)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func revision(req protocol.Request, current int64) int64 {
	if req.ExpectedRevision != nil {
		return *req.ExpectedRevision
	}
	return current
}

func fail(err error) protocol.Response {
	return protocol.Response{OK: false, Error: err.Error()}
}

func writeErr(w io.Writer, err error) {
	encoded, _ := protocol.EncodeResponse(fail(err))
	_, _ = w.Write(encoded)
}

func readLine(r io.Reader, max int) ([]byte, error) {
	buf := make([]byte, max+1)
	n := 0
	one := make([]byte, 1)
	for n < len(buf) {
		_, err := r.Read(one)
		if err != nil {
			if err == io.EOF && n == 0 {
				return nil, fmt.Errorf("empty request")
			}
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if one[0] == '\n' {
			break
		}
		buf[n] = one[0]
		n++
	}
	if n > max {
		return nil, fmt.Errorf("request exceeds %d bytes", max)
	}
	return buf[:n], nil
}

func copyTree(src, dest string) error {
	if err := os.MkdirAll(dest, 0o750); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		from := filepath.Join(src, entry.Name())
		to := filepath.Join(dest, entry.Name())
		if entry.IsDir() {
			if err := copyTree(from, to); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(from, to); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func chownControl(path, groupName string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	if groupName == "" {
		groupName = "devbox-control"
	}
	g, err := user.LookupGroup(groupName)
	if err != nil {
		return nil
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return err
	}
	return os.Chown(path, 0, gid)
}

func chownTree(root, unixUser string) error {
	u, err := user.Lookup(unixUser)
	if err != nil {
		return nil
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return syscall.Chown(path, uid, gid)
	})
}

func chownGroupTree(root, groupName string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	g, err := user.LookupGroup(groupName)
	if err != nil {
		return nil
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return err
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if err := os.Chown(path, 0, gid); err != nil {
			return err
		}
		return os.Chmod(path, 0o770)
	})
}

