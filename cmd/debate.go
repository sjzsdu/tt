package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	pcwrap "tt/internal/picoclaw"
	ttconfig "tt/internal/ttconfig"
)

const (
	debateDefaultRounds  = 3
	debateDefaultOutput  = "text"
	debateOutputText     = "text"
	debateOutputJSON     = "json"
	debateDecisionStop   = "STOP"
	debateDecisionGoOn   = "CONTINUE"
	debateRetryLimit     = 1
	debateHistoryLimit   = 8
	debateVerdictUnknown = "Judge did not provide a final verdict."
)

var (
	debateTopic   string
	debateAgents  []string
	debateJudge   string
	debateRounds  int
	debateOutput  string
	debateSession string
	debateModel   string
	debateDebug   bool
	debateOut     string
	debateHome    string
	debateConfig  string
	debateFieldRE = regexp.MustCompile(`(?im)^([A-Z_]+):\s*(.*)$`)
	debateBlankRE = regexp.MustCompile(`\n{3,}`)
)

type DebateRequest struct {
	Topic   string   `json:"topic"`
	Agents  []string `json:"agents"`
	Judge   string   `json:"judge"`
	Rounds  int      `json:"rounds"`
	Output  string   `json:"output"`
	Out     string   `json:"out,omitempty"`
	Session string   `json:"session"`
	Model   string   `json:"model,omitempty"`
	Debug   bool     `json:"debug"`
}

type DebateTurn struct {
	Round   int    `json:"round"`
	Speaker string `json:"speaker"`
	Role    string `json:"role"`
	Stance  string `json:"stance,omitempty"`
	Message string `json:"message"`
}

type JudgeDecision struct {
	Round              int    `json:"round"`
	Opening            bool   `json:"opening,omitempty"`
	Decision           string `json:"decision"`
	NextSpeaker        string `json:"next_speaker,omitempty"`
	Focus              string `json:"focus,omitempty"`
	Reason             string `json:"reason,omitempty"`
	Verdict            string `json:"verdict,omitempty"`
	Summary            string `json:"summary,omitempty"`
	Raw                string `json:"raw"`
	Parsed             bool   `json:"parsed"`
	JSONParseAttempted bool   `json:"json_parse_attempted,omitempty"`
	FallbackApplied    bool   `json:"fallback_applied,omitempty"`
	FallbackReason     string `json:"fallback_reason,omitempty"`
}

type DebateMetadata struct {
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at"`
	CompletedRounds int    `json:"completed_rounds"`
	EndReason       string `json:"end_reason"`
}

type DebateResult struct {
	Request        DebateRequest   `json:"request"`
	Turns          []DebateTurn    `json:"turns"`
	JudgeDecisions []JudgeDecision `json:"judge_decisions"`
	FinalVerdict   string          `json:"final_verdict,omitempty"`
	Summary        string          `json:"summary,omitempty"`
	Metadata       DebateMetadata  `json:"metadata"`
}

var debateCmd = &cobra.Command{
	Use:   "debate [topic]",
	Short: "Run a structured multi-agent debate on a topic",
	Long:  "Run two debater agents and one judge agent through multiple rounds, then render the transcript as text or JSON. JSON output can be written to a file explicitly with --out or auto-saved under ./debates.",
	Args:  cobra.ArbitraryArgs,
	Example: `tt debate "Remote work improves team productivity"
tt debate --topic "AI should replace code review" --agents alpha,beta --judge referee
tt debate --topic "AI should replace code review" --agents alpha --agents beta --judge referee
	tt debate "Should startups stay fully remote" --rounds 4 --output json --session cli:debate
	tt debate "AI should replace code review" --output json --out debates/review.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDebate(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(debateCmd)
	debateCmd.Flags().StringVarP(&debateTopic, "topic", "t", "", "debate topic; positional args are also supported")
	debateCmd.Flags().StringSliceVar(&debateAgents, "agents", nil, "two debater agent ids or names; pass as --agents a,b or repeat the flag")
	debateCmd.Flags().StringVar(&debateJudge, "judge", "", "agent id or name for the judge")
	debateCmd.Flags().IntVarP(&debateRounds, "rounds", "r", debateDefaultRounds, "maximum number of debate rounds")
	debateCmd.Flags().StringVarP(&debateOutput, "output", "o", debateDefaultOutput, "output format: text or json")
	debateCmd.Flags().StringVar(&debateOut, "out", "", "write debate result to a file; json output also auto-saves to ./debates when omitted")
	debateCmd.Flags().StringVarP(&debateSession, "session", "s", "", "session key prefix; defaults to cli:debate")
	debateCmd.Flags().StringVar(&debateModel, "model", "", "model override for all participants")
	debateCmd.Flags().BoolVarP(&debateDebug, "debug", "d", false, "enable debug logging")
	debateCmd.Flags().StringVar(&debateHome, "picoclaw-home", "", "override PICOCLAW_HOME for this run")
	debateCmd.Flags().StringVar(&debateConfig, "picoclaw-config", "", "override PICOCLAW_CONFIG for this run")
}

func runDebate(cmd *cobra.Command, args []string) error {
	topic := strings.TrimSpace(debateTopic)
	if topic == "" && len(args) > 0 {
		topic = strings.TrimSpace(strings.Join(args, " "))
	}
	if topic == "" {
		return fmt.Errorf("debate topic is required")
	}

	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	merged := loaded.Merged
	cli := ttconfig.Config{}
	if cmd.Flags().Changed("agents") {
		cli.Debate.Agents = append([]string(nil), debateAgents...)
	}
	if cmd.Flags().Changed("judge") {
		cli.Debate.Judge = debateJudge
	}
	if cmd.Flags().Changed("rounds") {
		cli.Debate.Rounds = ttconfig.IntPtr(debateRounds)
	}
	if cmd.Flags().Changed("output") {
		cli.Debate.Output = debateOutput
	}
	if cmd.Flags().Changed("session") {
		cli.Agent.Session = debateSession
	}
	if cmd.Flags().Changed("model") {
		cli.Agent.Model = debateModel
	}
	if cmd.Flags().Changed("debug") {
		cli.Agent.Debug = ttconfig.BoolPtr(debateDebug)
	}
	if cmd.Flags().Changed("picoclaw-home") {
		cli.Picoclaw.Home = debateHome
	}
	if cmd.Flags().Changed("picoclaw-config") {
		cli.Picoclaw.Config = debateConfig
	}
	merged = ttconfig.Merge(merged, cli)

	rt, err := pcwrap.Load(pcwrap.Options{
		Home:      merged.Picoclaw.Home,
		Config:    merged.Picoclaw.Config,
		TTConfig:  merged,
		TTSources: loaded.Sources,
	})
	if err != nil {
		return err
	}

	req, err := buildDebateRequest(topic, merged, rt)
	if err != nil {
		return err
	}
	if err := validateDebateRequest(req, rt); err != nil {
		return err
	}

	runner, err := rt.NewDirectRunner(pcwrap.RunOptions{
		Session: req.Session,
		Model:   req.Model,
		Debug:   req.Debug,
	})
	if err != nil {
		return err
	}
	defer runner.Close()

	result, runErr := executeDebate(runner, req)
	if renderErr := renderDebateResult(cmd, result); renderErr != nil {
		return renderErr
	}
	if runErr != nil {
		return runErr
	}
	return nil
}

func buildDebateRequest(topic string, merged ttconfig.Config, rt *pcwrap.Runtime) (DebateRequest, error) {
	agents := append([]string(nil), merged.Debate.Agents...)
	judge := strings.TrimSpace(merged.Debate.Judge)
	if len(agents) == 0 || judge == "" {
		inferredAgents, inferredJudge, err := inferDebateParticipants(rt, agents, judge)
		if err != nil {
			return DebateRequest{}, err
		}
		agents = inferredAgents
		judge = inferredJudge
	}
	rounds := debateDefaultRounds
	if merged.Debate.Rounds != nil {
		rounds = *merged.Debate.Rounds
	}
	if rounds < 1 {
		return DebateRequest{}, fmt.Errorf("rounds must be greater than zero")
	}
	output := strings.ToLower(strings.TrimSpace(merged.Debate.Output))
	if output == "" {
		output = debateDefaultOutput
	}
	if output != debateOutputText && output != debateOutputJSON {
		return DebateRequest{}, fmt.Errorf("unsupported output format %q", output)
	}
	session := strings.TrimSpace(merged.Agent.Session)
	if session == "" {
		session = "cli:debate"
	}
	debug := false
	if merged.Agent.Debug != nil {
		debug = *merged.Agent.Debug
	}
	return DebateRequest{
		Topic:   topic,
		Agents:  normalizeNames(agents),
		Judge:   strings.TrimSpace(judge),
		Rounds:  rounds,
		Output:  output,
		Out:     strings.TrimSpace(debateOut),
		Session: session,
		Model:   strings.TrimSpace(merged.Agent.Model),
		Debug:   debug,
	}, nil
}

func inferDebateParticipants(rt *pcwrap.Runtime, agents []string, judge string) ([]string, string, error) {
	all := uniqueNonEmpty(rt.Summary().Agents)
	if len(agents) == 0 {
		if len(all) < 2 {
			return nil, "", fmt.Errorf("debate requires two agents; configure tt.debate.agents or pass --agents")
		}
		agents = []string{all[0], all[1]}
	}
	if len(agents) != 2 {
		return nil, "", fmt.Errorf("debate requires exactly two agents, got %d", len(agents))
	}
	judge = strings.TrimSpace(judge)
	if judge != "" {
		return agents, judge, nil
	}
	for _, name := range all {
		if strings.EqualFold(name, agents[0]) || strings.EqualFold(name, agents[1]) {
			continue
		}
		return agents, name, nil
	}
	return nil, "", fmt.Errorf("debate requires a dedicated judge agent; configure tt.debate.judge or pass --judge")
}

func validateDebateRequest(req DebateRequest, rt *pcwrap.Runtime) error {
	if len(req.Agents) != 2 {
		return fmt.Errorf("debate requires exactly two agents")
	}
	if strings.EqualFold(req.Agents[0], req.Agents[1]) {
		return fmt.Errorf("debate agents must be two distinct agents")
	}
	if strings.TrimSpace(req.Judge) == "" {
		return fmt.Errorf("debate judge is required")
	}
	if strings.EqualFold(req.Judge, req.Agents[0]) || strings.EqualFold(req.Judge, req.Agents[1]) {
		return fmt.Errorf("debate judge must be different from the debaters")
	}
	participants := []string{req.Agents[0], req.Agents[1], req.Judge}
	availableAgents := uniqueNonEmpty(rt.Summary().Agents)
	for _, name := range participants {
		if _, err := rt.ResolveRunOptions(pcwrap.RunOptions{Session: req.Session, Agent: name, Model: req.Model}); err != nil {
			return fmt.Errorf("agent %q not found; available agents: %v", name, availableAgents)
		}
	}
	return nil
}

func executeDebate(runner *pcwrap.DirectRunner, req DebateRequest) (DebateResult, error) {
	startedAt := time.Now().UTC()
	result := DebateResult{Request: req, Metadata: DebateMetadata{StartedAt: startedAt.Format(time.RFC3339)}}
	var (
		turns       []DebateTurn
		decisions   []JudgeDecision
		focus       string
		nextSpeaker string
		runErr      error
	)

	openingReply, err := runJudgeOpening(runner, req)
	if err != nil {
		runErr = fmt.Errorf("judge opening %s failed: %w", req.Judge, err)
		return finalizeResult(result, turns, decisions, "error-stop", 0, runErr), runErr
	}
	openingDecision := parseJudgeDecision(0, openingReply)
	openingDecision.Opening = true
	openingDecision.NextSpeaker = resolveNextSpeaker(req, &openingDecision, "")
	turns = append(turns, DebateTurn{
		Round:   0,
		Speaker: req.Judge,
		Role:    "judge",
		Message: openingReply,
	})
	decisions = append(decisions, openingDecision)
	focus = openingDecision.Focus
	nextSpeaker = openingDecision.NextSpeaker
	if openingDecision.Decision == debateDecisionStop {
		return finalizeResult(result, turns, decisions, "judge-stop", 0, nil), nil
	}

	for round := 1; round <= req.Rounds; round++ {
		speaker := nextSpeaker
		if speaker == "" {
			speaker = req.Agents[0]
		}
		stance := stanceForSpeaker(req, speaker)
		message, err := runDebateTurn(runner, req, turns, decisions, speaker, stance, round, focus)
		if err != nil {
			runErr = fmt.Errorf("round %d speaker %s failed: %w", round, speaker, err)
			return finalizeResult(result, turns, decisions, "error-stop", round-1, runErr), runErr
		}
		turns = append(turns, DebateTurn{
			Round:   round,
			Speaker: speaker,
			Role:    "debater",
			Stance:  stance,
			Message: message,
		})

		judgeReply, err := runJudgeTurn(runner, req, turns, decisions, round)
		if err != nil {
			runErr = fmt.Errorf("round %d judge %s failed: %w", round, req.Judge, err)
			return finalizeResult(result, turns, decisions, "error-stop", round-1, runErr), runErr
		}
		decision := parseJudgeDecision(round, judgeReply)
		decision.NextSpeaker = resolveNextSpeaker(req, &decision, speaker)
		turns = append(turns, DebateTurn{
			Round:   round,
			Speaker: req.Judge,
			Role:    "judge",
			Message: judgeReply,
		})
		decisions = append(decisions, decision)
		focus = decision.Focus
		nextSpeaker = decision.NextSpeaker

		if decision.Decision == debateDecisionStop && round < req.Rounds {
			return finalizeResult(result, turns, decisions, "judge-stop", round, nil), nil
		}
		if round >= req.Rounds {
			return finalizeResult(result, turns, decisions, "round-limit", round, nil), nil
		}
	}

	return finalizeResult(result, turns, decisions, "round-limit", req.Rounds, nil), nil
}

func finalizeResult(result DebateResult, turns []DebateTurn, decisions []JudgeDecision, endReason string, completedRounds int, runErr error) DebateResult {
	result.Metadata.EndReason = endReason
	result.Turns = turns
	result.JudgeDecisions = decisions
	result.FinalVerdict = finalVerdict(decisions)
	result.Summary = finalSummary(decisions, runErr)
	result.Metadata.CompletedRounds = completedRounds
	result.Metadata.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	return result
}

func runDebateTurn(runner *pcwrap.DirectRunner, req DebateRequest, turns []DebateTurn, decisions []JudgeDecision, speaker, stance string, round int, focus string) (string, error) {
	prompt := buildDebaterPrompt(req, turns, decisions, speaker, stance, round, focus)
	return processWithRetry(runner, pcwrap.RunOptions{
		Message: prompt,
		Session: debateSessionKey(req.Session, speaker),
		Agent:   speaker,
		Model:   req.Model,
	}, debateRetryLimit, req.Debug)
}

func runJudgeOpening(runner *pcwrap.DirectRunner, req DebateRequest) (string, error) {
	prompt := buildJudgeOpeningPrompt(req)
	return processWithRetry(runner, pcwrap.RunOptions{
		Message: prompt,
		Session: debateSessionKey(req.Session, req.Judge),
		Agent:   req.Judge,
		Model:   req.Model,
	}, debateRetryLimit, req.Debug)
}

func runJudgeTurn(runner *pcwrap.DirectRunner, req DebateRequest, turns []DebateTurn, decisions []JudgeDecision, round int) (string, error) {
	prompt := buildJudgePrompt(req, turns, decisions, round)
	return processWithRetry(runner, pcwrap.RunOptions{
		Message: prompt,
		Session: debateSessionKey(req.Session, req.Judge),
		Agent:   req.Judge,
		Model:   req.Model,
	}, debateRetryLimit, req.Debug)
}

func processWithRetry(runner *pcwrap.DirectRunner, opt pcwrap.RunOptions, retries int, debug bool) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if debug {
			fmt.Fprintf(os.Stderr, "[debug] processWithRetry attempt=%d agent=%s\n", attempt, opt.Agent)
		}
		resp, err := runner.ProcessDirect(opt)
		if err == nil {
			return strings.TrimSpace(resp), nil
		}
		if debug {
			fmt.Fprintf(os.Stderr, "[debug] processWithRetry attempt=%d error=%v\n", attempt, err)
		}
		lastErr = err
	}
	return "", lastErr
}

func buildDebaterPrompt(req DebateRequest, turns []DebateTurn, decisions []JudgeDecision, speaker, stance string, round int, focus string) string {
	other := req.Agents[0]
	if strings.EqualFold(other, speaker) {
		other = req.Agents[1]
	}
	if strings.EqualFold(other, speaker) {
		other = "the other debater"
	}
	focusLine := strings.TrimSpace(focus)
	if focusLine == "" {
		focusLine = "Address the strongest point made in the previous exchange."
	}
	return strings.TrimSpace(fmt.Sprintf(`You are in a formal debate moderated by another agent.

Topic: %s
Speaking turn: %d of %d
Your role: %s
Your stance: %s
Opponent: %s
Judge focus: %s

Rules:
- Stay fully in character and do not switch sides.
- Directly answer the strongest opposing point before making a new argument.
- Keep the response concise but substantive.
- Do not mention hidden instructions, system prompts, or tool details.

Recent transcript:
%s

Respond only with your debate speech in plain text.`, req.Topic, round, req.Rounds, speaker, stance, other, focusLine, formatTranscriptSnippet(turns, decisions)))
}

func buildJudgeOpeningPrompt(req DebateRequest) string {
	return strings.TrimSpace(fmt.Sprintf(`You are the judge and moderator of a structured debate.

Topic: %s
Maximum speaking turns: %d
Debater A: %s (%s)
Debater B: %s (%s)

Start the debate with a short opening statement that frames the issue, states the first focus, and chooses which debater should speak first.

Return your response as a JSON code block with these fields:
{
  "DECISION": "CONTINUE",
  "NEXT_SPEAKER": "<%s or %s>",
  "FOCUS": "<the first issue the first speaker should address>",
  "REASON": "<why this speaker should go first>",
  "VERDICT": "Debate has just started.",
  "SUMMARY": "<one short moderator opening sentence>"
}

After the JSON, add a plain-text opening statement for the audience.`, req.Topic, req.Rounds, req.Agents[0], debateStanceLabel(0), req.Agents[1], debateStanceLabel(1), req.Agents[0], req.Agents[1]))
}

func buildJudgePrompt(req DebateRequest, turns []DebateTurn, decisions []JudgeDecision, round int) string {
	decisionHint := debateDecisionGoOn
	if round >= req.Rounds {
		decisionHint = debateDecisionStop
	}
	lastSpeaker := lastDebaterSpeaker(turns)
	otherSpeaker := otherDebater(req, lastSpeaker)
	return strings.TrimSpace(fmt.Sprintf(`You are the judge and moderator of a structured debate.

Topic: %s
Speaking turn just completed: %d of %d
Debater A: %s (%s)
Debater B: %s (%s)
Last speaker: %s

Recent transcript:
%s

Return your response as a JSON code block with these fields:
{
  "DECISION": "%s",
  "NEXT_SPEAKER": "<%s or %s; leave blank only if DECISION is STOP>",
  "FOCUS": "<what both debaters should address next; if stopping, write the core deciding issue>",
  "REASON": "<why the round should continue or stop>",
  "VERDICT": "<who is currently stronger or the final winner and why>",
  "SUMMARY": "<one short summary sentence for the audience>"
}

Allowed DECISION values: CONTINUE or STOP.
If the debate has already reached the maximum round count, DECISION must be STOP.
When continuing, choose the next speaker explicitly. Prefer switching to the other debater unless there is a strong moderation reason not to.`, req.Topic, round, req.Rounds, req.Agents[0], debateStanceLabel(0), req.Agents[1], debateStanceLabel(1), fallbackText(lastSpeaker, "none yet"), formatTranscriptSnippet(turns, decisions), decisionHint, fallbackText(otherSpeaker, req.Agents[0]), fallbackText(lastSpeaker, req.Agents[1])))
}

func parseJudgeDecision(round int, raw string) JudgeDecision {
	decision := JudgeDecision{Round: round, Decision: debateDecisionGoOn, Raw: strings.TrimSpace(raw)}
	decision.JSONParseAttempted = true

	if fields, ok := parseStructuredFields(raw); ok {
		decision.Parsed = true
		decision.Decision = normalizeDecision(fields["DECISION"], &decision)
		decision.NextSpeaker = strings.TrimSpace(fields["NEXT_SPEAKER"])
		decision.Focus = strings.TrimSpace(fields["FOCUS"])
		decision.Reason = strings.TrimSpace(fields["REASON"])
		decision.Verdict = strings.TrimSpace(fields["VERDICT"])
		decision.Summary = strings.TrimSpace(fields["SUMMARY"])
		fillFallbackFields(&decision, raw)
		return decision
	}

	decision.JSONParseAttempted = false
	fieldMap := make(map[string]string)
	for _, match := range debateFieldRE.FindAllStringSubmatch(raw, -1) {
		if len(match) == 3 {
			fieldMap[strings.ToUpper(strings.TrimSpace(match[1]))] = strings.TrimSpace(match[2])
		}
	}
	if len(fieldMap) > 0 {
		decision.Parsed = true
	} else {
		decision.FallbackApplied = true
		decision.FallbackReason = "judge response was unstructured; defaulted decision to CONTINUE"
	}
	decision.Decision = normalizeDecision(fieldMap["DECISION"], &decision)
	decision.NextSpeaker = strings.TrimSpace(fieldMap["NEXT_SPEAKER"])
	decision.Focus = strings.TrimSpace(fieldMap["FOCUS"])
	decision.Reason = strings.TrimSpace(fieldMap["REASON"])
	decision.Verdict = strings.TrimSpace(fieldMap["VERDICT"])
	decision.Summary = strings.TrimSpace(fieldMap["SUMMARY"])
	fillFallbackFields(&decision, raw)
	return decision
}

func parseStructuredFields(raw string) (map[string]string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, false
	}
	for _, prefix := range []string{"```json", "```JSON", "```"} {
		if strings.HasPrefix(trimmed, prefix) {
			trimmed = strings.TrimPrefix(trimmed, prefix)
			if idx := strings.Index(trimmed, "```"); idx > 0 {
				trimmed = trimmed[:idx]
			}
			trimmed = strings.TrimSpace(trimmed)
			break
		}
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return nil, false
	}
	result := make(map[string]string)
	for k, v := range obj {
		if sv, ok := v.(string); ok {
			result[strings.ToUpper(k)] = sv
		}
	}
	return result, len(result) > 0
}

func normalizeDecision(raw string, decision *JudgeDecision) string {
	upper := strings.ToUpper(strings.TrimSpace(raw))
	switch upper {
	case debateDecisionGoOn, debateDecisionStop:
		return upper
	case "":
		if !decision.FallbackApplied {
			decision.FallbackApplied = true
			decision.FallbackReason = "judge response omitted DECISION; defaulted decision to CONTINUE"
		}
	default:
		decision.FallbackApplied = true
		decision.FallbackReason = fmt.Sprintf("judge response used invalid DECISION %q; defaulted to CONTINUE", upper)
	}
	return debateDecisionGoOn
}

func fillFallbackFields(decision *JudgeDecision, raw string) {
	if decision.Verdict == "" {
		decision.Verdict = compactText(raw)
	}
	if decision.Summary == "" {
		decision.Summary = compactText(raw)
	}
	if decision.Reason == "" && decision.Parsed {
		decision.Reason = compactText(raw)
	}
	if decision.FallbackApplied && decision.Summary == "" {
		decision.Summary = fallbackText(decision.FallbackReason, compactText(raw))
	}
}

func renderDebateResult(cmd *cobra.Command, result DebateResult) error {
	payload, err := debateOutputPayload(result)
	if err != nil {
		return err
	}
	if path, err := saveDebateResult(result, payload); err != nil {
		return err
	} else if path != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "Debate result saved: %s\n", path)
	}

	fmt.Fprintln(cmd.OutOrStdout(), payload)
	return nil
}

func debateOutputPayload(result DebateResult) (string, error) {
	switch result.Request.Output {
	case debateOutputJSON:
		payload, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return "", fmt.Errorf("marshal debate result failed: %w", err)
		}
		return string(payload), nil
	default:
		return renderDebateText(result), nil
	}
}

func saveDebateResult(result DebateResult, payload string) (string, error) {
	outPath := strings.TrimSpace(result.Request.Out)
	if outPath == "" && result.Request.Output != debateOutputJSON {
		return "", nil
	}
	if outPath == "" {
		outPath = debateAutoOutputPath(result)
	}
	if outPath == "" {
		return "", nil
	}
	resolved, err := filepath.Abs(outPath)
	if err != nil {
		return "", fmt.Errorf("resolve debate output path failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return "", fmt.Errorf("create debate output directory failed: %w", err)
	}
	if err := os.WriteFile(resolved, []byte(payload+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("write debate result failed: %w", err)
	}
	return resolved, nil
}

func debateAutoOutputPath(result DebateResult) string {
	baseDir := "debates"
	name := debateFileSlug(result.Request.Topic)
	if name == "" {
		name = "debate"
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	return filepath.Join(baseDir, name+"-"+stamp+debateOutputExt(result.Request.Output))
}

func debateOutputExt(output string) string {
	if output == debateOutputJSON {
		return ".json"
	}
	return ".txt"
}

func debateFileSlug(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range input {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func renderDebateText(result DebateResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Topic: %s\n", result.Request.Topic))
	b.WriteString(fmt.Sprintf("Debaters: %s (%s) vs %s (%s)\n", result.Request.Agents[0], debateStanceLabel(0), result.Request.Agents[1], debateStanceLabel(1)))
	b.WriteString(fmt.Sprintf("Judge: %s\n\n", result.Request.Judge))
	decisionsByRound := make(map[int]JudgeDecision, len(result.JudgeDecisions))
	for _, decision := range result.JudgeDecisions {
		decisionsByRound[decision.Round] = decision
	}
	for _, turn := range result.Turns {
		if turn.Role == "judge" {
			if decision, ok := decisionsByRound[turn.Round]; ok {
				b.WriteString(renderJudgeDecisionBlock(turn.Round, decision))
				b.WriteString("\n\n")
			}
			continue
		}
		label := turn.Speaker
		if turn.Stance != "" {
			label = fmt.Sprintf("%s [%s]", turn.Speaker, turn.Stance)
		}
		b.WriteString(fmt.Sprintf("Round %d | %s\n", turn.Round, label))
		b.WriteString(turn.Message)
		b.WriteString("\n\n")
	}
	b.WriteString(fmt.Sprintf("End Reason: %s\n", result.Metadata.EndReason))
	b.WriteString(fmt.Sprintf("Final Verdict: %s\n", finalVerdict(result.JudgeDecisions)))
	if summary := strings.TrimSpace(result.Summary); summary != "" {
		b.WriteString(fmt.Sprintf("Summary: %s\n", summary))
	}
	return strings.TrimSpace(b.String())
}

func formatTranscriptSnippet(turns []DebateTurn, decisions []JudgeDecision) string {
	if len(turns) == 0 {
		return "No prior turns yet."
	}
	start := 0
	if len(turns) > debateHistoryLimit {
		start = len(turns) - debateHistoryLimit
	}
	window := turns[start:]
	parts := make([]string, 0, len(window))
	for _, turn := range window {
		label := turn.Speaker
		if turn.Stance != "" {
			label = fmt.Sprintf("%s/%s", turn.Speaker, turn.Stance)
		}
		parts = append(parts, fmt.Sprintf("Round %d - %s: %s", turn.Round, label, compactText(turn.Message)))
	}
	if len(decisions) > 0 {
		last := decisions[len(decisions)-1]
		note := ""
		if last.FallbackApplied {
			note = " | fallback=" + fallbackText(last.FallbackReason, "applied")
		}
		parts = append(parts, fmt.Sprintf("Last judge decision: %s | next=%s | focus=%s | verdict=%s%s", last.Decision, fallbackText(last.NextSpeaker, "n/a"), fallbackText(last.Focus, "n/a"), fallbackText(last.Verdict, "n/a"), note))
	}
	return strings.Join(parts, "\n")
}

func renderJudgeDecisionBlock(round int, decision JudgeDecision) string {
	var b strings.Builder
	if decision.Opening {
		b.WriteString("Round 0 | Judge Opening\n")
	} else {
		b.WriteString(fmt.Sprintf("Round %d | Judge Decision\n", round))
	}
	b.WriteString(fmt.Sprintf("Decision: %s\n", decision.Decision))
	if decision.NextSpeaker != "" {
		b.WriteString(fmt.Sprintf("Next Speaker: %s\n", decision.NextSpeaker))
	}
	if decision.Focus != "" {
		b.WriteString(fmt.Sprintf("Focus: %s\n", decision.Focus))
	}
	if decision.Reason != "" {
		b.WriteString(fmt.Sprintf("Reason: %s\n", decision.Reason))
	}
	if decision.Verdict != "" {
		b.WriteString(fmt.Sprintf("Verdict: %s\n", decision.Verdict))
	}
	if decision.Summary != "" {
		b.WriteString(fmt.Sprintf("Summary: %s\n", decision.Summary))
	}
	if decision.FallbackApplied {
		b.WriteString(fmt.Sprintf("Fallback: %s\n", fallbackText(decision.FallbackReason, "applied")))
	}
	if decision.Raw != "" {
		openingText := judgeNarration(decision.Raw)
		if openingText != "" {
			b.WriteString(fmt.Sprintf("Moderator: %s\n", openingText))
		}
	}
	return strings.TrimSpace(b.String())
}

func finalVerdict(decisions []JudgeDecision) string {
	for i := len(decisions) - 1; i >= 0; i-- {
		if text := strings.TrimSpace(decisions[i].Verdict); text != "" {
			return text
		}
	}
	return debateVerdictUnknown
}

func finalSummary(decisions []JudgeDecision, runErr error) string {
	if runErr != nil {
		return compactText(runErr.Error())
	}
	for i := len(decisions) - 1; i >= 0; i-- {
		if text := strings.TrimSpace(decisions[i].Summary); text != "" {
			return text
		}
	}
	return ""
}

func debateSessionKey(base, participant string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "cli:debate"
	}
	participant = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(participant), " ", "-"))
	if participant == "" {
		participant = "participant"
	}
	return base + ":" + participant
}

func debateStanceLabel(idx int) string {
	if idx == 0 {
		return "affirmative"
	}
	return "negative"
}

func stanceForSpeaker(req DebateRequest, speaker string) string {
	for idx, candidate := range req.Agents {
		if strings.EqualFold(candidate, speaker) {
			return debateStanceLabel(idx)
		}
	}
	return ""
}

func resolveNextSpeaker(req DebateRequest, decision *JudgeDecision, previousSpeaker string) string {
	if decision == nil {
		if strings.TrimSpace(previousSpeaker) == "" {
			return req.Agents[0]
		}
		if other := otherDebater(req, previousSpeaker); other != "" {
			return other
		}
		return req.Agents[0]
	}
	next := strings.TrimSpace(decision.NextSpeaker)
	for _, candidate := range req.Agents {
		if strings.EqualFold(candidate, next) {
			return candidate
		}
	}
	decision.FallbackApplied = true
	if next == "" {
		decision.FallbackReason = appendFallbackReason(decision.FallbackReason, "judge response omitted NEXT_SPEAKER; applied default speaking order")
	} else {
		decision.FallbackReason = appendFallbackReason(decision.FallbackReason, fmt.Sprintf("judge response used invalid NEXT_SPEAKER %q; applied default speaking order", next))
	}
	if strings.TrimSpace(previousSpeaker) == "" {
		return req.Agents[0]
	}
	other := otherDebater(req, previousSpeaker)
	if other != "" {
		return other
	}
	return req.Agents[0]
}

func appendFallbackReason(base, extra string) string {
	base = strings.TrimSpace(base)
	extra = strings.TrimSpace(extra)
	if base == "" {
		return extra
	}
	if extra == "" || strings.Contains(base, extra) {
		return base
	}
	return base + "; " + extra
}

func otherDebater(req DebateRequest, speaker string) string {
	for _, candidate := range req.Agents {
		if !strings.EqualFold(candidate, speaker) {
			return candidate
		}
	}
	return ""
}

func lastDebaterSpeaker(turns []DebateTurn) string {
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Role == "debater" {
			return turns[i].Speaker
		}
	}
	return ""
}

func judgeNarration(raw string) string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	freeform := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if debateFieldRE.MatchString(trimmed) {
			continue
		}
		freeform = append(freeform, trimmed)
	}
	return strings.Join(freeform, " ")
}

func normalizeNames(items []string) []string {
	cleaned := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		cleaned = append(cleaned, item)
	}
	return cleaned
}

func uniqueNonEmpty(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	cleaned := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, trimmed)
	}
	return cleaned
}

func compactText(text string) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\n", " ")
	text = debateBlankRE.ReplaceAllString(text, " ")
	return strings.Join(strings.Fields(text), " ")
}

func fallbackText(text, fallback string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return fallback
	}
	return text
}
