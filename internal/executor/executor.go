package executor

import (
	"context"
	"fmt"
	"strings"

	"github.com/sjzsdu/tt/internal/formula"
)

type StepRunner func(ctx context.Context, step *formula.RecipeStep, prompt string) (string, error)

type RunOptions struct {
	Vars      map[string]string
	Agent     string
	Model     string
	Session   string
	DryRun    bool
	Debug     bool
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
	StepID  string     `json:"step_id"`
	Title   string     `json:"title"`
	Status  StepStatus `json:"status"`
	Output  string     `json:"output,omitempty"`
	Error   string     `json:"error,omitempty"`
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
	context map[string]string
	results map[string]*StepResult
}

func New(recipe *formula.Recipe, opts RunOptions) *Executor {
	vars := make(map[string]string)
	for k, v := range opts.Vars {
		vars[k] = v
	}
	return &Executor{
		recipe:  recipe,
		opts:    opts,
		context: vars,
		results: make(map[string]*StepResult),
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
			if !step.IsRoot {
				lastStepID = step.ID
			}
		}
	}

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

	if lastStepID != "" {
		if final, ok := e.results[lastStepID]; ok && final.Output != "" {
			result.FinalOutput = final.Output
		}
	}

	return result, nil
}

func (e *Executor) executeStep(ctx context.Context, runner StepRunner, step *formula.RecipeStep) error {
	if step.IsRoot {
		e.results[step.ID] = &StepResult{
			StepID: step.ID,
			Title:  step.Title,
			Status: StatusCompleted,
		}
		return nil
	}

	if e.shouldSkip(step) {
		e.results[step.ID] = &StepResult{
			StepID: step.ID,
			Title:  step.Title,
			Status: StatusSkipped,
		}
		return nil
	}

	e.results[step.ID] = &StepResult{
		StepID: step.ID,
		Title:  step.Title,
		Status: StatusRunning,
	}

	if e.opts.DryRun {
		e.results[step.ID].Status = StatusCompleted
		e.results[step.ID].Output = "[dry-run] would execute with agent: " + e.resolveAgent(step).Name
		return nil
	}

	prompt := e.buildPrompt(step)

	output, err := runner(ctx, step, prompt)
	if err != nil {
		e.results[step.ID].Status = StatusFailed
		e.results[step.ID].Error = err.Error()
		return fmt.Errorf("step %s failed: %w", step.ID, err)
	}

	e.results[step.ID].Status = StatusCompleted
	e.results[step.ID].Output = output

	if step.OutputKey != "" {
		e.context[step.OutputKey] = output
	}

	return nil
}

func (e *Executor) shouldSkip(step *formula.RecipeStep) bool {
	if step.Condition == "" {
		return false
	}
	return !EvaluateCondition(step.Condition, e.context)
}

func (e *Executor) buildPrompt(step *formula.RecipeStep) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Task: %s\n\n", step.Title))

	if step.Description != "" {
		b.WriteString(fmt.Sprintf("## Description\n\n%s\n\n", step.Description))
	}

	if len(step.InputCtx) > 0 {
		b.WriteString("## Context from previous steps\n\n")
		for _, key := range step.InputCtx {
			if val, ok := e.context[key]; ok {
				b.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", key, val))
			}
		}
	}

	if step.Notes != "" {
		b.WriteString(fmt.Sprintf("## Notes\n\n%s\n\n", step.Notes))
	}

	return b.String()
}

func (e *Executor) resolveAgent(step *formula.RecipeStep) *formula.AgentConfig {
	if step.Agent != nil && step.Agent.Name != "" {
		return step.Agent
	}
	return &formula.AgentConfig{
		Name:    e.opts.Agent,
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
	return e.context
}
