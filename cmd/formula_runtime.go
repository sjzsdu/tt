package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

type executeFormulaRuntimeOptions struct {
	Recipe       *formula.Recipe
	RunStore     *formularun.Store
	Processor    formulaDirectProcessor
	DefaultAgent string
	DefaultModel string
	Session      string
	Workspace    string
	Debug        bool
	DryRun       bool
	AllowScripts bool
	Out          io.Writer
}

func executeFormulaRecipeRuntime(ctx context.Context, opt executeFormulaRuntimeOptions) error {
	agentRunner := formulaRuntimeAgentRunner{
		processor:    opt.Processor,
		defaultAgent: opt.DefaultAgent,
		defaultModel: opt.DefaultModel,
		session:      opt.Session,
		workspace:    opt.Workspace,
		debug:        opt.Debug,
		quiet:        true,
	}
	exec, err := newFormulaRuntimeExecutor(formulaRuntimeRunOptions{
		Recipe:       opt.Recipe,
		RunStore:     opt.RunStore,
		AgentRunner:  agentRunner,
		DryRun:       opt.DryRun,
		AllowScripts: opt.AllowScripts,
	})
	if err != nil {
		return err
	}
	if opt.Out != nil {
		fmt.Fprintf(opt.Out, "Executing formula with typed runtime: %s\n", opt.Recipe.Name)
	}
	result, err := exec.Run(ctx)
	if opt.Out != nil && result != nil {
		fmt.Fprintf(opt.Out, "Runtime status: %s\n", result.Status)
		fmt.Fprintf(opt.Out, "Runtime steps: %d\n", len(result.Nodes))
	}
	if err != nil {
		return err
	}
	return nil
}
