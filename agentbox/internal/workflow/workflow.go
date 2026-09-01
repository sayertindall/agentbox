package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	StageHandoff = "handoff"
	StageReclaim = "reclaim"

	OutcomePending = "pending"
	OutcomeKnown   = "known"
	OutcomeUnknown = "unknown"
)

var ErrUnknownOutcome = errors.New("workflow outcome is unknown")

type Child struct {
	OperationID             string `json:"operation_id"`
	ExpectedReceiptRevision int64  `json:"expected_receipt_revision"`
}

type Record struct {
	WorkflowID string `json:"workflow_id"`
	Child      Child  `json:"child"`
	Stage      string `json:"stage"`
	Outcome    string `json:"outcome"`
	Path       string `json:"-"`
}

type Store struct {
	root     string
	syncHook func() error
}

func NewStore(root string) *Store { return &Store{root: root} }

func NewStoreWithSyncHook(root string, hook func() error) *Store {
	return &Store{root: root, syncHook: hook}
}

func (store *Store) Handoff(workflowID, operationID string, revision int64, external func() (bool, error)) error {
	if err := store.persist(Record{WorkflowID: workflowID, Child: Child{OperationID: operationID, ExpectedReceiptRevision: revision}, Stage: StageHandoff, Outcome: OutcomePending}); err != nil {
		return err
	}
	known, requestErr := external()
	if !known {
		if err := store.persist(Record{WorkflowID: workflowID, Child: Child{OperationID: operationID, ExpectedReceiptRevision: revision}, Stage: StageHandoff, Outcome: OutcomeUnknown}); err != nil {
			return err
		}
		if requestErr != nil {
			return requestErr
		}
		return ErrUnknownOutcome
	}
	if err := store.persist(Record{WorkflowID: workflowID, Child: Child{OperationID: operationID, ExpectedReceiptRevision: revision}, Stage: StageHandoff, Outcome: OutcomeKnown}); err != nil {
		return err
	}
	return requestErr
}

func (store *Store) ReconcileUnknown(workflowID string, lookup func(Child) (bool, error)) error {
	record, err := store.Load(workflowID)
	if err != nil {
		return err
	}
	if record.Outcome != OutcomePending && record.Outcome != OutcomeUnknown {
		return nil
	}
	known, lookupErr := lookup(record.Child)
	if lookupErr != nil {
		return lookupErr
	}
	if !known {
		return ErrUnknownOutcome
	}
	record.Outcome = OutcomeKnown
	return store.persist(record)
}

func (store *Store) Reclaim(workflowID, operationID string, revision int64, apply func() error) error {
	record := Record{WorkflowID: workflowID, Child: Child{OperationID: operationID, ExpectedReceiptRevision: revision}, Stage: StageReclaim, Outcome: OutcomePending}
	if err := store.persist(record); err != nil {
		return err
	}
	if err := apply(); err != nil {
		record.Outcome = OutcomeUnknown
		if persistErr := store.persist(record); persistErr != nil {
			return persistErr
		}
		return err
	}
	record.Outcome = OutcomeKnown
	return store.persist(record)
}

func (store *Store) Load(workflowID string) (Record, error) {
	if err := validateID(workflowID); err != nil {
		return Record{}, err
	}
	path := store.path(workflowID)
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, fmt.Errorf("read workflow record: %w", err)
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, fmt.Errorf("decode workflow record: %w", err)
	}
	if record.WorkflowID != workflowID || record.Child.OperationID == "" || (record.Stage != StageHandoff && record.Stage != StageReclaim) || (record.Outcome != OutcomePending && record.Outcome != OutcomeKnown && record.Outcome != OutcomeUnknown) {
		return Record{}, fmt.Errorf("malformed workflow record")
	}
	record.Path = path
	return record, nil
}

func (store *Store) persist(record Record) error {
	if err := validateID(record.WorkflowID); err != nil {
		return err
	}
	if err := validateID(record.Child.OperationID); err != nil {
		return fmt.Errorf("invalid child operation ID: %w", err)
	}
	if record.Stage != StageHandoff && record.Stage != StageReclaim {
		return fmt.Errorf("invalid workflow stage")
	}
	if record.Outcome != OutcomePending && record.Outcome != OutcomeKnown && record.Outcome != OutcomeUnknown {
		return fmt.Errorf("invalid workflow outcome")
	}
	path := store.path(record.WorkflowID)
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode workflow record: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create workflow directory: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".workflow-*.tmp")
	if err != nil {
		return fmt.Errorf("create workflow temporary file: %w", err)
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return fmt.Errorf("set workflow permissions: %w", err)
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		file.Close()
		return fmt.Errorf("write workflow record: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync workflow record: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close workflow record: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("install workflow record: %w", err)
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open workflow directory: %w", err)
	}
	if err := directory.Sync(); err != nil {
		directory.Close()
		return fmt.Errorf("sync workflow directory: %w", err)
	}
	if err := directory.Close(); err != nil {
		return fmt.Errorf("close workflow directory: %w", err)
	}
	if store.syncHook != nil {
		if err := store.syncHook(); err != nil {
			return fmt.Errorf("workflow sync hook: %w", err)
		}
	}
	return nil
}

func (store *Store) path(workflowID string) string {
	return filepath.Join(store.root, ".agentbox", "workflows", workflowID+".json")
}

func validateID(value string) error {
	if len(value) == 0 || len(value) > 64 || strings.ContainsAny(value, `/\\`) {
		return fmt.Errorf("invalid workflow ID")
	}
	for index := 0; index < len(value); index++ {
		char := value[index]
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
			return fmt.Errorf("invalid workflow ID")
		}
		if index == 0 && char == '-' {
			return fmt.Errorf("invalid workflow ID")
		}
	}
	return nil
}
