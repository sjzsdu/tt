package cmd

import "testing"

func TestAgentOptimizeRequiresTargetAndAgent(t *testing.T) {
	agentOptimizeTarget = ""
	agentOptimizeBaseAgent = ""
	if err := runAgentOptimize(agentOptimizeCmd, nil); err == nil {
		t.Fatal("expected error when target and agent are missing")
	}
	agentOptimizeTarget = "./"
	agentOptimizeBaseAgent = ""
	if err := runAgentOptimize(agentOptimizeCmd, nil); err == nil {
		t.Fatal("expected error when agent is missing")
	}
}
