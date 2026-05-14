package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	ttconfig "github.com/sjzsdu/tt/internal/ttconfig"
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
	stockBullAgentID     = "stock-growth-investor"
	stockBearAgentID     = "stock-risk-investor"
	stockHostAgentID     = "stock-discussion-host"
)

var stockDiscussionSkills = []string{"tongstock-cli", "agent-browser"}

const stockGrowthInvestorPrompt = `# 成长型投资者

你是一位偏乐观但不盲目的成长型股票投资者。你关注业务增速、行业空间、竞争优势、管理层执行力、资金流向和催化剂。

表达风格：
- 像投资者交流，不像正式辩论。
- 先给观点，再给证据和假设。
- 承认风险，但重点说明为什么仍可能上涨。
- 用中文输出，语言自然、克制、信息密度高。

要求：
- 不要假装知道实时行情；如果题目没有给数据，要说明需要验证哪些数据。
- 可以并且应该使用 tongstock-cli skill、web/search、browser 等工具核验行情、财务、公告、新闻和行业资料；用到外部数据时说明来源或数据口径。
- 不给确定性投资建议，不喊单。
- 每次发言必须针对上一位投资者的具体观点回应，不要泛泛重复自己的立场。
- 每次发言尽量包含：核心观点、支撑信息、需要验证的数据、对另一位投资者观点的回应。`

const stockRiskInvestorPrompt = `# 风险型投资者

你是一位偏谨慎、重视估值和下行风险的股票投资者。你关注估值、安全边际、周期位置、财务质量、政策风险、竞争格局恶化和市场预期差。

表达风格：
- 像投资者交流，不像正式辩论。
- 先指出关键风险，再给证据和反例。
- 承认上涨可能，但重点说明为什么需要谨慎。
- 用中文输出，语言自然、克制、信息密度高。

要求：
- 不要假装知道实时行情；如果题目没有给数据，要说明需要验证哪些数据。
- 可以并且应该使用 tongstock-cli skill、web/search、browser 等工具核验行情、财务、公告、新闻和行业资料；用到外部数据时说明来源或数据口径。
- 不给确定性投资建议，不喊单。
- 每次发言必须针对上一位投资者的具体观点回应，不要泛泛重复自己的立场。
- 每次发言尽量包含：核心观点、风险依据、需要验证的数据、对另一位投资者观点的回应。`

const stockDiscussionHostPrompt = `# 股票讨论主持人

你是一个股票投资讨论的主持人和整理者。你的任务不是裁判谁赢，而是让两个不同风格的投资者把信息、假设、分歧和需要验证的数据讲清楚。

风格：
- 像投资圈朋友在认真讨论，不要像正式辩论会。
- 避免“正方/反方/裁判/胜负”等词。
- 用中文输出，简洁清楚。

职责：
- 开场时框定股票或主题、关键分歧、先让哪位投资者发言。
- 每轮后提炼共识、分歧、下一步要补充的信息。
- 最后给出讨论纪要，而不是投资建议。

结构化字段必须按要求输出，便于 CLI 解析。`

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
	Short: "Run a stock bull/bear investor discussion",
	Long:  "Run an embedded stock discussion between a growth-oriented investor, a risk-oriented investor, and a host who organizes the key information, assumptions, disagreements, and follow-up data points.",
	Args:  cobra.ArbitraryArgs,
	Example: `tt debate "贵州茅台接下来半年怎么看"
	tt debate --topic "英伟达估值是否还能支撑上涨" --rounds 4
	tt debate "比亚迪现在是机会还是风险" --output json --out debates/byd.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDebate(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(debateCmd)
	debateCmd.Flags().StringVarP(&debateTopic, "topic", "t", "", "debate topic; positional args are also supported")
	debateCmd.Flags().StringSliceVar(&debateAgents, "agents", nil, "two investor agent ids or names; defaults to embedded growth and risk investors")
	debateCmd.Flags().StringVar(&debateJudge, "judge", "", "host agent id or name; defaults to embedded stock discussion host")
	debateCmd.Flags().IntVarP(&debateRounds, "rounds", "r", debateDefaultRounds, "maximum number of discussion turns")
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
	if !cmd.Flags().Changed("agents") {
		merged.Debate.Agents = nil
	}
	if !cmd.Flags().Changed("judge") {
		merged.Debate.Judge = ""
	}
	if !cmd.Flags().Changed("output") {
		merged.Debate.Output = debateDefaultOutput
	}

	rt, err := pcwrap.Load(pcwrap.Options{
		Home:      merged.Picoclaw.Home,
		Config:    merged.Picoclaw.Config,
		TTConfig:  merged,
		TTSources: loaded.Sources,
	})
	if err != nil {
		return err
	}

	req, err := buildDebateRequest(topic, merged)
	if err != nil {
		return err
	}
	if err := validateDebateRequest(req, rt); err != nil {
		return err
	}

	runner, err := rt.NewDirectRunner(pcwrap.RunOptions{
		Session:        req.Session,
		Model:          req.Model,
		Debug:          req.Debug,
		Quiet:          !req.Debug,
		EmbeddedAgents: embeddedStockDiscussionAgents(),
	})
	if err != nil {
		return err
	}
	defer runner.Close()

	result, runErr := executeDebate(runner, req, cmd.OutOrStdout())
	if renderErr := renderDebateResult(cmd, result, req.Output == debateOutputJSON); renderErr != nil {
		return renderErr
	}
	if runErr != nil {
		return runErr
	}
	return nil
}

func buildDebateRequest(topic string, merged ttconfig.Config) (DebateRequest, error) {
	agents := append([]string(nil), merged.Debate.Agents...)
	judge := strings.TrimSpace(merged.Debate.Judge)
	if len(agents) == 0 {
		agents = []string{stockBullAgentID, stockBearAgentID}
	}
	if judge == "" {
		judge = stockHostAgentID
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

func embeddedStockDiscussionAgents() []pcwrap.EmbeddedAgent {
	return []pcwrap.EmbeddedAgent{
		{ID: stockBullAgentID, Name: "成长型投资者", Prompt: stockGrowthInvestorPrompt, Skills: stockDiscussionSkills, NoHistory: false, EnableResearchTools: true},
		{ID: stockBearAgentID, Name: "风险型投资者", Prompt: stockRiskInvestorPrompt, Skills: stockDiscussionSkills, NoHistory: false, EnableResearchTools: true},
		{ID: stockHostAgentID, Name: "讨论主持人", Prompt: stockDiscussionHostPrompt, Skills: stockDiscussionSkills, NoHistory: false, EnableResearchTools: true},
	}
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
	embeddedAgents := embeddedStockDiscussionAgents()
	for _, name := range participants {
		if _, err := rt.ResolveRunOptions(pcwrap.RunOptions{Session: req.Session, Agent: name, Model: req.Model, EmbeddedAgents: embeddedAgents}); err != nil {
			return fmt.Errorf("agent %q not found; available agents: %v", name, availableAgents)
		}
	}
	return nil
}

func executeDebate(runner *pcwrap.DirectRunner, req DebateRequest, out io.Writer) (DebateResult, error) {
	startedAt := time.Now().UTC()
	result := DebateResult{Request: req, Metadata: DebateMetadata{StartedAt: startedAt.Format(time.RFC3339)}}
	var (
		turns       []DebateTurn
		decisions   []JudgeDecision
		focus       string
		nextSpeaker string
		runErr      error
	)

	focus = "先给出你的核心投资观点、主要依据、关键风险和需要验证的数据。"
	nextSpeaker = req.Agents[0]

	for round := 1; round <= req.Rounds; round++ {
		speaker := nextSpeaker
		if speaker == "" {
			speaker = req.Agents[(round-1)%len(req.Agents)]
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
		renderLiveDebateTurn(out, DebateTurn{Round: round, Speaker: speaker, Role: "debater", Stance: stance, Message: message})

		focus = "回应上一位投资者的核心观点，并补充新的信息、假设或风险。"
		nextSpeaker = otherDebater(req, speaker)
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
		other = "另一位投资者"
	}
	focusLine := strings.TrimSpace(focus)
	if focusLine == "" {
		focusLine = "补充一个关键投资信息，并回应对方最强的一点。"
	}
	return strings.TrimSpace(fmt.Sprintf(`你正在参加一个股票投资讨论，不是正式辩论。

讨论主题: %s
当前发言: %d / %d
你的身份: %s
你的投资风格: %s
另一位投资者: %s
主持人提示: %s

要求:
- 像投资者之间认真交流，不要说“辩论”“正方”“反方”“裁判”。
- 先回应另一位投资者最值得重视的观点，再补充你的信息和判断。
- 清楚区分：已知信息、推测假设、需要进一步验证的数据。
- 不要给确定性买卖建议，不要喊单。
- 控制在 3 到 6 个要点，中文输出。

最近讨论记录:
%s

只输出你的本轮讨论发言。`, req.Topic, round, req.Rounds, speaker, stance, other, focusLine, formatTranscriptSnippet(turns, decisions)))
}

func buildJudgeOpeningPrompt(req DebateRequest) string {
	return strings.TrimSpace(fmt.Sprintf(`你是股票投资讨论的主持人。请组织两个不同风格投资者的讨论，不要把它说成正式辩论。

讨论主题: %s
最多发言轮次: %d
投资者 A: %s (%s)
投资者 B: %s (%s)

请用简短开场框定问题、指出关键分歧，并选择先发言的投资者。

Return your response as a JSON code block with these fields:
{
  "DECISION": "CONTINUE",
  "NEXT_SPEAKER": "<%s or %s>",
  "FOCUS": "<第一位投资者应先讨论的关键问题>",
  "REASON": "<为什么先让他发言>",
  "VERDICT": "讨论刚开始，暂不做结论。",
  "SUMMARY": "<一句话开场摘要>"
}

JSON 后面补一句自然的中文开场白。`, req.Topic, req.Rounds, req.Agents[0], debateStanceLabel(0), req.Agents[1], debateStanceLabel(1), req.Agents[0], req.Agents[1]))
}

func buildJudgePrompt(req DebateRequest, turns []DebateTurn, decisions []JudgeDecision, round int) string {
	decisionHint := debateDecisionGoOn
	if round >= req.Rounds {
		decisionHint = debateDecisionStop
	}
	lastSpeaker := lastDebaterSpeaker(turns)
	otherSpeaker := otherDebater(req, lastSpeaker)
	return strings.TrimSpace(fmt.Sprintf(`你是股票投资讨论的主持人和整理者。你的任务是让信息、假设、分歧和待验证数据更清楚，不是判输赢。

讨论主题: %s
刚完成的发言: %d / %d
投资者 A: %s (%s)
投资者 B: %s (%s)
上一位发言者: %s

最近讨论记录:
%s

Return your response as a JSON code block with these fields:
{
  "DECISION": "%s",
  "NEXT_SPEAKER": "<%s or %s; leave blank only if DECISION is STOP>",
  "FOCUS": "<下一步应补充或澄清的核心信息；如果结束，写核心分歧>",
  "REASON": "<为什么继续或结束>",
  "VERDICT": "<当前讨论纪要：主要共识、分歧和待验证数据，不要写胜负>",
  "SUMMARY": "<一句话给读者的摘要>"
}

Allowed DECISION values: CONTINUE or STOP.
如果已经达到最大轮次，DECISION 必须是 STOP。
继续时明确选择下一位发言者，通常切换给另一位投资者。`, req.Topic, round, req.Rounds, req.Agents[0], debateStanceLabel(0), req.Agents[1], debateStanceLabel(1), fallbackText(lastSpeaker, "none yet"), formatTranscriptSnippet(turns, decisions), decisionHint, fallbackText(otherSpeaker, req.Agents[0]), fallbackText(lastSpeaker, req.Agents[1])))
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

func renderDebateResult(cmd *cobra.Command, result DebateResult, printPayload bool) error {
	payload, err := debateOutputPayload(result)
	if err != nil {
		return err
	}
	jsonPayload, err := debateJSONPayload(result)
	if err != nil {
		return err
	}
	if path, err := saveDebateResult(result, jsonPayload); err != nil {
		return err
	} else if path != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "Discussion result saved: %s\n", path)
	}

	if printPayload {
		fmt.Fprintln(cmd.OutOrStdout(), payload)
	}
	return nil
}

func debateOutputPayload(result DebateResult) (string, error) {
	switch result.Request.Output {
	case debateOutputJSON:
		return debateJSONPayload(result)
	default:
		return renderDebateText(result), nil
	}
}

func debateJSONPayload(result DebateResult) (string, error) {
	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal debate result failed: %w", err)
	}
	return string(payload), nil
}

func saveDebateResult(result DebateResult, payload string) (string, error) {
	outPath := strings.TrimSpace(result.Request.Out)
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
		name = "stock-discussion"
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	return filepath.Join(baseDir, name+"-"+stamp+".json")
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
	b.WriteString(fmt.Sprintf("讨论主题: %s\n", result.Request.Topic))
	b.WriteString(fmt.Sprintf("参与者: %s（%s） / %s（%s）\n", result.Request.Agents[0], debateStanceLabel(0), result.Request.Agents[1], debateStanceLabel(1)))
	b.WriteString(fmt.Sprintf("主持整理: %s\n\n", result.Request.Judge))
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
		b.WriteString(fmt.Sprintf("第 %d 轮 | %s\n", turn.Round, label))
		b.WriteString(turn.Message)
		b.WriteString("\n\n")
	}
	b.WriteString(fmt.Sprintf("结束原因: %s\n", result.Metadata.EndReason))
	b.WriteString(fmt.Sprintf("讨论纪要: %s\n", finalVerdict(result.JudgeDecisions)))
	if summary := strings.TrimSpace(result.Summary); summary != "" {
		b.WriteString(fmt.Sprintf("摘要: %s\n", summary))
	}
	return strings.TrimSpace(b.String())
}

func renderLiveDebateTurn(out io.Writer, turn DebateTurn) {
	if out == nil || turn.Role != "debater" {
		return
	}
	name := displayInvestorName(turn.Speaker, turn.Stance)
	fmt.Fprintf(out, "\n%s：\n%s\n", name, strings.TrimSpace(turn.Message))
}

func displayInvestorName(speaker, stance string) string {
	stance = strings.TrimSpace(stance)
	if stance != "" {
		return stance
	}
	return strings.TrimSpace(speaker)
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
		b.WriteString("开场 | 主持人\n")
	} else {
		b.WriteString(fmt.Sprintf("第 %d 轮 | 主持整理\n", round))
	}
	b.WriteString(fmt.Sprintf("状态: %s\n", decision.Decision))
	if decision.NextSpeaker != "" {
		b.WriteString(fmt.Sprintf("下一位: %s\n", decision.NextSpeaker))
	}
	if decision.Focus != "" {
		b.WriteString(fmt.Sprintf("关注点: %s\n", decision.Focus))
	}
	if decision.Reason != "" {
		b.WriteString(fmt.Sprintf("原因: %s\n", decision.Reason))
	}
	if decision.Verdict != "" {
		b.WriteString(fmt.Sprintf("讨论纪要: %s\n", decision.Verdict))
	}
	if decision.Summary != "" {
		b.WriteString(fmt.Sprintf("摘要: %s\n", decision.Summary))
	}
	if decision.FallbackApplied {
		b.WriteString(fmt.Sprintf("Fallback: %s\n", fallbackText(decision.FallbackReason, "applied")))
	}
	if decision.Raw != "" {
		openingText := judgeNarration(decision.Raw)
		if openingText != "" {
			b.WriteString(fmt.Sprintf("主持人: %s\n", openingText))
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
	return "详见双方投资者观点记录。"
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
	return "讨论已完成，完整对话已保存为 JSON。"
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
		return "偏成长/看多"
	}
	return "偏风险/谨慎"
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
	cleaned := stripFirstFencedBlock(strings.TrimSpace(raw))
	lines := strings.Split(cleaned, "\n")
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

func stripFirstFencedBlock(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}
	lines := strings.Split(trimmed, "\n")
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
			end = i
			break
		}
	}
	if end == -1 || end+1 >= len(lines) {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
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
