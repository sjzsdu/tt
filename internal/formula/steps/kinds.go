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
	out, err := req.Capabilities.Agents.RunAgent(ctx, AgentRequest{NodeID: req.NodeID, Agent: s.Agent, Model: s.Model, Prompt: prompt})
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
	if err := json.Unmarshal([]byte(strings.TrimSpace(m[1])), &req); err != nil {
		return nil, true, fmt.Errorf("tt-human-input block must be valid JSON: %w", err)
	}
	if strings.TrimSpace(req.Type) == "" {
		req.Type = string(KindHumanInput)
	}
	if strings.TrimSpace(req.Reason) == "" && req.Form == nil {
		return nil, true, fmt.Errorf("tt-human-input request must include reason or form")
	}
	return &req, true, nil
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
	out, err := req.Capabilities.Scripts.RunScript(ctx, ScriptRequest{Command: renderContextTemplateSlice(s.Command, req.Context), Cwd: renderContextTemplates(s.Cwd, req.Context), Env: renderContextTemplateMap(s.Env, req.Context)})
	if err != nil {
		return &RunResult{Status: StatusFailed, Output: out, Error: &StepError{Message: "script step failed", Cause: err}}, err
	}
	return &RunResult{Status: StatusCompleted, Output: out}, nil
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

func runGitTool(ctx context.Context, req RunRequest, argv []string, buildErr error) (*RunResult, error) {
	if buildErr != nil {
		return failedRun(buildErr)
	}
	if len(argv) == 0 {
		return failedRun(fmt.Errorf("git command is required"))
	}
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
	raw, marshalErr := json.Marshal(map[string]any{"command": rendered, "exit_code": exitCode, "stdout": stdout.String(), "stderr": stderr.String()})
	if marshalErr != nil {
		return failedRun(marshalErr)
	}
	res := &RunResult{Status: StatusCompleted, Output: Value{Type: "json", Raw: raw}}
	if err != nil {
		res.Status = StatusFailed
		res.Error = &StepError{Message: "git tool failed", Cause: err}
		return res, err
	}
	return res, nil
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
	Until          string
	Max            int
}
type LoopDecoder struct{}

func (LoopDecoder) Kind() Kind { return KindLoop }
func (LoopDecoder) Decode(decl ast.StepDecl) (Step, error) {
	return LoopStep{Base: Base{metadataFromDecl(decl, KindLoop)}}, nil
}

func (s LoopStep) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
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
			if !stepConditionMatches(child.Meta().Condition, req.Context) {
				continue
			}
			exec, ok := child.(Executable)
			if !ok {
				continue
			}
			childNodeID := fmt.Sprintf("%s.iter%d.%s", req.NodeID, i, child.Meta().ID)
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
