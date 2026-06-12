package agentopt

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sjzsdu/tt/internal/agents"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	repo2skillpkg "github.com/sjzsdu/tt/internal/repo2skill"
)

type Collector interface {
	Collect(target string, opts repo2skillpkg.Options) (*repo2skillpkg.RepoProfile, func(), error)
}

type DirectProcessor interface {
	ProcessDirect(message string) (string, error)
}

type Options struct {
	Target         string
	BaseAgent      string
	OutputPath     string
	Force          bool
	Copy           bool
	OutputStdout   bool
	MaxFiles       int
	MaxFileSize    int64
	MaxPromptChars int
	Timeout        time.Duration
	KeepTemp       bool
}

type Optimizer struct {
	Collector Collector
	Processor DirectProcessor
}

type Result struct {
	Profile   *repo2skillpkg.RepoProfile
	BaseAgent pcwrap.EmbeddedAgent
	Generated pcwrap.EmbeddedAgent
	Markdown  string
	Output    string
	InPlace   bool
}

type agentOutput struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Soul      string   `json:"soul"`
	Skills    []string `json:"skills"`
	Tools     []string `json:"tools"`
	NoHistory bool     `json:"no_history"`
	Prompt    string   `json:"prompt"`
}

type repoCollector struct{}

func (repoCollector) Collect(target string, opts repo2skillpkg.Options) (*repo2skillpkg.RepoProfile, func(), error) {
	return repo2skillpkg.Collect(target, opts)
}

func New(processor DirectProcessor) Optimizer {
	return Optimizer{Collector: repoCollector{}, Processor: processor}
}

func (o Optimizer) Optimize(opts Options) (*Result, error) {
	if strings.TrimSpace(opts.Target) == "" {
		return nil, fmt.Errorf("target repository path or URL required")
	}
	if isWebsiteURL(opts.Target) {
		return nil, fmt.Errorf("website targets are not supported yet; provide a local path or cloneable repository URL")
	}
	base, basePath, err := resolveBaseAgent(opts.BaseAgent)
	if err != nil {
		return nil, err
	}
	collector := o.Collector
	if collector == nil {
		collector = repoCollector{}
	}
	profile, cleanup, err := collector.Collect(opts.Target, repo2skillpkg.Options{Intent: "agent-optimize", MaxFiles: opts.MaxFiles, MaxFileSize: opts.MaxFileSize, Timeout: opts.Timeout, KeepTemp: opts.KeepTemp})
	if err != nil {
		return nil, err
	}
	defer cleanup()
	if o.Processor == nil {
		return nil, fmt.Errorf("optimizer requires a direct processor")
	}
	maxPromptChars := opts.MaxPromptChars
	if maxPromptChars <= 0 {
		maxPromptChars = 12000
	}
	payload, err := json.MarshalIndent(buildRequest(profile, base, maxPromptChars), "", "  ")
	if err != nil {
		return nil, err
	}
	resp, err := o.Processor.ProcessDirect("Analyze this repository profile and return only the agent optimizer JSON object described in your instructions.\n\n" + string(payload))
	if err != nil {
		return nil, err
	}
	generated, err := parseAgentOutput(resp, base)
	if err != nil {
		return nil, err
	}
	if !opts.Copy {
		generated.ID = base.ID
		if strings.TrimSpace(generated.Name) == "" {
			generated.Name = base.Name
		}
		generated.Prompt = mergeOptimizedPrompt(base.Prompt, generated.Prompt, profile)
	}
	if err := validateGenerated(profile, base, generated, maxPromptChars); err != nil {
		return nil, err
	}
	markdown := RenderMarkdown(generated)
	result := &Result{Profile: profile, BaseAgent: base, Generated: generated, Markdown: markdown}
	if opts.OutputStdout || strings.TrimSpace(opts.OutputPath) == "" {
		if !opts.Copy && basePath == "" {
			return nil, fmt.Errorf("agent %q has no local markdown file to update; pass --copy to create a new local agent", base.ID)
		}
		if opts.Copy {
			if basePath == "" {
				basePath = filepath.Join(".tt", "agents", base.ID+".md")
			}
			result.Output = filepath.Join(filepath.Dir(basePath), generated.ID+".md")
		} else {
			result.Output = basePath
			result.InPlace = true
		}
		if err := writeOutput(result.Output, markdown, opts.Force || result.InPlace); err != nil {
			return nil, err
		}
		return result, nil
	}
	outputPath := opts.OutputPath
	if info, err := os.Stat(outputPath); err == nil && info.IsDir() {
		outputPath = filepath.Join(outputPath, generated.ID+".md")
	}
	if err := writeOutput(outputPath, markdown, opts.Force); err != nil {
		return nil, err
	}
	result.Output = outputPath
	return result, nil
}

func resolveBaseAgent(input string) (pcwrap.EmbeddedAgent, string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return pcwrap.EmbeddedAgent{}, "", fmt.Errorf("base agent id required")
	}
	if looksLikeMarkdownFile(trimmed) {
		agent, err := agents.LoadFromFile(trimmed)
		return agent, trimmed, err
	}
	agent, err := agents.Get(trimmed)
	if err != nil {
		return pcwrap.EmbeddedAgent{}, "", err
	}
	path, _ := agents.FilePathForID(trimmed)
	return agent, path, nil
}

func isWebsiteURL(s string) bool {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	if strings.HasSuffix(u.Path, ".git") || strings.Contains(u.Host, "github.com") || strings.Contains(u.Host, "gitlab.com") || strings.Contains(u.Host, "bitbucket.org") {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func looksLikeMarkdownFile(v string) bool {
	v = strings.TrimSpace(v)
	if filepath.Ext(v) != ".md" {
		return false
	}
	_, err := os.Stat(v)
	return err == nil
}

func buildRequest(profile *repo2skillpkg.RepoProfile, base pcwrap.EmbeddedAgent, maxPromptChars int) map[string]any {
	return map[string]any{
		"target_repo": compactProfile(profile),
		"optimization_policy": map[string]any{
			"target_kind":      "repository, library, application, service, CLI, website-backed project, or product codebase",
			"max_prompt_chars": maxPromptChars,
			"merge_strategy":   "preserve existing distilled repository/domain expertise and add this target's durable expertise; compress duplicate or generic guidance but do not drop earlier repositories unless explicitly obsolete",
			"dedupe":           "deduplicate repeated workflows, constraints, commands, and terminology while retaining repository-specific facts",
		},
		"base_agent": map[string]any{
			"id":         base.ID,
			"name":       base.Name,
			"soul":       base.Soul,
			"skills":     base.Skills,
			"tools":      base.Tools,
			"no_history": base.NoHistory,
			"prompt":     strings.TrimSpace(base.Prompt),
		},
	}
}

func compactProfile(p *repo2skillpkg.RepoProfile) map[string]any {
	if p == nil {
		return map[string]any{}
	}
	return map[string]any{
		"name":           p.Name,
		"source":         p.Source,
		"intent":         p.Intent,
		"languages":      p.Languages,
		"package_files":  limitSlice(p.PackageFiles, 20),
		"readmes":        compactDocs(limitSlice(p.Readmes, 6)),
		"docs":           compactDocs(limitSlice(p.Docs, 12)),
		"examples":       compactCode(limitSlice(p.Examples, 10)),
		"tests":          compactCode(limitSlice(p.Tests, 8)),
		"entrypoints":    limitSlice(p.EntryPoints, 30),
		"public_apis":    limitSlice(p.PublicAPIs, 80),
		"install_hints":  limitSlice(p.InstallHints, 20),
		"usage_snippets": limitSlice(p.UsageSnippets, 20),
	}
}

func compactDocs(in []repo2skillpkg.DocFile) []repo2skillpkg.DocFile {
	out := append([]repo2skillpkg.DocFile(nil), in...)
	for i := range out {
		if len(out[i].Content) > 4000 {
			out[i].Content = out[i].Content[:4000] + "\n..."
		}
	}
	return out
}

func compactCode(in []repo2skillpkg.CodeFile) []repo2skillpkg.CodeFile {
	out := append([]repo2skillpkg.CodeFile(nil), in...)
	for i := range out {
		if len(out[i].Content) > 2500 {
			out[i].Content = out[i].Content[:2500] + "\n..."
		}
	}
	return out
}

func mergeOptimizedPrompt(existing, generated string, profile *repo2skillpkg.RepoProfile) string {
	existing = strings.TrimSpace(existing)
	generated = strings.TrimSpace(generated)
	if existing == "" {
		return generated
	}
	if generated == "" || strings.Contains(existing, generated) {
		return existing
	}
	if strings.Contains(generated, existing) {
		return generated
	}
	repoName := "repository"
	if profile != nil && strings.TrimSpace(profile.Name) != "" {
		repoName = strings.TrimSpace(profile.Name)
	}
	section := fmt.Sprintf("## Repository Knowledge Distillation\n\n### %s\n%s", repoName, generated)
	if strings.Contains(existing, "## Repository Knowledge Distillation") {
		return existing + "\n\n### " + repoName + "\n" + generated
	}
	return existing + "\n\n" + section
}

func limitSlice[T any](in []T, n int) []T {
	if len(in) <= n {
		return append([]T(nil), in...)
	}
	return append([]T(nil), in[:n]...)
}

func parseAgentOutput(resp string, base pcwrap.EmbeddedAgent) (pcwrap.EmbeddedAgent, error) {
	var out agentOutput
	if err := json.Unmarshal([]byte(extractJSONObject(resp)), &out); err != nil {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("parse agent optimizer JSON: %w", err)
	}
	generated := pcwrap.EmbeddedAgent{
		ID:        normalizeID(out.ID, base.ID),
		Name:      firstNonEmpty(out.Name, strings.TrimSpace(base.Name)+" Optimized"),
		Prompt:    strings.TrimSpace(out.Prompt),
		Soul:      strings.TrimSpace(out.Soul),
		Skills:    compactStrings(out.Skills),
		Tools:     compactStrings(out.Tools),
		NoHistory: out.NoHistory,
	}
	if generated.Prompt == "" {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("generated agent prompt is empty")
	}
	if generated.Soul == "" {
		generated.Soul = strings.TrimSpace(base.Soul)
	}
	if len(generated.Skills) == 0 {
		generated.Skills = append([]string(nil), base.Skills...)
	}
	if len(generated.Tools) == 0 {
		generated.Tools = append([]string(nil), base.Tools...)
	}
	return generated, nil
}

func validateGenerated(profile *repo2skillpkg.RepoProfile, base, generated pcwrap.EmbeddedAgent, maxPromptChars int) error {
	prompt := strings.TrimSpace(generated.Prompt)
	if len(prompt) < 200 {
		return fmt.Errorf("generated agent prompt is too short to be useful")
	}
	if maxPromptChars > 0 && len(prompt) > maxPromptChars {
		return fmt.Errorf("generated agent prompt is %d chars, exceeds budget %d; rerun with a larger --max-prompt-chars or ask optimizer to compress", len(prompt), maxPromptChars)
	}
	if profile != nil && strings.TrimSpace(profile.Name) != "" && !strings.Contains(strings.ToLower(prompt), strings.ToLower(profile.Name)) {
		return fmt.Errorf("generated agent prompt does not mention target repository/domain %q", profile.Name)
	}
	allowed := map[string]bool{}
	for _, skill := range base.Skills {
		allowed[strings.TrimSpace(skill)] = true
	}
	for _, skill := range generated.Skills {
		skill = strings.TrimSpace(skill)
		if skill != "" && len(allowed) > 0 && !allowed[skill] {
			return fmt.Errorf("generated agent adds unknown skill %q; optimizer may only keep base skills", skill)
		}
	}
	return nil
}

func normalizeID(v string, base string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		v = base + "-optimized"
	}
	re := regexp.MustCompile(`[^a-z0-9._-]+`)
	v = re.ReplaceAllString(v, "-")
	v = strings.Trim(v, "-.")
	if v == "" {
		return "optimized-agent"
	}
	return v
}

func RenderMarkdown(agent pcwrap.EmbeddedAgent) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", agent.ID)
	fmt.Fprintf(&b, "name: %q\n", strings.TrimSpace(agent.Name))
	if strings.TrimSpace(agent.Soul) != "" {
		b.WriteString("soul: |\n")
		for _, line := range strings.Split(strings.TrimSpace(agent.Soul), "\n") {
			fmt.Fprintf(&b, "  %s\n", line)
		}
	}
	if len(agent.Skills) > 0 {
		b.WriteString("skills:\n")
		for _, skill := range agent.Skills {
			fmt.Fprintf(&b, "  - %q\n", strings.TrimSpace(skill))
		}
	}
	if len(agent.Tools) > 0 {
		b.WriteString("tools:\n")
		for _, tool := range agent.Tools {
			fmt.Fprintf(&b, "  - %q\n", strings.TrimSpace(tool))
		}
	}
	if agent.NoHistory {
		b.WriteString("no_history: true\n")
	}
	b.WriteString("---\n")
	b.WriteString(strings.TrimSpace(agent.Prompt))
	b.WriteString("\n")
	return b.String()
}

func writeOutput(path, content string, force bool) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("output path required")
	}
	if _, err := os.Stat(path); err == nil && !force {
		return fmt.Errorf("output file already exists: %s (use --force to overwrite)", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	return nil
}

func extractJSONObject(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 3 {
			lines = lines[1:]
			if strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
				lines = lines[:len(lines)-1]
			}
			s = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end >= start {
		return s[start : end+1]
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
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
