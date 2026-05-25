package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

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
	AllowScripts   bool
	AllowShell     bool
	StepAdvice     map[string]string
	OnStepUpdate   func(StepResult)
}

type StepStatus string

const (
	StatusPending      StepStatus = "pending"
	StatusRunning      StepStatus = "running"
	StatusCompleted    StepStatus = "completed"
	StatusFailed       StepStatus = "failed"
	StatusSkipped      StepStatus = "skipped"
	StatusWaitingInput StepStatus = "waiting_input"
)

const HumanInputExecution = "human_input"

type HumanInputRequest struct {
	Reason string            `json:"reason,omitempty"`
	Form   *formula.FormSpec `json:"form,omitempty"`
}

type WaitingInputError struct {
	StepID  string
	Request *HumanInputRequest
}

func (e WaitingInputError) Error() string {
	return "formula waiting for human input at step " + e.StepID
}

type StepResult struct {
	StepID            string             `json:"step_id"`
	Title             string             `json:"title"`
	Status            StepStatus         `json:"status"`
	Output            string             `json:"output,omitempty"`
	Error             string             `json:"error,omitempty"`
	HumanInputRequest *HumanInputRequest `json:"human_input_request,omitempty"`
}

type RunResult struct {
	RecipeName   string       `json:"recipe_name"`
	Steps        []StepResult `json:"steps"`
	FinalOutput  string       `json:"final_output,omitempty"`
	Total        int          `json:"total"`
	Completed    int          `json:"completed"`
	Failed       int          `json:"failed"`
	Skipped      int          `json:"skipped"`
	WaitingInput int          `json:"waiting_input"`
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
	hydrateImplicitVars(vars)
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

func hydrateImplicitVars(vars map[string]string) {
	if vars == nil {
		return
	}
	if value, ok := vars["repo_hint"]; ok && strings.TrimSpace(value) == "" {
		if repo, err := inferGitRemoteRepo("origin"); err == nil && repo != "" {
			vars["repo_hint"] = repo
		}
	}
}

func inferGitRemoteRepo(remote string) (string, error) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		remote = "origin"
	}
	cmd := exec.Command("git", "remote", "get-url", remote)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return parseGitRemoteRepo(string(out)), nil
}

func parseGitRemoteRepo(remoteURL string) string {
	raw := strings.TrimSpace(remoteURL)
	if raw == "" {
		return ""
	}

	var path string
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" {
		path = u.Path
	} else if idx := strings.Index(raw, ":"); idx >= 0 && !strings.Contains(raw[:idx], "/") {
		// SCP-style git URL, e.g. git@github.com:owner/repo.git or git@github_alias:owner/repo.git.
		path = raw[idx+1:]
	} else {
		path = raw
	}

	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return ""
	}
	owner := strings.TrimSpace(parts[len(parts)-2])
	repo := strings.TrimSpace(parts[len(parts)-1])
	if owner == "" || repo == "" {
		return ""
	}
	return owner + "/" + repo
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
				e.collectRunResults(result)
				return result, err
			}
			if !step.IsRoot && step.Execution != "noop" {
				lastStepID = step.ID
			}
		}
	}

	e.collectRunResults(result)

	if lastStepID != "" {
		e.mu.RLock()
		if final, ok := e.results[lastStepID]; ok && final.Output != "" {
			result.FinalOutput = final.Output
		}
		e.mu.RUnlock()
	}

	return result, nil
}

func (e *Executor) collectRunResults(result *RunResult) {
	if result == nil {
		return
	}
	result.Steps = nil
	result.Total = 0
	result.Completed = 0
	result.Failed = 0
	result.Skipped = 0
	result.WaitingInput = 0
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
		case StatusWaitingInput:
			result.WaitingInput++
		}
	}
	e.mu.RUnlock()
}

func (e *Executor) executeStep(ctx context.Context, runner StepRunner, step *formula.RecipeStep) error {
	e.mu.RLock()
	if existing, ok := e.results[step.ID]; ok && (existing.Status == StatusCompleted || existing.Status == StatusSkipped) {
		e.mu.RUnlock()
		return nil
	}
	e.mu.RUnlock()

	if e.shouldSkip(step) {
		e.markStepSkipped(step)
		return nil
	}

	handler, err := newDefaultStepRegistry().Resolve(step)
	if err != nil {
		return err
	}

	if !step.IsRoot && step.Execution != "noop" && handler.Kind() != "loop.foreach" && handler.Kind() != "loop.until" {
		e.markStepRunning(step)
		if e.opts.DryRun {
			e.markStepDryRunCompleted(step)
			return nil
		}
	}

	result, err := handler.Execute(ctx, stepRuntime{executor: e, runner: runner}, step)
	if err != nil {
		e.markStepFailed(step, result.Output, err)
		return fmt.Errorf("step %s failed: %w", step.ID, err)
	}

	if handler.Kind() == "root" || handler.Kind() == "noop" {
		e.mu.Lock()
		e.results[step.ID] = &StepResult{StepID: step.ID, Title: step.Title, Status: StatusCompleted}
		e.mu.Unlock()
		return nil
	}

	if result.Status == StatusWaitingInput {
		return e.pauseForHumanInput(step, result.HumanInputRequest)
	}

	if result.Status == StatusCompleted && result.Output != "" {
		if err := validateStepOutput(step, result.Output); err != nil {
			e.markStepFailed(step, result.Output, err)
			e.emitStepUpdate(StepResult{StepID: step.ID, Title: step.Title, Status: StatusFailed, Output: result.Output, Error: err.Error()})
			return fmt.Errorf("step %s output validation failed: %w", step.ID, err)
		}
	}

	switch result.Status {
	case StatusCompleted:
		e.markStepCompleted(step, result.Output)
	case StatusSkipped:
		e.markStepSkipped(step)
	case StatusFailed:
		err := fmt.Errorf("step failed")
		e.markStepFailed(step, result.Output, err)
		return fmt.Errorf("step %s failed", step.ID)
	}
	return nil
}

func (e *Executor) markStepRunning(step *formula.RecipeStep) {
	e.mu.Lock()
	e.results[step.ID] = &StepResult{StepID: step.ID, Title: step.Title, Status: StatusRunning}
	e.mu.Unlock()
	e.emitStepUpdate(StepResult{StepID: step.ID, Title: step.Title, Status: StatusRunning})
}

func (e *Executor) markStepCompleted(step *formula.RecipeStep, output string) {
	e.mu.Lock()
	if e.results[step.ID] == nil {
		e.results[step.ID] = &StepResult{StepID: step.ID, Title: step.Title}
	}
	e.results[step.ID].Status = StatusCompleted
	e.results[step.ID].Output = output
	if outputKey := stepOutputKey(step); outputKey != "" && output != "" {
		e.context[outputKey] = output
	}
	e.mu.Unlock()
	e.emitStepUpdate(StepResult{StepID: step.ID, Title: step.Title, Status: StatusCompleted, Output: output})
}

func (e *Executor) markStepFailed(step *formula.RecipeStep, output string, err error) {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	e.mu.Lock()
	if e.results[step.ID] == nil {
		e.results[step.ID] = &StepResult{StepID: step.ID, Title: step.Title}
	}
	e.results[step.ID].Status = StatusFailed
	e.results[step.ID].Error = errMsg
	e.results[step.ID].Output = output
	e.mu.Unlock()
	e.emitStepUpdate(StepResult{StepID: step.ID, Title: step.Title, Status: StatusFailed, Output: output, Error: errMsg})
}

func (e *Executor) markStepSkipped(step *formula.RecipeStep) {
	e.mu.Lock()
	e.results[step.ID] = &StepResult{StepID: step.ID, Title: step.Title, Status: StatusSkipped}
	e.mu.Unlock()
	e.emitStepUpdate(StepResult{StepID: step.ID, Title: step.Title, Status: StatusSkipped})
}

func (e *Executor) markStepDryRunCompleted(step *formula.RecipeStep) {
	output := "[dry-run] would execute with agent: " + e.resolveAgent(step).Name
	if step.Execution == "script" {
		output = "[dry-run] would execute script: " + strings.Join(renderScriptCommand(step.Script, e.renderTemplate), " ")
	}
	e.markStepCompleted(step, output)
}

func stepOutputKey(step *formula.RecipeStep) string {
	if step == nil {
		return ""
	}
	if key := strings.TrimSpace(step.OutputKey); key != "" {
		return key
	}
	id := strings.TrimSpace(step.ID)
	if id == "" {
		return ""
	}
	if idx := strings.LastIndex(id, "."); idx >= 0 && idx < len(id)-1 {
		return id[idx+1:]
	}
	return id
}

func (e *Executor) pauseForHumanInput(step *formula.RecipeStep, request *HumanInputRequest) error {
	if request == nil {
		request = &HumanInputRequest{Reason: "step requires human input"}
	}
	waiting := StepResult{StepID: step.ID, Title: step.Title, Status: StatusWaitingInput, HumanInputRequest: request}
	e.mu.Lock()
	e.results[step.ID] = &StepResult{StepID: step.ID, Title: step.Title, Status: StatusWaitingInput, HumanInputRequest: request}
	e.mu.Unlock()
	e.emitStepUpdate(waiting)
	return WaitingInputError{StepID: step.ID, Request: request}
}

func ParseHumanInputRequest(output string) *HumanInputRequest {
	req, _, err := ParseHumanInputRequestStrict(output)
	if err != nil {
		return nil
	}
	return req
}

func ParseHumanInputRequestStrict(output string) (*HumanInputRequest, bool, error) {
	block, found := extractHumanInputBlock(output)
	if !found {
		return nil, false, nil
	}
	var req HumanInputRequest
	if err := json.Unmarshal([]byte(block), &req); err != nil {
		return nil, true, fmt.Errorf("tt-human-input block must be valid JSON: %w", err)
	}
	if req.Form == nil && strings.TrimSpace(req.Reason) == "" {
		wrapped, err := unwrapHumanInputRequest(block)
		if err != nil {
			return nil, true, err
		}
		if wrapped != nil {
			req = *wrapped
		}
	}
	if err := validateHumanInputRequest(&req); err != nil {
		return nil, true, err
	}
	return &req, true, nil
}

func unwrapHumanInputRequest(block string) (*HumanInputRequest, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(block), &envelope); err != nil {
		return nil, fmt.Errorf("tt-human-input block must be a JSON object: %w", err)
	}
	for _, key := range []string{"human_input_request", "humanInputRequest", "human_input", "request"} {
		raw, ok := envelope[key]
		if !ok {
			continue
		}
		var req HumanInputRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, fmt.Errorf("%s must match human input request schema: %w", key, err)
		}
		return &req, nil
	}
	return nil, nil
}

func validateHumanInputRequest(req *HumanInputRequest) error {
	if req == nil {
		return fmt.Errorf("tt-human-input request is required")
	}
	if req.Form == nil && strings.TrimSpace(req.Reason) == "" {
		return fmt.Errorf("tt-human-input request must include reason or form")
	}
	if req.Form == nil {
		return nil
	}
	if len(req.Form.Fields) == 0 {
		return fmt.Errorf("tt-human-input form.fields must contain at least one field")
	}
	seen := map[string]bool{}
	for i, field := range req.Form.Fields {
		if field == nil {
			return fmt.Errorf("tt-human-input form.fields[%d] is null", i)
		}
		field.Name = strings.TrimSpace(field.Name)
		field.Label = strings.TrimSpace(field.Label)
		field.Type = normalizeHumanInputFieldType(field.Type)
		if field.Name == "" {
			return fmt.Errorf("tt-human-input form.fields[%d].name is required", i)
		}
		if !regexp.MustCompile(`^[a-z][a-z0-9_]*$`).MatchString(field.Name) {
			return fmt.Errorf("tt-human-input form field name %q must match ^[a-z][a-z0-9_]*$", field.Name)
		}
		if seen[field.Name] {
			return fmt.Errorf("tt-human-input form field %q is duplicated", field.Name)
		}
		seen[field.Name] = true
		if field.Label == "" {
			field.Label = field.Name
		}
		if field.Type == "" {
			field.Type = "input"
		}
		if !isSupportedHumanInputFieldType(field.Type) {
			return fmt.Errorf("tt-human-input form field %q has unsupported type %q; supported: input, textarea, radio, checkbox, select", field.Name, field.Type)
		}
		if requiresHumanInputOptions(field.Type) && len(field.Options) == 0 {
			return fmt.Errorf("tt-human-input form field %q type %q requires options", field.Name, field.Type)
		}
	}
	return nil
}

func normalizeHumanInputFieldType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "text", "string", "short_text":
		return "input"
	case "long_text", "multiline":
		return "textarea"
	case "dropdown", "choice":
		return "select"
	case "multi_select", "multiselect":
		return "checkbox"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func isSupportedHumanInputFieldType(value string) bool {
	switch value {
	case "input", "textarea", "radio", "checkbox", "select":
		return true
	default:
		return false
	}
}

func requiresHumanInputOptions(value string) bool {
	switch value {
	case "radio", "checkbox", "select":
		return true
	default:
		return false
	}
}

func extractHumanInputBlock(output string) (string, bool) {
	const marker = "```tt-human-input"
	idx := strings.Index(output, marker)
	if idx < 0 {
		return "", false
	}
	rest := output[idx+len(marker):]
	rest = strings.TrimLeft(rest, " \t\r\n")
	if len(rest) >= 4 && strings.EqualFold(rest[:4], "json") {
		rest = strings.TrimLeft(rest[4:], " \t\r\n")
	}
	end := strings.Index(rest, "```")
	if end < 0 {
		return "", true
	}
	return strings.TrimSpace(rest[:end]), true
}

type scriptOutput struct {
	Command    []string        `json:"command"`
	Cwd        string          `json:"cwd,omitempty"`
	ExitCode   int             `json:"exit_code"`
	Stdout     string          `json:"stdout"`
	Stderr     string          `json:"stderr,omitempty"`
	JSON       json.RawMessage `json:"json,omitempty"`
	DurationMS int64           `json:"duration_ms"`
}

func (e *Executor) executeScriptStep(ctx context.Context, step *formula.RecipeStep) (string, error) {
	return e.executeScriptStepWithRender(ctx, step, e.renderTemplate)
}

func (e *Executor) executeScriptStepWithRender(ctx context.Context, step *formula.RecipeStep, render func(string) string) (string, error) {
	spec := step.Script
	if spec == nil {
		return "", fmt.Errorf("script spec is required")
	}
	argv := renderScriptCommand(spec, render)
	if len(argv) == 0 {
		return "", fmt.Errorf("script command is required")
	}
	if strings.TrimSpace(spec.Shell) != "" && !e.opts.AllowShell {
		return "", fmt.Errorf("script shell mode is disabled by default; use argv command or rerun with --allow-shell-script")
	}
	if err := validateScriptDenylist(argv); err != nil {
		return "", err
	}
	timeout := firstDuration(spec.Timeout, step.Timeout, 5*time.Minute)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	if cwd := strings.TrimSpace(render(spec.Cwd)); cwd != "" {
		if !filepath.IsAbs(cwd) {
			if wd, err := os.Getwd(); err == nil {
				cwd = filepath.Join(wd, cwd)
			}
		}
		cmd.Dir = cwd
	}
	cmd.Env = os.Environ()
	for k, v := range spec.Env {
		cmd.Env = append(cmd.Env, k+"="+render(v))
	}
	started := time.Now()
	stdout, stderr := &strings.Builder{}, &strings.Builder{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = 1
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
	}
	if runCtx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("script timed out after %s", timeout)
	}
	out := scriptOutput{Command: argv, Cwd: cmd.Dir, ExitCode: exitCode, Stdout: stdout.String(), Stderr: stderr.String(), DurationMS: time.Since(started).Milliseconds()}
	if strings.EqualFold(spec.Format, "json") && strings.TrimSpace(out.Stdout) != "" {
		var raw json.RawMessage
		if parseErr := json.Unmarshal([]byte(out.Stdout), &raw); parseErr != nil {
			if err == nil {
				err = fmt.Errorf("script stdout is not valid json: %w", parseErr)
			}
		} else {
			out.JSON = raw
		}
	}
	data, marshalErr := json.MarshalIndent(out, "", "  ")
	if marshalErr != nil {
		return strings.TrimSpace(out.Stdout), marshalErr
	}
	if err != nil && !spec.ContinueOnError {
		return string(data), err
	}
	return string(data), nil
}

func renderScriptCommand(spec *formula.ScriptSpec, render func(string) string) []string {
	if spec == nil {
		return nil
	}
	if strings.TrimSpace(spec.Shell) != "" {
		return []string{spec.Shell, "-c", render(strings.Join(spec.Command, " "))}
	}
	out := make([]string, 0, len(spec.Command))
	for _, part := range spec.Command {
		part = strings.TrimSpace(render(part))
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func validateScriptDenylist(argv []string) error {
	base := filepath.Base(argv[0])
	dangerous := map[string]bool{"rm": true, "rmdir": true, "mkfs": true, "dd": true, "shutdown": true, "reboot": true, "halt": true, "poweroff": true, "sudo": true, "su": true, "chmod": true, "chown": true}
	if dangerous[base] {
		return fmt.Errorf("script command %q is denied by formula safety policy", base)
	}
	joined := strings.ToLower(strings.Join(argv, " "))
	patterns := []string{"rm -rf", ":(){", "> /dev/", "mkfs.", "curl | sh", "wget | sh"}
	for _, p := range patterns {
		if strings.Contains(joined, p) {
			return fmt.Errorf("script command contains denied pattern %q", p)
		}
	}
	return nil
}

func firstDuration(values ...any) time.Duration {
	for _, value := range values {
		s, _ := value.(string)
		if strings.TrimSpace(s) == "" {
			continue
		}
		if d, err := time.ParseDuration(s); err == nil {
			return d
		}
	}
	if len(values) > 0 {
		if d, ok := values[len(values)-1].(time.Duration); ok {
			return d
		}
	}
	return 5 * time.Minute
}

func (e *Executor) executeForEachLoop(ctx context.Context, runner StepRunner, step *formula.RecipeStep) error {
	items, err := e.loopItems(step.Loop.ForEach)
	if err != nil {
		return fmt.Errorf("loop %s: %w", step.ID, err)
	}
	varName := strings.TrimSpace(step.Loop.Var)
	if varName == "" {
		varName = "item"
	}
	e.mu.Lock()
	e.results[step.ID] = &StepResult{StepID: step.ID, Title: step.Title, Status: StatusRunning}
	e.mu.Unlock()

	maxConcurrency := 1
	if step.Loop.Parallel {
		maxConcurrency = len(items)
		if step.Loop.MaxConcurrency > 0 && step.Loop.MaxConcurrency < maxConcurrency {
			maxConcurrency = step.Loop.MaxConcurrency
		}
	}
	if maxConcurrency < 1 {
		maxConcurrency = 1
	}

	sem := make(chan struct{}, maxConcurrency)
	errCh := make(chan error, len(items))
	outputs := make([]json.RawMessage, len(items))
	for i, item := range items {
		i, item := i, item
		sem <- struct{}{}
		go func() {
			defer func() { <-sem }()
			out, err := e.executeForEachItem(ctx, runner, step, varName, i, item)
			if strings.TrimSpace(out) == "" {
				out = "{}"
			}
			outputs[i] = json.RawMessage(out)
			errCh <- err
		}()
		if !step.Loop.Parallel {
			if err := <-errCh; err != nil {
				e.markLoopFailed(step, err.Error())
				return err
			}
		}
	}
	if step.Loop.Parallel {
		for range items {
			if err := <-errCh; err != nil {
				e.markLoopFailed(step, err.Error())
				return err
			}
		}
	}

	data, _ := json.MarshalIndent(outputs, "", "  ")
	completed := StepResult{StepID: step.ID, Title: step.Title, Status: StatusCompleted, Output: string(data)}
	e.mu.Lock()
	e.results[step.ID].Status = StatusCompleted
	e.results[step.ID].Output = completed.Output
	if step.OutputKey != "" {
		e.context[step.OutputKey] = completed.Output
	}
	e.mu.Unlock()
	e.emitStepUpdate(completed)
	return nil
}

func (e *Executor) loopItems(source string) ([]json.RawMessage, error) {
	key := strings.TrimSpace(source)
	key = strings.TrimPrefix(key, "output.")
	if key == "" {
		return nil, fmt.Errorf("for_each is required")
	}
	e.mu.RLock()
	raw := strings.TrimSpace(e.context[key])
	e.mu.RUnlock()
	if raw == "" {
		return nil, fmt.Errorf("for_each source %q is empty", source)
	}
	var items []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("for_each source %q must be a JSON array: %w", source, err)
	}
	return items, nil
}

func (e *Executor) executeForEachItem(ctx context.Context, runner StepRunner, parent *formula.RecipeStep, varName string, index int, item json.RawMessage) (string, error) {
	local := map[string]string{
		varName:            string(item),
		varName + ".index": fmt.Sprintf("%d", index+1),
		"iteration":        fmt.Sprintf("%d", index+1),
	}
	batches, err := loopBodyBatches(parent.Loop.Body)
	if err != nil {
		return "", fmt.Errorf("loop %s item %d: %w", parent.ID, index+1, err)
	}
	for _, batch := range batches {
		for _, body := range batch {
			bodyStep := recipeStepFromLoopBody(parent, body, index+1)
			if err := e.executeLoopBodyStep(ctx, runner, &bodyStep, local); err != nil {
				return "", err
			}
		}
	}
	out := map[string]string{}
	for k, v := range local {
		if strings.HasPrefix(k, "_output.") {
			out[strings.TrimPrefix(k, "_output.")] = v
		}
	}
	data, _ := json.Marshal(out)
	return string(data), nil
}

func (e *Executor) executeLoopBodyStep(ctx context.Context, runner StepRunner, step *formula.RecipeStep, local map[string]string) error {
	if e.shouldSkipWithContext(step, local) {
		e.mu.Lock()
		e.results[step.ID] = &StepResult{StepID: step.ID, Title: step.Title, Status: StatusSkipped}
		e.mu.Unlock()
		e.emitStepUpdate(StepResult{StepID: step.ID, Title: step.Title, Status: StatusSkipped})
		return nil
	}
	if step.Execution == HumanInputExecution {
		return fmt.Errorf("step %s uses human_input inside parallel foreach; this is not supported yet", step.ID)
	}
	e.mu.Lock()
	e.results[step.ID] = &StepResult{StepID: step.ID, Title: step.Title, Status: StatusRunning}
	e.mu.Unlock()
	e.emitStepUpdate(StepResult{StepID: step.ID, Title: step.Title, Status: StatusRunning})

	var output string
	var err error
	if step.Execution == "script" {
		if !e.opts.AllowScripts {
			err = fmt.Errorf("step %s uses script execution; rerun with formula script execution enabled", step.ID)
		} else {
			output, err = e.executeScriptStepWithRender(ctx, step, func(s string) string { return e.renderTemplateWithContext(s, local) })
		}
	} else {
		prompt := e.buildPromptWithContext(step, local)
		output, err = runner(ctx, step, prompt)
	}
	if err != nil {
		failed := StepResult{StepID: step.ID, Title: step.Title, Status: StatusFailed, Output: output, Error: err.Error()}
		e.mu.Lock()
		e.results[step.ID].Status = StatusFailed
		e.results[step.ID].Error = err.Error()
		e.results[step.ID].Output = output
		e.mu.Unlock()
		e.emitStepUpdate(failed)
		return fmt.Errorf("step %s failed: %w", step.ID, err)
	}
	if request := ParseHumanInputRequest(output); request != nil {
		return fmt.Errorf("step %s requested human input inside foreach; this is not supported yet", step.ID)
	}
	completed := StepResult{StepID: step.ID, Title: step.Title, Status: StatusCompleted, Output: output}
	e.mu.Lock()
	e.results[step.ID].Status = StatusCompleted
	e.results[step.ID].Output = output
	e.mu.Unlock()
	if step.OutputKey != "" {
		local[step.OutputKey] = output
		local["_output."+step.OutputKey] = output
	}
	e.emitStepUpdate(completed)
	return nil
}

func (e *Executor) markLoopFailed(step *formula.RecipeStep, errMsg string) {
	e.mu.Lock()
	if e.results[step.ID] == nil {
		e.results[step.ID] = &StepResult{StepID: step.ID, Title: step.Title}
	}
	e.results[step.ID].Status = StatusFailed
	e.results[step.ID].Error = errMsg
	e.mu.Unlock()
	e.emitStepUpdate(StepResult{StepID: step.ID, Title: step.Title, Status: StatusFailed, Error: errMsg})
}

func loopBodyBatches(body []*formula.Step) ([][]*formula.Step, error) {
	stepMap := map[string]*formula.Step{}
	inDegree := map[string]int{}
	adj := map[string][]string{}
	for _, step := range body {
		if step == nil {
			continue
		}
		if step.ID == "" {
			return nil, fmt.Errorf("loop body step id is required")
		}
		if _, exists := stepMap[step.ID]; exists {
			return nil, fmt.Errorf("duplicate loop body step id %q", step.ID)
		}
		stepMap[step.ID] = step
		inDegree[step.ID] = 0
	}
	for _, step := range body {
		if step == nil {
			continue
		}
		for _, dep := range append([]string{}, append(step.DependsOn, step.Needs...)...) {
			if _, ok := stepMap[dep]; !ok {
				return nil, fmt.Errorf("loop body step %s depends on unknown step %q", step.ID, dep)
			}
			adj[dep] = append(adj[dep], step.ID)
			inDegree[step.ID]++
		}
	}
	var batches [][]*formula.Step
	remaining := len(stepMap)
	for remaining > 0 {
		var ids []string
		for id, deg := range inDegree {
			if deg == 0 && stepMap[id] != nil {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		if len(ids) == 0 {
			return nil, fmt.Errorf("cycle detected in loop body")
		}
		batch := make([]*formula.Step, 0, len(ids))
		for _, id := range ids {
			batch = append(batch, stepMap[id])
			delete(inDegree, id)
			delete(stepMap, id)
			remaining--
			for _, next := range adj[id] {
				inDegree[next]--
			}
		}
		batches = append(batches, batch)
	}
	return batches, nil
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

			var output string
			var err error
			if bodyStep.Execution == HumanInputExecution {
				request := &HumanInputRequest{Reason: strings.TrimSpace(bodyStep.Description), Form: bodyStep.Form}
				if request.Reason == "" {
					request.Reason = "step requires human input"
				}
				return e.pauseForHumanInput(&bodyStep, request)
			}
			if bodyStep.Execution == "script" {
				if !e.opts.AllowScripts {
					err = fmt.Errorf("step %s uses script execution; rerun with formula script execution enabled", bodyStep.ID)
				} else {
					output, err = e.executeScriptStep(ctx, &bodyStep)
				}
			} else {
				prompt := e.buildPrompt(&bodyStep)
				output, err = runner(ctx, &bodyStep, prompt)
			}
			if err != nil {
				failed := StepResult{StepID: step.ID, Title: step.Title, Status: StatusFailed, Output: output, Error: err.Error()}
				e.mu.Lock()
				e.results[bodyStep.ID].Status = StatusFailed
				e.results[bodyStep.ID].Error = err.Error()
				e.results[bodyStep.ID].Output = output
				e.results[step.ID].Status = StatusFailed
				e.results[step.ID].Error = err.Error()
				e.mu.Unlock()
				e.emitStepUpdate(failed)
				return fmt.Errorf("loop %s iteration %d step %s failed: %w", step.ID, iter, bodyStep.ID, err)
			}
			if request := ParseHumanInputRequest(output); request != nil {
				return e.pauseForHumanInput(&bodyStep, request)
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
			completed := StepResult{StepID: step.ID, Title: step.Title, Status: StatusCompleted, Output: fmt.Sprintf("loop completed after %d iteration(s)", iter)}
			e.mu.Lock()
			e.results[step.ID].Status = StatusCompleted
			e.results[step.ID].Output = completed.Output
			e.mu.Unlock()
			e.emitStepUpdate(completed)
			return nil
		}
	}

	completed := StepResult{StepID: step.ID, Title: step.Title, Status: StatusCompleted, Output: fmt.Sprintf("loop reached max iterations (%d)", max)}
	e.mu.Lock()
	e.results[step.ID].Status = StatusCompleted
	e.results[step.ID].Output = completed.Output
	e.mu.Unlock()
	e.emitStepUpdate(completed)
	return nil
}

func (e *Executor) emitStepUpdate(result StepResult) {
	if e.opts.OnStepUpdate != nil {
		e.opts.OnStepUpdate(result)
	}
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
		Script:      body.Script,
		Form:        body.Form,
		Timeout:     body.Timeout,
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

func (e *Executor) shouldSkipWithContext(step *formula.RecipeStep, local map[string]string) bool {
	if step.Condition == "" {
		return false
	}
	return !EvaluateCondition(step.Condition, e.mergedContext(local))
}

func (e *Executor) buildPrompt(step *formula.RecipeStep) string {
	return e.buildPromptWithContext(step, nil)
}

func (e *Executor) buildPromptWithContext(step *formula.RecipeStep, local map[string]string) string {
	var b strings.Builder

	render := func(s string) string { return e.renderTemplateWithContext(s, local) }
	ctx := e.mergedContext(local)

	b.WriteString(fmt.Sprintf("# Task: %s\n\n", render(step.Title)))

	if step.Description != "" {
		b.WriteString(fmt.Sprintf("## Description\n\n%s\n\n", render(step.Description)))
	}

	if len(step.InputCtx) > 0 {
		b.WriteString("## Context from previous steps\n\n")
		for _, key := range step.InputCtx {
			if val, ok := resolveContextValue(ctx, key); ok {
				b.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", key, val))
			}
		}
	}

	if step.Notes != "" {
		b.WriteString(fmt.Sprintf("## Notes\n\n%s\n\n", render(step.Notes)))
	}

	if step.DynamicForm {
		b.WriteString("## Dynamic human input\n\n")
		b.WriteString("If you need user clarification before completing this step, output ONLY a fenced `tt-human-input` JSON block using this shape:\n\n")
		b.WriteString("```tt-human-input json\n")
		b.WriteString(`{"reason":"why input is needed","form":{"title":"Short title","description":"What to provide","fields":[{"name":"field_name","label":"Field label","type":"input|textarea|radio|checkbox|select","required":true,"options":["only for radio/checkbox/select"],"placeholder":"optional"}]}}`)
		b.WriteString("\n```\n\n")
		b.WriteString("Use field names matching ^[a-z][a-z0-9_]*$. If no clarification is needed, do not include a tt-human-input block and complete the normal task output.\n\n")
	}

	if step.Validate != nil && strings.EqualFold(strings.TrimSpace(step.Validate.Format), "json") {
		b.WriteString("## Output validation\n\n")
		b.WriteString("Your final output must be valid JSON")
		if len(step.Validate.Required) > 0 {
			b.WriteString(" and include these required fields: ")
			b.WriteString(strings.Join(step.Validate.Required, ", "))
		}
		b.WriteString(". Do not wrap the final JSON in markdown fences.\n\n")
	}

	if advice := strings.TrimSpace(e.opts.StepAdvice[step.ID]); advice != "" {
		b.WriteString("## User retry instructions\n\n")
		b.WriteString(advice)
		b.WriteString("\n\n")
	}

	return b.String()
}

func resolveContextValue(ctx map[string]string, key string) (string, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false
	}
	if val, ok := ctx[key]; ok {
		return val, true
	}
	if strings.Contains(key, ".") {
		path := strings.Split(key, ".")
		if raw, ok := ctx[path[0]]; ok {
			return resolveJSONPath(raw, path[1:])
		}
	}
	return "", false
}

func validateStepOutput(step *formula.RecipeStep, output string) error {
	if step == nil || step.Validate == nil {
		return nil
	}
	spec := step.Validate
	format := strings.ToLower(strings.TrimSpace(spec.Format))
	if format == "" {
		return nil
	}
	switch format {
	case "json":
		var value any
		if err := json.Unmarshal([]byte(normalizeJSONOutput(output)), &value); err != nil {
			return fmt.Errorf("output must be valid json: %w", err)
		}
		for _, required := range spec.Required {
			path := strings.TrimSpace(required)
			if path == "" {
				continue
			}
			if !jsonPathExists(value, strings.Split(path, ".")) {
				return fmt.Errorf("output json missing required field %q", path)
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported validation format %q", spec.Format)
	}
}

func normalizeJSONOutput(output string) string {
	trimmed := strings.TrimSpace(output)
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) >= 3 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
			return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
		}
	}
	return trimmed
}

func jsonPathExists(value any, path []string) bool {
	if len(path) == 0 {
		return true
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return false
	}
	next, ok := obj[path[0]]
	if !ok || next == nil {
		return false
	}
	return jsonPathExists(next, path[1:])
}

func (e *Executor) renderTemplate(s string) string {
	return e.renderTemplateWithContext(s, nil)
}

var templatePathPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*(?:\.[a-zA-Z0-9_]+)*)\s*\}\}`)

func (e *Executor) renderTemplateWithContext(s string, local map[string]string) string {
	if s == "" {
		return s
	}
	ctx := e.mergedContext(local)
	out := templatePathPattern.ReplaceAllStringFunc(s, func(match string) string {
		parts := templatePathPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		key := parts[1]
		if val, ok := ctx[key]; ok {
			return val
		}
		if strings.Contains(key, ".") {
			path := strings.Split(key, ".")
			if raw, ok := ctx[path[0]]; ok {
				if val, ok := resolveJSONPath(raw, path[1:]); ok {
					return val
				}
			}
		}
		return match
	})
	return out
}

func (e *Executor) mergedContext(local map[string]string) map[string]string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make(map[string]string, len(e.context)+len(local))
	for k, v := range e.context {
		out[k] = v
	}
	for k, v := range local {
		out[k] = v
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
