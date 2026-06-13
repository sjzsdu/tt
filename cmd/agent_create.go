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

var (
	agentCreateRole       string
	agentCreateName       string
	agentCreateSuggestion string
	agentCreateOutput     string
	agentCreateSkills     []string
	agentCreateNoHist     bool
	agentCreateTools      bool
	agentCreateModel      string
	agentCreateSession    string
	agentCreateDebug      bool
	agentCreateHome       string
	agentCreateConfig     string
	agentCreateForce      bool
)

var agentCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new embedded agent",
	Long: `Create a new embedded agent markdown file. You can create from a role description
or from a suggestion for an existing agent.`,
	Example: `tt agent create --role "前端开发工程师"
tt agent create --role "产品经理" --context "偏增长型 SaaS"
tt agent create --name coder --suggestion "更注重性能优化"
tt agent create --role "Go 后端工程师" --output .agents/go-backend`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAgentCreate(cmd, args)
	},
}

func init() {
	agentCmd.AddCommand(agentCreateCmd)
	agentCreateCmd.Flags().StringVar(&agentCreateRole, "role", "", "role description for creating a new agent")
	agentCreateCmd.Flags().StringVar(&agentCreateName, "name", "", "agent name; used with --suggestion to create from suggestion")
	agentCreateCmd.Flags().StringVar(&agentCreateSuggestion, "suggestion", "", "suggestion for agent creation or optimization")
	agentCreateCmd.Flags().StringVarP(&agentCreateOutput, "output", "o", filepath.Join(".tt", "agents"), "output directory for generated agent")
	agentCreateCmd.Flags().StringSliceVar(&agentCreateSkills, "skill", nil, "agent skills; repeat for multiple")
	agentCreateCmd.Flags().BoolVar(&agentCreateNoHist, "no-history", false, "disable conversation history")
	agentCreateCmd.Flags().BoolVar(&agentCreateTools, "research-tools", false, "include research tools")
	agentCreateCmd.Flags().StringVar(&agentCreateModel, "model", "", "model to use")
	agentCreateCmd.Flags().StringVarP(&agentCreateSession, "session", "s", "cli:agent-create", "session key")
	agentCreateCmd.Flags().BoolVarP(&agentCreateDebug, "debug", "d", false, "enable debug logging")
	agentCreateCmd.Flags().StringVar(&agentCreateHome, "picoclaw-home", "", "override PICOCLAW_HOME")
	agentCreateCmd.Flags().StringVar(&agentCreateConfig, "picoclaw-config", "", "override PICOCLAW_CONFIG")
}

func runAgentCreate(cmd *cobra.Command, args []string) error {
	applyConfiguredAgentDirEnv()

	role := strings.TrimSpace(agentCreateRole)
	name := strings.TrimSpace(agentCreateName)
	suggestion := strings.TrimSpace(agentCreateSuggestion)

	if role == "" && name == "" {
		return fmt.Errorf("--role or --name is required")
	}

	if role != "" && name != "" {
		return fmt.Errorf("use --role for new agent creation, or --name with --suggestion for existing agent")
	}

	if name != "" && suggestion == "" {
		return fmt.Errorf("--suggestion is required when using --name")
	}

	if role != "" {
		return runAgentCreateFromRole(cmd, role)
	}

	return runAgentCreateFromSuggestion(cmd, name, suggestion, false)
}

func runAgentCreateFromRole(cmd *cobra.Command, role string) error {
	prompt, err := nvwa.BuildGenerationPrompt(nvwa.PromptOptions{Role: role})
	if err != nil {
		return err
	}

	files, err := generateAgentFiles(cmd, prompt)
	if err != nil {
		return err
	}

	return writeAgentFiles(files, "both", agentCreateOutput, false)
}

func runAgentCreateFromSuggestion(cmd *cobra.Command, name, suggestion string, isOptimize bool) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if suggestion == "" {
		return fmt.Errorf("suggestion is required")
	}

	id := nvwa.DefaultEmbeddedID(name)
	displayName := strings.TrimSpace(agentCreateName)
	if displayName == "" {
		displayName = name
	}

	if isOptimize {
		if existingPath, ok := findExistingAgentFileByID(id); ok {
			if existingName, ok := readEmbeddedAgentName(existingPath); ok {
				displayName = existingName
			}
		}
	}

	prompt, err := nvwa.BuildGenerationPrompt(nvwa.PromptOptions{Role: name, Context: suggestion})
	if err != nil {
		return err
	}

	files, err := generateAgentFiles(cmd, prompt)
	if err != nil {
		return err
	}

	doc, err := nvwa.RenderEmbeddedMarkdown(files, nvwa.EmbeddedOptions{
		ID:        id,
		Name:      displayName,
		Skills:    agentCreateSkills,
		Tools:     agentResearchTools(agentCreateTools),
		NoHistory: agentCreateNoHist,
	})
	if err != nil {
		return err
	}

	outDir := strings.TrimSpace(agentCreateOutput)
	if outDir == "" || outDir == "." {
		outDir = filepath.Join(".tt", "agents")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	path := filepath.Join(outDir, id+".md")

	if !isOptimize {
		if existing, err := agents.List(); err == nil {
			for _, a := range existing {
				if a.ID == id || strings.EqualFold(strings.TrimSpace(a.Name), strings.TrimSpace(displayName)) {
					return fmt.Errorf("agent %q already exists (id=%s, name=%s); use 'tt agent optimize' to update", displayName, a.ID, a.Name)
				}
			}
		}
	}

	force := isOptimize || agentCreateForce
	if err := writeAgentFile(path, doc, force); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "created agent at %s\n", path)
	return nil
}

func generateAgentFiles(cmd *cobra.Command, prompt string) (nvwa.Files, error) {
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
		merged.Picoclaw.Home = agentCreateHome
	}
	if cmd.Flags().Changed("picoclaw-config") {
		merged.Picoclaw.Config = agentCreateConfig
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
	loading := startLLMLoading("正在生成 agent 提示词", agentCreateDebug)
	designer := promptDesignerAgent()
	dr, err := rt.NewDirectRunner(pcwrap.RunOptions{
		Session:        agentCreateSession,
		Agent:          designer.ID,
		Model:          agentCreateModel,
		Workspace:      workspace,
		Debug:          agentCreateDebug,
		Quiet:          !agentCreateDebug,
		EmbeddedAgents: []pcwrap.EmbeddedAgent{designer},
	})
	if err != nil {
		loading.Stop()
		return nvwa.Files{}, picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	defer dr.Close()

	response, err := dr.ProcessDirect(pcwrap.RunOptions{
		Message:        prompt,
		Session:        agentCreateSession,
		Agent:          designer.ID,
		Model:          agentCreateModel,
		Workspace:      workspace,
		Debug:          agentCreateDebug,
		Quiet:          !agentCreateDebug,
		EmbeddedAgents: []pcwrap.EmbeddedAgent{designer},
	})
	loading.Stop()
	if err != nil {
		return nvwa.Files{}, picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	files, err := nvwa.ParseResponse(response)
	if err != nil {
		return nvwa.Files{}, fmt.Errorf("parse agent output failed: %w\n\nRaw output:\n%s", err, response)
	}
	return files, nil
}

func promptDesignerAgent() pcwrap.EmbeddedAgent {
	return pcwrap.EmbeddedAgent{
		ID:   "agent-prompt-designer",
		Name: "提示词设计师",
		Soul: strings.TrimSpace(`你把提示词当成可执行系统，而不是文案。你在意边界、可验证性和长期可维护性，讨厌空话、套话和无法执行的规则。`),
		Prompt: strings.TrimSpace(`你是一个专门设计 Agent.md 与 SOUL.md 的提示词架构师。

定位铁律（必须遵守）：
1) SOUL.md 定人格（你是谁）
- 解决价值观、性格、语气、偏好、底线。
- 用"形容词 + 取舍偏好 + 禁词/禁风格"来约束表达。
- 不写流程步骤，不写工具调用细节。

2) Agent.md 定行为（你怎么做）
- 解决工作流、权限、步骤、失败处理、澄清策略、交付标准。
- 用"动词 + 条件判断 + 可执行动作"来约束行为。
- 明确遇到模糊需求怎么办、风险动作是否要确认、输出格式是什么。

输出要求：
- 仅输出两个标签，不要解释：
<Agent.md>...</Agent.md>
<soul.md>...</soul.md>
- 两者都必须是完整 Markdown。

质量红线：
- 禁止空话：如"专业、高效、负责"。
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
		NoHistory: true,
	}
}

func agentResearchTools(enabled bool) []string {
	if !enabled {
		return nil
	}
	return []string{"skills", "find_skills", "web_search", "web_fetch", "exec"}
}

func writeAgentFiles(files nvwa.Files, format, dir string, force bool) error {
	if strings.TrimSpace(dir) == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if format == "agent" || format == "both" {
		if err := writeAgentFile(filepath.Join(dir, "Agent.md"), files.Agent, force); err != nil {
			return err
		}
	}
	if format == "soul" || format == "both" {
		if err := writeAgentFile(filepath.Join(dir, "soul.md"), files.Soul, force); err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stdout, "generated %s prompt file(s) in %s\n", format, dir)
	return nil
}

func writeAgentFile(path, content string, force bool) error {
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

func applyConfiguredAgentDirEnv() {
	loaded := mustLoadTTConfig()
	os.Setenv("TT_AGENT_DIR", resolveAgentDir(loaded))
}
