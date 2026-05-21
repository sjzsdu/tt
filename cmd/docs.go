package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/agents"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	ttconfig "github.com/sjzsdu/tt/internal/ttconfig"
)

var (
	docsAnalyzeOutput  string
	docsAnalyzeSession string
	docsAnalyzeModel   string
	docsAnalyzeDebug   bool
	docsAnalyzeHome    string
	docsAnalyzeConfig  string
	docsAnalyzeDryRun  bool
	docsAnalyzeTimeout time.Duration
	docsAnalyzeKeepTmp bool
)

type docsAnalyzeTarget struct {
	AnalysisDir string
	DisplayName string
	RepoName    string
	Remote      bool
	TempDir     string
}

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Analyze code and generate understanding-oriented documents",
	Long: `Analyze a codebase or subdirectory with the embedded docs-analyst agent
and generate high-value Chinese Markdown documents such as overview,
architecture, module analysis, flowcharts, or onboarding guides when they help
readers understand the code.`,
}

var docsAnalyzeCmd = &cobra.Command{
	Use:   "analyze [path-or-github]",
	Short: "Generate understanding-oriented documents for a code directory",
	Long: `Study the target directory or GitHub repository with the embedded
docs-analyst agent and write the most valuable Chinese Markdown documents into
an output directory.

By default, the command analyzes the current tt project root when available and
writes into <target>/ai-docs for local directories. For remote repositories, it
clones into a temporary directory and writes into ./ai-docs/<repo-name> unless
--output-dir is provided. Existing documents should be updated incrementally
instead of being blindly replaced.`,
	Args: cobra.MaximumNArgs(1),
	Example: `tt docs analyze
 tt docs analyze ./internal
 tt docs analyze github.com/owner/repo
 tt docs analyze https://github.com/owner/repo --dry-run
 tt docs analyze ./web --output-dir ./ai-docs/backend --model gpt-5.4 --debug`,
	RunE: runDocsAnalyze,
}

func init() {
	docsAnalyzeCmd.Flags().StringVarP(&docsAnalyzeOutput, "output-dir", "o", "", "directory to write generated documents (default: local=<analysis-dir>/ai-docs, remote=./ai-docs/<repo-name>)")
	docsAnalyzeCmd.Flags().StringVar(&docsAnalyzeSession, "session", "cli:docs", "session key for docs analysis")
	docsAnalyzeCmd.Flags().StringVar(&docsAnalyzeModel, "model", "", "model override for the embedded docs-analyst agent")
	docsAnalyzeCmd.Flags().BoolVarP(&docsAnalyzeDebug, "debug", "d", false, "enable debug logging")
	docsAnalyzeCmd.Flags().StringVar(&docsAnalyzeHome, "picoclaw-home", "", "override PICOCLAW_HOME for this run")
	docsAnalyzeCmd.Flags().StringVar(&docsAnalyzeConfig, "picoclaw-config", "", "override PICOCLAW_CONFIG for this run")
	docsAnalyzeCmd.Flags().BoolVar(&docsAnalyzeDryRun, "dry-run", false, "only output the proposed document plan; do not write files")
	docsAnalyzeCmd.Flags().DurationVar(&docsAnalyzeTimeout, "timeout", 2*time.Minute, "timeout for cloning remote repositories and docs analysis preparation")
	docsAnalyzeCmd.Flags().BoolVar(&docsAnalyzeKeepTmp, "keep-temp", false, "keep the cloned temporary repository for debugging when analyzing a remote repo")
	docsCmd.AddCommand(docsAnalyzeCmd)
	rootCmd.AddCommand(docsCmd)
}

func runDocsAnalyze(cmd *cobra.Command, args []string) error {
	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	merged := loaded.Merged
	cli := ttconfig.Config{}
	if cmd.Flags().Changed("model") {
		cli.Agent.Model = docsAnalyzeModel
	}
	if cmd.Flags().Changed("debug") {
		cli.Agent.Debug = ttconfig.BoolPtr(docsAnalyzeDebug)
	}
	if cmd.Flags().Changed("picoclaw-home") {
		cli.Picoclaw.Home = docsAnalyzeHome
	}
	if cmd.Flags().Changed("picoclaw-config") {
		cli.Picoclaw.Config = docsAnalyzeConfig
	}
	merged = ttconfig.Merge(merged, cli)
	if err := ensurePicoclawConfigAvailable(merged.Picoclaw.Home, merged.Picoclaw.Config); err != nil {
		return err
	}

	target, cleanup, err := resolveDocsAnalyzeTarget(args, loaded)
	if err != nil {
		return err
	}
	defer cleanup()

	outputDir, err := resolveDocsOutputDir(target)
	if err != nil {
		return err
	}
	if !docsAnalyzeDryRun {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}

	rt, err := pcwrap.Load(pcwrap.Options{Home: merged.Picoclaw.Home, Config: merged.Picoclaw.Config, TTConfig: merged, TTSources: loaded.Sources})
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}

	debug := docsAnalyzeDebug
	if merged.Agent.Debug != nil {
		debug = *merged.Agent.Debug
	}
	model := strings.TrimSpace(docsAnalyzeModel)
	if model == "" {
		model = merged.Agent.Model
	}
	loadingMessage := "正在分析代码并生成文档"
	if docsAnalyzeDryRun {
		loadingMessage = "正在分析代码并生成文档计划"
	}
	loading := startLLMLoading(loadingMessage, debug)
	dr, err := rt.NewDirectRunner(pcwrap.RunOptions{
		Session:        docsAnalyzeSession,
		Agent:          agents.DocsAnalystID,
		Model:          model,
		Workspace:      target.AnalysisDir,
		Debug:          debug,
		Quiet:          !debug,
		EmbeddedAgents: []pcwrap.EmbeddedAgent{agents.DocsAnalyst()},
	})
	if err != nil {
		loading.Stop()
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	defer dr.Close()

	prompt := buildDocsAnalyzePrompt(target, outputDir, docsAnalyzeDryRun)
	response, err := dr.ProcessDirect(pcwrap.RunOptions{
		Message:        prompt,
		Session:        docsAnalyzeSession,
		Agent:          agents.DocsAnalystID,
		Model:          model,
		Workspace:      target.AnalysisDir,
		Debug:          debug,
		Quiet:          !debug,
		EmbeddedAgents: []pcwrap.EmbeddedAgent{agents.DocsAnalyst()},
	})
	loading.Stop()
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	response = strings.TrimSpace(response)
	if response != "" {
		fmt.Fprintln(cmd.OutOrStdout(), response)
		fmt.Fprintln(cmd.OutOrStdout())
	}
	if docsAnalyzeDryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "docs dry-run completed for %s\nplanned output directory: %s\n", target.DisplayName, outputDir)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "docs analysis completed for %s\noutput directory: %s\n", target.DisplayName, outputDir)
	return nil
}

func resolveDocsAnalyzeTarget(args []string, loaded ttconfig.Loaded) (docsAnalyzeTarget, func(), error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		abs, err := resolveExistingDir(projectRootFromConfig(loaded))
		if err != nil {
			return docsAnalyzeTarget{}, nil, err
		}
		return docsAnalyzeTarget{AnalysisDir: abs, DisplayName: abs, RepoName: filepath.Base(abs)}, func() {}, nil
	}
	input := strings.TrimSpace(args[0])
	if abs, err := resolveExistingDir(input); err == nil {
		return docsAnalyzeTarget{AnalysisDir: abs, DisplayName: abs, RepoName: filepath.Base(abs)}, func() {}, nil
	}
	repoURL, repoName, ok := normalizeDocsRepoInput(input)
	if !ok {
		return docsAnalyzeTarget{}, nil, fmt.Errorf("analysis path must be a directory or GitHub repository: %s", input)
	}
	tmp, err := os.MkdirTemp("", "tt-docs-*")
	if err != nil {
		return docsAnalyzeTarget{}, nil, fmt.Errorf("create temporary repository directory: %w", err)
	}
	cleanup := func() {
		if !docsAnalyzeKeepTmp {
			_ = os.RemoveAll(tmp)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), docsTimeoutOr(docsAnalyzeTimeout, 2*time.Minute))
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", repoURL, tmp)
	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return docsAnalyzeTarget{}, nil, fmt.Errorf("clone repository failed: %w - %s", err, strings.TrimSpace(string(out)))
	}
	return docsAnalyzeTarget{AnalysisDir: tmp, DisplayName: input, RepoName: repoName, Remote: true, TempDir: tmp}, cleanup, nil
}

func resolveExistingDir(target string) (string, error) {
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("analysis path must be a directory: %s", abs)
	}
	return abs, nil
}

func resolveDocsOutputDir(target docsAnalyzeTarget) (string, error) {
	outputDir := strings.TrimSpace(docsAnalyzeOutput)
	if outputDir != "" {
		return filepath.Abs(outputDir)
	}
	if !target.Remote {
		return filepath.Join(target.AnalysisDir, "ai-docs"), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current directory: %w", err)
	}
	return filepath.Join(cwd, "ai-docs", docsSafeFileName(target.RepoName)), nil
}

func buildDocsAnalyzePrompt(target docsAnalyzeTarget, outputDir string, dryRun bool) string {
	modeInstruction := "6. 直接把最终 Markdown 文件写入输出目录；不要只给建议或计划。"
	finalInstruction := "请先完成代码研究，再生成最有价值的文档集合，并在最后简要说明本次新增或更新了哪些文件。"
	if dryRun {
		modeInstruction = "6. 本次是 dry-run：不要写任何文件，不要修改输出目录，只输出一份中文 Markdown 计划。"
		finalInstruction = "请输出一份简洁的中文 Markdown 计划，至少包含：建议新增/更新的文件、每个文件的目的、优先级、关键证据来源，以及为什么这样组织。"
	}
	return fmt.Sprintf(`请分析代码并生成最有助于理解该项目的中文文档。

分析目录：%s
分析来源：%s
输出目录：%s

执行要求：
1. 研究代码结构、模块划分、核心功能、技术栈与关键流程。
2. 根据代码事实和复杂度动态决定生成哪些文档；不要机械套模板，也不要为了凑数量而输出低价值内容。
3. 优先更新现有文档：如果输出目录中已经存在 Markdown 文档，请先阅读并在保留合理结构的前提下增量更新、合并去重。
4. 关键结论必须有代码依据，但默认不要在正文频繁插入“代码证据：...”这类会打断阅读节奏的句式。
5. 正文请优先保证阅读体验：先讲清“这是什么、为什么重要、应该先理解什么、模块如何协作”，再补必要实现细节。
6. 如需保留依据，请优先放到文末“实现参考 / 延伸阅读 / 参考位置”小节中集中列出，而不是每一节都重复证据陈述。
7. README 或入口页必须承担导读职责，告诉读者先看什么、不同类型读者该怎么读，而不是只做文件列表。
8. 尽量多使用 Mermaid 图来解释架构、分层、关键流程、模块关系、数据流和状态流；复杂项目默认应提供“总览图 + 关键图”组合，而不是只靠长段文字。
9. 图表不要只做装饰。每张图都应服务于一个明确的理解问题，并在图后补几句简短说明，告诉读者应该从图里看什么。
10. 仅在图表确实不能提升理解效率时，才退回纯文字说明。
%s

%s`, target.AnalysisDir, target.DisplayName, outputDir, modeInstruction, finalInstruction)
}

func normalizeDocsRepoInput(raw string) (cloneURL string, repoName string, ok bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", false
	}
	if strings.HasPrefix(trimmed, "git@github.com:") {
		return docsGitHubRepoFromPath(strings.TrimPrefix(trimmed, "git@github.com:"))
	}
	if strings.HasPrefix(trimmed, "github.com/") {
		return docsGitHubRepoFromPath(strings.TrimPrefix(trimmed, "github.com/"))
	}
	if docsGitHubShorthand(trimmed) {
		return docsGitHubRepoFromPath(trimmed)
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return "", "", false
	}
	if strings.EqualFold(parsed.Host, "github.com") || strings.EqualFold(parsed.Host, "www.github.com") {
		return docsGitHubRepoFromPath(strings.TrimPrefix(parsed.Path, "/"))
	}
	name := strings.TrimSuffix(filepath.Base(parsed.Path), ".git")
	if name == "" || name == "." || name == "/" {
		name = "repo"
	}
	return trimmed, name, true
}

func docsGitHubRepoFromPath(path string) (cloneURL string, repoName string, ok bool) {
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	name := strings.TrimSuffix(parts[1], ".git")
	if name == "" || parts[0] == "." || name == "." {
		return "", "", false
	}
	return fmt.Sprintf("https://github.com/%s/%s.git", parts[0], name), name, true
}

func docsGitHubShorthand(s string) bool {
	return regexp.MustCompile(`^[^/\s]+/[^/\s]+$`).MatchString(s) && !strings.HasPrefix(s, ".")
}

func docsSafeFileName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = regexp.MustCompile(`[^a-z0-9._-]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-.")
	if s == "" {
		return "repo"
	}
	return s
}

func docsTimeoutOr(v, def time.Duration) time.Duration {
	if v == 0 {
		return def
	}
	return v
}
