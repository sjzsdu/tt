package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sjzsdu/tt/internal/agents"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	repo2skillpkg "github.com/sjzsdu/tt/internal/repo2skill"
	"github.com/spf13/cobra"
)

var (
	repo2skillTargetDir       string
	repo2skillDryRun          bool
	repo2skillMarkdown        bool
	repo2skillIntent          string
	repo2skillLanguage        string
	repo2skillMaxFiles        int
	repo2skillMaxFileSize     int64
	repo2skillTimeout         time.Duration
	repo2skillKeepTemp        bool
	repo2skillIncludeEvidence bool
	repo2skillAnalyzerMode    string
	repo2skillAgentModel      string
	repo2skillAgentSession    string
	repo2skillAgentDebug      bool
)

var repo2skillCmd = &cobra.Command{
	Use:   "repo2skill [repo-path-or-url]",
	Short: "Convert a repository into an agent-oriented library skill",
	Long: `Analyze a local or remote repository and generate skill files that help an
agent understand what the library solves, which public APIs to use, and how to
apply it in development tasks. The default analyzer mode is auto: use the
embedded repo2skill Picoclaw agent when available, otherwise fall back to the
deterministic heuristic analyzer.`,
	Example: `tt repo2skill ./my-library
tt repo2skill https://github.com/colinhacks/zod
tt repo2skill github.com/gin-gonic/gin --dry-run
tt repo2skill ./repo --analyzer agent --model gpt-5.4
tt repo2skill ./repo --target-dir ./.agents/skills`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error { return runRepo2Skill(args) },
}

func init() {
	rootCmd.AddCommand(repo2skillCmd)
	repo2skillCmd.Flags().StringVar(&repo2skillTargetDir, "target-dir", "~/.agents/skills", "directory to write skill files")
	repo2skillCmd.Flags().BoolVar(&repo2skillDryRun, "dry-run", false, "print skill content to stdout instead of writing files")
	repo2skillCmd.Flags().BoolVar(&repo2skillMarkdown, "markdown", false, "open generated skill content with markdown command instead of writing files")
	repo2skillCmd.Flags().StringVar(&repo2skillIntent, "intent", "use-library", "skill intent: use-library, contribute, api-reference, architecture")
	repo2skillCmd.Flags().StringVar(&repo2skillLanguage, "language", "", "preferred output language hint for future agent analysis")
	repo2skillCmd.Flags().IntVar(&repo2skillMaxFiles, "max-files", 200, "maximum relevant files to collect")
	repo2skillCmd.Flags().Int64Var(&repo2skillMaxFileSize, "max-file-size", 256*1024, "maximum bytes per collected file")
	repo2skillCmd.Flags().DurationVar(&repo2skillTimeout, "timeout", 2*time.Minute, "timeout for git clone and analysis steps")
	repo2skillCmd.Flags().BoolVar(&repo2skillKeepTemp, "keep-temp", false, "keep cloned temporary repository for debugging")
	repo2skillCmd.Flags().BoolVar(&repo2skillIncludeEvidence, "include-evidence", false, "write references/evidence.md and link it from SKILL.md for audit/debugging")
	repo2skillCmd.Flags().StringVar(&repo2skillAnalyzerMode, "analyzer", "auto", "analysis mode: auto, agent, or heuristic")
	repo2skillCmd.Flags().StringVar(&repo2skillAgentModel, "model", "", "Picoclaw model override for --analyzer agent/auto")
	repo2skillCmd.Flags().StringVar(&repo2skillAgentSession, "session", "cli:repo2skill", "Picoclaw session key for agent analysis")
	repo2skillCmd.Flags().BoolVarP(&repo2skillAgentDebug, "debug", "d", false, "enable debug logging for agent analysis")
}

func runRepo2Skill(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("repository path or URL required")
	}
	analyzer, cleanup, err := buildRepo2SkillAnalyzer()
	if err != nil {
		return err
	}
	defer cleanup()
	return repo2skillpkg.Run(args[0], repo2skillpkg.Options{TargetDir: repo2skillTargetDir, DryRun: repo2skillDryRun, Markdown: repo2skillMarkdown, Intent: repo2skillIntent, Language: repo2skillLanguage, MaxFiles: repo2skillMaxFiles, MaxFileSize: repo2skillMaxFileSize, Timeout: repo2skillTimeout, KeepTemp: repo2skillKeepTemp, IncludeEvidence: repo2skillIncludeEvidence, Analyzer: analyzer}, os.Stdout)
}

func buildRepo2SkillAnalyzer() (repo2skillpkg.Analyzer, func(), error) {
	mode := strings.ToLower(strings.TrimSpace(repo2skillAnalyzerMode))
	if mode == "" {
		mode = "auto"
	}
	if mode == "heuristic" {
		return repo2skillpkg.HeuristicAnalyzer{}, func() {}, nil
	}
	if mode != "auto" && mode != "agent" {
		return nil, nil, fmt.Errorf("invalid --analyzer %q: expected auto, agent, or heuristic", repo2skillAnalyzerMode)
	}

	analyzer, cleanup, err := newRepo2SkillAgentAnalyzer()
	if err == nil {
		if mode == "auto" {
			return repo2skillpkg.FallbackAnalyzer{Primary: analyzer, Fallback: repo2skillpkg.HeuristicAnalyzer{}, Log: os.Stderr}, cleanup, nil
		}
		return analyzer, cleanup, nil
	}
	if mode == "agent" {
		return nil, nil, err
	}
	fmt.Fprintf(os.Stderr, "repo2skill: agent analyzer unavailable, falling back to heuristic analyzer: %v\n", err)
	return repo2skillpkg.HeuristicAnalyzer{}, func() {}, nil
}

type repo2SkillDirectProcessor struct {
	runner *pcwrap.DirectRunner
	base   pcwrap.RunOptions
	debug  bool
}

func (p repo2SkillDirectProcessor) ProcessDirect(message string) (string, error) {
	loading := startLLMLoading("正在用 repo2skill agent 分析仓库", p.debug)
	defer loading.Stop()
	opt := p.base
	opt.Message = message
	return p.runner.ProcessDirect(opt)
}

func newRepo2SkillAgentAnalyzer() (repo2skillpkg.Analyzer, func(), error) {
	loaded, err := loadTTConfig()
	if err != nil {
		return nil, nil, err
	}
	merged := loaded.Merged
	if repo2skillAgentModel != "" {
		merged.Agent.Model = repo2skillAgentModel
	}
	if repo2skillAgentDebug {
		merged.Agent.Debug = &repo2skillAgentDebug
	}
	workspace, resolvedHome, resolvedConfig, restoreStorage, err := useTTAgentStorage(merged.Picoclaw.Home, merged.Picoclaw.Config)
	if err != nil {
		return nil, nil, err
	}
	defer restoreStorage()
	merged.Picoclaw.Home = resolvedHome
	merged.Picoclaw.Config = resolvedConfig
	if err := ensurePicoclawConfigAvailable(merged.Picoclaw.Home, merged.Picoclaw.Config); err != nil {
		return nil, nil, err
	}
	rt, err := pcwrap.Load(pcwrap.Options{Home: merged.Picoclaw.Home, Config: merged.Picoclaw.Config, TTConfig: merged, TTSources: loaded.Sources})
	if err != nil {
		return nil, nil, picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	repo2skillAgent, err := agents.Repo2Skill()
	if err != nil {
		return nil, nil, fmt.Errorf("load repo2skill agent failed: %w", err)
	}
	embedded := []pcwrap.EmbeddedAgent{repo2skillAgent}
	runner, err := rt.NewDirectRunner(pcwrap.RunOptions{Session: repo2skillAgentSession, Agent: agents.Repo2SkillID, Model: merged.Agent.Model, Workspace: workspace, Debug: repo2skillAgentDebug, Quiet: !repo2skillAgentDebug, EmbeddedAgents: embedded})
	if err != nil {
		return nil, nil, err
	}
	processor := repo2SkillDirectProcessor{runner: runner, base: pcwrap.RunOptions{Session: repo2skillAgentSession, Agent: agents.Repo2SkillID, Model: merged.Agent.Model, Workspace: workspace, Debug: repo2skillAgentDebug, Quiet: !repo2skillAgentDebug, EmbeddedAgents: embedded}, debug: repo2skillAgentDebug}
	return repo2skillpkg.AgentAnalyzer{Processor: processor}, runner.Close, nil
}
