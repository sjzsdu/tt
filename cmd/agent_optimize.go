package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/agentopt"
	"github.com/sjzsdu/tt/internal/agents"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
)

var (
	agentOptimizeTarget         string
	agentOptimizeBaseAgent      string
	agentOptimizeSuggestion     string
	agentOptimizeOutput         string
	agentOptimizeForce          bool
	agentOptimizeCopy           bool
	agentOptimizeSession        string
	agentOptimizeModel          string
	agentOptimizeDebug          bool
	agentOptimizeMaxFiles       int
	agentOptimizeMaxFileSize    int64
	agentOptimizeMaxPromptChars int
	agentOptimizeTimeout        time.Duration
	agentOptimizeKeepTemp       bool
)

var agentOptimizeCmd = &cobra.Command{
	Use:   "optimize",
	Short: "Optimize an existing agent",
	Long: `Optimize an existing agent. You can optimize by:
1. Analyzing a repository and distilling its knowledge into the agent (--target)
2. Applying a natural language suggestion to improve the agent (--suggestion)

These two approaches can be combined: provide both --target and --suggestion.`,
	Example: `tt agent optimize --agent coder --target ./repo
tt agent optimize --agent coder --suggestion "更注重性能优化"
tt agent optimize --agent coder --target ./repo --suggestion "关注安全方面"
tt agent optimize --agent .tt/agents/custom.md --target github.com/gin-gonic/gin --copy`,
	RunE: runAgentOptimize,
}

func init() {
	agentCmd.AddCommand(agentOptimizeCmd)
	agentOptimizeCmd.Flags().StringVar(&agentOptimizeTarget, "target", "", "target repository path or cloneable URL")
	agentOptimizeCmd.Flags().StringVar(&agentOptimizeBaseAgent, "agent", "", "base agent id or local .md agent file")
	agentOptimizeCmd.Flags().StringVar(&agentOptimizeSuggestion, "suggestion", "", "natural language suggestion for optimization")
	agentOptimizeCmd.Flags().StringVarP(&agentOptimizeOutput, "output", "o", "", "write generated embedded-agent Markdown to explicit file or directory")
	agentOptimizeCmd.Flags().BoolVarP(&agentOptimizeForce, "force", "f", false, "overwrite existing output file")
	agentOptimizeCmd.Flags().BoolVar(&agentOptimizeCopy, "copy", false, "create a new optimized agent next to the source agent instead of updating it in place")
	agentOptimizeCmd.Flags().StringVar(&agentOptimizeSession, "session", "cli:agent-optimize", "session key for agent optimization")
	agentOptimizeCmd.Flags().StringVar(&agentOptimizeModel, "model", "", "model override for the agent optimizer")
	agentOptimizeCmd.Flags().BoolVarP(&agentOptimizeDebug, "debug", "d", false, "enable debug logging")
	agentOptimizeCmd.Flags().IntVar(&agentOptimizeMaxFiles, "max-files", 200, "maximum relevant files to collect")
	agentOptimizeCmd.Flags().Int64Var(&agentOptimizeMaxFileSize, "max-file-size", 256*1024, "maximum bytes per collected file")
	agentOptimizeCmd.Flags().IntVar(&agentOptimizeMaxPromptChars, "max-prompt-chars", 12000, "maximum characters allowed in the optimized agent prompt")
	agentOptimizeCmd.Flags().DurationVar(&agentOptimizeTimeout, "timeout", 2*time.Minute, "timeout for repository preparation and optimization")
	agentOptimizeCmd.Flags().BoolVar(&agentOptimizeKeepTemp, "keep-temp", false, "keep temporary cloned repository for debugging")
}

func runAgentOptimize(cmd *cobra.Command, args []string) error {
	agentID := strings.TrimSpace(agentOptimizeBaseAgent)
	suggestion := strings.TrimSpace(agentOptimizeSuggestion)
	target := strings.TrimSpace(agentOptimizeTarget)

	if agentID == "" {
		return fmt.Errorf("--agent is required")
	}

	if target == "" && suggestion == "" {
		return fmt.Errorf("at least one of --target or --suggestion is required")
	}

	if target != "" {
		return runAgentOptimizeWithRepo(cmd, agentID, target, suggestion)
	}

	return runAgentOptimizeWithSuggestion(cmd, agentID, suggestion)
}

func runAgentOptimizeWithRepo(cmd *cobra.Command, agentID, target, suggestion string) error {
	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	merged := loaded.Merged
	if agentOptimizeModel != "" {
		merged.Agent.Model = agentOptimizeModel
	}
	if agentOptimizeDebug {
		merged.Agent.Debug = &agentOptimizeDebug
	}
	workspace, resolvedHome, resolvedConfig, restoreStorage, err := useTTAgentStorage(merged.Picoclaw.Home, merged.Picoclaw.Config)
	if err != nil {
		return err
	}
	defer restoreStorage()
	merged.Picoclaw.Home = resolvedHome
	merged.Picoclaw.Config = resolvedConfig
	if err := ensurePicoclawConfigAvailable(merged.Picoclaw.Home, merged.Picoclaw.Config); err != nil {
		return err
	}
	rt, err := pcwrap.Load(pcwrap.Options{Home: merged.Picoclaw.Home, Config: merged.Picoclaw.Config, TTConfig: merged, TTSources: loaded.Sources})
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	embedded := []pcwrap.EmbeddedAgent{agents.AgentOptimizer()}
	runner, err := rt.NewDirectRunner(pcwrap.RunOptions{Session: agentOptimizeSession, Agent: agents.AgentOptimizerID, Model: merged.Agent.Model, Workspace: workspace, Debug: agentOptimizeDebug, Quiet: !agentOptimizeDebug, EmbeddedAgents: embedded})
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	defer runner.Close()
	optimizer := agentopt.New(agentOptimizeDirectProcessor{runner: runner, base: pcwrap.RunOptions{Session: agentOptimizeSession, Agent: agents.AgentOptimizerID, Model: merged.Agent.Model, Workspace: workspace, Debug: agentOptimizeDebug, Quiet: !agentOptimizeDebug, EmbeddedAgents: embedded}, debug: agentOptimizeDebug})
	result, err := optimizer.Optimize(agentopt.Options{Target: target, BaseAgent: agentID, OutputPath: agentOptimizeOutput, Force: agentOptimizeForce, Copy: agentOptimizeCopy, MaxFiles: agentOptimizeMaxFiles, MaxFileSize: agentOptimizeMaxFileSize, MaxPromptChars: agentOptimizeMaxPromptChars, Timeout: agentOptimizeTimeout, KeepTemp: agentOptimizeKeepTemp})
	if err != nil {
		return err
	}
	if result.Output == "" {
		fmt.Fprint(cmd.OutOrStdout(), result.Markdown)
		return nil
	}
	abs, _ := filepath.Abs(result.Output)
	if result.InPlace {
		fmt.Fprintf(cmd.OutOrStdout(), "updated optimized agent in place: %s\n", abs)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "generated optimized agent: %s\n", abs)
	}
	return nil
}

func runAgentOptimizeWithSuggestion(cmd *cobra.Command, agentID, suggestion string) error {
	return runAgentCreateFromSuggestion(cmd, agentID, suggestion, true)
}

type agentOptimizeDirectProcessor struct {
	runner *pcwrap.DirectRunner
	base   pcwrap.RunOptions
	debug  bool
}

func (p agentOptimizeDirectProcessor) ProcessDirect(message string) (string, error) {
	loading := startLLMLoading("正在优化 agent 提示词", p.debug)
	defer loading.Stop()
	opt := p.base
	opt.Message = message
	return p.runner.ProcessDirect(opt)
}
