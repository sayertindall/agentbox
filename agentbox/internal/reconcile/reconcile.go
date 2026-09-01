package reconcile

import (
	"fmt"

	"devbox/agentbox/internal/paseo"
	"devbox/agentbox/internal/store"
)

type Receipt struct {
	State       store.ReceiptState
	AgentID     string
	RunID       string
	OperationID string
}

type Controller struct {
	Receipt Receipt
	paseo   paseo.Client
	commit  func(store.ReceiptState)
}

func New(client paseo.Client, commit func(store.ReceiptState)) *Controller {
	if commit == nil {
		commit = func(store.ReceiptState) {}
	}
	return &Controller{Receipt: Receipt{State: store.RemoteOwned}, paseo: client, commit: commit}
}

func (c *Controller) Start(operationID, receiptID string) (string, error) {
	if c.blocksWriters() {
		return "", fmt.Errorf("unknown operation blocks writer mutation")
	}
	c.set(store.RemoteStarting)
	id, err := c.paseo.Start("read DEVBOX_HANDOFF_FILE", map[string]string{"operation": operationID, "receipt": receiptID})
	if err != nil {
		c.set(store.UnknownRemoteRun)
		c.Receipt.OperationID = operationID
		return "", err
	}
	c.Receipt.AgentID = id
	c.Receipt.OperationID = operationID
	c.set(store.RemoteRunning)
	return id, nil
}

func (c *Controller) Reconcile(unitActive, socketOK bool) error {
	if !socketOK {
		c.set(store.UnknownRemoteRun)
		return fmt.Errorf("run socket unavailable; not proof of terminal state")
	}
	agents, err := c.paseo.List()
	if err != nil {
		c.set(store.UnknownRemoteRun)
		return err
	}
	for _, agent := range agents {
		if agent.Labels["operation"] == c.Receipt.OperationID {
			c.Receipt.AgentID = agent.ID
			if unitActive {
				c.set(store.RemoteRunning)
			}
			return nil
		}
	}
	if !unitActive {
		c.set(store.UnknownRemoteRun)
		return fmt.Errorf("no matching agent; unit inactive is not enough")
	}
	return fmt.Errorf("no agent with operation label %s", c.Receipt.OperationID)
}

func (c *Controller) Status() Receipt { return c.Receipt }

func (c *Controller) PrepareReturn() error {
	if c.blocksWriters() {
		return fmt.Errorf("unknown operation blocks return")
	}
	if c.Receipt.State != store.RemoteRunning && c.Receipt.State != store.Failed && c.Receipt.State != store.Closed {
		return fmt.Errorf("receipt state %s cannot return", c.Receipt.State)
	}
	return nil
}

func (c *Controller) Resume(operationID string) (string, error) {
	if c.Receipt.State != store.Failed && c.Receipt.State != store.Closed {
		return "", fmt.Errorf("resume requires a terminal or abandoned receipt")
	}
	c.Receipt.State = store.RemoteOwned
	return c.Start(operationID, "resume")
}

func (c *Controller) blocksWriters() bool {
	switch c.Receipt.State {
	case store.UnknownRemoteRun, store.RemoteStarting, store.ArchivedCleanupPending:
		return true
	default:
		return false
	}
}

func (c *Controller) set(state store.ReceiptState) {
	c.Receipt.State = state
	c.commit(state)
}
