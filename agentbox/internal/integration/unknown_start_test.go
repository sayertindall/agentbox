package integration

import (
	"testing"

	"devbox/agentbox/internal/paseo"
	"devbox/agentbox/internal/reconcile"
	"devbox/agentbox/internal/store"
)

func TestUnknownStartBlocksSecondRunAndReturn(t *testing.T) {
	client := paseo.NewFake()
	client.HangStart = true
	ctrl := reconcile.New(client, nil)
	if _, err := ctrl.Start("op-1", "r-1"); err == nil {
		t.Fatal("unknown start succeeded")
	}
	if _, err := ctrl.Start("op-2", "r-1"); err == nil {
		t.Fatal("second run started while unknown")
	}
	if err := ctrl.PrepareReturn(); err == nil {
		t.Fatal("return started while unknown")
	}
	if ctrl.Status().State != store.UnknownRemoteRun {
		t.Fatalf("state = %s", ctrl.Status().State)
	}
}
