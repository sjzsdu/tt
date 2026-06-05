package formulacmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sjzsdu/tt/internal/agents"
	"github.com/sjzsdu/tt/internal/formula/ir"
	spec "github.com/sjzsdu/tt/internal/formula/spec"
	"github.com/sjzsdu/tt/internal/formula/steps"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	ttconfig "github.com/sjzsdu/tt/internal/ttconfig"
)

type App struct {
	opts formulaOptions
}

type formulaOptions struct {
	Dir             string
	Vars            []string
	Output          string
	Title           string
	Markdown        bool
	Port            int
	Agent           string
	Model           string
	ExternalDriver  string
	Session         string
	Web             bool
	NoWeb           bool
	WebPort         int
	DryRun          bool
	Debug           bool
	Verbose         bool
	NoSave          bool
	NoScript        bool
	AllowShell      bool
	CreateOutput    string
	CreateForce     bool
	CreateStdout    bool
	OptimizeOutput  string
	OptimizeStdout  bool
	OptimizeBuiltin bool
	RunsLimit       int
	RunsFormula     string
	RunsStatus      string
	RunShowStep     string
	RunRmYes        bool
	InputFields     []string
	ListBuiltin     bool
	ListUser        bool
	ListCategory    string
}

var (
	formulaDir             string
	formulaVars            []string
	formulaOutput          string
	formulaTitle           string
	formulaMarkdown        bool
	formulaPort            int
	formulaAgent           string
	formulaModel           string
	formulaExternalDriver  string
	formulaSession         string
	formulaWeb             bool
	formulaNoWeb           bool
	formulaWebPort         int
	formulaDryRun          bool
	formulaDebug           bool
	formulaVerbose         bool
	formulaNoSave          bool
	formulaNoScript        bool
	formulaAllowShell      bool
	formulaCreateOutput    string
	formulaCreateForce     bool
	formulaCreateStdout    bool
	formulaOptimizeOutput  string
	formulaOptimizeStdout  bool
	formulaOptimizeBuiltin bool
	formulaRunsLimit       int
	formulaRunsFormula     string
	formulaRunsStatus      string
	formulaRunShowStep     string
	formulaRunRmYes        bool
	formulaInputFields     []string
	formulaListBuiltin     bool
	formulaListUser        bool
	formulaListCategory    string
	formulaRunSessionSeq   uint64
)

func (a *App) installOptions() {
	formulaDir = a.opts.Dir
	formulaVars = a.opts.Vars
	formulaOutput = a.opts.Output
	formulaTitle = a.opts.Title
	formulaMarkdown = a.opts.Markdown
	formulaPort = a.opts.Port
	formulaAgent = a.opts.Agent
	formulaModel = a.opts.Model
	formulaExternalDriver = a.opts.ExternalDriver
	formulaSession = a.opts.Session
	formulaWeb = a.opts.Web
	formulaNoWeb = a.opts.NoWeb
	formulaWebPort = a.opts.WebPort
	formulaDryRun = a.opts.DryRun
	formulaDebug = a.opts.Debug
	formulaVerbose = a.opts.Verbose
	formulaNoSave = a.opts.NoSave
	formulaNoScript = a.opts.NoScript
	formulaAllowShell = a.opts.AllowShell
	formulaCreateOutput = a.opts.CreateOutput
	formulaCreateForce = a.opts.CreateForce
	formulaCreateStdout = a.opts.CreateStdout
	formulaOptimizeOutput = a.opts.OptimizeOutput
	formulaOptimizeStdout = a.opts.OptimizeStdout
	formulaOptimizeBuiltin = a.opts.OptimizeBuiltin
	formulaRunsLimit = a.opts.RunsLimit
	formulaRunsFormula = a.opts.RunsFormula
	formulaRunsStatus = a.opts.RunsStatus
	formulaRunShowStep = a.opts.RunShowStep
	formulaRunRmYes = a.opts.RunRmYes
	formulaInputFields = a.opts.InputFields
	formulaListBuiltin = a.opts.ListBuiltin
	formulaListUser = a.opts.ListUser
	formulaListCategory = a.opts.ListCategory
}

func getSearchPaths() []string {
	homeDir, _ := os.UserHomeDir()
	return formulaSearchPaths(formulaMustLoadTTConfig(), formulaDir, homeDir)
}

func formulaSearchPaths(loaded ttconfig.Loaded, explicitDir, homeDir string) []string {
	if explicitDir != "" {
		return []string{explicitDir}
	}

	paths := []string{formulaDefaultDir(loaded)}
	if homeDir != "" {
		paths = appendUniquePath(paths, filepath.Join(homeDir, ".tt", "formulas"))
	}
	return paths
}

func formulaLoadTTConfig() (ttconfig.Loaded, error) {
	return ttconfig.Load("")
}

func formulaMustLoadTTConfig() ttconfig.Loaded {
	loaded, err := formulaLoadTTConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: load tt config failed: %v\n", err)
		return ttconfig.Loaded{}
	}
	return loaded
}

func formulaDefaultDir(loaded ttconfig.Loaded) string {
	return ttconfig.FormulaDir(loaded, formulaWorkingDir())
}

func formulaDefaultRunDir(loaded ttconfig.Loaded) string {
	if strings.TrimSpace(loaded.Merged.Paths.FormulaRunDir) != "" {
		return ttconfig.FormulaRunDir(loaded, formulaWorkingDir())
	}
	return filepath.Join(formulaWorkingDir(), ".tt", "runs", "formula")
}

func formulaWorkingDir() string {
	if preserved := strings.TrimSpace(os.Getenv("TT_INVOCATION_CWD")); preserved != "" {
		return preserved
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func appendUniquePath(paths []string, path string) []string {
	clean := filepath.Clean(path)
	for _, existing := range paths {
		if filepath.Clean(existing) == clean {
			return paths
		}
	}
	return append(paths, clean)
}

func uniqueFormulaRunSession(base, formulaName string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "cli:formula"
	}
	formulaSlug := sessionSlug(formulaName)
	if formulaSlug == "" {
		formulaSlug = "formula"
	}
	seq := atomic.AddUint64(&formulaRunSessionSeq, 1)
	return fmt.Sprintf("%s:%s:%s-%d", base, formulaSlug, time.Now().UTC().Format("20060102T150405.000000000Z"), seq)
}

func sessionSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func parseVars() map[string]string {
	vars := make(map[string]string)
	for _, v := range formulaVars {
		key, value, ok := strings.Cut(v, "=")
		if ok && key != "" {
			vars[key] = value
		}
	}
	return vars
}

func applyFormulaRunPositionalVars(f *spec.Formula, values []string, vars map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	if f == nil {
		return fmt.Errorf("formula is required for positional variables")
	}
	required := f.RequiredVarNames()
	if len(required) != 1 {
		return fmt.Errorf("positional value shorthand requires exactly one required variable, found %d; use --var key=value", len(required))
	}
	name := required[0]
	if _, exists := vars[name]; exists {
		return fmt.Errorf("variable %q is already set via --var; remove the positional value or the --var override", name)
	}
	value := strings.TrimSpace(strings.Join(values, " "))
	if value == "" {
		return fmt.Errorf("positional value for required variable %q cannot be empty", name)
	}
	vars[name] = value
	return nil
}

func defaultFormulaAgent(agent string) string {
	if strings.TrimSpace(agent) == "" {
		return pcwrap.DefaultAgentID
	}
	return agent
}

type formulaAgentRequirement struct {
	Name      string
	StepID    string
	StepTitle string
	Source    string
}

func validateFormulaAgentConfiguration(rt *pcwrap.Runtime, workflow *ir.Workflow, defaultAgent, model, session string) error {
	if rt == nil {
		return fmt.Errorf("picoclaw runtime not loaded")
	}
	requirements := collectFormulaAgentRequirements(workflow, defaultAgent)
	embeddedAgents, err := agents.List()
	if err != nil {
		return fmt.Errorf("list embedded agents failed: %w", err)
	}
	availableConfigured := uniqueSortedStrings(rt.Summary().Agents)
	availableEmbedded := embeddedAgentIDs(embeddedAgents)
	for _, req := range requirements {
		_, err := rt.ResolveRunOptions(pcwrap.RunOptions{
			Session:        session,
			Agent:          req.Name,
			Model:          model,
			EmbeddedAgents: embeddedAgents,
		})
		if err != nil {
			return fmt.Errorf("formula agent preflight failed for %s %q (%s): %w\navailable configured agents: %s\navailable embedded agents: %s", req.Source, req.Name, formulaAgentRequirementLabel(req), err, joinOrNone(availableConfigured), joinOrNone(availableEmbedded))
		}
	}
	return nil
}

func collectFormulaAgentRequirements(workflow *ir.Workflow, defaultAgent string) []formulaAgentRequirement {
	seen := map[string]formulaAgentRequirement{}
	add := func(name, stepID, stepTitle, source string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		key := strings.ToLower(name) + "|" + stepID + "|" + source
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = formulaAgentRequirement{Name: name, StepID: stepID, StepTitle: stepTitle, Source: source}
	}
	add(defaultAgent, "", "", "default agent")
	if workflow != nil {
		for nodeID, node := range workflow.Graph.Nodes {
			if node == nil || node.Step == nil {
				continue
			}
			agentStep, ok := node.Step.(steps.AgentStep)
			if !ok || strings.TrimSpace(agentStep.Agent) == "" {
				continue
			}
			meta := node.Step.Meta()
			add(agentStep.Agent, string(nodeID), meta.Title, "step agent")
		}
	}
	out := make([]formulaAgentRequirement, 0, len(seen))
	for _, req := range seen {
		out = append(out, req)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		if out[i].StepID != out[j].StepID {
			return out[i].StepID < out[j].StepID
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func formulaAgentRequirementLabel(req formulaAgentRequirement) string {
	if strings.TrimSpace(req.StepID) == "" {
		return "formula default"
	}
	if strings.TrimSpace(req.StepTitle) == "" {
		return req.StepID
	}
	return fmt.Sprintf("%s / %s", req.StepID, req.StepTitle)
}

func embeddedAgentIDs(items []pcwrap.EmbeddedAgent) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if id := strings.TrimSpace(item.ID); id != "" {
			out = append(out, id)
		}
	}
	return uniqueSortedStrings(out)
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ", ")
}
