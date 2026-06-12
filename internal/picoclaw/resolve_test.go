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

func TestCloneConfigDeepCopy(t *testing.T) {
	original := &pcconfig.Config{
		Agents: pcconfig.AgentsConfig{
			Defaults: pcconfig.AgentDefaults{
				ModelName:      "test-model",
				ModelFallbacks: []string{"fallback1", "fallback2"},
			},
			List: []pcconfig.AgentConfig{
				{ID: "agent1", Model: &pcconfig.AgentModelConfig{Primary: "model1"}},
			},
		},
		ModelList: []*pcconfig.ModelConfig{
			{ModelName: "test", Model: "test/model"},
		},
	}

	cloned := cloneConfig(original)

	// Modify cloned to verify deep copy
	cloned.Agents.Defaults.ModelName = "modified"
	cloned.Agents.Defaults.ModelFallbacks[0] = "modified-fallback"
	cloned.Agents.List[0].Model.Primary = "modified-model"
	cloned.ModelList[0].ModelName = "modified-test"

	// Original should be unchanged
	if original.Agents.Defaults.ModelName != "test-model" {
		t.Errorf("original.Agents.Defaults.ModelName = %q, want %q", original.Agents.Defaults.ModelName, "test-model")
	}
	if original.Agents.Defaults.ModelFallbacks[0] != "fallback1" {
		t.Errorf("original.Agents.Defaults.ModelFallbacks[0] = %q, want %q", original.Agents.Defaults.ModelFallbacks[0], "fallback1")
	}
	if original.Agents.List[0].Model.Primary != "model1" {
		t.Errorf("original.Agents.List[0].Model.Primary = %q, want %q", original.Agents.List[0].Model.Primary, "model1")
	}
	if original.ModelList[0].ModelName != "test" {
		t.Errorf("original.ModelList[0].ModelName = %q, want %q", original.ModelList[0].ModelName, "test")
	}
}

func TestCloneConfigNilSafe(t *testing.T) {
	result := cloneConfig(nil)
	if result != nil {
		t.Errorf("cloneConfig(nil) = %v, want nil", result)
	}
}

func TestConfigureProjectWorkspaceDoesNotMutateOriginal(t *testing.T) {
	original := &pcconfig.Config{
		Tools: pcconfig.ToolsConfig{},
	}
	original.Tools.AllowReadPaths = []string{"/original"}

	result := configureProjectWorkspace(original, "/workspace")

	// Original should be unchanged
	if len(original.Tools.AllowReadPaths) != 1 || original.Tools.AllowReadPaths[0] != "/original" {
		t.Errorf("original.Tools.AllowReadPaths = %v, want [/original]", original.Tools.AllowReadPaths)
	}

	// Result should have new paths
	if !containsPath(result.Tools.AllowReadPaths, "/workspace") {
		t.Errorf("result.Tools.AllowReadPaths should contain /workspace, got %v", result.Tools.AllowReadPaths)
	}
}

func containsPath(paths []string, target string) bool {
	for _, p := range paths {
		if p == target {
			return true
		}
	}
	return false
}
