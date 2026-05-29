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
	Short: "Optimize an agent for a target repository",
	Long: `Analyze a local or remote repository, then generate an embedded-agent
Markdown file that specializes an existing agent for that repository domain.

First version: repository input only. Website ingestion is not implemented yet.`,
	Example: `tt agent optimize --target ./repo --agent .tt/agents/custom.md
 tt agent optimize --target github.com/gin-gonic/gin --agent .tt/agents/custom.md --copy
 tt agent optimize --target ./repo --agent coder --copy`,
	RunE: runAgentOptimize,
}

func init() {
	agentOptimizeCmd.Flags().StringVar(&agentOptimizeTarget, "target", "", "target repository path or cloneable URL")
	agentOptimizeCmd.Flags().StringVar(&agentOptimizeBaseAgent, "agent", "", "base agent id or local .md agent file")
	agentOptimizeCmd.Flags().StringVarP(&agentOptimizeOutput, "output", "o", "", "advanced: write generated embedded-agent Markdown to explicit file or directory")
	agentOptimizeCmd.Flags().BoolVarP(&agentOptimizeForce, "force", "f", false, "overwrite existing output file")
	agentOptimizeCmd.Flags().BoolVar(&agentOptimizeCopy, "copy", false, "create a new optimized agent next to the source agent instead of updating it in place")
	agentOptimizeCmd.Flags().StringVar(&agentOptimizeSession, "session", "cli:agent-optimize", "session key for agent optimization")
	agentOptimizeCmd.Flags().StringVar(&agentOptimizeModel, "model", "", "model override for the embedded agent optimizer")
	agentOptimizeCmd.Flags().BoolVarP(&agentOptimizeDebug, "debug", "d", false, "enable debug logging")
	agentOptimizeCmd.Flags().IntVar(&agentOptimizeMaxFiles, "max-files", 200, "maximum relevant files to collect")
	agentOptimizeCmd.Flags().Int64Var(&agentOptimizeMaxFileSize, "max-file-size", 256*1024, "maximum bytes per collected file")
	agentOptimizeCmd.Flags().IntVar(&agentOptimizeMaxPromptChars, "max-prompt-chars", 12000, "maximum characters allowed in the optimized agent prompt to prevent prompt bloat")
	agentOptimizeCmd.Flags().DurationVar(&agentOptimizeTimeout, "timeout", 2*time.Minute, "timeout for repository preparation and optimization")
	agentOptimizeCmd.Flags().BoolVar(&agentOptimizeKeepTemp, "keep-temp", false, "keep temporary cloned repository for debugging")
	agentCmd.AddCommand(agentOptimizeCmd)
}

func runAgentOptimize(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(agentOptimizeTarget) == "" {
		return fmt.Errorf("--target is required")
	}
	if strings.TrimSpace(agentOptimizeBaseAgent) == "" {
		return fmt.Errorf("--agent is required")
	}
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
	result, err := optimizer.Optimize(agentopt.Options{Target: agentOptimizeTarget, BaseAgent: agentOptimizeBaseAgent, OutputPath: agentOptimizeOutput, Force: agentOptimizeForce, Copy: agentOptimizeCopy, MaxFiles: agentOptimizeMaxFiles, MaxFileSize: agentOptimizeMaxFileSize, MaxPromptChars: agentOptimizeMaxPromptChars, Timeout: agentOptimizeTimeout, KeepTemp: agentOptimizeKeepTemp})
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
