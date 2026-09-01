package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"devbox/agentbox/internal/id"
)

const (
	Version         = 1
	MaxRequestBytes = 64 << 10
)

type Request struct {
	Version             int    `json:"version"`
	OperationID         string `json:"operation_id"`
	Operation           string `json:"operation"`
	ProjectID           string `json:"project_id"`
	ReceiptID           string `json:"receipt_id,omitempty"`
	Provider            string `json:"provider,omitempty"`
	ExpectedRevision    *int64 `json:"expected_revision,omitempty"`
	SourceManifestSHA   string `json:"source_manifest_sha256,omitempty"`
	BaselineManifestSHA string `json:"baseline_manifest_sha256,omitempty"`
	EnrollmentHash      string `json:"enrollment_hash,omitempty"`
	StagingToken        string `json:"staging_token,omitempty"`
}

var allowed = map[string]bool{
	"enroll":              true,
	"issue_staging_token": true,
	"stage":               true,
	"activate":            true,
	"start_run":           true,
	"status":              true,
	"cancel":              true,
	"prepare_return":      true,
	"reclaim_complete":    true,
	"resolve":             true,
	"recover":             true,
	"resume":              true,
}

func Decode(line []byte) (Request, error) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return Request{}, fmt.Errorf("empty request")
	}
	if len(line) > MaxRequestBytes {
		return Request{}, fmt.Errorf("request exceeds %d bytes", MaxRequestBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	var req Request
	if err := decoder.Decode(&req); err != nil {
		if strings.Contains(err.Error(), "unknown field") {
			return Request{}, fmt.Errorf("unknown field: %w", err)
		}
		return Request{}, fmt.Errorf("decode request: %w", err)
	}
	if decoder.More() {
		return Request{}, fmt.Errorf("request contains trailing data")
	}
	if err := req.Validate(); err != nil {
		return Request{}, err
	}
	return req, nil
}

func (r Request) Validate() error {
	if r.Version != Version {
		return fmt.Errorf("unsupported request version %d", r.Version)
	}
	if !allowed[r.Operation] {
		return fmt.Errorf("unknown operation %q", r.Operation)
	}
	if r.OperationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	if _, err := id.ParseProjectID(r.ProjectID); err != nil {
		return fmt.Errorf("invalid project_id: %w", err)
	}
	if strings.Contains(r.ReceiptID, "/") || strings.Contains(r.Provider, "/") || strings.Contains(r.EnrollmentHash, "/") || strings.Contains(r.StagingToken, "/") {
		return fmt.Errorf("workspace and profile paths are not accepted")
	}
	if r.Operation == "enroll" && r.EnrollmentHash == "" {
		return fmt.Errorf("enrollment_hash is required")
	}
	if (r.Operation == "activate" || r.Operation == "stage") && r.StagingToken == "" {
		return fmt.Errorf("staging_token is required")
	}
	return nil
}
