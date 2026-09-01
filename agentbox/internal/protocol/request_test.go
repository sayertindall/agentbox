package protocol

import (
	"strings"
	"testing"
)

func TestGatewayRejectsUnknownFieldAndOperation(t *testing.T) {
	if _, err := Decode([]byte(`{"version":1,"operation_id":"11111111-1111-4111-8111-111111111111","operation":"explode","project_id":"example-api"}`)); err == nil || !strings.Contains(strings.ToLower(err.Error()), "operation") {
		t.Fatalf("unknown operation error = %v", err)
	}
	if _, err := Decode([]byte(`{"version":1,"operation_id":"11111111-1111-4111-8111-111111111111","operation":"start_run","project_id":"example-api","extra":true}`)); err == nil || !strings.Contains(strings.ToLower(err.Error()), "unknown") {
		t.Fatalf("unknown field error = %v", err)
	}
}

func TestDecodeAcceptsStartRun(t *testing.T) {
	req, err := Decode([]byte(`{"version":1,"operation_id":"11111111-1111-4111-8111-111111111111","operation":"start_run","project_id":"example-api","receipt_id":"22222222-2222-4222-8222-222222222222","provider":"codex"}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if req.Operation != "start_run" || req.Provider != "codex" {
		t.Fatalf("request = %+v", req)
	}
}
