package reconcile

import (
	"strings"
	"testing"

	"devbox/agentbox/internal/paseo"
	"devbox/agentbox/internal/store"
)

func TestStartCommitsIntentBeforeExternalCall(t *testing.T) {
	var events []string
	client := paseo.NewFake()
	ctrl := New(client, func(state store.ReceiptState) { events = append(events, string(state)) })
	client.OnStart = func() {
		events = append(events, "external")
	}
	if _, err := ctrl.Start("op-1", "r-1"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(events, ",") != "remote_starting,external,remote_running" {
		t.Fatalf("events = %v, want intent before external call", events)
	}
}

func TestUnknownStartBlocksSecondRunAndReturn(t *testing.T) {
	client := paseo.NewFake()
	client.HangStart = true
	ctrl := New(client, nil)
	if _, err := ctrl.Start("op-1", "r-1"); err == nil {
		t.Fatal("unknown start succeeded")
	}
	if ctrl.Receipt.State != store.UnknownRemoteRun {
		t.Fatalf("state = %s", ctrl.Receipt.State)
	}
	if _, err := ctrl.Start("op-2", "r-1"); err == nil {
		t.Fatal("second start was allowed during unknown")
	}
	if err := ctrl.PrepareReturn(); err == nil {
		t.Fatal("return was allowed during unknown")
	}
}

func TestReconcileFindsAgentByOperationLabel(t *testing.T) {
	client := paseo.NewFake()
	_, _ = client.Start("a", map[string]string{"operation": "op-other"})
	want, _ := client.Start("b", map[string]string{"operation": "op-1"})
	ctrl := New(client, nil)
	ctrl.Receipt.State = store.UnknownRemoteRun
	ctrl.Receipt.OperationID = "op-1"
	if err := ctrl.Reconcile(true, true); err != nil {
		t.Fatal(err)
	}
	if ctrl.Receipt.AgentID != want {
		t.Fatalf("agent = %q, want %q", ctrl.Receipt.AgentID, want)
	}
}

func TestStatusUsesServerReceipt(t *testing.T) {
	ctrl := New(paseo.NewFake(), nil)
	ctrl.Receipt.State = store.RemoteRunning
	ctrl.Receipt.AgentID = "agent-server"
	st := ctrl.Status()
	if st.State != store.RemoteRunning || st.AgentID != "agent-server" {
		t.Fatalf("status = %+v, want server receipt", st)
	}
}

func TestResumeCreatesFreshSandboxFromTerminalReceipt(t *testing.T) {
	ctrl := New(paseo.NewFake(), nil)
	ctrl.Receipt.State = store.UnknownRemoteRun
	if _, err := ctrl.Resume("op-resume"); err == nil {
		t.Fatal("resume from unknown was allowed")
	}
	ctrl.Receipt.State = store.Failed
	id, err := ctrl.Resume("op-resume")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" || ctrl.Receipt.State != store.RemoteRunning {
		t.Fatalf("resume = %q state=%s", id, ctrl.Receipt.State)
	}
}

func TestUnavailableSocketDoesNotProveTerminal(t *testing.T) {
	ctrl := New(paseo.NewFake(), nil)
	ctrl.Receipt.State = store.RemoteRunning
	if err := ctrl.Reconcile(true, false); err == nil {
		t.Fatal("unavailable socket was treated as terminal")
	}
	if ctrl.Receipt.State != store.UnknownRemoteRun {
		t.Fatalf("state = %s", ctrl.Receipt.State)
	}
}
