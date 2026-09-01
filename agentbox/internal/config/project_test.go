package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"devbox/agentbox/internal/enrollment"
)

func TestInitWritesProjectAndEnrollmentRecord(t *testing.T) {
	root := t.TempDir()
	project, err := Init(root, "example-api")
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if project.ID.String() != "example-api" {
		t.Fatalf("project ID = %q, want %q", project.ID, "example-api")
	}
	projectPath := filepath.Join(root, ".agentbox", "project.toml")
	enrollmentPath := filepath.Join(root, ".agentbox", "enrollment.json")
	if _, err := os.Stat(projectPath); err != nil {
		t.Fatalf("project record missing: %v", err)
	}
	record, err := enrollment.Load(enrollmentPath)
	if err != nil {
		t.Fatalf("enrollment.Load() error = %v", err)
	}
	if record.EnrollmentID == "" {
		t.Fatal("enrollment record has empty ID")
	}
	loaded, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.ID != project.ID || loaded.LocalRoot != root {
		t.Fatalf("Load() = %#v, want %#v", loaded, project)
	}
}

func TestInitPreservesExistingEnrollment(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root, "example-api"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".agentbox", "enrollment.json")
	before, err := enrollment.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Init(root, "example-api"); err != nil {
		t.Fatal(err)
	}
	after, err := enrollment.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("repeat Init() changed enrollment record from %#v to %#v", before, after)
	}
}

func TestInitRejectsMalformedExistingEnrollmentWithoutOverwrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agentbox", "enrollment.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"enrollment_id":"broken"}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(root, "example-api"); err == nil {
		t.Fatal("Init() accepted malformed existing enrollment")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("Init() overwrote malformed enrollment with %q", got)
	}
}

func TestEnrollRequiresControlTransport(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root, "example-api"); err != nil {
		t.Fatal(err)
	}
	result := runAgentbox(t, root, "enroll")
	if result.exitCode == 0 || !strings.Contains(result.output, "control transport") {
		t.Fatalf("enroll result = %#v, want control transport error", result)
	}
}

func TestPrepareRequiresEnrollment(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root, "example-api"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, ".agentbox", "enrollment.json")); err != nil {
		t.Fatal(err)
	}
	result := runAgentbox(t, root, "prepare")
	if result.exitCode == 0 || !strings.Contains(result.output, "enrollment") {
		t.Fatalf("prepare result = %#v, want enrollment error", result)
	}
}

func TestPrepareRejectsPacketStageFlagsAsUnimplemented(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root, "example-api"); err != nil {
		t.Fatal(err)
	}
	result := runAgentbox(t, root, "prepare", "--task", "implement feature")
	if result.exitCode == 0 || !strings.Contains(result.output, "unknown option") {
		t.Fatalf("prepare result = %#v, want unimplemented packet-stage flag rejection", result)
	}
}

func TestRunRequiresReceiptAndProvider(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root, "example-api"); err != nil {
		t.Fatal(err)
	}
	result := runAgentbox(t, root, "run")
	if result.exitCode == 0 || !strings.Contains(result.output, "receipt") || !strings.Contains(result.output, "provider") {
		t.Fatalf("run result = %#v, want receipt and provider error", result)
	}
}

func TestResumeRequiresReceipt(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root, "example-api"); err != nil {
		t.Fatal(err)
	}
	result := runAgentbox(t, root, "resume")
	if result.exitCode == 0 || !strings.Contains(result.output, "receipt") {
		t.Fatalf("resume result = %#v, want receipt error", result)
	}
}

func TestCommandNeverAcceptsWorkspacePath(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root, "example-api"); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"status", "--workspace", "/tmp/other"}, {"status", "/tmp/other"}} {
		result := runAgentbox(t, root, args...)
		if result.exitCode == 0 || !strings.Contains(result.output, "workspace") {
			t.Errorf("args %v result = %#v, want workspace path rejection", args, result)
		}
	}
}

type commandResult struct {
	output   string
	exitCode int
}

var agentboxBinary struct {
	sync.Once
	path string
	err  error
}

func runAgentbox(t *testing.T, root string, args ...string) commandResult {
	t.Helper()
	agentboxBinary.Do(func() {
		_, source, _, _ := runtime.Caller(0)
		moduleRoot := filepath.Dir(filepath.Dir(filepath.Dir(source)))
		file, err := os.CreateTemp("", "agentbox-test-*")
		if err != nil {
			agentboxBinary.err = err
			return
		}

		agentboxBinary.path = file.Name()
		if err := file.Close(); err != nil {
			agentboxBinary.err = err
			return
		}
		build := exec.Command("go", "-C", moduleRoot, "build", "-o", agentboxBinary.path, "./cmd/agentbox")
		if output, err := build.CombinedOutput(); err != nil {
			agentboxBinary.err = fmt.Errorf("build agentbox: %w: %s", err, output)
		}
	})
	if agentboxBinary.err != nil {
		t.Fatal(agentboxBinary.err)
	}
	command := exec.Command(agentboxBinary.path, args...)
	command.Dir = root
	command.Env = append(os.Environ(), "HOME="+t.TempDir(), "DEVBOX_CONTROL=", "DEVBOX_TRANSFER=", "DEVBOX_CONTROL_KEY=", "DEVBOX_TRANSFER_KEY=", "DEVBOX_SSH_PROXY=")
	output, err := command.CombinedOutput()
	result := commandResult{output: string(output)}
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			result.exitCode = exitError.ExitCode()
		} else {
			t.Fatalf("run agentbox: %v", err)
		}
	}
	return result
}
func TestResumeAcceptsExactlyReceipt(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root, "example-api"); err != nil {
		t.Fatal(err)
	}
	withProvider := runAgentbox(t, root, "resume", "--receipt", "receipt-1", "--provider", "codex")
	if withProvider.exitCode == 0 || !strings.Contains(withProvider.output, "provider") {
		t.Fatalf("resume with provider result = %#v, want provider rejection", withProvider)
	}
	receiptOnly := runAgentbox(t, root, "resume", "--receipt", "receipt-1")
	if receiptOnly.exitCode == 0 || !strings.Contains(receiptOnly.output, "control transport") {
		t.Fatalf("receipt-only resume result = %#v, want control transport boundary", receiptOnly)
	}
}
