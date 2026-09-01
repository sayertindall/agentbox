package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type Response struct {
	OK           bool   `json:"ok"`
	Error        string `json:"error,omitempty"`
	Operation    string `json:"operation,omitempty"`
	OperationID  string `json:"operation_id,omitempty"`
	Revision     int64  `json:"revision,omitempty"`
	State        string `json:"state,omitempty"`
	StagingToken string `json:"staging_token,omitempty"`
	RunID        string `json:"run_id,omitempty"`
	AgentID      string `json:"agent_id,omitempty"`
	Generation   string `json:"generation,omitempty"`
	Detail       string `json:"detail,omitempty"`
}

func EncodeResponse(resp Response) ([]byte, error) {
	encoded, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("encode response: %w", err)
	}
	return append(encoded, '\n'), nil
}

func DecodeResponse(line []byte) (Response, error) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return Response{}, fmt.Errorf("empty response")
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return Response{}, fmt.Errorf("decode response: %w", err)
	}
	return resp, nil
}
