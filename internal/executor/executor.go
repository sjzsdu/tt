package executor

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/sjzsdu/tt/internal/formula"
)

const defaultAgentID = "main"

type StepRunner func(ctx context.Context, step *formula.RecipeStep, prompt string) (string, error)

type RunOptions struct {
	Vars           map[string]string
	InitialContext map[string]string
	InitialResults []StepResult
	Agent          string
	Model          string
	Session        string
	DryRun         bool
	Debug          bool
}

type StepStatus string

const (
	StatusPending   StepStatus = "pending"
	StatusRunning   StepStatus = "running"
	StatusCompleted StepStatus = "completed"
	StatusFailed    StepStatus = "failed"
	StatusSkipped   StepStatus = "skipped"
)

type StepResult struct {
	StepID string     `json:"step_id"`
	Title  string     `json:"title"`
	Status StepStatus `json:"status"`
	Output string     `json:"output,omitempty"`
	Error  string     `json:"error,omitempty"`
}

type RunResult struct {
	RecipeName  string       `json:"recipe_name"`
	Steps       []StepResult `json:"steps"`
	FinalOutput string       `json:"final_output,omitempty"`
	Total       int          `json:"total"`
	Completed   int          `json:"completed"`
	Failed      int          `json:"failed"`
	Skipped     int          `json:"skipped"`
}

type Executor struct {
	recipe  *formula.Recipe
	opts    RunOptions
	mu      sync.RWMutex
	context map[string]string
	results map[string]*StepResult
}

func New(recipe *formula.Recipe, opts RunOptions) *Executor {
	vars := make(map[string]string)
	if recipe != nil {
		for k, def := range recipe.Vars {
			if def != nil && def.Default != nil {
				vars[k] = *def.Default
			}
		}
	}
	for k, v := range opts.Vars {
		vars[k] = v
	}
	for k, v := range opts.InitialContext {
		vars[k] = v
	}
	results := make(map[string]*StepResult)
	for _, result := range opts.InitialResults {
		result := result
		results[result.StepID] = &result
	}
	return &Executor{
		recipe:  recipe,
		opts:    opts,
		context: vars,
		results: results,
	}
}

func (e *Executor) Run(ctx context.Context, runner StepRunner) (*RunResult, error) {
	batches, err := TopologicalBatches(e.recipe)
	if err != nil {
		return nil, err
	}

	result := &RunResult{
		RecipeName: e.recipe.Name,
	}

	var lastStepID string
	for _, batch := range batches {
		errCh := make(chan error, len(batch))
		for _, step := range batch {
			go func(s *formula.RecipeStep) {
				errCh <- e.executeStep(ctx, runner, s)
			}(step)
		}

		for _, step := range batch {
			if err := <-errCh; err != nil {
				return result, err
			}
			if !step.IsRoot && step.Execution != "noop" {
				lastStepID = step.ID
			}
		}
	}

	e.mu.RLock()
	for _, r := range e.results {
		result.Steps = append(result.Steps, *r)
		result.Total++
		switch r.Status {
		case StatusCompleted:
			result.Completed++
		case StatusFailed:
			result.Failed++
		case StatusSkipped:
			result.Skipped++
		}
	}
	e.mu.RUnlock()

	if lastStepID != "" {
		e.mu.RLock()
		if final, ok := e.results[lastStepID]; ok && final.Output != "" {
			result.FinalOutput = final.Output
		}
		e.mu.RUnlock()
	}

	return result, nil
}

func (e *Executor) executeStep(ctx context.Context, runner StepRunner, step *formula.RecipeStep) error {
	e.mu.RLock()
	if existing, ok := e.results[step.ID]; ok && (existing.Status == StatusCompleted || existing.Status == StatusSkipped) {
		e.mu.RUnlock()
		return nil
	}
	e.mu.RUnlock()

	if step.IsRoot {
		e.mu.Lock()
		e.results[step.ID] = &StepResult{
			StepID: step.ID,
			Title:  step.Title,
			Status: StatusCompleted,
		}
		e.mu.Unlock()
		return nil
	}

	if step.Execution == "noop" {
		e.mu.Lock()
		e.results[step.ID] = &StepResult{
			StepID: step.ID,
			Title:  step.Title,
			Status: StatusCompleted,
		}
		e.mu.Unlock()
		return nil
	}

	if step.Loop != nil && step.Loop.Until != "" {
		return e.executeRuntimeLoop(ctx, runner, step)
	}

	if e.shouldSkip(step) {
		e.mu.Lock()
		e.results[step.ID] = &StepResult{
			StepID: step.ID,
			Title:  step.Title,
			Status: StatusSkipped,
		}
		e.mu.Unlock()
		return nil
	}

	e.mu.Lock()
	e.results[step.ID] = &StepResult{
		StepID: step.ID,
		Title:  step.Title,
		Status: StatusRunning,
	}
	e.mu.Unlock()

	if e.opts.DryRun {
		e.mu.Lock()
		e.results[step.ID].Status = StatusCompleted
		e.results[step.ID].Output = "[dry-run] would execute with agent: " + e.resolveAgent(step).Name
		e.mu.Unlock()
		return nil
	}

	prompt := e.buildPrompt(step)

	output, err := runner(ctx, step, prompt)
	if err != nil {
		e.mu.Lock()
		e.results[step.ID].Status = StatusFailed
		e.results[step.ID].Error = err.Error()
		e.mu.Unlock()
		return fmt.Errorf("step %s failed: %w", step.ID, err)
	}

	e.mu.Lock()
	e.results[step.ID].Status = StatusCompleted
	e.results[step.ID].Output = output

	if step.OutputKey != "" {
		e.context[step.OutputKey] = output
	}
	e.mu.Unlock()

	return nil
}

func (e *Executor) executeRuntimeLoop(ctx context.Context, runner StepRunner, step *formula.RecipeStep) error {
	max := step.Loop.Max
	if max <= 0 {
		max = 1
	}
	e.mu.Lock()
	e.results[step.ID] = &StepResult{StepID: step.ID, Title: step.Title, Status: StatusRunning}
	e.mu.Unlock()

	for iter := 1; iter <= max; iter++ {
		for _, body := range step.Loop.Body {
			if body == nil {
				continue
			}
			bodyStep := recipeStepFromLoopBody(step, body, iter)
			if e.shouldSkip(&bodyStep) {
				e.mu.Lock()
				e.results[bodyStep.ID] = &StepResult{StepID: bodyStep.ID, Title: bodyStep.Title, Status: StatusSkipped}
				e.mu.Unlock()
				continue
			}
			e.mu.Lock()
			e.context["iteration"] = fmt.Sprintf("%d", iter)
			e.results[bodyStep.ID] = &StepResult{StepID: bodyStep.ID, Title: bodyStep.Title, Status: StatusRunning}
			e.mu.Unlock()

			prompt := e.buildPrompt(&bodyStep)
			output, err := runner(ctx, &bodyStep, prompt)
			if err != nil {
				e.mu.Lock()
				e.results[bodyStep.ID].Status = StatusFailed
				e.results[bodyStep.ID].Error = err.Error()
				e.results[step.ID].Status = StatusFailed
				e.results[step.ID].Error = err.Error()
				e.mu.Unlock()
				return fmt.Errorf("loop %s iteration %d step %s failed: %w", step.ID, iter, bodyStep.ID, err)
			}

			e.mu.Lock()
			e.results[bodyStep.ID].Status = StatusCompleted
			e.results[bodyStep.ID].Output = output
			if bodyStep.OutputKey != "" {
				e.context[bodyStep.OutputKey] = output
			}
			e.mu.Unlock()
		}

		if EvaluateCondition(step.Loop.Until, e.Context()) {
			e.mu.Lock()
			e.results[step.ID].Status = StatusCompleted
			e.results[step.ID].Output = fmt.Sprintf("loop completed after %d iteration(s)", iter)
			e.mu.Unlock()
			return nil
		}
	}

	e.mu.Lock()
	e.results[step.ID].Status = StatusCompleted
	e.results[step.ID].Output = fmt.Sprintf("loop reached max iterations (%d)", max)
	e.mu.Unlock()
	return nil
}

func recipeStepFromLoopBody(parent *formula.RecipeStep, body *formula.Step, iter int) formula.RecipeStep {
	id := fmt.Sprintf("%s.iter%d.%s", parent.ID, iter, body.ID)
	return formula.RecipeStep{
		ID:          id,
		Title:       strings.ReplaceAll(body.Title, "{{iteration}}", fmt.Sprintf("%d", iter)),
		Description: strings.ReplaceAll(body.Description, "{{iteration}}", fmt.Sprintf("%d", iter)),
		Notes:       strings.ReplaceAll(body.Notes, "{{iteration}}", fmt.Sprintf("%d", iter)),
		Type:        body.Type,
		Priority:    body.Priority,
		Labels:      append([]string(nil), body.Labels...),
		Assignee:    body.Assignee,
		Metadata:    body.Metadata,
		Agent:       body.Agent,
		OutputKey:   body.OutputKey,
		InputCtx:    append([]string(nil), body.InputCtx...),
		Execution:   body.Execution,
		Condition:   body.Condition,
	}
}

func (e *Executor) shouldSkip(step *formula.RecipeStep) bool {
	if step.Condition == "" {
		return false
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return !EvaluateCondition(step.Condition, e.context)
}

func (e *Executor) buildPrompt(step *formula.RecipeStep) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Task: %s\n\n", e.renderTemplate(step.Title)))

	if step.Description != "" {
		b.WriteString(fmt.Sprintf("## Description\n\n%s\n\n", e.renderTemplate(step.Description)))
	}

	if len(step.InputCtx) > 0 {
		e.mu.RLock()
		b.WriteString("## Context from previous steps\n\n")
		for _, key := range step.InputCtx {
			if val, ok := e.context[key]; ok {
				b.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", key, val))
			}
		}
		e.mu.RUnlock()
	}

	if step.Notes != "" {
		b.WriteString(fmt.Sprintf("## Notes\n\n%s\n\n", e.renderTemplate(step.Notes)))
	}

	return b.String()
}

func (e *Executor) renderTemplate(s string) string {
	if s == "" {
		return s
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := s
	for k, v := range e.context {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

func (e *Executor) resolveAgent(step *formula.RecipeStep) *formula.AgentConfig {
	if step.Agent != nil && step.Agent.Name != "" {
		return step.Agent
	}
	agentName := e.opts.Agent
	if agentName == "" {
		agentName = defaultAgentID
	}
	return &formula.AgentConfig{
		Name:    agentName,
		Model:   e.opts.Model,
		Session: e.opts.Session,
	}
}

func (e *Executor) resolveSession(step *formula.RecipeStep) string {
	if step.Agent != nil && step.Agent.Session != "" {
		return e.opts.Session + ":" + step.Agent.Name + ":" + step.Agent.Session
	}
	return e.opts.Session + ":" + step.ID
}

func (e *Executor) Context() map[string]string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	cp := make(map[string]string, len(e.context))
	for k, v := range e.context {
		cp[k] = v
	}
	return cp
}
