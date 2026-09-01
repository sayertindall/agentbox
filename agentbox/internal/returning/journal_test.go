package returning

import (
	"os"
	"path/filepath"
	"testing"

	"devbox/agentbox/internal/store"
)

func TestCandidateVerifiesBeforeLocalMutation(t *testing.T) {
	orig := t.TempDir()
	write(t, orig, "main.go", "package main\n")
	local := t.TempDir()
	write(t, local, "main.go", "package main\n")
	ws := t.TempDir()
	write(t, ws, "main.go", "package main\nchanged\n")
	cand, err := Prepare(Input{State: store.Failed, Workspace: ws, CandidateRoot: t.TempDir(), Stop: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	j := Journal{Dir: t.TempDir(), Original: orig, Local: local, Candidate: cand, Hook: func(e string) { events = append(events, e) }}
	if err := j.Apply(); err != nil {
		t.Fatal(err)
	}
	if len(events) < 2 || events[0] != "verify" || events[1] != "journal" {
		t.Fatalf("events = %v, want verify then journal before apply", events)
	}
}

func TestConflictLeavesBothTreesUntouched(t *testing.T) {
	orig := t.TempDir()
	write(t, orig, "main.go", "package main\n")
	local := t.TempDir()
	write(t, local, "main.go", "package main\nlocal edit\n")
	ws := t.TempDir()
	write(t, ws, "main.go", "package main\nremote\n")
	cand, err := Prepare(Input{State: store.Failed, Workspace: ws, CandidateRoot: t.TempDir(), Stop: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	beforeLocal, _ := os.ReadFile(filepath.Join(local, "main.go"))
	beforeCand, _ := os.ReadFile(filepath.Join(cand.Dir, "main.go"))
	j := Journal{Dir: t.TempDir(), Original: orig, Local: local, Candidate: cand}
	if err := j.Apply(); err == nil {
		t.Fatal("conflict was not detected")
	}
	afterLocal, _ := os.ReadFile(filepath.Join(local, "main.go"))
	afterCand, _ := os.ReadFile(filepath.Join(cand.Dir, "main.go"))
	if string(beforeLocal) != string(afterLocal) || string(beforeCand) != string(afterCand) {
		t.Fatal("conflict mutated a tree")
	}
}

func TestJournalRestoresOriginalAfterApplyFailure(t *testing.T) {
	orig := t.TempDir()
	write(t, orig, "main.go", "original\n")
	local := t.TempDir()
	write(t, local, "main.go", "original\n")
	ws := t.TempDir()
	write(t, ws, "main.go", "remote\n")
	cand, err := Prepare(Input{State: store.Failed, Workspace: ws, CandidateRoot: t.TempDir(), Stop: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	j := Journal{Dir: t.TempDir(), Original: orig, Local: local, Candidate: cand, FailApply: true}
	if err := j.Apply(); err == nil {
		t.Fatal("failed apply succeeded")
	}
	got, _ := os.ReadFile(filepath.Join(local, "main.go"))
	if string(got) != "original\n" {
		t.Fatalf("local = %q, want original restored", got)
	}
}

func TestResolveReturnsLeaseAfterConfirmation(t *testing.T) {
	j := Journal{Dir: t.TempDir(), Confirmed: false}
	if _, err := j.Resolve(false); err == nil {
		t.Fatal("resolve without confirmation succeeded")
	}
	state, err := j.Resolve(true)
	if err != nil {
		t.Fatal(err)
	}
	if state != "local_owned" {
		t.Fatalf("state = %s", state)
	}
}


