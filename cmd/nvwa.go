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
	nvwaWrite   bool
	nvwaOutput  string
	nvwaForce   bool
	nvwaFormat  string
	nvwaStyle   string
	nvwaID      string
	nvwaName    string
	nvwaSkills  []string
	nvwaModel   string
	nvwaSession string
	nvwaContext string
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

func init() {
	rootCmd.AddCommand(nvwaCmd)
	nvwaCmd.Flags().BoolVarP(&nvwaWrite, "write", "w", true, "write generated file(s); set --write=false to print to stdout")
	nvwaCmd.Flags().StringVarP(&nvwaOutput, "output", "o", ".", "output directory when writing files")
	nvwaCmd.Flags().BoolVarP(&nvwaForce, "force", "f", false, "overwrite existing files when --write is set")
	nvwaCmd.Flags().StringVar(&nvwaFormat, "format", "both", "content to generate: agent, soul, or both")
	nvwaCmd.Flags().StringVar(&nvwaStyle, "style", "files", "output style: files or embedded")
	nvwaCmd.Flags().StringVar(&nvwaID, "id", "", "embedded agent id when --style embedded is used")
	nvwaCmd.Flags().StringVar(&nvwaName, "name", "", "embedded agent display name when --style embedded is used; defaults to role")
	nvwaCmd.Flags().StringSliceVar(&nvwaSkills, "skill", nil, "embedded agent skill; repeat or comma-separate for multiple skills")
	nvwaCmd.Flags().BoolVar(&nvwaNoHist, "no-history", false, "set no_history: true when --style embedded is used")
	nvwaCmd.Flags().BoolVar(&nvwaTools, "research-tools", false, "set enable_research_tools: true when --style embedded is used")
	nvwaCmd.Flags().StringVar(&nvwaContext, "context", "", "extra role context, target scenario, style, or constraints")
	nvwaCmd.Flags().StringVar(&nvwaModel, "model", "", "model to use; defaults to the picoclaw default model")
	nvwaCmd.Flags().StringVarP(&nvwaSession, "session", "s", "cli:nvwa", "session key")
	nvwaCmd.Flags().BoolVarP(&nvwaDebug, "debug", "d", false, "enable debug logging")
	nvwaCmd.Flags().StringVar(&nvwaHome, "picoclaw-home", "", "override PICOCLAW_HOME for this run")
	nvwaCmd.Flags().StringVar(&nvwaConfig, "picoclaw-config", "", "override PICOCLAW_CONFIG for this run")
}

func runNvwa(cmd *cobra.Command, args []string) error {
	role := strings.TrimSpace(strings.Join(args, " "))
	prompt, err := nvwa.BuildGenerationPrompt(nvwa.PromptOptions{Role: role, Context: nvwaContext})
	if err != nil {
		return err
	}
	style := strings.ToLower(strings.TrimSpace(nvwaStyle))
	switch style {
	case "files", "embedded":
	default:
		return fmt.Errorf("invalid --style %q: must be files or embedded", nvwaStyle)
	}

	format := strings.ToLower(strings.TrimSpace(nvwaFormat))
	switch format {
	case "agent", "soul", "both":
	default:
		return fmt.Errorf("invalid --format %q: must be agent, soul, or both", nvwaFormat)
	}

	files, err := generateNvwaFiles(cmd, prompt)
	if err != nil {
		return err
	}
	if style == "embedded" {
		if format != "both" {
			return fmt.Errorf("--style embedded requires --format both because soul.md is stored in YAML frontmatter")
		}
		return outputNvwaEmbedded(files, role)
	}
	if nvwaWrite {
		return writeNvwaFiles(files, format, nvwaOutput, nvwaForce)
	}
	printNvwaFiles(files, format)
	return nil
}

func outputNvwaEmbedded(files nvwa.Files, role string) error {
	id := strings.TrimSpace(nvwaID)
	if id == "" {
		id = nvwa.DefaultEmbeddedID(role)
	}
	name := strings.TrimSpace(nvwaName)
	if name == "" {
		name = role
	}
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
	if !nvwaWrite {
		fmt.Print(doc)
		return nil
	}
	dir := strings.TrimSpace(nvwaOutput)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	path := filepath.Join(dir, id+".md")
	if err := writeNvwaFile(path, doc, nvwaForce); err != nil {
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
	workspace, err := ensureNvwaWorkspace()
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
	dr, err := rt.NewDirectRunner(pcwrap.RunOptions{
		Session:        nvwaSession,
		Agent:          agents.NvwaPromptDesignerID,
		Model:          nvwaModel,
		Workspace:      workspace,
		Debug:          nvwaDebug,
		Quiet:          !nvwaDebug,
		EmbeddedAgents: []pcwrap.EmbeddedAgent{agents.NvwaPromptDesigner()},
	})
	if err != nil {
		loading.Stop()
		return nvwa.Files{}, picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	defer dr.Close()

	response, err := dr.ProcessDirect(pcwrap.RunOptions{
		Message:        prompt,
		Session:        nvwaSession,
		Agent:          agents.NvwaPromptDesignerID,
		Model:          nvwaModel,
		Workspace:      workspace,
		Debug:          nvwaDebug,
		Quiet:          !nvwaDebug,
		EmbeddedAgents: []pcwrap.EmbeddedAgent{agents.NvwaPromptDesigner()},
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

func ensureNvwaWorkspace() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current directory: %w", err)
	}
	workspace := filepath.Join(cwd, ".tt")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return "", fmt.Errorf("create nvwa workspace: %w", err)
	}
	return workspace, nil
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
