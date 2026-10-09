package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/agents"
	"github.com/sjzsdu/tt/internal/docsvisual"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	ttconfig "github.com/sjzsdu/tt/internal/ttconfig"
)

var (
	docsVisualOutput      string
	docsVisualFormat      string
	docsVisualDetail      string
	docsVisualAudience    string
	docsVisualSize        string
	docsVisualMaxDiagrams int
	docsVisualTypes       string
	docsVisualBrand       string
	docsVisualSession     string
	docsVisualModel       string
	docsVisualDebug       bool
	docsVisualHome        string
	docsVisualConfig      string
	docsVisualDryRun      bool
	docsVisualTimeout     time.Duration
	docsVisualKeepTmp     bool
)

var docsVisualFormatChoices = []string{"site", "md", "both"}
var docsVisualDetailChoices = []string{"faithful", "balanced", "simplified"}
var docsVisualAudienceChoices = []string{"engineer", "mixed", "executive"}

var docsVisualCmd = &cobra.Command{
	Use:   "visual [path-or-github]",
	Short: "Generate diagram-centric visual documentation for a codebase",
	Long: `Study the target directory or GitHub repository with the embedded
docs-visual-writer agent and produce a diagram-centric Chinese documentation
site based on the diagram-design skill (cathrynlavery/diagram-design).

All artifacts for one project live in a single output directory: the HTML
site, chart files, and Markdown stay together under <target>/ai-docs/visual
by default, side by side with docs analyze Markdown in ai-docs/.

Charts are self-contained inline SVG in editorial style - no Mermaid, no
generic rounded boxes. Each diagram answers one explicit comprehension
question.

Output formats:
  site  single self-contained HTML doc site with side navigation (default)
  md    Chinese Markdown plus one standalone chart file per diagram
  both  generate site and md side by side`,
	Args: cobra.MaximumNArgs(1),
	Example: `tt docs visual
 tt docs visual ./internal
 tt docs visual github.com/owner/repo
 tt docs visual ./cmd --format md --detail simplified
 tt docs visual github.com/owner/repo --dry-run
 tt docs visual ./internal --diagram-types architecture,sequence --max-diagrams 6`,
	RunE: runDocsVisual,
}

func init() {
	docsVisualCmd.Flags().StringVarP(&docsVisualOutput, "output-dir", "o", "", "directory to write generated visual documents (default: local=<target>/ai-docs/visual, remote=./ai-docs/<repo-name>/visual)")
	docsVisualCmd.Flags().StringVar(&docsVisualFormat, "format", "site", "output format: site, md, or both")
	docsVisualCmd.Flags().StringVar(&docsVisualDetail, "detail", "balanced", "chart detail level: faithful (<=24 nodes), balanced (<=12), simplified (<=7)")
	docsVisualCmd.Flags().StringVar(&docsVisualAudience, "audience", "mixed", "wording audience: engineer, mixed, or executive")
	docsVisualCmd.Flags().StringVar(&docsVisualSize, "size", "doc-wide", "chart size preset from diagram-design (doc-inline, doc-wide, slide-16x9, print-a4-landscape, ...)")
	docsVisualCmd.Flags().IntVar(&docsVisualMaxDiagrams, "max-diagrams", 0, "hard cap on diagram count; 0 lets the agent decide by repo complexity")
	docsVisualCmd.Flags().StringVar(&docsVisualTypes, "diagram-types", "", "comma-separated preferred diagram types, e.g. architecture,sequence; empty lets the agent choose")
	docsVisualCmd.Flags().StringVar(&docsVisualBrand, "brand", "", "website URL to sample brand colors and fonts from (writes diagram token style guide)")
	docsVisualCmd.Flags().StringVar(&docsVisualSession, "session", "cli:docs-visual", "session key for visual docs analysis")
	docsVisualCmd.Flags().StringVar(&docsVisualModel, "model", "", "model override for the embedded docs-visual-writer agent")
	docsVisualCmd.Flags().BoolVarP(&docsVisualDebug, "debug", "d", false, "enable debug logging")
	docsVisualCmd.Flags().StringVar(&docsVisualHome, "picoclaw-home", "", "override PICOCLAW_HOME for this run")
	docsVisualCmd.Flags().StringVar(&docsVisualConfig, "picoclaw-config", "", "override PICOCLAW_CONFIG for this run")
	docsVisualCmd.Flags().BoolVar(&docsVisualDryRun, "dry-run", false, "only output the proposed diagram plan; do not write files")
	docsVisualCmd.Flags().DurationVar(&docsVisualTimeout, "timeout", 2*time.Minute, "timeout for cloning remote repositories and docs analysis preparation")
	docsVisualCmd.Flags().BoolVar(&docsVisualKeepTmp, "keep-temp", false, "keep the cloned temporary repository for debugging when analyzing a remote repo")
	docsCmd.AddCommand(docsVisualCmd)
}

func runDocsVisual(cmd *cobra.Command, args []string) error {
	for name, value := range map[string]struct {
		value   string
		choices []string
	}{
		"format":   {docsVisualFormat, docsVisualFormatChoices},
		"detail":   {docsVisualDetail, docsVisualDetailChoices},
		"audience": {docsVisualAudience, docsVisualAudienceChoices},
	} {
		if err := validateEnumChoice(name, value.value, value.choices); err != nil {
			return err
		}
	}
	if docsVisualMaxDiagrams < 0 {
		return fmt.Errorf("--max-diagrams must be >= 0")
	}

	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	merged := loaded.Merged
	cli := ttconfig.Config{}
	if cmd.Flags().Changed("model") {
		cli.Agent.Model = docsVisualModel
	}
	if cmd.Flags().Changed("debug") {
		cli.Agent.Debug = ttconfig.BoolPtr(docsVisualDebug)
	}
	if cmd.Flags().Changed("picoclaw-home") {
		cli.Picoclaw.Home = docsVisualHome
	}
	if cmd.Flags().Changed("picoclaw-config") {
		cli.Picoclaw.Config = docsVisualConfig
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

	outputDir, err := resolveDocsVisualOutputDir(target)
	if err != nil {
		return err
	}
	if !docsVisualDryRun {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}

	rt, err := pcwrap.Load(pcwrap.Options{Home: merged.Picoclaw.Home, Config: merged.Picoclaw.Config, TTConfig: merged, TTSources: loaded.Sources})
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	if err := ensureDiagramDesignSkill(rt.Home); err != nil {
		msg := fmt.Sprintf("diagram-design skill provisioning failed: %v", err)
		if docsVisualDebug {
			fmt.Fprintln(cmd.ErrOrStderr(), msg)
		}
		return fmt.Errorf("%s", msg)
	}

	debug := docsVisualDebug
	if merged.Agent.Debug != nil {
		debug = *merged.Agent.Debug
	}
	model := strings.TrimSpace(docsVisualModel)
	if model == "" {
		model = merged.Agent.Model
	}
	loadingMessage := "正在分析代码并生成图表文档"
	if docsVisualDryRun {
		loadingMessage = "正在分析代码并生成图表文档计划"
	}
	loading := startLLMLoading(loadingMessage, debug)
	docsVisualWriter, err := agents.DocsVisualWriter()
	if err != nil {
		loading.Stop()
		return fmt.Errorf("load docs visual writer agent failed: %w", err)
	}
	dr, err := rt.NewDirectRunner(pcwrap.RunOptions{
		Session:        docsVisualSession,
		Agent:          agents.DocsVisualWriterID,
		Model:          model,
		Workspace:      target.AnalysisDir,
		Debug:          debug,
		Quiet:          !debug,
		EmbeddedAgents: []pcwrap.EmbeddedAgent{docsVisualWriter},
	})
	if err != nil {
		loading.Stop()
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	defer dr.Close()

	prompt := buildDocsVisualPrompt(target, outputDir, docsVisualDryRun)
	response, err := dr.ProcessDirect(pcwrap.RunOptions{
		Message:        prompt,
		Session:        docsVisualSession,
		Agent:          agents.DocsVisualWriterID,
		Model:          model,
		Workspace:      target.AnalysisDir,
		Debug:          debug,
		Quiet:          !debug,
		EmbeddedAgents: []pcwrap.EmbeddedAgent{docsVisualWriter},
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
	if docsVisualDryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "docs visual dry-run completed for %s\nplanned output directory: %s\n", target.DisplayName, outputDir)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "docs visual completed for %s\noutput directory: %s\n", target.DisplayName, outputDir)
	return nil
}

func validateEnumChoice(name, value string, allowed []string) error {
	for _, choice := range allowed {
		if value == choice {
			return nil
		}
	}
	return fmt.Errorf("--%s must be one of: %s (got %q)", name, strings.Join(allowed, ", "), value)
}

func resolveDocsVisualOutputDir(target docsAnalyzeTarget) (string, error) {
	outputDir := strings.TrimSpace(docsVisualOutput)
	if outputDir != "" {
		return filepath.Abs(outputDir)
	}
	// Keep every artifact of one project under a single ai-docs root, with
	// visual output isolated in its own visual/ subdirectory.
	if !target.Remote {
		return filepath.Join(target.AnalysisDir, "ai-docs", "visual"), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current directory: %w", err)
	}
	return filepath.Join(cwd, "ai-docs", docsSafeFileName(target.RepoName), "visual"), nil
}

func buildDocsVisualPrompt(target docsAnalyzeTarget, outputDir string, dryRun bool) string {
	formatInstruction := "本次交付形态为 site：把最终成果写成一个可直接用浏览器打开的自包含 HTML 中文文档站（内联 SVG 图表 + 侧边导航），入口文件建议命名为 index.html；不要只给建议或计划。"
	finalInstruction := "请先完成代码研究，再按理解问题选型并绘制图表，最后组装文档站，并简要说明本次新增或更新了哪些文件、每张图回答了什么理解问题。"
	if dryRun {
		formatInstruction = "本次是 dry-run：不要写任何文件，不要修改输出目录，只输出一份中文 Markdown 计划。"
		finalInstruction = "请输出一份简洁的中文 Markdown 图表规划，至少包含：拟回答的理解问题清单、每个问题对应的图表类型与理由、拟产出文件清单与优先级、关键证据来源。"
	}
	detailTxt := map[string]string{
		"faithful":   "faithful（≤24 节点，分区块呈现）",
		"balanced":   "balanced（≤12 节点）",
		"simplified": "simplified（≤7 节点）",
	}[docsVisualDetail]
	audienceTxt := map[string]string{
		"engineer":  "engineer：面向工程师，保留精确名称与细节措辞",
		"mixed":     "mixed：面向混合读者，专有名词旁边给一句通俗解释",
		"executive": "executive：面向决策者，用意图和效果措辞替代实现细节",
	}[docsVisualAudience]

	diagramTypes := strings.TrimSpace(docsVisualTypes)
	diagramTypeLine := "图表类型由你根据理解问题自行选择，不设硬性清单。"
	if diagramTypes != "" {
		diagramTypeLine = fmt.Sprintf("用户优先指定了图表类型（逗号分隔，不合适时可说明理由后调整）：%s。", diagramTypes)
	}
	maxDiagramsLine := "图表数量按代码复杂度自适应：小项目宁少而精，不要为了凑数量画装饰图。"
	if docsVisualMaxDiagrams > 0 {
		maxDiagramsLine = fmt.Sprintf("图表数量上限为 %d 张（含总览图），宁可少而准。", docsVisualMaxDiagrams)
	}
	brandLine := "未提供品牌 URL，使用 diagram-design 默认 token（白烟纸面 / 墨黑 / 原子橙强调色）。首次输出前请检查 references/style-guide.md 是否可用。"
	if brand := strings.TrimSpace(docsVisualBrand); brand != "" {
		brandLine = fmt.Sprintf("品牌取样：%s。读取 diagram-design skill 的 onboarding 流程，把站点配色与字体映射为 paper/ink/muted/accent token，并做对比度校验后才使用。", brand)
	}

	return fmt.Sprintf(`请分析代码并生成以图表为核心的中文技术文档。图表使用 diagram-design skill（编辑部级 SVG 设计系统），不要使用 Mermaid。

分析目录：%s
分析来源：%s
输出目录：%s

执行要求：
1. 遵循 docs-visual-writer soul 的五步流程：研究代码 → 提炼理解问题（3-8 个）→ 为每个问题选择最合适的 diagram-design 图表类型 → 按 references/ 下对应 type-*.md 规范绘制 → 组装与解读。
2. 细节级别：%s。受众措辞：%s。
3. 图表尺寸：使用 %s 预设。%s
4. %s
5. 图表不是装饰：每张图必须回答一个明确的理解问题，并在图后用 2-4 句中文告诉读者该看什么；总览图要让读者 30 秒建立全局感。
6. 输出形态：%s
7. 所有产物（HTML 站点、图表文件、Markdown）必须全部放在输出目录内，不要散落到仓库其他位置；目标目录 ai-docs 下已有其他文档时，在入口页链接互补而不是重复覆盖。
%s

%s`, target.AnalysisDir, target.DisplayName, outputDir, detailTxt, audienceTxt, docsVisualSize, diagramTypeLine, maxDiagramsLine, formatInstruction, brandLine, finalInstruction)
}

// ensureDiagramDesignSkill extracts the binary-embedded diagram-design skill
// into the picoclaw skills home so the skills loader can discover it. The
// skill data is staged at build time by scripts/fetch-diagram-design.sh
// (commit + sha256 pinned) and compiled in via go:embed.
func ensureDiagramDesignSkill(home string) error {
	return docsvisual.EnsureProvisioned(home)
}
