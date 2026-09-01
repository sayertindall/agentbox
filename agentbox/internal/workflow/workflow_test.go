package workflow

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHandoffWorkflowFsyncsBeforeExternalRequest(t *testing.T) {
	events := []string{}
	store := NewStoreWithSyncHook(t.TempDir(), func() error {
		events = append(events, "fsync")
		return nil
	})
	if err := store.Handoff("handoff-1", "child-1", 7, func() (bool, error) {
		events = append(events, "external")
		return true, nil
	}); err != nil {
		t.Fatalf("Handoff() error = %v", err)
	}
	if !reflect.DeepEqual(events, []string{"fsync", "external", "fsync"}) {
		t.Fatalf("events = %v, want fsync before request and after response", events)
	}
}

func TestHandoffWorkflowReconcilesUnknownChildOperation(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Handoff("handoff-1", "child-1", 7, func() (bool, error) { return false, nil }); err == nil {
		t.Fatal("Handoff() succeeded for unknown child outcome")
	}
	before, err := store.Load("handoff-1")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if before.Outcome != OutcomeUnknown {
		t.Fatalf("outcome = %q, want %q", before.Outcome, OutcomeUnknown)
	}
	if err := store.ReconcileUnknown("handoff-1", func(child Child) (bool, error) {
		if child.OperationID != "child-1" || child.ExpectedReceiptRevision != 7 {
			t.Fatalf("child = %#v", child)
		}
		return true, nil
	}); err != nil {
		t.Fatalf("ReconcileUnknown() error = %v", err)
	}
	after, err := store.Load("handoff-1")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if after.Outcome != OutcomeKnown {
		t.Fatalf("outcome = %q, want %q", after.Outcome, OutcomeKnown)
	}
}

func TestReclaimWorkflowFsyncsBeforeLocalApply(t *testing.T) {
	events := []string{}
	store := NewStoreWithSyncHook(t.TempDir(), func() error {
		events = append(events, "fsync")
		return nil
	})
	if err := store.Reclaim("reclaim-1", "child-1", 9, func() error {
		events = append(events, "apply")
		return nil
	}); err != nil {
		t.Fatalf("Reclaim() error = %v", err)
	}
	if got, want := events, []string{"fsync", "apply", "fsync"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	record, err := store.Load("reclaim-1")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if record.Stage != StageReclaim || record.WorkflowID != "reclaim-1" || filepath.Base(record.Path) != "reclaim-1.json" {
		t.Fatalf("record = %#v", record)
	}
}

func TestHandoffWorkflowReconcilesPendingChildOperation(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.persist(Record{
		WorkflowID: "handoff-1",
		Child:      Child{OperationID: "child-1", ExpectedReceiptRevision: 7},
		Stage:      StageHandoff,
		Outcome:    OutcomePending,
	}); err != nil {
		t.Fatalf("persist() error = %v", err)
	}
	called := false
	if err := store.ReconcileUnknown("handoff-1", func(child Child) (bool, error) {
		called = true
		return child.OperationID == "child-1" && child.ExpectedReceiptRevision == 7, nil
	}); err != nil {
		t.Fatalf("ReconcileUnknown() error = %v", err)
	}
	if !called {
		t.Fatal("ReconcileUnknown() did not reconcile pending child operation")
	}
	record, err := store.Load("handoff-1")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if record.Outcome != OutcomeKnown {
		t.Fatalf("outcome = %q, want %q", record.Outcome, OutcomeKnown)
	}
}

func TestHandoffWorkflowRecordsKnownOutcomeWithError(t *testing.T) {
	store := NewStore(t.TempDir())
	callbackErr := errors.New("response body was malformed")
	err := store.Handoff("handoff-1", "child-1", 7, func() (bool, error) {
		return true, callbackErr
	})
	if !errors.Is(err, callbackErr) {
		t.Fatalf("Handoff() error = %v, want %v", err, callbackErr)
	}
	record, err := store.Load("handoff-1")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if record.Outcome != OutcomeKnown {
		t.Fatalf("outcome = %q, want %q", record.Outcome, OutcomeKnown)
	}
}
