package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sjzsdu/tt/internal/agents"
	"github.com/sjzsdu/tt/internal/formula"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	ttconfig "github.com/sjzsdu/tt/internal/ttconfig"
)

var (
	formulaDir             string
	formulaVars            []string
	formulaOutput          string
	formulaTitle           string
	formulaMarkdown        bool
	formulaPort            int
	formulaAgent           string
	formulaModel           string
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
	formulaRuntimeEngine   bool
	formulaLegacyEngine    bool
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
	formulaCompileWorkflow bool
	formulaRunSessionSeq   uint64
)

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
	return ttconfig.FormulaRunDir(loaded, formulaWorkingDir())
}

func formulaWorkingDir() string {
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

func applyFormulaRunPositionalVars(f *formula.Formula, values []string, vars map[string]string) error {
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

func validateFormulaAgentConfiguration(rt *pcwrap.Runtime, recipe *formula.Recipe, defaultAgent, model, session string) error {
	if rt == nil {
		return fmt.Errorf("picoclaw runtime not loaded")
	}
	requirements := collectFormulaAgentRequirements(recipe, defaultAgent)
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

func collectFormulaAgentRequirements(recipe *formula.Recipe, defaultAgent string) []formulaAgentRequirement {
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
	var walkSteps func([]formula.RecipeStep)
	walkSteps = func(steps []formula.RecipeStep) {
		for _, step := range steps {
			if step.IsRoot || step.Execution == "noop" || step.Execution == "script" {
				continue
			}
			if step.Agent != nil && strings.TrimSpace(step.Agent.Name) != "" {
				add(step.Agent.Name, step.ID, step.Title, "step agent")
			}
			if step.Loop != nil {
				for _, body := range step.Loop.Body {
					if body == nil || strings.TrimSpace(body.Execution) == "noop" || strings.TrimSpace(body.Execution) == "script" {
						continue
					}
					if body.Agent != nil && strings.TrimSpace(body.Agent.Name) != "" {
						add(body.Agent.Name, step.ID+".loop."+body.ID, body.Title, "loop body agent")
					}
				}
			}
		}
	}
	if recipe != nil {
		walkSteps(recipe.Steps)
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
