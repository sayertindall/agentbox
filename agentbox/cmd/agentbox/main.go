package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"devbox/agentbox/internal/config"
	"devbox/agentbox/internal/control"
	"devbox/agentbox/internal/enrollment"
	"devbox/agentbox/internal/manifest"
	"devbox/agentbox/internal/packet"
	"devbox/agentbox/internal/protocol"
)

var errControlTransportUnavailable = errors.New("control transport unavailable")

func main() {
	if err := Run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func Run(args []string, output io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("command is required")
	}
	command := args[0]
	if !recognized(command) {
		return fmt.Errorf("unknown command %q", command)
	}
	if err := rejectWorkspacePath(args[1:]); err != nil {
		return err
	}
	options, positional, err := parseArguments(args[1:])
	if err != nil {
		return err
	}
	switch command {
	case "init":
		if len(positional) != 1 || len(options) != 0 {
			return fmt.Errorf("usage: agentbox init <project-id>")
		}
		project, err := config.Init(".", positional[0])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(output, "initialized project %s\n", project.ID)
		return err
	case "enroll":
		if len(positional) != 0 || len(options) != 0 {
			return fmt.Errorf("usage: agentbox enroll")
		}
		record, err := loadEnrollment()
		if err != nil {
			return err
		}
		project, err := config.Load(".")
		if err != nil {
			return err
		}
		ctrl, err := control.FromEnv()
		if err != nil {
			return errControlTransportUnavailable
		}
		resp, err := ctrl.Call(protocol.Request{
			Version: protocol.Version, OperationID: mustUUID(), Operation: "enroll",
			ProjectID: project.ID.String(), EnrollmentHash: record.EnrollmentHash,
		})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(output, "enrolled project %s revision %d\n", project.ID, resp.Revision)
		return err
	case "prepare":
		if len(positional) != 0 || len(options) != 0 {
			return fmt.Errorf("usage: agentbox prepare")
		}
		if err := requireEnrollment(); err != nil {
			return err
		}
		return prepare(output)
	case "run":
		if len(positional) != 0 {
			return fmt.Errorf("usage: agentbox run --receipt <receipt-id> --provider <provider>")
		}
		if options["receipt"] == "" || options["provider"] == "" {
			return fmt.Errorf("run requires receipt and provider")
		}
		return startRun(output, options["receipt"], options["provider"])
	case "status":
		return simpleOp(output, "status", options, positional)
	case "cancel":
		return simpleOp(output, "cancel", options, positional)
	case "resume":
		if len(positional) != 0 {
			return fmt.Errorf("usage: agentbox resume --receipt <receipt-id>")
		}
		if options["receipt"] == "" {
			return fmt.Errorf("resume requires receipt")
		}
		if len(options) != 1 {
			return fmt.Errorf("resume accepts only --receipt; provider is not accepted")
		}
		return simpleOp(output, "resume", options, positional)
	default:
		if len(positional) != 0 || len(options) != 0 {
			return fmt.Errorf("%s does not accept arguments", command)
		}
		return simpleOp(output, command, options, positional)
	}
}

func prepare(output io.Writer) error {
	ctrl, err := control.FromEnv()
	if err != nil {
		return errControlTransportUnavailable
	}
	project, err := config.Load(".")
	if err != nil {
		return err
	}
	root, err := filepath.Abs(".")
	if err != nil {
		return err
	}
	prep := filepath.Join(root, ".agentbox", "prepare")
	_ = os.RemoveAll(prep)
	srcDir := filepath.Join(prep, "source")
	baseDir := filepath.Join(prep, "baseline")
	if err := os.MkdirAll(srcDir, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return err
	}
	srcRoot, err := os.OpenRoot(srcDir)
	if err != nil {
		return err
	}
	defer srcRoot.Close()
	baseRoot, err := os.OpenRoot(baseDir)
	if err != nil {
		return err
	}
	defer baseRoot.Close()
	prepared, err := packet.Prepare(packet.PrepareInput{
		SourceRoot: root, Policy: manifest.Policy{}, SourceDest: srcRoot, BaselineDest: baseRoot,
		RecordDir: filepath.Join(prep, "record"), ProjectID: project.ID.String(),
		Task: "Continue the current work.", CurrentState: "Local source prepared by agentbox.",
		NextAction: "Read DEVBOX_HANDOFF_FILE and do the work.", Constraints: []string{"Do not commit"},
	})
	if err != nil {
		return err
	}
	token, err := ctrl.Call(protocol.Request{
		Version: protocol.Version, OperationID: mustUUID(), Operation: "issue_staging_token",
		ProjectID: project.ID.String(),
	})
	if err != nil {
		return err
	}
	remote := "/srv/devbox/staging/" + token.StagingToken
	if err := ctrl.Rsync(srcDir, remote+"/source"); err != nil {
		return err
	}
	if err := ctrl.Rsync(baseDir, remote+"/baseline"); err != nil {
		return err
	}
	if err := ctrl.Rsync(filepath.Join(prep, "record"), remote+"/manifests"); err != nil {
		return err
	}
	packetOnly := filepath.Join(prep, "packet-only")
	if err := os.MkdirAll(packetOnly, 0o700); err != nil {
		return err
	}
	packetBytes, err := os.ReadFile(filepath.Join(prep, "record", "packet.json"))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(packetOnly, "packet.json"), packetBytes, 0o600); err != nil {
		return err
	}
	if err := ctrl.Rsync(packetOnly, remote); err != nil {
		return err
	}
	resp, err := ctrl.Call(protocol.Request{
		Version: protocol.Version, OperationID: prepared.Handoff.PrepareOperationID, Operation: "activate",
		ProjectID: project.ID.String(), StagingToken: token.StagingToken,
		SourceManifestSHA: prepared.Source.SHA256, BaselineManifestSHA: prepared.Baseline.SHA256,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "prepared generation %s revision %d\n", resp.Generation, resp.Revision)
	return err
}

func startRun(output io.Writer, receipt, provider string) error {
	ctrl, err := control.FromEnv()
	if err != nil {
		return errControlTransportUnavailable
	}
	project, err := config.Load(".")
	if err != nil {
		return err
	}
	resp, err := ctrl.Call(protocol.Request{
		Version: protocol.Version, OperationID: mustUUID(), Operation: "start_run",
		ProjectID: project.ID.String(), ReceiptID: receipt, Provider: provider,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "run %s state %s\n", resp.RunID, resp.State)
	return err
}

func simpleOp(output io.Writer, op string, options map[string]string, positional []string) error {
	if len(positional) != 0 {
		return fmt.Errorf("%s does not accept arguments", op)
	}
	ctrl, err := control.FromEnv()
	if err != nil {
		return errControlTransportUnavailable
	}
	project, err := config.Load(".")
	if err != nil {
		return err
	}
	mapped := op
	switch op {
	case "reclaim":
		mapped = "reclaim_complete"
	case "handoff", "attach", "logs", "diff", "doctor", "recover", "resolve":
		return errControlTransportUnavailable
	}
	req := protocol.Request{Version: protocol.Version, OperationID: mustUUID(), Operation: mapped, ProjectID: project.ID.String(), ReceiptID: options["receipt"], Provider: options["provider"]}
	if mapped == "resume" || mapped == "status" || mapped == "cancel" {
		resp, err := ctrl.Call(req)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(output, "%s state %s revision %d\n", mapped, resp.State, resp.Revision)
		return err
	}
	return errControlTransportUnavailable
}

func recognized(command string) bool {
	switch command {
	case "init", "enroll", "prepare", "handoff", "run", "status", "attach", "logs", "diff", "cancel", "reclaim", "resolve", "recover", "resume", "doctor":
		return true
	default:
		return false
	}
}

func parseArguments(args []string) (map[string]string, []string, error) {
	options := make(map[string]string)
	positional := []string{}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if !strings.HasPrefix(arg, "-") {
			positional = append(positional, arg)
			continue
		}
		name := strings.TrimPrefix(arg, "--")
		if name != "receipt" && name != "provider" {
			return nil, nil, fmt.Errorf("unknown option %q", arg)
		}
		if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
			return nil, nil, fmt.Errorf("option %q requires a value", arg)
		}
		if _, exists := options[name]; exists {
			return nil, nil, fmt.Errorf("duplicate option %q", arg)
		}
		options[name] = args[index+1]
		index++
	}
	return options, positional, nil
}

func rejectWorkspacePath(args []string) error {
	for _, arg := range args {
		if arg == "--workspace" || strings.HasPrefix(arg, "--workspace=") || strings.ContainsAny(arg, `/\\`) {
			return fmt.Errorf("workspace paths are not accepted")
		}
	}
	return nil
}

func loadEnrollment() (enrollment.Record, error) {
	return enrollment.Load(filepath.Join(".agentbox", "enrollment.json"))
}

func requireEnrollment() error {
	if _, err := config.Load("."); err != nil {
		return fmt.Errorf("prepare requires enrollment: %w", err)
	}
	if _, err := loadEnrollment(); err != nil {
		return fmt.Errorf("prepare requires enrollment: %w", err)
	}
	return nil
}

func mustUUID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	bytes[6] = bytes[6]&0x0f | 0x40
	bytes[8] = bytes[8]&0x3f | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s", hex.EncodeToString(bytes[0:4]), hex.EncodeToString(bytes[4:6]), hex.EncodeToString(bytes[6:8]), hex.EncodeToString(bytes[8:10]), hex.EncodeToString(bytes[10:16]))
}
