package paseo

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

type Agent struct {
	ID     string
	Labels map[string]string
}

type Client interface {
	Diagnose() error
	Start(prompt string, labels map[string]string) (string, error)
	List() ([]Agent, error)
	Attach(agentID string) error
	Logs(agentID string) ([]byte, error)
	Diff() (string, error)
	Cancel(agentID string) error
}

type Fake struct {
	Agents        []Agent
	LastAgentID   string
	WrapperEnv    map[string]string
	WrapperUsed   bool
	SyntheticDiff string
	FailStart     bool
	HangStart     bool
	OnStart       func()
}

func NewFake() *Fake { return &Fake{} }

func (f *Fake) Diagnose() error { return nil }

func (f *Fake) Start(prompt string, labels map[string]string) (string, error) {
	if f.OnStart != nil {
		f.OnStart()
	}
	if f.HangStart {
		return "", fmt.Errorf("start result unknown")
	}
	if f.FailStart {
		return "", fmt.Errorf("start failed")
	}
	if f.WrapperEnv != nil {
		f.WrapperUsed = true
	}
	var bytes [8]byte
	_, _ = rand.Read(bytes[:])
	id := "agent-" + hex.EncodeToString(bytes[:])
	copied := map[string]string{}
	for k, v := range labels {
		copied[k] = v
	}
	f.Agents = append(f.Agents, Agent{ID: id, Labels: copied})
	f.LastAgentID = id
	return id, nil
}

func (f *Fake) List() ([]Agent, error) { return append([]Agent{}, f.Agents...), nil }

func (f *Fake) Attach(agentID string) error {
	if !f.has(agentID) {
		return fmt.Errorf("unknown agent")
	}
	return nil
}

func (f *Fake) Logs(agentID string) ([]byte, error) {
	if !f.has(agentID) {
		return nil, fmt.Errorf("unknown agent")
	}
	return []byte("log\n"), nil
}

func (f *Fake) Diff() (string, error) { return f.SyntheticDiff, nil }

func (f *Fake) Cancel(agentID string) error {
	if !f.has(agentID) {
		return fmt.Errorf("unknown agent")
	}
	return nil
}

func (f *Fake) has(id string) bool {
	for _, agent := range f.Agents {
		if agent.ID == id {
			return true
		}
	}
	return false
}
