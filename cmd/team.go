package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/agents"
	formularuntime "github.com/sjzsdu/tt/internal/formula/runtime"
	formulasteps "github.com/sjzsdu/tt/internal/formula/steps"
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
	teamWeb            bool
	teamWebPort        int
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
tt team run .tt/teams/product-review.toml "给出 MVP 方案"`,
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

var teamOpenCmd = &cobra.Command{
	Use:   "open [thread]",
	Short: "Open a persisted team thread in the web dashboard",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runTeamOpen,
}

var (
	teamListBuiltin bool
	teamListUser    bool
)

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

var teamMemoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Inspect, retry, and roll back durable team memory",
}

var teamMemoryShowCmd = &cobra.Command{
	Use:   "show [thread]",
	Short: "Show memory provenance, versions, and proposal diffs",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runTeamMemoryShow,
}

var teamMemoryRollbackCmd = &cobra.Command{
	Use:   "rollback <thread> <version>",
	Short: "Restore a prior memory version as a new auditable version",
	Args:  cobra.ExactArgs(2),
	RunE:  runTeamMemoryRollback,
}

var teamMemoryRetryCmd = &cobra.Command{
	Use:   "retry [thread]",
	Short: "Retry memory maintenance without rerunning team collaboration",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runTeamMemoryRetry,
}

func init() {
	rootCmd.AddCommand(teamCmd)
	teamCmd.AddCommand(teamRunCmd)
	teamCmd.AddCommand(teamAskCmd)
	teamCmd.AddCommand(teamResumeCmd)
	teamCmd.AddCommand(teamShowCmd)
	teamCmd.AddCommand(teamOpenCmd)
	teamCmd.AddCommand(teamListCmd)
	teamCmd.AddCommand(teamInitCmd)
	teamCmd.AddCommand(teamMemoryCmd)
	teamMemoryCmd.AddCommand(teamMemoryShowCmd, teamMemoryRollbackCmd, teamMemoryRetryCmd)

	teamCmd.PersistentFlags().StringVarP(&teamSession, "session", "s", "cli:team", "session key prefix for team agents")
	teamCmd.PersistentFlags().StringVar(&teamModel, "model", "", "fallback model for team agents without agents.model")
	teamCmd.PersistentFlags().BoolVarP(&teamDebug, "debug", "d", false, "enable agent runtime debug logging")
	teamCmd.PersistentFlags().StringVar(&teamHome, "picoclaw-home", "", "override PICOCLAW_HOME for this run")
	teamCmd.PersistentFlags().StringVar(&teamConfig, "picoclaw-config", "", "override PICOCLAW_CONFIG for this run")
	teamCmd.PersistentFlags().BoolVar(&teamNoMemory, "no-memory", false, "do not read or update team memory for this invocation")
	teamCmd.PersistentFlags().BoolVar(&teamShowDiscussion, "show-discussion", true, "stream public team discussion before the final answer")
	teamCmd.PersistentFlags().BoolVar(&teamWeb, "web", false, "open and keep a live team web dashboard")
	teamCmd.PersistentFlags().IntVar(&teamWebPort, "web-port", 9715, "preferred team dashboard web server port")

	teamListCmd.Flags().BoolVar(&teamListBuiltin, "builtin", false, "show only builtin teams")
	teamListCmd.Flags().BoolVar(&teamListUser, "user", false, "show only user teams from search paths")
}

func resolveTeamStore(_ *cobra.Command, args []string) (ttconfig.Loaded, string, *teamruntime.Store, error) {
	loaded, err := loadTTConfig()
	if err != nil {
		return ttconfig.Loaded{}, "", nil, err
	}
	id := "latest"
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		id = args[0]
	}
	projectRoot := projectRootFromConfig(loaded)
	store, err := teamruntime.ResolveStore(projectRoot, id)
	return loaded, projectRoot, store, err
}

func runTeamMemoryShow(cmd *cobra.Command, args []string) error {
	_, _, store, err := resolveTeamStore(cmd, args)
	if err != nil {
		return err
	}
	review, err := teamruntime.LoadMemoryReview(store.Thread.MemoryPath, store.Thread.Team)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Memory: %s\nCurrent: v%d", store.Thread.MemoryPath, review.Current.Version)
	if review.Current.SourceRound > 0 {
		fmt.Fprintf(out, " from %s round %d events %v", review.Current.SourceThread, review.Current.SourceRound, review.Current.SourceEvents)
	}
	fmt.Fprintln(out)
	for _, proposal := range review.Proposals {
		fmt.Fprintf(out, "\nProposal %s | %s | v%d -> v%d | round %d events %v\n%s",
			proposal.ID, proposal.Status, proposal.BaseVersion, proposal.ProposedVersion,
			proposal.SourceRound, proposal.SourceEvents, proposal.Diff)
		if proposal.Error != "" {
			fmt.Fprintf(out, "Rejected: %s\n", proposal.Error)
		}
	}
	fmt.Fprintln(out, "\nVersions:")
	for _, version := range review.Versions {
		fmt.Fprintf(out, "  v%-4d %s round %-3d", version.Version, version.UpdatedAt, version.SourceRound)
		if version.RestoredFrom > 0 {
			fmt.Fprintf(out, " restored from v%d", version.RestoredFrom)
		}
		fmt.Fprintln(out)
	}
	return nil
}

func runTeamMemoryRollback(cmd *cobra.Command, args []string) error {
	version, err := strconv.Atoi(args[1])
	if err != nil || version < 1 {
		return fmt.Errorf("invalid memory version %q", args[1])
	}
	_, _, store, err := resolveTeamStore(cmd, args[:1])
	if err != nil {
		return err
	}
	definition, err := store.LoadDefinition()
	if err != nil {
		return err
	}
	engine := &teamruntime.Engine{Definition: definition, Store: store}
	updated, err := engine.RollbackMemory(version)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Restored memory v%d as v%d: %s\n", version, updated.Version, updated.Path)
	return nil
}

func runTeamMemoryRetry(cmd *cobra.Command, args []string) error {
	loaded, projectRoot, store, err := resolveTeamStore(cmd, args)
	if err != nil {
		return err
	}
	definition, err := store.LoadDefinition()
	if err != nil {
		return err
	}
	processor, model, _, cleanup, err := prepareTeamExecution(cmd, loaded, projectRoot, definition)
	if err != nil {
		return err
	}
	defer cleanup()
	engine := &teamruntime.Engine{
		Definition: definition, Store: store, Processor: processor,
		SessionPrefix: teamSession, Model: model,
	}
	updated, err := engine.RetryMemory(cmd.Context())
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Team memory retry succeeded: v%d %s\n", updated.Version, updated.Path)
	return nil
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
	processor, model, debug, cleanup, err := prepareTeamExecution(cmd, loaded, projectRoot, definition)
	if err != nil {
		return err
	}
	defer cleanup()
	if store == nil {
		store, err = teamruntime.NewStore(projectRoot, definition)
		if err != nil {
			return err
		}
	}
	runCtx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	engine := &teamruntime.Engine{
		Definition:    definition,
		Store:         store,
		Processor:     processor,
		SessionPrefix: teamSession,
		Model:         model,
		DisableMemory: teamNoMemory,
	}
	controller := newTeamRunController(engine, runCtx)
	var dashboard *teamDashboardServer
	if teamWeb {
		dashboard = newTeamDashboardServer(store, definition)
		dashboard.setActions(controller)
		controller.SetOnChange(dashboard.notifyState)
		if err := dashboard.start(teamWebPort, true); err != nil {
			return err
		}
		defer dashboard.close()
		fmt.Fprintf(cmd.ErrOrStderr(), "Team dashboard: %s\n", dashboard.url())
	}
	engine.OnEvent = func(event teamruntime.Event) {
		if dashboard != nil {
			dashboard.notifyState()
		}
		if !teamShowDiscussion {
			return
		}
		switch event.Type {
		case "agent_message":
			signal := ""
			if event.Signal != "" {
				signal = " [" + event.Signal + "]"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n@%s%s:\n%s\n", event.From, signal, strings.TrimSpace(event.Content))
		case "agent_yield":
			fmt.Fprintf(cmd.OutOrStdout(), "\n@%s: [YIELD]\n", event.From)
		case "convergence_reached":
			fmt.Fprintln(cmd.OutOrStdout(), "\n[team converged]")
		case "forced_stop":
			fmt.Fprintf(cmd.OutOrStdout(), "\n[team forced stop: %s]\n", event.Content)
		}
	}
	loading := startLLMLoading("正在等待 team 协作", debug)
	defer loading.Stop()

	result, runErr := controller.Run(runCtx, question, resume)
	loading.Stop()
	if runErr == nil {
		fmt.Fprintf(cmd.OutOrStdout(), "\nFinal answer:\n%s\n", strings.TrimSpace(result.Answer))
		fmt.Fprintf(cmd.ErrOrStderr(), "Team thread: %s\n", result.ThreadID)
		if result.MemoryWarning != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: team memory was not updated: %v\n", result.MemoryWarning)
		} else if definition.MemoryEnabled() && !teamNoMemory {
			fmt.Fprintf(cmd.ErrOrStderr(), "Team memory: %s (version %d)\n", result.Memory.Path, result.Memory.Version)
		}
	} else if dashboard == nil {
		return runErr
	} else {
		fmt.Fprintf(cmd.ErrOrStderr(), "Team run stopped: %v\n", runErr)
	}
	if dashboard != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Use the dashboard to ask follow-ups, stop, or resume. Press Ctrl-C to close it.")
		dashboard.wait(runCtx)
		controller.Wait()
	}
	return nil
}

func prepareTeamExecution(cmd *cobra.Command, loaded ttconfig.Loaded, projectRoot string, definition *teamruntime.Definition) (*teamPicoclawProcessor, string, bool, func(), error) {
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

	var rt *pcwrap.Runtime
	var embedded []pcwrap.EmbeddedAgent
	cleanup := func() {}
	needsEmbeddedRuntime := false
	for _, member := range definition.Agents {
		if member.External == nil {
			needsEmbeddedRuntime = true
			break
		}
	}
	if needsEmbeddedRuntime {
		_, resolvedHome, resolvedConfig, restoreStorage, err := useTTAgentStorage(merged.Picoclaw.Home, merged.Picoclaw.Config)
		if err != nil {
			return nil, "", false, nil, err
		}
		cleanup = restoreStorage
		merged.Picoclaw.Home = resolvedHome
		merged.Picoclaw.Config = resolvedConfig
		if err := ensurePicoclawConfigAvailable(merged.Picoclaw.Home, merged.Picoclaw.Config); err != nil {
			cleanup()
			return nil, "", false, nil, err
		}
		rt, err = pcwrap.Load(pcwrap.Options{
			Home:      merged.Picoclaw.Home,
			Config:    merged.Picoclaw.Config,
			TTConfig:  merged,
			TTSources: loaded.Sources,
		})
		if err != nil {
			cleanup()
			return nil, "", false, nil, picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
		}
		embedded, err = agents.All()
		if err != nil {
			cleanup()
			return nil, "", false, nil, fmt.Errorf("load embedded agents for team: %w", err)
		}
	}
	model := resolveTeamDefaultModel(
		cmd.Flags().Changed("model") || teamCmd.PersistentFlags().Changed("model"),
		teamModel,
		definition.DefaultModel,
		merged.Agent.Model,
	)
	debug := teamDebug
	if merged.Agent.Debug != nil {
		debug = *merged.Agent.Debug
	}
	processor := newTeamPicoclawProcessor(rt, embedded, projectRoot, merged.Agent.Agent, model, debug)
	if err := processor.ValidateDefinition(definition); err != nil {
		processor.Close()
		cleanup()
		return nil, "", false, nil, err
	}
	return processor, model, debug, func() {
		processor.Close()
		cleanup()
	}, nil
}

func resolveTeamDefaultModel(cliChanged bool, cliModel, teamDefault, globalDefault string) string {
	if cliChanged && strings.TrimSpace(cliModel) != "" {
		return strings.TrimSpace(cliModel)
	}
	return firstTeamValue(teamDefault, globalDefault)
}

func runTeamOpen(cmd *cobra.Command, args []string) error {
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
	definition, err := store.LoadDefinition()
	if err != nil {
		return err
	}
	runCtx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	dashboard := newTeamDashboardServer(store, definition)
	processor, model, _, cleanup, runtimeErr := prepareTeamExecution(cmd, loaded, projectRootFromConfig(loaded), definition)
	var controller *teamRunController
	if runtimeErr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: interactive controls unavailable: %v\n", runtimeErr)
	} else {
		defer cleanup()
		engine := &teamruntime.Engine{
			Definition:    definition,
			Store:         store,
			Processor:     processor,
			SessionPrefix: teamSession,
			Model:         model,
			DisableMemory: teamNoMemory,
		}
		controller = newTeamRunController(engine, runCtx)
		controller.SetOnChange(dashboard.notifyState)
		dashboard.setActions(controller)
		engine.OnEvent = func(teamruntime.Event) {
			dashboard.notifyState()
		}
	}
	if err := dashboard.start(teamWebPort, true); err != nil {
		return err
	}
	defer dashboard.close()
	fmt.Fprintf(cmd.OutOrStdout(), "Opened team thread: %s\n", store.Thread.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "Team dashboard: %s\n", dashboard.url())
	fmt.Fprintln(cmd.OutOrStdout(), "Press Ctrl-C to stop the dashboard.")
	dashboard.wait(runCtx)
	if controller != nil {
		controller.Wait()
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
			if event.Signal != "" {
				fmt.Fprintf(out, " | %s", event.Signal)
			}
			if event.Metrics != nil {
				fmt.Fprintf(out, " | turn %d | %s | %dms | %d→%d chars",
					event.Metrics.Turn, event.Metrics.Model, event.Metrics.DurationMS,
					event.Metrics.InputChars, event.Metrics.OutputChars)
			}
			fmt.Fprintf(out, "\n%s\n", event.Content)
		case "agent_yield":
			fmt.Fprintf(out, "\nRound %d | @%s | %s %d\n[YIELD]\n", event.Round, event.From, event.Phase, event.Wave)
		case "final_answer":
			fmt.Fprintf(out, "\nRound %d | final answer by @%s\n%s\n", event.Round, event.From, event.Content)
		case "memory_updated":
			fmt.Fprintf(out, "\nRound %d | %s\n", event.Round, event.Content)
		case "convergence_reached", "forced_stop":
			fmt.Fprintf(out, "\nRound %d | %s | %s\n", event.Round, event.Type, event.Content)
		case "blackboard_upsert", "blackboard_resolve":
			if event.Blackboard != nil {
				fmt.Fprintf(
					out,
					"\nRound %d | blackboard %s/%s | %s by @%s | source event %d\n",
					event.Round,
					event.Blackboard.Kind,
					event.Blackboard.Key,
					event.Blackboard.Action,
					event.From,
					event.Ref,
				)
				if event.Blackboard.Content != "" {
					fmt.Fprintln(out, event.Blackboard.Content)
				}
			}
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
	out := cmd.OutOrStdout()
	showBuiltin := !teamListUser
	showUser := !teamListBuiltin

	if showBuiltin {
		builtinEntries, err := teamruntime.BuiltinTeams()
		if err != nil {
			return err
		}
		if len(builtinEntries) > 0 {
			fmt.Fprintln(out, "BUILTIN")
			for _, entry := range builtinEntries {
				title := entry.Title
				if title == "" {
					title = entry.Name
				}
				fmt.Fprintf(out, "  %-24s %2d agents  %s\n", entry.Name, entry.Agents, title)
			}
			if showUser {
				fmt.Fprintln(out)
			}
		}
	}

	if showUser {
		definitions, err := teamruntime.List(teamruntime.DefaultSearchPaths(projectRoot)...)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, "USER")
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
	}

	threads, err := teamruntime.ListThreads(projectRoot)
	if err != nil {
		return err
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
	projectRoot := projectRootFromConfig(loaded)
	dir := filepath.Join(projectRoot, ".tt", "teams")
	path := filepath.Join(dir, name+".toml")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("team definition already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create team definition directory: %w", err)
	}
	if data, ok, err := teamruntime.BuiltinTeamContent(name); err == nil && ok {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("write team definition: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Copied builtin team %q to %s\n", name, path)
	} else {
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
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Run it with: tt team run %s \"your question\"\n", name)
	return nil
}

type teamPicoclawProcessor struct {
	runtime          *pcwrap.Runtime
	embedded         []pcwrap.EmbeddedAgent
	workspace        string
	defaultAgent     string
	defaultModel     string
	debug            bool
	mu               sync.Mutex
	runners          map[string]*pcwrap.DirectRunner
	runnerLocks      map[string]*sync.Mutex
	externalRunner   formulasteps.ExternalAgentRunner
	externalSessions map[string]string
	externalLocks    map[string]*sync.Mutex
}

func newTeamPicoclawProcessor(rt *pcwrap.Runtime, embedded []pcwrap.EmbeddedAgent, workspace, defaultAgent, defaultModel string, debug bool) *teamPicoclawProcessor {
	return &teamPicoclawProcessor{
		runtime:          rt,
		embedded:         embedded,
		workspace:        workspace,
		defaultAgent:     defaultAgent,
		defaultModel:     defaultModel,
		debug:            debug,
		runners:          map[string]*pcwrap.DirectRunner{},
		runnerLocks:      map[string]*sync.Mutex{},
		externalRunner:   formularuntime.ExternalAgentCapability{},
		externalSessions: map[string]string{},
		externalLocks:    map[string]*sync.Mutex{},
	}
}

func (p *teamPicoclawProcessor) ValidateDefinition(definition *teamruntime.Definition) error {
	var failures []string
	for _, member := range definition.Agents {
		if member.External != nil {
			binary := member.External.Driver
			if binary == "bl" {
				binary = "jcode"
			}
			if _, err := exec.LookPath(binary); err != nil {
				failures = append(failures, fmt.Sprintf("@%s (%s): external agent executable not found on PATH", member.ID, binary))
			}
			continue
		}
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
	if call.External != nil {
		return p.processExternal(ctx, call)
	}
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

type teamExternalAgentOutput struct {
	SessionID string `json:"session_id"`
	Text      string `json:"text"`
	Stderr    string `json:"stderr"`
}

func (p *teamPicoclawProcessor) processExternal(ctx context.Context, call teamruntime.AgentCall) (string, error) {
	external := call.External
	key := strings.ToLower(firstTeamValue(call.Session, call.MemberID))
	p.mu.Lock()
	lock := p.externalLocks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		p.externalLocks[key] = lock
	}
	p.mu.Unlock()

	lock.Lock()
	defer lock.Unlock()

	p.mu.Lock()
	resume := p.externalSessions[key]
	p.mu.Unlock()
	if resume == "" && call.ExternalSessions != nil {
		resume = call.ExternalSessions.ExternalSession(key)
	}
	resume = firstTeamValue(resume, external.Resume)
	timeout := time.Duration(0)
	if external.Timeout != "" {
		timeout, _ = time.ParseDuration(external.Timeout)
	}
	workspace := firstTeamValue(external.Cwd, call.Workspace, p.workspace)
	if workspace != "" && !filepath.IsAbs(workspace) {
		workspace = filepath.Join(firstTeamValue(call.Workspace, p.workspace), workspace)
	}
	value, runErr := p.externalRunner.RunExternalAgent(ctx, formulasteps.ExternalAgentRequest{
		NodeID:    call.MemberID,
		Driver:    external.Driver,
		Provider:  external.Provider,
		Model:     firstTeamValue(call.Model, p.defaultModel),
		Mode:      external.Mode,
		Resume:    resume,
		Workspace: workspace,
		Prompt:    call.Prompt,
		ExtraArgs: append([]string(nil), external.ExtraArgs...),
		Timeout:   timeout,
	})
	var output teamExternalAgentOutput
	if err := json.Unmarshal(value.Raw, &output); err != nil {
		if runErr != nil {
			return "", runErr
		}
		return "", fmt.Errorf("decode external agent %s output: %w", external.Driver, err)
	}
	if output.SessionID != "" {
		p.mu.Lock()
		p.externalSessions[key] = output.SessionID
		p.mu.Unlock()
		if call.ExternalSessions != nil {
			if err := call.ExternalSessions.SetExternalSession(key, output.SessionID); err != nil {
				return output.Text, fmt.Errorf("persist external agent %s session: %w", external.Driver, err)
			}
		}
	}
	if runErr != nil {
		if stderr := strings.TrimSpace(output.Stderr); stderr != "" {
			return output.Text, fmt.Errorf("%w: %s", runErr, stderr)
		}
		return output.Text, runErr
	}
	return output.Text, nil
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
# Optional Team fallback. Per-agent model values take precedence.
default_model = ""

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
model = ""
can_finalize = true
prompt = """
Clarify the goal, expose unresolved disagreements, and synthesize a decisive answer.
You are a normal team member, not a message broker or manager with special authority.
"""

[[agents]]
id = "architect"
role = "Software architect"
agent = "planner"
model = ""
prompt = """
Focus on system boundaries, failure modes, evolution paths, and technical tradeoffs.
Challenge vague assumptions and propose concrete architecture.
"""

[[agents]]
id = "engineer"
role = "Implementation engineer"
agent = "coder"
model = ""
prompt = """
Focus on implementation cost, compatibility, tests, operations, and incremental delivery.
Turn broad ideas into changes that can actually ship.
"""
`, name, strings.ReplaceAll(name, "-", " "))
}
