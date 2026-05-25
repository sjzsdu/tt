package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sjzsdu/tt/internal/formula"
	formularuntime "github.com/sjzsdu/tt/internal/formula/runtime"
	"github.com/sjzsdu/tt/internal/formula/steps"
	"github.com/sjzsdu/tt/internal/formularun"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
)

type formulaDirectProcessor interface {
	ProcessDirect(pcwrap.RunOptions) (string, error)
}

type formulaRuntimeAgentRunner struct {
	processor    formulaDirectProcessor
	defaultAgent string
	defaultModel string
	session      string
	workspace    string
	debug        bool
	quiet        bool
}

func (r formulaRuntimeAgentRunner) RunAgent(_ context.Context, req steps.AgentRequest) (steps.Value, error) {
	if r.processor == nil {
		return steps.Value{}, fmt.Errorf("picoclaw direct runner is required")
	}
	agent := strings.TrimSpace(req.Agent)
	if agent == "" {
		agent = r.defaultAgent
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = r.defaultModel
	}
	resp, err := r.processor.ProcessDirect(pcwrap.RunOptions{
		Message:   req.Prompt,
		Session:   r.session,
		Agent:     agent,
		Model:     model,
		Workspace: r.workspace,
		Debug:     r.debug,
		Quiet:     r.quiet,
	})
	if err != nil {
		return steps.Value{}, err
	}
	data, err := json.Marshal(strings.TrimSpace(resp))
	if err != nil {
		return steps.Value{}, err
	}
	return steps.Value{Type: "json", Raw: data}, nil
}

type formulaRuntimeRunOptions struct {
	Recipe       *formula.Recipe
	RunStore     *formularun.Store
	AgentRunner  steps.AgentRunner
	DryRun       bool
	AllowScripts bool
}

func newFormulaRuntimeExecutor(opt formulaRuntimeRunOptions) (*formularuntime.Executor, error) {
	if opt.Recipe == nil {
		return nil, fmt.Errorf("recipe is required")
	}
	workflow := formula.WorkflowFromRecipe(opt.Recipe)
	capabilities := steps.Capabilities{}
	if opt.DryRun {
		capabilities.Agents = formularuntime.DryRunAgentCapability{}
		capabilities.Scripts = formularuntime.DryRunScriptCapability{}
	} else {
		capabilities.Agents = opt.AgentRunner
		if opt.AllowScripts {
			capabilities.Scripts = formularuntime.ScriptCapability{DenyUnsafe: true}
		}
	}
	exec := formularuntime.NewExecutor(workflow, capabilities)
	if opt.RunStore != nil {
		exec.Store = formularuntime.NewFormulaRunStateStore(opt.RunStore)
	}
	return exec, nil
}
