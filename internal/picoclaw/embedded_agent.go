package picoclaw

import (
	"context"
	"fmt"
	"strings"

	pcagent "github.com/sipeed/picoclaw/pkg/agent"
	pcconfig "github.com/sipeed/picoclaw/pkg/config"
)

const embeddedPromptSourceID pcagent.PromptSourceID = "tt.embedded.agent"

type embeddedAgentPromptContributor struct {
	name    string
	content string
}

func (c embeddedAgentPromptContributor) PromptSource() pcagent.PromptSourceDescriptor {
	return pcagent.PromptSourceDescriptor{
		ID:              embeddedPromptSourceID,
		Owner:           "tt",
		Description:     "Embedded tt agent instructions",
		Allowed:         []pcagent.PromptPlacement{{Layer: pcagent.PromptLayerInstruction, Slot: pcagent.PromptSlotWorkspace}},
		StableByDefault: true,
	}
}

func (c embeddedAgentPromptContributor) ContributePrompt(context.Context, pcagent.PromptBuildRequest) ([]pcagent.PromptPart, error) {
	content := strings.TrimSpace(c.content)
	if content == "" {
		return nil, nil
	}
	return []pcagent.PromptPart{{
		ID:      "tt.embedded.agent",
		Layer:   pcagent.PromptLayerInstruction,
		Slot:    pcagent.PromptSlotWorkspace,
		Source:  pcagent.PromptSource{ID: embeddedPromptSourceID, Name: c.name},
		Title:   c.name,
		Content: content,
		Stable:  true,
	}}, nil
}

func applyEmbeddedAgentConfig(cfg *pcconfig.Config, agent *EmbeddedAgent, model string) error {
	if cfg == nil || agent == nil {
		return nil
	}
	id := strings.TrimSpace(agent.ID)
	if id == "" {
		return fmt.Errorf("embedded agent id required")
	}
	workspace := strings.TrimSpace(cfg.Agents.Defaults.Workspace)
	if workspace == "" {
		workspace = Workspace(cfg)
	}
	name := strings.TrimSpace(agent.Name)
	if name == "" {
		name = id
	}
	var agentModel *pcconfig.AgentModelConfig
	if primaryModel := strings.TrimSpace(model); primaryModel != "" {
		agentModel = &pcconfig.AgentModelConfig{Primary: primaryModel}
	}
	removeAgentByID(cfg, id)
	cfg.Agents.List = append(cfg.Agents.List, pcconfig.AgentConfig{
		ID:        id,
		Name:      name,
		Workspace: workspace,
		Model:     agentModel,
		Skills:    append([]string(nil), agent.Skills...),
		NoHistory: agent.NoHistory,
	})
	if agent.EnableResearchTools {
		enableResearchTools(cfg)
	}
	return nil
}

func enableResearchTools(cfg *pcconfig.Config) {
	if cfg == nil {
		return
	}
	cfg.Tools.Skills.Enabled = true
	cfg.Tools.FindSkills.Enabled = true
	cfg.Tools.Web.Enabled = true
	cfg.Tools.WebFetch.Enabled = true
	cfg.Tools.Exec.Enabled = true
}

func applyEmbeddedAgentConfigs(cfg *pcconfig.Config, agents []EmbeddedAgent, model string) error {
	for i := range agents {
		if err := applyEmbeddedAgentConfig(cfg, &agents[i], model); err != nil {
			return err
		}
	}
	return nil
}

func registerEmbeddedAgentPrompt(loop *pcagent.AgentLoop, agent *EmbeddedAgent) error {
	if loop == nil || agent == nil {
		return nil
	}
	id := strings.TrimSpace(agent.ID)
	if id == "" {
		return fmt.Errorf("embedded agent id required")
	}
	instance, ok := loop.GetRegistry().GetAgent(id)
	if !ok || instance == nil || instance.ContextBuilder == nil {
		return fmt.Errorf("embedded agent %q not registered", id)
	}
	content := strings.TrimSpace(agent.Prompt)
	if soul := strings.TrimSpace(agent.Soul); soul != "" {
		content = strings.TrimSpace(content + "\n\n## SOUL.md\n\n" + soul)
	}
	return instance.ContextBuilder.RegisterPromptContributor(embeddedAgentPromptContributor{
		name:    strings.TrimSpace(agent.Name),
		content: content,
	})
}

func registerEmbeddedAgentPrompts(loop *pcagent.AgentLoop, agents []EmbeddedAgent) error {
	for i := range agents {
		if err := registerEmbeddedAgentPrompt(loop, &agents[i]); err != nil {
			return err
		}
	}
	return nil
}

func removeAgentByID(cfg *pcconfig.Config, id string) {
	if cfg == nil {
		return
	}
	out := cfg.Agents.List[:0]
	for _, item := range cfg.Agents.List {
		if !strings.EqualFold(strings.TrimSpace(item.ID), id) {
			out = append(out, item)
		}
	}
	cfg.Agents.List = out
}
