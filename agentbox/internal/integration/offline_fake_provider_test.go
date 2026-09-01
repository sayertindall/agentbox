package integration

import (
	"os"
	"path/filepath"
	"testing"

	"devbox/agentbox/internal/paseo"
	"devbox/agentbox/internal/reconcile"
	"devbox/agentbox/internal/store"
)

func TestOfflineFakeProviderSurvivesControlDisconnect(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "marker")
	client := paseo.NewFake()
	client.OnStart = func() {
		_ = os.WriteFile(marker, []byte("started"), 0o600)
	}
	ctrl := reconcile.New(client, nil)
	id, err := ctrl.Start("op-offline", "r-offline")
	if err != nil {
		t.Fatal(err)
	}
	// New request after the control connection is gone.
	again := reconcile.New(client, nil)
	again.Receipt.State = store.UnknownRemoteRun
	again.Receipt.OperationID = "op-offline"
	if err := again.Reconcile(true, true); err != nil {
		t.Fatal(err)
	}
	if again.Receipt.AgentID != id {
		t.Fatalf("reconciled agent = %q, want %q", again.Receipt.AgentID, id)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("fake provider did not write its marker")
	}
}
