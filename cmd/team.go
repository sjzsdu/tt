package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/agents"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	teamruntime "github.com/sjzsdu/tt/internal/team"
	ttconfig "github.com/sjzsdu/tt/internal/ttconfig"
)

var (
	teamSession        string
	teamModel          string
	teamDebug          bool
	teamHome           string
	teamConfig         string
	teamNoMemory       bool
	teamShowDiscussion bool
)

var teamCmd = &cobra.Command{
	Use:   "team",
	Short: "Run persistent teams of collaborating agents",
	Long: `Run a TOML-defined team whose agents share a public discussion, keep
isolated sessions, persist thread history, and maintain durable team memory.`,
}

var teamRunCmd = &cobra.Command{
	Use:   "run <team> <message>",
	Short: "Start a new persistent team thread",
	Args:  cobra.MinimumNArgs(2),
	Example: `tt team init product-review
tt team run product-review "评估这个需求的产品和技术风险"
tt team run .tt/teams/product-review/team.toml "给出 MVP 方案"`,
	RunE: runTeamRun,
}

var teamAskCmd = &cobra.Command{
	Use:   "ask <thread> <message>",
	Short: "Ask a follow-up question in an existing team thread",
	Args:  cobra.MinimumNArgs(2),
	Example: `tt team ask latest "再考虑一下迁移成本"
tt team ask product-review/20260723-120000-a1b2c3 "给出实施顺序"`,
	RunE: runTeamAsk,
}

var teamResumeCmd = &cobra.Command{
	Use:   "resume [thread]",
	Short: "Resume an interrupted or failed team round",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runTeamResume,
}

var teamShowCmd = &cobra.Command{
	Use:   "show [thread]",
	Short: "Show a persisted team thread and its public events",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runTeamShow,
}

var teamListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available team definitions and persisted threads",
	Args:  cobra.NoArgs,
	RunE:  runTeamList,
}

var teamInitCmd = &cobra.Command{
	Use:   "init <name>",
	Short: "Create a starter team definition",
	Args:  cobra.ExactArgs(1),
	RunE:  runTeamInit,
}

func init() {
	rootCmd.AddCommand(teamCmd)
	teamCmd.AddCommand(teamRunCmd)
	teamCmd.AddCommand(teamAskCmd)
	teamCmd.AddCommand(teamResumeCmd)
	teamCmd.AddCommand(teamShowCmd)
	teamCmd.AddCommand(teamListCmd)
	teamCmd.AddCommand(teamInitCmd)

	teamCmd.PersistentFlags().StringVarP(&teamSession, "session", "s", "cli:team", "session key prefix for team agents")
	teamCmd.PersistentFlags().StringVar(&teamModel, "model", "", "model override for all team agents")
	teamCmd.PersistentFlags().BoolVarP(&teamDebug, "debug", "d", false, "enable agent runtime debug logging")
	teamCmd.PersistentFlags().StringVar(&teamHome, "picoclaw-home", "", "override PICOCLAW_HOME for this run")
	teamCmd.PersistentFlags().StringVar(&teamConfig, "picoclaw-config", "", "override PICOCLAW_CONFIG for this run")
	teamCmd.PersistentFlags().BoolVar(&teamNoMemory, "no-memory", false, "do not read or update team memory for this invocation")
	teamCmd.PersistentFlags().BoolVar(&teamShowDiscussion, "show-discussion", true, "stream public team discussion before the final answer")
}

func runTeamRun(cmd *cobra.Command, args []string) error {
	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	projectRoot := projectRootFromConfig(loaded)
	definition, err := teamruntime.Load(args[0], teamruntime.DefaultSearchPaths(projectRoot)...)
	if err != nil {
		return err
	}
	question := strings.TrimSpace(strings.Join(args[1:], " "))
	return executeTeamRound(cmd, loaded, projectRoot, definition, nil, question, false)
}

func runTeamAsk(cmd *cobra.Command, args []string) error {
	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	projectRoot := projectRootFromConfig(loaded)
	store, err := teamruntime.ResolveStore(projectRoot, args[0])
	if err != nil {
		return err
	}
	definition, err := store.LoadDefinition()
	if err != nil {
		return err
	}
	question := strings.TrimSpace(strings.Join(args[1:], " "))
	return executeTeamRound(cmd, loaded, projectRoot, definition, store, question, false)
}

func runTeamResume(cmd *cobra.Command, args []string) error {
	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	projectRoot := projectRootFromConfig(loaded)
	id := "latest"
	if len(args) == 1 {
		id = args[0]
	}
	store, err := teamruntime.ResolveStore(projectRoot, id)
	if err != nil {
		return err
	}
	definition, err := store.LoadDefinition()
	if err != nil {
		return err
	}
	return executeTeamRound(cmd, loaded, projectRoot, definition, store, "", true)
}

func executeTeamRound(cmd *cobra.Command, loaded ttconfig.Loaded, projectRoot string, definition *teamruntime.Definition, store *teamruntime.Store, question string, resume bool) error {
	merged := loaded.Merged
	cli := ttconfig.Config{}
	if cmd.Flags().Changed("model") || teamCmd.PersistentFlags().Changed("model") {
		cli.Agent.Model = teamModel
	}
	if cmd.Flags().Changed("debug") || teamCmd.PersistentFlags().Changed("debug") {
		cli.Agent.Debug = ttconfig.BoolPtr(teamDebug)
	}
	if cmd.Flags().Changed("picoclaw-home") || teamCmd.PersistentFlags().Changed("picoclaw-home") {
		cli.Picoclaw.Home = teamHome
	}
	if cmd.Flags().Changed("picoclaw-config") || teamCmd.PersistentFlags().Changed("picoclaw-config") {
		cli.Picoclaw.Config = teamConfig
	}
	merged = ttconfig.Merge(merged, cli)

	_, resolvedHome, resolvedConfig, restoreStorage, err := useTTAgentStorage(merged.Picoclaw.Home, merged.Picoclaw.Config)
	if err != nil {
		return err
	}
	defer restoreStorage()
	merged.Picoclaw.Home = resolvedHome
	merged.Picoclaw.Config = resolvedConfig
	if err := ensurePicoclawConfigAvailable(merged.Picoclaw.Home, merged.Picoclaw.Config); err != nil {
		return err
	}
	rt, err := pcwrap.Load(pcwrap.Options{
		Home:      merged.Picoclaw.Home,
		Config:    merged.Picoclaw.Config,
		TTConfig:  merged,
		TTSources: loaded.Sources,
	})
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	embedded, err := agents.All()
	if err != nil {
		return fmt.Errorf("load embedded agents for team: %w", err)
	}
	model := strings.TrimSpace(teamModel)
	if model == "" {
		model = strings.TrimSpace(merged.Agent.Model)
	}
	debug := teamDebug
	if merged.Agent.Debug != nil {
		debug = *merged.Agent.Debug
	}
	processor := newTeamPicoclawProcessor(rt, embedded, projectRoot, merged.Agent.Agent, model, debug)
	defer processor.Close()
	if err := processor.ValidateDefinition(definition); err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	if store == nil {
		store, err = teamruntime.NewStore(projectRoot, definition)
		if err != nil {
			return err
		}
	}

	engine := &teamruntime.Engine{
		Definition:    definition,
		Store:         store,
		Processor:     processor,
		SessionPrefix: teamSession,
		Model:         model,
		DisableMemory: teamNoMemory,
	}
	if teamShowDiscussion {
		engine.OnEvent = func(event teamruntime.Event) {
			switch event.Type {
			case "agent_message":
				fmt.Fprintf(cmd.OutOrStdout(), "\n@%s:\n%s\n", event.From, strings.TrimSpace(event.Content))
			case "agent_yield":
				fmt.Fprintf(cmd.OutOrStdout(), "\n@%s: [YIELD]\n", event.From)
			}
		}
	}
	runCtx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	loading := startLLMLoading("正在等待 team 协作", debug)
	defer loading.Stop()

	var result teamruntime.RunResult
	if resume {
		result, err = engine.Resume(runCtx)
	} else {
		result, err = engine.RunRound(runCtx, question)
	}
	loading.Stop()
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nFinal answer:\n%s\n", strings.TrimSpace(result.Answer))
	fmt.Fprintf(cmd.ErrOrStderr(), "Team thread: %s\n", result.ThreadID)
	if result.MemoryWarning != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: team memory was not updated: %v\n", result.MemoryWarning)
	} else if definition.MemoryEnabled() && !teamNoMemory {
		fmt.Fprintf(cmd.ErrOrStderr(), "Team memory: %s (version %d)\n", result.Memory.Path, result.Memory.Version)
	}
	return nil
}

func runTeamShow(cmd *cobra.Command, args []string) error {
	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	id := "latest"
	if len(args) == 1 {
		id = args[0]
	}
	store, err := teamruntime.ResolveStore(projectRootFromConfig(loaded), id)
	if err != nil {
		return err
	}
	events, err := store.Events()
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Thread: %s\nTeam: %s\nStatus: %s\nRound: %d\nMemory: %s\nUpdated: %s\n",
		store.Thread.ID,
		store.Thread.Team,
		store.Thread.Status,
		store.Thread.CurrentRound,
		store.Thread.MemoryPath,
		store.Thread.UpdatedAt,
	)
	for _, event := range events {
		switch event.Type {
		case "user_message":
			fmt.Fprintf(out, "\nRound %d | user\n%s\n", event.Round, event.Content)
		case "agent_message":
			fmt.Fprintf(out, "\nRound %d | @%s | %s", event.Round, event.From, event.Phase)
			if event.Wave > 0 {
				fmt.Fprintf(out, " %d", event.Wave)
			}
			fmt.Fprintf(out, "\n%s\n", event.Content)
		case "agent_yield":
			fmt.Fprintf(out, "\nRound %d | @%s | %s %d\n[YIELD]\n", event.Round, event.From, event.Phase, event.Wave)
		case "final_answer":
			fmt.Fprintf(out, "\nRound %d | final answer by @%s\n%s\n", event.Round, event.From, event.Content)
		case "memory_updated":
			fmt.Fprintf(out, "\nRound %d | %s\n", event.Round, event.Content)
		}
	}
	return nil
}

func runTeamList(cmd *cobra.Command, _ []string) error {
	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	projectRoot := projectRootFromConfig(loaded)
	definitions, err := teamruntime.List(teamruntime.DefaultSearchPaths(projectRoot)...)
	if err != nil {
		return err
	}
	threads, err := teamruntime.ListThreads(projectRoot)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Teams:")
	if len(definitions) == 0 {
		fmt.Fprintln(out, "  (none; run `tt team init <name>`)")
	}
	for _, record := range definitions {
		title := record.Title
		if title == "" {
			title = record.Name
		}
		fmt.Fprintf(out, "  %-24s %2d agents  %s\n", record.Name, record.Agents, title)
	}
	fmt.Fprintln(out, "\nThreads:")
	if len(threads) == 0 {
		fmt.Fprintln(out, "  (none)")
	}
	for _, record := range threads {
		fmt.Fprintf(out, "  %-42s %-12s round %-3d %s\n",
			record.Thread.ID,
			record.Thread.Status,
			record.Thread.CurrentRound,
			record.Thread.UpdatedAt,
		)
	}
	return nil
}

func runTeamInit(cmd *cobra.Command, args []string) error {
	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	name := teamNameSlug(args[0])
	if name == "" {
		return fmt.Errorf("team name must contain at least one letter or number")
	}
	dir := filepath.Join(projectRootFromConfig(loaded), ".tt", "teams", name)
	path := filepath.Join(dir, teamruntime.DefinitionFilename)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("team definition already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create team definition directory: %w", err)
	}
	content := starterTeamDefinition(name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create team definition: %w", err)
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		return fmt.Errorf("write team definition: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close team definition: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Created team definition: %s\n", path)
	fmt.Fprintf(cmd.OutOrStdout(), "Run it with: tt team run %s \"your question\"\n", name)
	return nil
}

type teamPicoclawProcessor struct {
	runtime      *pcwrap.Runtime
	embedded     []pcwrap.EmbeddedAgent
	workspace    string
	defaultAgent string
	defaultModel string
	debug        bool
	mu           sync.Mutex
	runners      map[string]*pcwrap.DirectRunner
	runnerLocks  map[string]*sync.Mutex
}

func newTeamPicoclawProcessor(rt *pcwrap.Runtime, embedded []pcwrap.EmbeddedAgent, workspace, defaultAgent, defaultModel string, debug bool) *teamPicoclawProcessor {
	return &teamPicoclawProcessor{
		runtime:      rt,
		embedded:     embedded,
		workspace:    workspace,
		defaultAgent: defaultAgent,
		defaultModel: defaultModel,
		debug:        debug,
		runners:      map[string]*pcwrap.DirectRunner{},
		runnerLocks:  map[string]*sync.Mutex{},
	}
}

func (p *teamPicoclawProcessor) ValidateDefinition(definition *teamruntime.Definition) error {
	var failures []string
	for _, member := range definition.Agents {
		agentName := firstTeamValue(member.Agent, p.defaultAgent)
		model := firstTeamValue(member.Model, p.defaultModel)
		if _, err := p.runtime.ResolveRunOptions(pcwrap.RunOptions{
			Session:        "cli:team:preflight:" + member.ID,
			Agent:          agentName,
			Model:          model,
			EmbeddedAgents: p.embedded,
		}); err != nil {
			failures = append(failures, fmt.Sprintf("@%s (%s): %v", member.ID, agentName, err))
		}
	}
	if len(failures) > 0 {
		sort.Strings(failures)
		return fmt.Errorf("team agent preflight failed:\n  %s", strings.Join(failures, "\n  "))
	}
	return nil
}

func (p *teamPicoclawProcessor) Process(ctx context.Context, call teamruntime.AgentCall) (string, error) {
	key := strings.ToLower(call.MemberID + "|" + firstTeamValue(call.Model, p.defaultModel))
	runner, lock, err := p.runner(key, call)
	if err != nil {
		return "", err
	}
	lock.Lock()
	defer lock.Unlock()
	return runner.ProcessDirectContext(ctx, pcwrap.RunOptions{
		Message:        call.Prompt,
		Session:        call.Session,
		Agent:          firstTeamValue(call.Agent, p.defaultAgent),
		Model:          firstTeamValue(call.Model, p.defaultModel),
		Workspace:      p.workspace,
		Debug:          p.debug,
		Quiet:          !p.debug,
		EmbeddedAgents: p.embedded,
	})
}

func (p *teamPicoclawProcessor) runner(key string, call teamruntime.AgentCall) (*pcwrap.DirectRunner, *sync.Mutex, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if runner := p.runners[key]; runner != nil {
		return runner, p.runnerLocks[key], nil
	}
	runner, err := p.runtime.NewDirectRunner(pcwrap.RunOptions{
		Session:        call.Session,
		Agent:          firstTeamValue(call.Agent, p.defaultAgent),
		Model:          firstTeamValue(call.Model, p.defaultModel),
		Workspace:      p.workspace,
		Debug:          p.debug,
		Quiet:          !p.debug,
		EmbeddedAgents: p.embedded,
	})
	if err != nil {
		return nil, nil, err
	}
	lock := &sync.Mutex{}
	p.runners[key] = runner
	p.runnerLocks[key] = lock
	return runner, lock, nil
}

func (p *teamPicoclawProcessor) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, runner := range p.runners {
		runner.Close()
	}
	p.runners = map[string]*pcwrap.DirectRunner{}
	p.runnerLocks = map[string]*sync.Mutex{}
}

func firstTeamValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func teamNameSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastDash = false
		case !lastDash:
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func starterTeamDefinition(name string) string {
	return fmt.Sprintf(`team = %q
title = %q
description = "A persistent cross-functional team."
version = 1

[coordination]
facilitator = "facilitator"
finalizer = "facilitator"
review_waves = 1
max_concurrency = 3

[limits]
max_agent_turns = 8
max_wall_time = "15m"

[memory]
enabled = true
maintainer = "facilitator"
max_chars = 20000

[[agents]]
id = "facilitator"
role = "Facilitator and product lead"
agent = "assistant"
can_finalize = true
prompt = """
Clarify the goal, expose unresolved disagreements, and synthesize a decisive answer.
You are a normal team member, not a message broker or manager with special authority.
"""

[[agents]]
id = "architect"
role = "Software architect"
agent = "planner"
prompt = """
Focus on system boundaries, failure modes, evolution paths, and technical tradeoffs.
Challenge vague assumptions and propose concrete architecture.
"""

[[agents]]
id = "engineer"
role = "Implementation engineer"
agent = "coder"
prompt = """
Focus on implementation cost, compatibility, tests, operations, and incremental delivery.
Turn broad ideas into changes that can actually ship.
"""
`, name, strings.ReplaceAll(name, "-", " "))
}
