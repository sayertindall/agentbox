package paseo

import "testing"

func TestStartRecordsAgentIDBeforeReply(t *testing.T) {
	c := NewFake()
	id, err := c.Start("read DEVBOX_HANDOFF_FILE", map[string]string{"operation": "op-1", "receipt": "r-1"})
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("start returned no agent ID")
	}
	if c.LastAgentID != id {
		t.Fatal("agent ID was not recorded before reply")
	}
}

func TestAttachAndLogsDoNotCreateReplacementAgent(t *testing.T) {
	c := NewFake()
	id, err := c.Start("read DEVBOX_HANDOFF_FILE", map[string]string{"operation": "op-1"})
	if err != nil {
		t.Fatal(err)
	}
	before := len(c.Agents)
	if err := c.Attach(id); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Logs(id); err != nil {
		t.Fatal(err)
	}
	if len(c.Agents) != before {
		t.Fatal("attach or logs created a replacement agent")
	}
}

func TestRealPaseoRoundTripsOperationAndReceiptLabels(t *testing.T) {
	c := NewFake()
	if _, err := c.Start("read DEVBOX_HANDOFF_FILE", map[string]string{"operation": "op-9", "receipt": "r-9"}); err != nil {
		t.Fatal(err)
	}
	agents, err := c.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].Labels["operation"] != "op-9" || agents[0].Labels["receipt"] != "r-9" {
		t.Fatalf("agents = %+v", agents)
	}
}

func TestRealPaseoInvokesEnvironmentWrapper(t *testing.T) {
	c := NewFake()
	c.WrapperEnv = map[string]string{"DEVBOX_HANDOFF_FILE": "/meta/handoff.json"}
	if _, err := c.Start("read DEVBOX_HANDOFF_FILE", map[string]string{"operation": "op-1"}); err != nil {
		t.Fatal(err)
	}
	if !c.WrapperUsed {
		t.Fatal("start did not invoke the environment wrapper")
	}
}

func TestDiffUsesSyntheticGitForNonGitProject(t *testing.T) {
	c := NewFake()
	c.SyntheticDiff = "diff --git a/main.go b/main.go\n"
	got, err := c.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if got != c.SyntheticDiff {
		t.Fatalf("diff = %q", got)
	}
}
