package picoclaw

import (
	"testing"

	pcconfig "github.com/sipeed/picoclaw/pkg/config"
)

func TestResolveAgentConfigTreatsMainAsDefaultAlias(t *testing.T) {
	rt := &Runtime{Config: &pcconfig.Config{}}
	rt.Config.Agents.List = []pcconfig.AgentConfig{{ID: "coder", Name: "Coder", Default: true}}

	agent, err := rt.resolveAgentConfig(DefaultAgentID)
	if err != nil {
		t.Fatalf("resolve main alias returned error: %v", err)
	}
	if agent == nil || agent.ID != "coder" {
		t.Fatalf("resolved agent = %+v, want configured default coder", agent)
	}
}

func TestResolveAgentConfigAllowsMissingMainWhenNoDefaultExists(t *testing.T) {
	rt := &Runtime{Config: &pcconfig.Config{}}
	agent, err := rt.resolveAgentConfig(DefaultAgentID)
	if err != nil {
		t.Fatalf("resolve missing main without default should not fail: %v", err)
	}
	if agent != nil {
		t.Fatalf("resolved agent = %+v, want nil default", agent)
	}
}
