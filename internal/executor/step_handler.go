package executor

import (
	"context"
	"fmt"
	"strings"

	"github.com/sjzsdu/tt/internal/formula"
)

type stepExecutionResult struct {
	Output            string
	HumanInputRequest *HumanInputRequest
	Status            StepStatus
}

type stepRuntime struct {
	executor *Executor
	runner   StepRunner
	local    map[string]string
}

func (r stepRuntime) Options() RunOptions { return r.executor.opts }

func (r stepRuntime) RenderTemplate(value string) string {
	return r.executor.renderTemplateWithContext(value, r.local)
}

func (r stepRuntime) BuildPrompt(step *formula.RecipeStep) string {
	return r.executor.buildPromptWithContext(step, r.local)
}

func (r stepRuntime) RunAgent(ctx context.Context, step *formula.RecipeStep, prompt string) (string, error) {
	if r.runner == nil {
		return "", fmt.Errorf("agent runner is required")
	}
	return r.runner(ctx, step, prompt)
}

type stepHandler interface {
	Kind() string
	Match(step *formula.RecipeStep) bool
	Execute(ctx context.Context, rt stepRuntime, step *formula.RecipeStep) (stepExecutionResult, error)
}

type stepRegistry struct {
	handlers []stepHandler
}

func newDefaultStepRegistry() stepRegistry {
	return stepRegistry{handlers: []stepHandler{
		rootStepHandler{},
		noopStepHandler{},
		forEachLoopStepHandler{},
		runtimeLoopStepHandler{},
		humanInputStepHandler{},
		scriptStepHandler{},
		agentStepHandler{},
	}}
}

func (r stepRegistry) Resolve(step *formula.RecipeStep) (stepHandler, error) {
	for _, handler := range r.handlers {
		if handler.Match(step) {
			return handler, nil
		}
	}
	return nil, fmt.Errorf("no step handler registered for step %s", step.ID)
}

type rootStepHandler struct{}

func (rootStepHandler) Kind() string                        { return "root" }
func (rootStepHandler) Match(step *formula.RecipeStep) bool { return step != nil && step.IsRoot }
func (rootStepHandler) Execute(context.Context, stepRuntime, *formula.RecipeStep) (stepExecutionResult, error) {
	return stepExecutionResult{Status: StatusCompleted}, nil
}

type noopStepHandler struct{}

func (noopStepHandler) Kind() string { return "noop" }
func (noopStepHandler) Match(step *formula.RecipeStep) bool {
	return step != nil && strings.EqualFold(strings.TrimSpace(step.Execution), "noop")
}
func (noopStepHandler) Execute(context.Context, stepRuntime, *formula.RecipeStep) (stepExecutionResult, error) {
	return stepExecutionResult{Status: StatusCompleted}, nil
}

type forEachLoopStepHandler struct{}

func (forEachLoopStepHandler) Kind() string { return "loop.foreach" }
func (forEachLoopStepHandler) Match(step *formula.RecipeStep) bool {
	return step != nil && step.Loop != nil && strings.TrimSpace(step.Loop.ForEach) != ""
}
func (forEachLoopStepHandler) Execute(ctx context.Context, rt stepRuntime, step *formula.RecipeStep) (stepExecutionResult, error) {
	if err := rt.executor.executeForEachLoop(ctx, rt.runner, step); err != nil {
		return stepExecutionResult{}, err
	}
	return stepExecutionResult{}, nil
}

type runtimeLoopStepHandler struct{}

func (runtimeLoopStepHandler) Kind() string { return "loop.until" }
func (runtimeLoopStepHandler) Match(step *formula.RecipeStep) bool {
	return step != nil && step.Loop != nil && strings.TrimSpace(step.Loop.Until) != ""
}
func (runtimeLoopStepHandler) Execute(ctx context.Context, rt stepRuntime, step *formula.RecipeStep) (stepExecutionResult, error) {
	if err := rt.executor.executeRuntimeLoop(ctx, rt.runner, step); err != nil {
		return stepExecutionResult{}, err
	}
	return stepExecutionResult{}, nil
}

type humanInputStepHandler struct{}

func (humanInputStepHandler) Kind() string { return HumanInputExecution }
func (humanInputStepHandler) Match(step *formula.RecipeStep) bool {
	return step != nil && strings.EqualFold(strings.TrimSpace(step.Execution), HumanInputExecution)
}
func (humanInputStepHandler) Execute(_ context.Context, _ stepRuntime, step *formula.RecipeStep) (stepExecutionResult, error) {
	request := &HumanInputRequest{Reason: strings.TrimSpace(step.Description), Form: step.Form}
	if request.Reason == "" {
		request.Reason = "step requires human input"
	}
	return stepExecutionResult{Status: StatusWaitingInput, HumanInputRequest: request}, nil
}

type scriptStepHandler struct{}

func (scriptStepHandler) Kind() string { return "script" }
func (scriptStepHandler) Match(step *formula.RecipeStep) bool {
	return step != nil && strings.EqualFold(strings.TrimSpace(step.Execution), "script")
}
func (scriptStepHandler) Execute(ctx context.Context, rt stepRuntime, step *formula.RecipeStep) (stepExecutionResult, error) {
	if !rt.executor.opts.AllowScripts {
		return stepExecutionResult{}, fmt.Errorf("step %s uses script execution; rerun with formula script execution enabled", step.ID)
	}
	output, err := rt.executor.executeScriptStepWithRender(ctx, step, rt.RenderTemplate)
	if err != nil {
		return stepExecutionResult{Status: StatusFailed, Output: output}, err
	}
	return stepExecutionResult{Status: StatusCompleted, Output: output}, nil
}

type agentStepHandler struct{}

func (agentStepHandler) Kind() string                        { return "agent" }
func (agentStepHandler) Match(step *formula.RecipeStep) bool { return step != nil }
func (agentStepHandler) Execute(ctx context.Context, rt stepRuntime, step *formula.RecipeStep) (stepExecutionResult, error) {
	prompt := rt.BuildPrompt(step)
	output, err := rt.RunAgent(ctx, step, prompt)
	if err != nil {
		return stepExecutionResult{Status: StatusFailed, Output: output}, err
	}
	request, foundHumanInput, parseHumanInputErr := ParseHumanInputRequestStrict(output)
	if parseHumanInputErr != nil {
		return stepExecutionResult{Status: StatusFailed, Output: output}, fmt.Errorf("produced invalid human input request: %w", parseHumanInputErr)
	}
	if foundHumanInput && request != nil {
		return stepExecutionResult{Status: StatusWaitingInput, Output: output, HumanInputRequest: request}, nil
	}
	return stepExecutionResult{Status: StatusCompleted, Output: output}, nil
}
