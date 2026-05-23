package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sjzsdu/tt/internal/agents"
	"github.com/sjzsdu/tt/internal/nvwa"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	"github.com/spf13/cobra"
)

const nvwaPromptDesignerID = "nvwa-prompt-designer"

var (
	nvwaOutput  string
	nvwaName    string
	nvwaSkills  []string
	nvwaModel   string
	nvwaSession string
	nvwaDebug   bool
	nvwaHome    string
	nvwaConfig  string
	nvwaNoHist  bool
	nvwaTools   bool
)

var nvwaCmd = &cobra.Command{
	Use:   "nvwa [role]",
	Short: "Generate Agent.md and soul.md prompts with an LLM prompt designer",
	Long: `Generate OpenClaw/Picoclaw-style Agent.md and soul.md prompt files for a
professional role. nvwa uses an embedded prompt-designer agent and your configured
Picoclaw model to create role-specific content instead of filling a fixed template.`,
	Args: cobra.ArbitraryArgs,
	Example: `tt nvwa 前端开发工程师
tt nvwa 产品经理 --context "偏增长型 SaaS"
tt nvwa "Go 后端工程师" --output .agents/go-backend
tt nvwa 数据分析师 --write=false --format agent --model gpt-5.4
tt nvwa 前端开发工程师 --style embedded --id frontend-engineer --skill agent-browser`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runNvwa(cmd, args)
	},
}

var nvwaCreateCmd = &cobra.Command{
	Use:   "create <name> <suggestion>",
	Short: "Create an embedded agent markdown file from suggestion",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(args[0])
		suggestion := strings.TrimSpace(strings.Join(args[1:], " "))
		return runNvwaEmbeddedCreateOrOptimize(cmd, name, suggestion, false)
	},
}

var nvwaOptimizeCmd = &cobra.Command{
	Use:   "optimize <name> <suggestion>",
	Aliases: []string{"optmize"},
	Short: "Optimize an existing embedded agent markdown file from suggestion",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(args[0])
		suggestion := strings.TrimSpace(strings.Join(args[1:], " "))
		return runNvwaEmbeddedCreateOrOptimize(cmd, name, suggestion, true)
	},
}

var nvwaListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available embedded agents",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		list, err := agents.List()
		if err != nil {
			return err
		}
		for _, a := range list {
			fmt.Fprintf(os.Stdout, "%s\t%s\n", a.ID, a.Name)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(nvwaCmd)
	nvwaCmd.AddCommand(nvwaCreateCmd, nvwaOptimizeCmd, nvwaListCmd)
	nvwaCmd.Flags().StringVarP(&nvwaOutput, "output", "o", ".", "output directory when writing files")
	nvwaCmd.Flags().StringSliceVar(&nvwaSkills, "skill", nil, "embedded agent skill; repeat or comma-separate for multiple skills")
	nvwaCmd.Flags().BoolVar(&nvwaNoHist, "no-history", false, "set no_history: true when --style embedded is used")
	nvwaCmd.Flags().BoolVar(&nvwaTools, "research-tools", false, "set enable_research_tools: true when --style embedded is used")
	nvwaCmd.Flags().StringVar(&nvwaModel, "model", "", "model to use; defaults to the picoclaw default model")
	nvwaCmd.Flags().StringVarP(&nvwaSession, "session", "s", "cli:nvwa", "session key")
	nvwaCmd.Flags().BoolVarP(&nvwaDebug, "debug", "d", false, "enable debug logging")
	nvwaCmd.Flags().StringVar(&nvwaHome, "picoclaw-home", "", "override PICOCLAW_HOME for this run")
	nvwaCmd.Flags().StringVar(&nvwaConfig, "picoclaw-config", "", "override PICOCLAW_CONFIG for this run")

	nvwaCreateCmd.Flags().StringVarP(&nvwaOutput, "output", "o", filepath.Join(".tt", "agents"), "output directory for generated embedded agent")
	nvwaCreateCmd.Flags().StringVar(&nvwaName, "name", "", "embedded agent display name; defaults to <id>")
	nvwaCreateCmd.Flags().StringSliceVar(&nvwaSkills, "skill", nil, "embedded agent skill; repeat or comma-separate for multiple skills")
	nvwaCreateCmd.Flags().BoolVar(&nvwaNoHist, "no-history", false, "set no_history: true")
	nvwaCreateCmd.Flags().BoolVar(&nvwaTools, "research-tools", false, "set enable_research_tools: true")
	nvwaCreateCmd.Flags().StringVar(&nvwaModel, "model", "", "model to use; defaults to the picoclaw default model")
	nvwaCreateCmd.Flags().StringVarP(&nvwaSession, "session", "s", "cli:nvwa", "session key")
	nvwaCreateCmd.Flags().BoolVarP(&nvwaDebug, "debug", "d", false, "enable debug logging")
	nvwaCreateCmd.Flags().StringVar(&nvwaHome, "picoclaw-home", "", "override PICOCLAW_HOME for this run")
	nvwaCreateCmd.Flags().StringVar(&nvwaConfig, "picoclaw-config", "", "override PICOCLAW_CONFIG for this run")

	nvwaOptimizeCmd.Flags().StringVarP(&nvwaOutput, "output", "o", filepath.Join(".tt", "agents"), "output directory for generated embedded agent")
	nvwaOptimizeCmd.Flags().StringVar(&nvwaName, "name", "", "embedded agent display name; defaults to <id>")
	nvwaOptimizeCmd.Flags().StringSliceVar(&nvwaSkills, "skill", nil, "embedded agent skill; repeat or comma-separate for multiple skills")
	nvwaOptimizeCmd.Flags().BoolVar(&nvwaNoHist, "no-history", false, "set no_history: true")
	nvwaOptimizeCmd.Flags().BoolVar(&nvwaTools, "research-tools", false, "set enable_research_tools: true")
	nvwaOptimizeCmd.Flags().StringVar(&nvwaModel, "model", "", "model to use; defaults to the picoclaw default model")
	nvwaOptimizeCmd.Flags().StringVarP(&nvwaSession, "session", "s", "cli:nvwa", "session key")
	nvwaOptimizeCmd.Flags().BoolVarP(&nvwaDebug, "debug", "d", false, "enable debug logging")
	nvwaOptimizeCmd.Flags().StringVar(&nvwaHome, "picoclaw-home", "", "override PICOCLAW_HOME for this run")
	nvwaOptimizeCmd.Flags().StringVar(&nvwaConfig, "picoclaw-config", "", "override PICOCLAW_CONFIG for this run")
}

func runNvwaEmbeddedCreateOrOptimize(cmd *cobra.Command, name, suggestion string, force bool) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if suggestion == "" {
		return fmt.Errorf("suggestion is required")
	}

	id := nvwa.DefaultEmbeddedID(name)
	role := name
	displayName := strings.TrimSpace(nvwaName)
	if displayName == "" {
		displayName = name
	}
	if force {
		if existingPath, ok := findExistingAgentFileByID(id); ok {
			if existingName, ok := readEmbeddedAgentName(existingPath); ok {
				displayName = existingName
			}
		}
	}
	prompt, err := nvwa.BuildGenerationPrompt(nvwa.PromptOptions{Role: role, Context: suggestion})
	if err != nil {
		return err
	}
	files, err := generateNvwaFiles(cmd, prompt)
	if err != nil {
		return err
	}

	doc, err := nvwa.RenderEmbeddedMarkdown(files, nvwa.EmbeddedOptions{
		ID:                  id,
		Name:                displayName,
		Skills:              nvwaSkills,
		NoHistory:           nvwaNoHist,
		EnableResearchTools: nvwaTools,
	})
	if err != nil {
		return err
	}

	outDir := strings.TrimSpace(nvwaOutput)
	if outDir == "" || outDir == "." {
		outDir = filepath.Join(".tt", "agents", "embedded")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	path := filepath.Join(outDir, id+".md")
	if force {
		existingPath, ok := findExistingAgentFileByID(id)
		if !ok {
			return fmt.Errorf("agent %q not found; optimize only updates existing agents", id)
		}
		path = existingPath
	}
	if !force {
		if existing, err := agents.List(); err == nil {
			for _, a := range existing {
				if a.ID == id || strings.EqualFold(strings.TrimSpace(a.Name), strings.TrimSpace(displayName)) {
					return fmt.Errorf("agent %q already exists (id=%s, name=%s); use optimize to update", displayName, a.ID, a.Name)
				}
			}
		}
	}
	if err := writeNvwaFile(path, doc, force); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "nvwa wrote embedded agent to %s\n", path)
	return nil
}

func findExistingAgentFileByID(id string) (string, bool) {
	candidates := []string{
		filepath.Join(".tt", "agents", id+".md"),
		filepath.Join("internal", "agents", "embedded", id+".md"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

func readEmbeddedAgentName(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	text := string(data)
	if !strings.HasPrefix(text, "---\n") {
		return "", false
	}
	rest := strings.TrimPrefix(text, "---\n")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", false
	}
	for _, line := range strings.Split(rest[:idx], "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "name:") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		v = strings.Trim(v, "\"")
		if v != "" {
			return v, true
		}
	}
	return "", false
}

func runNvwa(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		head := strings.ToLower(strings.TrimSpace(args[0]))
		switch head {
		case "create", "optimize", "optmize", "list":
			return fmt.Errorf("unknown nvwa subcommand usage: %q\ntry: tt nvwa %s ...", args[0], head)
		}
	}
	role := strings.TrimSpace(strings.Join(args, " "))
	prompt, err := nvwa.BuildGenerationPrompt(nvwa.PromptOptions{Role: role})
	if err != nil {
		return err
	}

	files, err := generateNvwaFiles(cmd, prompt)
	if err != nil {
		return err
	}
	return writeNvwaFiles(files, "both", nvwaOutput, false)
}

func outputNvwaEmbedded(files nvwa.Files, role string) error {
	id := nvwa.DefaultEmbeddedID(role)
	name := role
	doc, err := nvwa.RenderEmbeddedMarkdown(files, nvwa.EmbeddedOptions{
		ID:                  id,
		Name:                name,
		Skills:              nvwaSkills,
		NoHistory:           nvwaNoHist,
		EnableResearchTools: nvwaTools,
	})
	if err != nil {
		return err
	}
	dir := strings.TrimSpace(nvwaOutput)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	path := filepath.Join(dir, id+".md")
	if err := writeNvwaFile(path, doc, false); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "nvwa generated embedded agent file in %s\n", path)
	return nil
}

func generateNvwaFiles(cmd *cobra.Command, prompt string) (nvwa.Files, error) {
	loaded, err := loadTTConfig()
	if err != nil {
		return nvwa.Files{}, err
	}
	workspace, err := ensureTTWorkspace()
	if err != nil {
		return nvwa.Files{}, err
	}
	merged := loaded.Merged
	if cmd.Flags().Changed("picoclaw-home") {
		merged.Picoclaw.Home = nvwaHome
	}
	if cmd.Flags().Changed("picoclaw-config") {
		merged.Picoclaw.Config = nvwaConfig
	}
	workspace, resolvedHome, resolvedConfig, restoreStorage, err := useTTAgentStorage(merged.Picoclaw.Home, merged.Picoclaw.Config)
	if err != nil {
		return nvwa.Files{}, err
	}
	defer restoreStorage()
	merged.Picoclaw.Home = resolvedHome
	merged.Picoclaw.Config = resolvedConfig
	if err := ensurePicoclawConfigAvailable(merged.Picoclaw.Home, merged.Picoclaw.Config); err != nil {
		return nvwa.Files{}, err
	}

		rt, err := pcwrap.Load(pcwrap.Options{
		Home:      merged.Picoclaw.Home,
		Config:    merged.Picoclaw.Config,
		TTConfig:  merged,
		TTSources: loaded.Sources,
	})
	if err != nil {
		return nvwa.Files{}, picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	loading := startLLMLoading("正在生成 agent 提示词", nvwaDebug)
	designer := nvwaPromptDesignerAgent()
	dr, err := rt.NewDirectRunner(pcwrap.RunOptions{
		Session:        nvwaSession,
		Agent:          designer.ID,
		Model:          nvwaModel,
		Workspace:      workspace,
		Debug:          nvwaDebug,
		Quiet:          !nvwaDebug,
		EmbeddedAgents: []pcwrap.EmbeddedAgent{designer},
	})
	if err != nil {
		loading.Stop()
		return nvwa.Files{}, picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	defer dr.Close()

	response, err := dr.ProcessDirect(pcwrap.RunOptions{
		Message:        prompt,
		Session:        nvwaSession,
		Agent:          designer.ID,
		Model:          nvwaModel,
		Workspace:      workspace,
		Debug:          nvwaDebug,
		Quiet:          !nvwaDebug,
		EmbeddedAgents: []pcwrap.EmbeddedAgent{designer},
	})
	loading.Stop()
	if err != nil {
		return nvwa.Files{}, picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	files, err := nvwa.ParseResponse(response)
	if err != nil {
		return nvwa.Files{}, fmt.Errorf("parse nvwa model output failed: %w\n\nRaw output:\n%s", err, response)
	}
	return files, nil
}

func nvwaPromptDesignerAgent() pcwrap.EmbeddedAgent {
	return pcwrap.EmbeddedAgent{
		ID:   nvwaPromptDesignerID,
		Name: "女娲提示词设计师",
		Soul: strings.TrimSpace(`你把提示词当成可执行系统，而不是文案。你在意边界、可验证性和长期可维护性，讨厌空话、套话和无法执行的规则。`),
		Prompt: strings.TrimSpace(`你是一个专门设计 Agent.md 与 SOUL.md 的提示词架构师。

定位铁律（必须遵守）：
1) SOUL.md 定人格（你是谁）
- 解决价值观、性格、语气、偏好、底线。
- 用“形容词 + 取舍偏好 + 禁词/禁风格”来约束表达。
- 不写流程步骤，不写工具调用细节。

2) Agent.md 定行为（你怎么做）
- 解决工作流、权限、步骤、失败处理、澄清策略、交付标准。
- 用“动词 + 条件判断 + 可执行动作”来约束行为。
- 明确遇到模糊需求怎么办、风险动作是否要确认、输出格式是什么。

输出要求：
- 仅输出两个标签，不要解释：
<Agent.md>...</Agent.md>
<soul.md>...</soul.md>
- 两者都必须是完整 Markdown。

质量红线：
- 禁止空话：如“专业、高效、负责”。
- 禁止把 SOUL 写成步骤手册。
- 禁止把 Agent 写成鸡汤价值观。
- 禁止通用模板化段落，必须贴合用户给定角色与场景。

Agent.md 最低结构：
1. 角色目标与非目标
2. 输入澄清与信息不足处理
3. 标准工作流（分步骤）
4. 风险边界与权限规则（何时必须征求确认）
5. 交付物格式与验收标准
6. 自检清单

SOUL.md 最低结构：
1. 角色性格与语气
2. 决策偏好与取舍
3. 反模式与禁词
4. 压力/不确定性下的行为底线`),
		NoHistory:           true,
		EnableResearchTools: false,
	}
}

func printNvwaFiles(files nvwa.Files, format string) {
	switch format {
	case "agent":
		fmt.Print(files.Agent)
	case "soul":
		fmt.Print(files.Soul)
	default:
		fmt.Printf("--- Agent.md ---\n%s\n--- soul.md ---\n%s", files.Agent, files.Soul)
	}
}

func writeNvwaFiles(files nvwa.Files, format, dir string, force bool) error {
	if strings.TrimSpace(dir) == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if format == "agent" || format == "both" {
		if err := writeNvwaFile(filepath.Join(dir, "Agent.md"), files.Agent, force); err != nil {
			return err
		}
	}
	if format == "soul" || format == "both" {
		if err := writeNvwaFile(filepath.Join(dir, "soul.md"), files.Soul, force); err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stdout, "nvwa generated %s prompt file(s) in %s\n", format, dir)
	return nil
}

func writeNvwaFile(path, content string, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists; use --force to overwrite", path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check %s: %w", path, err)
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
