package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sjzsdu/tt/internal/formula/ast"
)

type NoopStep struct{ Base }
type NoopDecoder struct{}

func (NoopDecoder) Kind() Kind { return KindNoop }
func (NoopDecoder) Decode(decl ast.StepDecl) (Step, error) {
	return NoopStep{Base{metadataFromDecl(decl, KindNoop)}}, nil
}
func (s NoopStep) Run(context.Context, RunRequest) (*RunResult, error) {
	return &RunResult{Status: StatusCompleted}, nil
}

type AgentStep struct {
	Base
	Agent       string
	Model       string
	Cwd         string
	Prompt      string
	InputCtx    []string
	DynamicForm bool
	OutputKey   string
	Validation  *OutputValidationSpec `json:"validate,omitempty"`
}
type AgentDecoder struct{}

func (AgentDecoder) Kind() Kind { return KindAgent }
func (AgentDecoder) Decode(decl ast.StepDecl) (Step, error) {
	var s AgentStep
	_ = json.Unmarshal(decl.Raw, &s)
	s.Base = Base{metadataFromDecl(decl, KindAgent)}
	return s, nil
}
func (s AgentStep) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	if req.Capabilities.Agents == nil {
		err := &StepError{Message: "agent capability is required"}
		return &RunResult{Status: StatusFailed, Error: err}, err
	}
	prompt := renderContextTemplates(s.Prompt, req.Context)
	prompt = appendInputContext(prompt, s.InputCtx, req.Context)
	if s.DynamicForm {
		prompt = appendDynamicHumanInputProtocol(prompt)
	}
	prompt = appendFormulaAgentExecutionGuard(prompt)
	out, err := req.Capabilities.Agents.RunAgent(ctx, AgentRequest{NodeID: req.NodeID, Agent: s.Agent, Model: s.Model, Workspace: renderStepCwd(s.Cwd, req.Context), Prompt: prompt})
	if err != nil {
		return &RunResult{Status: StatusFailed, Error: &StepError{Message: "agent step failed", Cause: err}}, err
	}
	if s.DynamicForm {
		request, found, parseErr := parseDynamicHumanInputRequest(out)
		if parseErr != nil {
			err := &StepError{Message: "agent produced invalid dynamic human input request", Cause: parseErr}
			return &RunResult{Status: StatusFailed, Output: out, Error: err}, parseErr
		}
		if found {
			return &RunResult{Status: StatusWaiting, Output: out, Await: request}, nil
		}
	}
	return &RunResult{Status: StatusCompleted, Output: out}, nil
}

func appendFormulaAgentExecutionGuard(prompt string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(prompt))
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString("## Formula step execution guard\n\n")
	b.WriteString("You are executing a formula workflow step now. Do not merely acknowledge project rules, repository instructions, memory policies, or system constraints.\n")
	b.WriteString("Treat such rules as background constraints only. Perform the concrete step task above and return the requested step output immediately.\n")
	b.WriteString("If the step requires JSON, return only that JSON. If dynamic human input is required, return only the tt-human-input block described above.\n")
	b.WriteString("Do not say that you have received or will follow rules unless the step explicitly asks for that.\n")
	return strings.TrimSpace(b.String())
}

func appendInputContext(prompt string, keys []string, ctx ContextView) string {
	if len(keys) == 0 || ctx == nil {
		return prompt
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(prompt))
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString("## Input context\n\n")
	b.WriteString("The following values are outputs from upstream steps. A plain step id contains the complete JSON output for that step.\n\n")
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		b.WriteString("### ")
		b.WriteString(key)
		b.WriteString("\n\n")
		if value, ok := ctx.Get(key); ok {
			b.WriteString(valueForPrompt(value))
		} else {
			b.WriteString("(not available)")
		}
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

func valueForPrompt(out Value) string {
	var text string
	if err := json.Unmarshal(out.Raw, &text); err == nil {
		return text
	}
	var decoded any
	if err := json.Unmarshal(out.Raw, &decoded); err == nil {
		pretty, err := json.MarshalIndent(decoded, "", "  ")
		if err == nil {
			return string(pretty)
		}
	}
	return strings.TrimSpace(string(out.Raw))
}

func appendDynamicHumanInputProtocol(prompt string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(prompt))
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString("## Dynamic human input\n\n")
	b.WriteString("This step has dynamic clarification enabled. Before completing the normal task, decide whether missing user information blocks safe progress.\n\n")
	b.WriteString("If clarification is required, output ONLY a fenced `tt-human-input` JSON block using this shape:\n\n")
	b.WriteString("```tt-human-input json\n")
	b.WriteString(`{"reason":"why input is needed","form":{"title":"Short title","description":"What to provide","fields":[{"name":"field_name","label":"Field label","type":"input|textarea|radio|checkbox|select","required":true,"options":["only for radio/checkbox/select"],"placeholder":"optional"}]}}`)
	b.WriteString("\n```\n\n")
	b.WriteString("Clarification rules:\n")
	b.WriteString("- Ask only for information that blocks this step or downstream work; do not ask for nice-to-have details.\n")
	b.WriteString("- Generate the minimum necessary form at runtime. Do not assume fixed fields.\n")
	b.WriteString("- Prefer 1-5 fields. Use field names matching ^[a-z][a-z0-9_]*$.\n")
	b.WriteString("- For radio, checkbox, and select fields, include options.\n")
	b.WriteString("- If no clarification is needed, do not include a tt-human-input block and complete the normal task output exactly as requested by the step.\n")
	return b.String()
}

var dynamicHumanInputBlockPattern = regexp.MustCompile("(?s)```tt-human-input(?:\\s+json)?\\s*\\n(.*?)\\n```")
var runtimeTemplatePattern = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_-]*(?:\.[a-zA-Z_][a-zA-Z0-9_-]*)*)\s*\}\}`)

func renderContextTemplates(input string, ctx ContextView) string {
	if input == "" || ctx == nil {
		return input
	}
	return runtimeTemplatePattern.ReplaceAllStringFunc(input, func(match string) string {
		parts := runtimeTemplatePattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		value, ok := ctx.Get(parts[1])
		if !ok {
			return match
		}
		return valueForPrompt(value)
	})
}

func parseDynamicHumanInputRequest(out Value) (*AwaitRequest, bool, error) {
	text := valueText(out)
	m := dynamicHumanInputBlockPattern.FindStringSubmatch(text)
	if m == nil {
		return nil, false, nil
	}
	var req AwaitRequest
	payload := strings.TrimSpace(m[1])
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		repaired := repairMissingJSONClosers(payload)
		if repaired == payload || json.Unmarshal([]byte(repaired), &req) != nil {
			return nil, true, fmt.Errorf("tt-human-input block must be valid JSON: %w", err)
		}
	}
	if strings.TrimSpace(req.Type) == "" {
		req.Type = string(KindHumanInput)
	}
	if strings.TrimSpace(req.Reason) == "" && req.Form == nil {
		return nil, true, fmt.Errorf("tt-human-input request must include reason or form")
	}
	return &req, true, nil
}

func repairMissingJSONClosers(input string) string {
	if strings.TrimSpace(input) == "" {
		return input
	}
	var out strings.Builder
	out.Grow(len(input) + 8)
	stack := make([]rune, 0, 8)
	inString := false
	escaped := false
	changed := false
	for _, r := range input {
		if inString {
			out.WriteRune(r)
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			for len(stack) > 0 && stack[len(stack)-1] != r {
				out.WriteRune(stack[len(stack)-1])
				stack = stack[:len(stack)-1]
				changed = true
			}
			if len(stack) > 0 && stack[len(stack)-1] == r {
				stack = stack[:len(stack)-1]
			}
		}
		out.WriteRune(r)
	}
	for len(stack) > 0 {
		out.WriteRune(stack[len(stack)-1])
		stack = stack[:len(stack)-1]
		changed = true
	}
	if !changed {
		return input
	}
	return out.String()
}

func valueText(out Value) string {
	var text string
	if err := json.Unmarshal(out.Raw, &text); err == nil {
		return text
	}
	return strings.TrimSpace(string(out.Raw))
}

type ScriptStep struct {
	Base
	Command    []string
	Cwd        string
	Env        map[string]string
	OutputKey  string
	Validation *OutputValidationSpec `json:"validate,omitempty"`
}
type ScriptDecoder struct{}

func (ScriptDecoder) Kind() Kind { return KindScript }
func (ScriptDecoder) Decode(decl ast.StepDecl) (Step, error) {
	var s ScriptStep
	_ = json.Unmarshal(decl.Raw, &s)
	s.Base = Base{metadataFromDecl(decl, KindScript)}
	return s, nil
}
func (s ScriptStep) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	if req.Capabilities.Scripts == nil {
		err := &StepError{Message: "script capability is required"}
		return &RunResult{Status: StatusFailed, Error: err}, err
	}
	out, err := req.Capabilities.Scripts.RunScript(ctx, ScriptRequest{Command: renderContextTemplateSlice(s.Command, req.Context), Cwd: renderStepCwd(s.Cwd, req.Context), Env: renderScriptEnv(s.Env, req.Context)})
	if err != nil {
		return &RunResult{Status: StatusFailed, Output: out, Error: &StepError{Message: "script step failed", Cause: err}}, err
	}
	return &RunResult{Status: StatusCompleted, Output: out}, nil
}

func renderStepCwd(cwd string, ctx ContextView) string {
	rendered := strings.TrimSpace(renderContextTemplates(cwd, ctx))
	if rendered != "" {
		return rendered
	}
	if ctx == nil {
		return ""
	}
	value, ok := ctx.Get("env.cwd")
	if !ok {
		return ""
	}
	return strings.TrimSpace(valueForPrompt(value))
}

func renderScriptEnv(values map[string]string, ctx ContextView) map[string]string {
	out := defaultRuntimeEnv(ctx)
	for key, value := range renderContextTemplateMap(values, ctx) {
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func defaultRuntimeEnv(ctx ContextView) map[string]string {
	if ctx == nil {
		return nil
	}
	out := map[string]string{}
	add := func(envName, path string) {
		if value, ok := ctx.Get(path); ok {
			if text := strings.TrimSpace(valueForPrompt(value)); text != "" {
				out[envName] = text
			}
		}
	}
	add("TT_INVOCATION_CWD", "env.invocation_cwd")
	add("TT_WORKSPACE_CWD", "env.workspace_cwd")
	add("TT_FORMULA_RUN_DIR", "env.formula_run_dir")
	return out
}

func renderContextTemplateSlice(values []string, ctx ContextView) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = renderContextTemplates(value, ctx)
	}
	return out
}

func renderContextTemplateMap(values map[string]string, ctx ContextView) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = renderContextTemplates(value, ctx)
	}
	return out
}

type HumanInputStep struct {
	Base
	Reason     string
	Form       any
	OutputKey  string
	Validation *OutputValidationSpec `json:"validate,omitempty"`
}
type HumanInputDecoder struct{}

func (HumanInputDecoder) Kind() Kind { return KindHumanInput }
func (HumanInputDecoder) Decode(decl ast.StepDecl) (Step, error) {
	var s HumanInputStep
	_ = json.Unmarshal(decl.Raw, &s)
	s.Base = Base{metadataFromDecl(decl, KindHumanInput)}
	return s, nil
}
func (s HumanInputStep) Run(context.Context, RunRequest) (*RunResult, error) {
	return &RunResult{Status: StatusWaiting, Await: &AwaitRequest{Type: string(KindHumanInput), Reason: s.Reason, Form: s.Form}}, nil
}

type AggregateStep struct {
	Base
	Source     string
	As         string
	Require    []string
	Include    []string
	Exclude    []string
	Flatten    bool
	OutputKey  string
	Validation *OutputValidationSpec `json:"validate,omitempty"`
}

func (s AggregateStep) Run(_ context.Context, req RunRequest) (*RunResult, error) {
	if req.Context == nil {
		err := fmt.Errorf("aggregate context is required")
		return &RunResult{Status: StatusFailed, Error: &StepError{Message: err.Error(), Cause: err}}, err
	}
	source := strings.TrimSpace(s.Source)
	if source == "" {
		err := fmt.Errorf("aggregate source is required")
		return &RunResult{Status: StatusFailed, Error: &StepError{Message: err.Error(), Cause: err}}, err
	}
	value, ok := req.Context.Get(source)
	if !ok {
		err := fmt.Errorf("aggregate source %q not found", source)
		return &RunResult{Status: StatusFailed, Error: &StepError{Message: err.Error(), Cause: err}}, err
	}
	var data any
	if err := json.Unmarshal(value.Raw, &data); err != nil {
		err = fmt.Errorf("aggregate source %q must be JSON: %w", source, err)
		return &RunResult{Status: StatusFailed, Error: &StepError{Message: err.Error(), Cause: err}}, err
	}

	items := aggregateCollect(data, s.Require)
	projected := make([]map[string]any, 0, len(items))
	for _, item := range items {
		projected = append(projected, aggregateProject(item, s.Include, s.Exclude))
	}
	var out any = projected
	if as := strings.TrimSpace(s.As); as != "" {
		out = map[string]any{as: projected}
	} else if s.Flatten && len(projected) == 1 {
		out = projected[0]
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return &RunResult{Status: StatusFailed, Error: &StepError{Message: err.Error(), Cause: err}}, err
	}
	return &RunResult{Status: StatusCompleted, Output: Value{Type: "json", Raw: raw}}, nil
}

func aggregateCollect(value any, required []string) []map[string]any {
	requiredSet := compactStringSet(required)
	var out []map[string]any
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case []any:
			for _, item := range x {
				walk(item)
			}
		case map[string]any:
			if objectHasKeys(x, requiredSet) {
				out = append(out, x)
			}
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(value)
	return out
}

func aggregateProject(item map[string]any, include, exclude []string) map[string]any {
	out := map[string]any{}
	includeSet := compactStringSet(include)
	excludeSet := compactStringSet(exclude)
	if len(includeSet) > 0 {
		for key := range includeSet {
			if value, ok := item[key]; ok {
				out[key] = value
			}
		}
		return out
	}
	for key, value := range item {
		if !excludeSet[key] {
			out[key] = value
		}
	}
	return out
}

func compactStringSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func objectHasKeys(object map[string]any, keys map[string]bool) bool {
	if len(keys) == 0 {
		return true
	}
	for key := range keys {
		if _, ok := object[key]; !ok {
			return false
		}
	}
	return true
}

type WriteFilesStep struct {
	Base
	Source      string
	Root        string
	DirName     string
	FilenameKey string
	TitleKey    string
	SummaryKey  string
	ContentKey  string
	OutputKey   string
	Validation  *OutputValidationSpec `json:"validate,omitempty"`
}

type ToolStep struct {
	Base
	Name        string
	WriteFiles  *WriteFilesStep
	Sleep       *SleepStep
	GitFetch    *GitFetchStep
	GitPush     *GitPushStep
	GitBranch   *GitBranchStep
	GitCheckout *GitCheckoutStep
	GitWorktree *GitWorktreeStep
	OutputKey   string
	Validation  *OutputValidationSpec `json:"validate,omitempty"`
}

func (s ToolStep) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	switch strings.TrimSpace(s.Name) {
	case "write_files":
		if s.WriteFiles == nil {
			return failedRun(fmt.Errorf("tool write_files config is required"))
		}
		child := *s.WriteFiles
		if child.OutputKey == "" {
			child.OutputKey = s.OutputKey
		}
		child.Validation = s.Validation
		return child.Run(ctx, req)
	case "sleep":
		if s.Sleep == nil {
			return failedRun(fmt.Errorf("tool sleep config is required"))
		}
		child := *s.Sleep
		child.Validation = s.Validation
		return child.Run(ctx, req)
	case "git_fetch":
		if s.GitFetch == nil {
			return failedRun(fmt.Errorf("tool git_fetch config is required"))
		}
		return s.GitFetch.Run(ctx, req)
	case "git_push":
		if s.GitPush == nil {
			return failedRun(fmt.Errorf("tool git_push config is required"))
		}
		return s.GitPush.Run(ctx, req)
	case "git_branch":
		if s.GitBranch == nil {
			return failedRun(fmt.Errorf("tool git_branch config is required"))
		}
		return s.GitBranch.Run(ctx, req)
	case "git_checkout":
		if s.GitCheckout == nil {
			return failedRun(fmt.Errorf("tool git_checkout config is required"))
		}
		return s.GitCheckout.Run(ctx, req)
	case "git_worktree":
		if s.GitWorktree == nil {
			return failedRun(fmt.Errorf("tool git_worktree config is required"))
		}
		return s.GitWorktree.Run(ctx, req)
	default:
		return failedRun(fmt.Errorf("unknown tool %q", s.Name))
	}
}

type GitFetchStep struct {
	Remote string
	Prune  bool
	Tags   bool
	All    bool
}

type GitPushStep struct {
	Remote         string
	Branch         string
	Refspec        string
	SetUpstream    bool
	ForceWithLease bool
	Tags           bool
}

type GitBranchStep struct {
	Name        string
	StartPoint  string
	All         bool
	Delete      string
	ForceDelete string
}

type GitCheckoutStep struct {
	Branch     string
	Create     bool
	StartPoint string
}

type GitWorktreeStep struct {
	Path        string
	Branch      string
	StartPoint  string
	SparseMode  string
	Create      bool
	Detach      bool
	Force       bool
	List        bool
	Porcelain   bool
	Remove      string
	Prune       bool
	SparsePaths []string
}

func (s GitFetchStep) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	argv, err := buildGitFetchCommand(s)
	return runGitTool(ctx, req, argv, err)
}

func (s GitPushStep) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	argv, err := buildGitPushCommand(s)
	return runGitTool(ctx, req, argv, err)
}

func (s GitBranchStep) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	argv, err := buildGitBranchCommand(s)
	return runGitTool(ctx, req, argv, err)
}

func (s GitCheckoutStep) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	argv, err := buildGitCheckoutCommand(s)
	return runGitTool(ctx, req, argv, err)
}

func (s GitWorktreeStep) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	commands, err := buildGitWorktreeCommands(s)
	return runGitToolCommands(ctx, req, commands, err)
}

func buildGitFetchCommand(s GitFetchStep) ([]string, error) {
	args := []string{"git", "fetch"}
	if s.All {
		args = append(args, "--all")
	} else if remote := strings.TrimSpace(s.Remote); remote != "" {
		args = append(args, remote)
	}
	if s.Prune {
		args = append(args, "--prune")
	}
	if s.Tags {
		args = append(args, "--tags")
	}
	return args, nil
}

func buildGitPushCommand(s GitPushStep) ([]string, error) {
	args := []string{"git", "push"}
	if s.SetUpstream {
		args = append(args, "--set-upstream")
	}
	if s.ForceWithLease {
		args = append(args, "--force-with-lease")
	}
	if s.Tags {
		args = append(args, "--tags")
	}
	if remote := strings.TrimSpace(s.Remote); remote != "" {
		args = append(args, remote)
	}
	if refspec := strings.TrimSpace(s.Refspec); refspec != "" {
		args = append(args, refspec)
	} else if branch := strings.TrimSpace(s.Branch); branch != "" {
		args = append(args, branch)
	}
	return args, nil
}

func buildGitBranchCommand(s GitBranchStep) ([]string, error) {
	args := []string{"git", "branch"}
	if target := strings.TrimSpace(s.Delete); target != "" {
		return append(args, "-d", target), nil
	}
	if target := strings.TrimSpace(s.ForceDelete); target != "" {
		return append(args, "-D", target), nil
	}
	if s.All {
		args = append(args, "--all")
	}
	if name := strings.TrimSpace(s.Name); name != "" {
		args = append(args, name)
		if start := strings.TrimSpace(s.StartPoint); start != "" {
			args = append(args, start)
		}
	}
	return args, nil
}

func buildGitCheckoutCommand(s GitCheckoutStep) ([]string, error) {
	branch := strings.TrimSpace(s.Branch)
	if branch == "" {
		return nil, fmt.Errorf("git_checkout branch is required")
	}
	args := []string{"git", "checkout"}
	if s.Create {
		args = append(args, "-b")
	}
	args = append(args, branch)
	if start := strings.TrimSpace(s.StartPoint); start != "" {
		args = append(args, start)
	}
	return args, nil
}

func buildGitWorktreeCommand(s GitWorktreeStep) ([]string, error) {
	commands, err := buildGitWorktreeCommands(s)
	if err != nil {
		return nil, err
	}
	if len(commands) == 0 {
		return nil, fmt.Errorf("git_worktree command is required")
	}
	return commands[0], nil
}

func buildGitWorktreeCommands(s GitWorktreeStep) ([][]string, error) {
	args := []string{"git", "worktree"}
	if s.Prune {
		return [][]string{append(args, "prune")}, nil
	}
	if s.List {
		args = append(args, "list")
		if s.Porcelain {
			args = append(args, "--porcelain")
		}
		return [][]string{args}, nil
	}
	if target := strings.TrimSpace(s.Remove); target != "" {
		args = append(args, "remove")
		if s.Force {
			args = append(args, "--force")
		}
		return [][]string{append(args, target)}, nil
	}
	path := strings.TrimSpace(s.Path)
	if path == "" {
		return nil, fmt.Errorf("git_worktree path is required")
	}
	sparsePaths := compactStrings(s.SparsePaths)
	args = append(args, "add")
	if s.Force {
		args = append(args, "--force")
	}
	if len(sparsePaths) > 0 {
		args = append(args, "--no-checkout")
	}
	if s.Detach {
		args = append(args, "--detach")
	}
	branch := strings.TrimSpace(s.Branch)
	if branch != "" && s.Create {
		args = append(args, "-b", branch)
	}
	args = append(args, path)
	if start := strings.TrimSpace(s.StartPoint); start != "" {
		args = append(args, start)
	} else if branch != "" && !s.Create {
		args = append(args, branch)
	}
	if len(sparsePaths) == 0 {
		return [][]string{args}, nil
	}
	sparseArgs := []string{"git", "-C", path, "sparse-checkout", "set"}
	switch strings.TrimSpace(s.SparseMode) {
	case "", "cone":
		sparseArgs = append(sparseArgs, "--cone")
	case "no-cone":
		sparseArgs = append(sparseArgs, "--no-cone")
	default:
		return nil, fmt.Errorf("git_worktree sparse_mode must be cone or no-cone")
	}
	sparseArgs = append(sparseArgs, sparsePaths...)
	checkoutArgs := []string{"git", "-C", path, "checkout"}
	return [][]string{args, sparseArgs, checkoutArgs}, nil
}

func runGitTool(ctx context.Context, req RunRequest, argv []string, buildErr error) (*RunResult, error) {
	return runGitToolCommands(ctx, req, [][]string{argv}, buildErr)
}

func runGitToolCommands(ctx context.Context, req RunRequest, commands [][]string, buildErr error) (*RunResult, error) {
	if buildErr != nil {
		return failedRun(buildErr)
	}
	if len(commands) == 0 {
		return failedRun(fmt.Errorf("git command is required"))
	}
	results := make([]map[string]any, 0, len(commands))
	for _, argv := range commands {
		if len(argv) == 0 {
			return failedRun(fmt.Errorf("git command is required"))
		}
		result, err := runOneGitToolCommand(ctx, req, argv)
		results = append(results, result)
		if err != nil {
			raw, marshalErr := json.Marshal(map[string]any{"commands": results})
			if marshalErr != nil {
				return failedRun(marshalErr)
			}
			res := &RunResult{Status: StatusFailed, Output: Value{Type: "json", Raw: raw}, Error: &StepError{Message: "git tool failed", Cause: err}}
			return res, err
		}
	}
	raw, err := json.Marshal(map[string]any{"command": results[len(results)-1]["command"], "commands": results, "exit_code": 0, "stdout": results[len(results)-1]["stdout"], "stderr": results[len(results)-1]["stderr"]})
	if err != nil {
		return failedRun(err)
	}
	return &RunResult{Status: StatusCompleted, Output: Value{Type: "json", Raw: raw}}, nil
}

func runOneGitToolCommand(ctx context.Context, req RunRequest, argv []string) (map[string]any, error) {
	rendered := renderContextTemplateSlice(argv, req.Context)
	cmd := exec.CommandContext(ctx, rendered[0], rendered[1:]...)
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
	return map[string]any{"command": rendered, "exit_code": exitCode, "stdout": stdout.String(), "stderr": stderr.String()}, err
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

type SleepStep struct {
	Duration   string
	Seconds    int
	Validation *OutputValidationSpec `json:"validate,omitempty"`
}

func (s SleepStep) Run(ctx context.Context, _ RunRequest) (*RunResult, error) {
	duration, err := s.sleepDuration()
	if err != nil {
		return failedRun(err)
	}
	if duration > 0 {
		timer := time.NewTimer(duration)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return failedRun(ctx.Err())
		case <-timer.C:
		}
	}
	raw, err := json.Marshal(map[string]any{"slept_ms": duration.Milliseconds(), "duration": duration.String()})
	if err != nil {
		return failedRun(err)
	}
	return &RunResult{Status: StatusCompleted, Output: Value{Type: "json", Raw: raw}}, nil
}

func (s SleepStep) sleepDuration() (time.Duration, error) {
	if strings.TrimSpace(s.Duration) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(s.Duration))
		if err != nil {
			return 0, fmt.Errorf("sleep duration is invalid: %w", err)
		}
		if d < 0 {
			return 0, fmt.Errorf("sleep duration must be non-negative")
		}
		return d, nil
	}
	if s.Seconds < 0 {
		return 0, fmt.Errorf("sleep seconds must be non-negative")
	}
	return time.Duration(s.Seconds) * time.Second, nil
}

func (s WriteFilesStep) Run(_ context.Context, req RunRequest) (*RunResult, error) {
	if req.Context == nil {
		return failedRun(fmt.Errorf("write_files context is required"))
	}
	source := strings.TrimSpace(s.Source)
	if source == "" {
		return failedRun(fmt.Errorf("write_files source is required"))
	}
	value, ok := req.Context.Get(source)
	if !ok {
		return failedRun(fmt.Errorf("write_files source %q not found", source))
	}
	var data any
	if err := json.Unmarshal(value.Raw, &data); err != nil {
		return failedRun(fmt.Errorf("write_files source %q must be JSON: %w", source, err))
	}
	data = normalizeJSONTextValue(data)
	filenameKey := defaultString(s.FilenameKey, "filename")
	titleKey := defaultString(s.TitleKey, "title")
	summaryKey := defaultString(s.SummaryKey, "summary")
	contentKey := defaultString(s.ContentKey, "content")
	entries := collectWriteFileEntries(data, filenameKey, titleKey, summaryKey, contentKey)
	if len(entries) == 0 {
		return failedRun(fmt.Errorf("write_files source %q contains no objects with %s and %s", source, filenameKey, contentKey))
	}
	root := strings.TrimSpace(renderContextTemplates(defaultString(s.Root, "docs"), req.Context))
	dirName := strings.TrimSpace(renderContextTemplates(s.DirName, req.Context))
	if dirName == "" {
		return failedRun(fmt.Errorf("write_files dir_name is required"))
	}
	if !safePathSegment(dirName) {
		return failedRun(fmt.Errorf("write_files dir_name %q is unsafe", dirName))
	}
	dir := filepath.Join(root, dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return failedRun(fmt.Errorf("create directory %s: %w", dir, err))
	}
	written := make([]map[string]string, 0, len(entries))
	for _, entry := range entries {
		filename := strings.TrimSpace(entry[filenameKey])
		if !safeMarkdownFilename(filename) {
			return failedRun(fmt.Errorf("write_files filename %q is unsafe", filename))
		}
		path := filepath.Join(dir, filename)
		if err := os.WriteFile(path, []byte(strings.TrimRight(entry[contentKey], "\n")+"\n"), 0o644); err != nil {
			return failedRun(fmt.Errorf("write file %s: %w", path, err))
		}
		written = append(written, map[string]string{"filename": filename, "title": entry[titleKey], "summary": entry[summaryKey], "path": path})
	}
	raw, err := json.Marshal(map[string]any{"directory": dir, "root": root, "dir_name": dirName, "files": written})
	if err != nil {
		return failedRun(err)
	}
	return &RunResult{Status: StatusCompleted, Output: Value{Type: "json", Raw: raw}}, nil
}

func failedRun(err error) (*RunResult, error) {
	return &RunResult{Status: StatusFailed, Error: &StepError{Message: err.Error(), Cause: err}}, err
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func collectWriteFileEntries(value any, filenameKey, titleKey, summaryKey, contentKey string) []map[string]string {
	var out []map[string]string
	var walk func(any)
	walk = func(v any) {
		v = normalizeJSONTextValue(v)
		switch x := v.(type) {
		case []any:
			for _, item := range x {
				walk(item)
			}
		case map[string]any:
			filename, fok := stringValue(x[filenameKey])
			content, cok := stringValue(x[contentKey])
			if fok && cok {
				title, _ := stringValue(x[titleKey])
				summary, _ := stringValue(x[summaryKey])
				out = append(out, map[string]string{filenameKey: filename, titleKey: title, summaryKey: summary, contentKey: content})
			}
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(value)
	return out
}

func normalizeJSONTextValue(value any) any {
	switch x := value.(type) {
	case string:
		text := strings.TrimSpace(x)
		if text == "" {
			return value
		}
		for _, candidate := range jsonContainerTextCandidates(text) {
			var decoded any
			if err := json.Unmarshal([]byte(candidate), &decoded); err == nil {
				return normalizeJSONTextValue(decoded)
			}
		}
	case []any:
		for i, item := range x {
			x[i] = normalizeJSONTextValue(item)
		}
		return x
	case map[string]any:
		for key, item := range x {
			x[key] = normalizeJSONTextValue(item)
		}
		return x
	}
	return value
}

func jsonContainerTextCandidates(text string) []string {
	candidates := []string{text}
	if fenced, ok := extractFencedJSONText(text); ok {
		candidates = append(candidates, fenced)
	}
	if extracted, ok := extractFirstJSONContainerText(text); ok {
		candidates = append(candidates, extracted)
	}
	return candidates
}

func extractFirstJSONContainerText(text string) (string, bool) {
	arrayStart := strings.Index(text, "[")
	objectStart := strings.Index(text, "{")
	start := -1
	close := byte(0)
	switch {
	case arrayStart >= 0 && (objectStart < 0 || arrayStart < objectStart):
		start, close = arrayStart, ']'
	case objectStart >= 0:
		start, close = objectStart, '}'
	default:
		return "", false
	}
	end := strings.LastIndexByte(text, close)
	if end <= start {
		return "", false
	}
	return strings.TrimSpace(text[start : end+1]), true
}

func stringValue(value any) (string, bool) {
	s, ok := value.(string)
	return s, ok && strings.TrimSpace(s) != ""
}

func safePathSegment(value string) bool {
	return value != "" && !strings.Contains(value, "/") && !strings.Contains(value, "\\") && !strings.HasPrefix(value, ".")
}

func safeMarkdownFilename(filename string) bool {
	return safePathSegment(filename) && strings.HasSuffix(filename, ".md")
}

type LoopStep struct {
	Base
	Body           []Step
	Parallel       bool
	MaxConcurrency int
	ForEach        string
	Var            string
	Until          string
	Max            int
}
type LoopDecoder struct{}

func (LoopDecoder) Kind() Kind { return KindLoop }
func (LoopDecoder) Decode(decl ast.StepDecl) (Step, error) {
	return LoopStep{Base: Base{metadataFromDecl(decl, KindLoop)}}, nil
}

func (s LoopStep) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	if strings.TrimSpace(s.ForEach) != "" {
		return s.runForEach(ctx, req)
	}
	max := s.Max
	if max <= 0 {
		max = 1
	}
	var last Value
	for i := 1; i <= max; i++ {
		if req.Outputs != nil {
			raw, _ := json.Marshal(i)
			_ = req.Outputs.Set("iteration", Value{Type: "json", Raw: raw})
		}
		for _, child := range s.Body {
			if child == nil {
				continue
			}
			childNodeID := loopChildNodeID(req.NodeID, i, child.Meta().ID)
			if !stepConditionMatches(child.Meta().Condition, req.Context) {
				if req.Emit != nil {
					req.Emit(childNodeID, "step.skipped", skippedRun("loop body condition evaluated to false"))
				}
				continue
			}
			exec, ok := child.(Executable)
			if !ok {
				continue
			}
			if req.Emit != nil {
				req.Emit(childNodeID, "step.started", nil)
			}
			res, err := exec.Run(ctx, RunRequest{RunID: req.RunID, NodeID: childNodeID, Step: child, Context: req.Context, Outputs: req.Outputs, Capabilities: req.Capabilities, Emit: req.Emit})
			if res == nil {
				res = &RunResult{}
			}
			if err != nil || res.Status == StatusFailed {
				if req.Emit != nil {
					req.Emit(childNodeID, "step.failed", res)
				}
				return res, err
			}
			if res.Status == StatusWaiting {
				if req.Emit != nil {
					req.Emit(childNodeID, "step.waiting", res.Await)
				}
				return res, nil
			}
			if req.Emit != nil {
				req.Emit(childNodeID, "step.completed", res)
			}
			if len(res.Output.Raw) > 0 {
				last = res.Output
				if req.Outputs != nil {
					_ = req.Outputs.Set(string(child.Meta().ID), res.Output)
				}
			}
		}
		if stepConditionMatches(s.Until, req.Context) {
			return &RunResult{Status: StatusCompleted, Output: last}, nil
		}
	}
	return &RunResult{Status: StatusCompleted, Output: last}, nil
}

func (s LoopStep) runForEach(ctx context.Context, req RunRequest) (*RunResult, error) {
	if req.Context == nil {
		return failedRun(fmt.Errorf("loop context is required"))
	}
	source := strings.TrimSpace(s.ForEach)
	value, ok := req.Context.Get(source)
	if !ok {
		return failedRun(fmt.Errorf("loop for_each source %q not found", source))
	}
	items, err := decodeJSONArrayValue(value.Raw)
	if err != nil {
		return failedRun(fmt.Errorf("loop for_each source %q must be a JSON array: %w", source, err))
	}
	varName := strings.TrimSpace(s.Var)
	if varName == "" {
		return failedRun(fmt.Errorf("loop var is required when for_each is set"))
	}
	if s.Parallel {
		return s.runForEachParallel(ctx, req, items, varName)
	}
	outputs := make([]any, 0, len(items))
	for i, item := range items {
		iteration := i + 1
		iterReq, err := loopIterationRequest(req, iteration, item, varName)
		if err != nil {
			return failedRun(err)
		}
		last, res, err := s.runBodyOnce(ctx, iterReq, iteration)
		if err != nil || res != nil {
			if res != nil && res.Status == StatusWaiting {
				return res, nil
			}
			return res, err
		}
		if len(last.Raw) > 0 {
			var decoded any
			if err := json.Unmarshal(last.Raw, &decoded); err == nil {
				decoded = normalizeJSONTextValue(decoded)
				outputs = append(outputs, decoded)
			}
		}
	}
	raw, err := json.Marshal(outputs)
	if err != nil {
		return failedRun(err)
	}
	return &RunResult{Status: StatusCompleted, Output: Value{Type: "json", Raw: raw}}, nil
}

type loopIterationResult struct {
	index int
	last  Value
	res   *RunResult
	err   error
}

func (s LoopStep) runForEachParallel(ctx context.Context, req RunRequest, items []any, varName string) (*RunResult, error) {
	limit := s.MaxConcurrency
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	if limit <= 0 {
		return &RunResult{Status: StatusCompleted, Output: Value{Type: "json", Raw: []byte(`[]`)}}, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sem := make(chan struct{}, limit)
	results := make(chan loopIterationResult, len(items))
	var wg sync.WaitGroup
	scheduled := 0
	scheduling := true
	for i, item := range items {
		select {
		case <-ctx.Done():
			scheduling = false
		case sem <- struct{}{}:
		}
		if !scheduling {
			break
		}
		scheduled++
		wg.Add(1)
		go func(index int, item any) {
			defer wg.Done()
			defer func() { <-sem }()
			iteration := index + 1
			iterReq, err := loopIterationRequest(req, iteration, item, varName)
			if err != nil {
				results <- loopIterationResult{index: index, err: err}
				cancel()
				return
			}
			last, res, err := s.runBodyOnce(ctx, iterReq, iteration)
			if err != nil || res != nil {
				cancel()
			}
			results <- loopIterationResult{index: index, last: last, res: res, err: err}
		}(i, item)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	lastByIndex := make([]Value, scheduled)
	for result := range results {
		if result.err != nil || result.res != nil {
			if result.res != nil && result.res.Status == StatusWaiting {
				return result.res, nil
			}
			return result.res, result.err
		}
		lastByIndex[result.index] = result.last
	}

	outputs := make([]any, 0, len(items))
	for _, last := range lastByIndex {
		if len(last.Raw) > 0 {
			var decoded any
			if err := json.Unmarshal(last.Raw, &decoded); err == nil {
				decoded = normalizeJSONTextValue(decoded)
				outputs = append(outputs, decoded)
			}
		}
	}
	raw, err := json.Marshal(outputs)
	if err != nil {
		return failedRun(err)
	}
	return &RunResult{Status: StatusCompleted, Output: Value{Type: "json", Raw: raw}}, nil
}

func loopIterationRequest(req RunRequest, iteration int, item any, varName string) (RunRequest, error) {
	iterationRaw, _ := json.Marshal(iteration)
	itemRaw, err := json.Marshal(item)
	if err != nil {
		return RunRequest{}, fmt.Errorf("loop item %d must be JSON: %w", iteration, err)
	}
	store := newLoopIterationStore(req.Context)
	_ = store.Set("iteration", Value{Type: "json", Raw: iterationRaw})
	_ = store.Set(varName, Value{Type: "json", Raw: itemRaw})
	req.Context = store
	req.Outputs = store
	return req, nil
}

type loopIterationStore struct {
	parent ContextView
	mu     sync.RWMutex
	values map[string]Value
}

func newLoopIterationStore(parent ContextView) *loopIterationStore {
	return &loopIterationStore{parent: parent, values: map[string]Value{}}
}

func (s *loopIterationStore) Get(path string) (Value, bool) {
	s.mu.RLock()
	value, ok := s.values[path]
	s.mu.RUnlock()
	if ok {
		return value, true
	}
	if value, fieldPath, ok := s.longestStoredPrefixValue(path); ok {
		return getNestedJSONValue(value, fieldPath)
	}
	if root, rest, split := strings.Cut(path, "."); split {
		s.mu.RLock()
		value, ok = s.values[root]
		s.mu.RUnlock()
		if ok {
			return getNestedJSONValue(value, rest)
		}
	}
	if s.parent == nil {
		return Value{}, false
	}
	return s.parent.Get(path)
}

func (s *loopIterationStore) longestStoredPrefixValue(path string) (Value, string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	best := ""
	for key := range s.values {
		if key == "" || len(key) <= len(best) {
			continue
		}
		if strings.HasPrefix(path, key+".") {
			best = key
		}
	}
	if best == "" {
		return Value{}, "", false
	}
	return s.values[best], strings.TrimPrefix(path, best+"."), true
}

func (s *loopIterationStore) Set(path string, value Value) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[path] = value
	return nil
}

func getNestedJSONValue(value Value, path string) (Value, bool) {
	var current any
	if err := json.Unmarshal(value.Raw, &current); err != nil {
		return Value{}, false
	}
	current = normalizeJSONTextValue(current)
	for _, part := range strings.Split(path, ".") {
		current = normalizeJSONTextValue(current)
		object, ok := current.(map[string]any)
		if !ok {
			return Value{}, false
		}
		current, ok = object[part]
		if !ok {
			return Value{}, false
		}
	}
	raw, err := json.Marshal(current)
	if err != nil {
		return Value{}, false
	}
	return Value{Type: "json", Raw: raw}, true
}

func decodeJSONArrayValue(raw []byte) ([]any, error) {
	var items []any
	if err := json.Unmarshal(raw, &items); err == nil {
		return items, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("empty JSON string")
	}
	for _, candidate := range jsonArrayTextCandidates(text) {
		items = nil
		if err := json.Unmarshal([]byte(candidate), &items); err == nil {
			return items, nil
		}
	}
	return nil, fmt.Errorf("JSON string did not contain an array")
}

func jsonArrayTextCandidates(text string) []string {
	candidates := []string{text}
	if fenced, ok := extractFencedJSONText(text); ok {
		candidates = append(candidates, fenced)
	}
	if extracted, ok := extractFirstJSONArrayText(text); ok {
		candidates = append(candidates, extracted)
	}
	return candidates
}

func extractFencedJSONText(text string) (string, bool) {
	start := strings.Index(text, "```")
	if start < 0 {
		return "", false
	}
	rest := text[start+3:]
	if newline := strings.Index(rest, "\n"); newline >= 0 {
		rest = rest[newline+1:]
	}
	end := strings.Index(rest, "```")
	if end < 0 {
		return "", false
	}
	return strings.TrimSpace(rest[:end]), true
}

func extractFirstJSONArrayText(text string) (string, bool) {
	start := strings.Index(text, "[")
	if start < 0 {
		return "", false
	}
	end := strings.LastIndex(text, "]")
	if end <= start {
		return "", false
	}
	return strings.TrimSpace(text[start : end+1]), true
}

func (s LoopStep) runBodyOnce(ctx context.Context, req RunRequest, iteration int) (Value, *RunResult, error) {
	var last Value
	for _, child := range s.Body {
		if child == nil {
			continue
		}
		childNodeID := loopChildNodeID(req.NodeID, iteration, child.Meta().ID)
		if !stepConditionMatches(child.Meta().Condition, req.Context) {
			if req.Emit != nil {
				req.Emit(childNodeID, "step.skipped", skippedRun("loop body condition evaluated to false"))
			}
			continue
		}
		exec, ok := child.(Executable)
		if !ok {
			continue
		}
		if req.Emit != nil {
			req.Emit(childNodeID, "step.started", nil)
		}
		res, err := exec.Run(ctx, RunRequest{RunID: req.RunID, NodeID: childNodeID, Step: child, Context: req.Context, Outputs: req.Outputs, Capabilities: req.Capabilities, Emit: req.Emit})
		if res == nil {
			res = &RunResult{}
		}
		if err != nil || res.Status == StatusFailed {
			if req.Emit != nil {
				req.Emit(childNodeID, "step.failed", res)
			}
			return last, res, err
		}
		if res.Status == StatusWaiting {
			if req.Emit != nil {
				req.Emit(childNodeID, "step.waiting", res.Await)
			}
			return last, res, nil
		}
		if req.Emit != nil {
			req.Emit(childNodeID, "step.completed", res)
		}
		if len(res.Output.Raw) > 0 {
			last = res.Output
			if req.Outputs != nil {
				_ = req.Outputs.Set(string(child.Meta().ID), res.Output)
			}
		}
	}
	return last, nil, nil
}

func loopChildNodeID(parentNodeID string, iteration int, childID ID) string {
	return fmt.Sprintf("%s.iter%d.%s", parentNodeID, iteration, childID)
}

func skippedRun(reason string) *RunResult {
	return &RunResult{Status: StatusSkipped, Error: &StepError{Message: reason}}
}

func stepConditionMatches(condition string, ctx ContextView) bool {
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return true
	}
	for _, op := range []string{"==", "!="} {
		parts := strings.SplitN(condition, op, 2)
		if len(parts) != 2 {
			continue
		}
		left := strings.TrimSpace(parts[0])
		expected := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		actual := ""
		if ctx != nil {
			if value, ok := ctx.Get(left); ok {
				actual = valueForPrompt(value)
			}
		}
		if op == "==" {
			return actual == expected
		}
		return actual != expected
	}
	return false
}

type RetryStep struct {
	Base
	MaxAttempts int
	Child       Step
}
type RetryDecoder struct{}

func (RetryDecoder) Kind() Kind { return KindRetry }
func (RetryDecoder) Decode(decl ast.StepDecl) (Step, error) {
	var s RetryStep
	_ = json.Unmarshal(decl.Raw, &s)
	s.Base = Base{metadataFromDecl(decl, KindRetry)}
	return s, nil
}

func metadataFromDecl(decl ast.StepDecl, kind Kind) Metadata {
	deps := make([]ID, 0, len(decl.DependsOn))
	for _, dep := range decl.DependsOn {
		deps = append(deps, ID(dep))
	}
	return Metadata{ID: ID(decl.ID), Kind: kind, Title: decl.Title, DependsOn: deps}
}
