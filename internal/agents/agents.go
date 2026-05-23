package agents

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	"gopkg.in/yaml.v3"
)

const (
	TranslateMasterID     = "translate-master"
	CoderID               = "coder"
	ReporterID            = "reporter"
	legacyReporterID      = "nvwa-agent"
	FullStackID           = "full-stack"
	PlannerID             = "planner"
	ProductManagerID      = "product-manager"
	TesterID              = "tester"
	UIID                  = "ui"
	StockBeginnerID       = "stock-growth-investor"
	StockOldHandID        = "stock-risk-investor"
	StockDiscussionHostID = "stock-discussion-host"
	Repo2SkillID          = "repo2skill"
	FormulaWriterID       = "formula-writer"
	DocsAnalystID         = "docs-analyst"
)

//go:embed embedded/*.md
var embeddedFS embed.FS

type definition struct {
	ID                  string   `yaml:"id"`
	Name                string   `yaml:"name"`
	Soul                string   `yaml:"soul"`
	Skills              []string `yaml:"skills"`
	NoHistory           bool     `yaml:"no_history"`
	EnableResearchTools bool     `yaml:"enable_research_tools"`
}

func TranslateMaster() pcwrap.EmbeddedAgent {
	return mustGet(TranslateMasterID)
}

func Coder() pcwrap.EmbeddedAgent {
	return mustGet(CoderID)
}

func Reporter() pcwrap.EmbeddedAgent {
	return mustGet(ReporterID)
}

func FullStack() pcwrap.EmbeddedAgent {
	return mustGet(FullStackID)
}

func Planner() pcwrap.EmbeddedAgent {
	return mustGet(PlannerID)
}

func ProductManager() pcwrap.EmbeddedAgent {
	return mustGet(ProductManagerID)
}

func Tester() pcwrap.EmbeddedAgent {
	return mustGet(TesterID)
}

func UI() pcwrap.EmbeddedAgent {
	return mustGet(UIID)
}

func Repo2Skill() pcwrap.EmbeddedAgent {
	return mustGet(Repo2SkillID)
}

func FormulaWriter() pcwrap.EmbeddedAgent {
	return mustGet(FormulaWriterID)
}

func DocsAnalyst() pcwrap.EmbeddedAgent {
	return mustGet(DocsAnalystID)
}

func Core() []pcwrap.EmbeddedAgent {
	return mustGetMany(CoderID, FullStackID, PlannerID, ProductManagerID, TesterID, UIID)
}

func All() []pcwrap.EmbeddedAgent {
	agents, err := List()
	if err != nil {
		panic(err)
	}
	return agents
}

func StockDiscussion() []pcwrap.EmbeddedAgent {
	return mustGetMany(StockBeginnerID, StockOldHandID, StockDiscussionHostID)
}

func Get(id string) (pcwrap.EmbeddedAgent, error) {
	id = canonicalAgentID(strings.TrimSpace(id))
	if id == "" {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("agent id is required")
	}
	agents, err := List()
	if err != nil {
		return pcwrap.EmbeddedAgent{}, err
	}
	for _, agent := range agents {
		if agent.ID == id {
			return agent, nil
		}
	}
	return pcwrap.EmbeddedAgent{}, fmt.Errorf("embedded agent %q not found", id)
}

func List() ([]pcwrap.EmbeddedAgent, error) {
	entries, err := embeddedFS.ReadDir("embedded")
	if err != nil {
		return nil, fmt.Errorf("read embedded agents failed: %w", err)
	}
	agents := make([]pcwrap.EmbeddedAgent, 0, len(entries))
	seen := map[string]struct{}{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		agent, err := loadMarkdownAgent("embedded/" + entry.Name())
		if err != nil {
			return nil, err
		}
		if _, exists := seen[agent.ID]; exists {
			return nil, fmt.Errorf("duplicate embedded agent id %q", agent.ID)
		}
		seen[agent.ID] = struct{}{}
		agents = append(agents, agent)
	}

	fsAgents, err := loadFilesystemAgents()
	if err != nil {
		return nil, err
	}
	for _, agent := range fsAgents {
		if _, exists := seen[agent.ID]; exists {
			return nil, fmt.Errorf("duplicate embedded agent id %q", agent.ID)
		}
		seen[agent.ID] = struct{}{}
		agents = append(agents, agent)
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })
	return agents, nil
}

func loadFilesystemAgents() ([]pcwrap.EmbeddedAgent, error) {
	searchRoots := []string{".tt/agents"}
	collected := make([]pcwrap.EmbeddedAgent, 0)
	for _, root := range searchRoots {
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read embedded agents from %s failed: %w", root, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
				continue
			}
			agent, err := loadMarkdownAgentFile(filepath.Join(root, entry.Name()))
			if err != nil {
				return nil, err
			}
			collected = append(collected, agent)
		}
	}
	return collected, nil
}

func mustGet(id string) pcwrap.EmbeddedAgent {
	agent, err := Get(id)
	if err != nil {
		panic(err)
	}
	return agent
}

func mustGetMany(ids ...string) []pcwrap.EmbeddedAgent {
	agents := make([]pcwrap.EmbeddedAgent, 0, len(ids))
	for _, id := range ids {
		agents = append(agents, mustGet(id))
	}
	return agents
}

func loadMarkdownAgent(path string) (pcwrap.EmbeddedAgent, error) {
	data, err := embeddedFS.ReadFile(path)
	if err != nil {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("read embedded agent %s failed: %w", path, err)
	}
	meta, body, err := splitFrontMatter(string(data))
	if err != nil {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("parse embedded agent %s failed: %w", path, err)
	}
	var def definition
	if err := yaml.Unmarshal([]byte(meta), &def); err != nil {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("parse embedded agent %s frontmatter failed: %w", path, err)
	}
	def.ID = canonicalAgentID(strings.TrimSpace(def.ID))
	def.Name = strings.TrimSpace(def.Name)
	if def.ID == "" {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("embedded agent %s missing id", path)
	}
	if def.Name == "" {
		def.Name = def.ID
	}
	return pcwrap.EmbeddedAgent{
		ID:                  def.ID,
		Name:                def.Name,
		Prompt:              strings.TrimSpace(body),
		Soul:                strings.TrimSpace(def.Soul),
		Skills:              compactStrings(def.Skills),
		NoHistory:           def.NoHistory,
		EnableResearchTools: def.EnableResearchTools,
	}, nil
}

func loadMarkdownAgentFile(path string) (pcwrap.EmbeddedAgent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("read embedded agent %s failed: %w", path, err)
	}
	meta, body, err := splitFrontMatter(string(data))
	if err != nil {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("parse embedded agent %s failed: %w", path, err)
	}
	var def definition
	if err := yaml.Unmarshal([]byte(meta), &def); err != nil {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("parse embedded agent %s frontmatter failed: %w", path, err)
	}
	def.ID = canonicalAgentID(strings.TrimSpace(def.ID))
	def.Name = strings.TrimSpace(def.Name)
	if def.ID == "" {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("embedded agent %s missing id", path)
	}
	if def.Name == "" {
		def.Name = def.ID
	}
	return pcwrap.EmbeddedAgent{
		ID:                  def.ID,
		Name:                def.Name,
		Prompt:              strings.TrimSpace(body),
		Soul:                strings.TrimSpace(def.Soul),
		Skills:              compactStrings(def.Skills),
		NoHistory:           def.NoHistory,
		EnableResearchTools: def.EnableResearchTools,
	}, nil
}

func canonicalAgentID(id string) string {
	switch strings.TrimSpace(id) {
	case legacyReporterID:
		return ReporterID
	default:
		return strings.TrimSpace(id)
	}
}

func splitFrontMatter(content string) (string, string, error) {
	content = strings.TrimPrefix(content, "\ufeff")
	if !strings.HasPrefix(content, "---\n") {
		return "", "", fmt.Errorf("missing YAML frontmatter")
	}
	rest := strings.TrimPrefix(content, "---\n")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", "", fmt.Errorf("unterminated YAML frontmatter")
	}
	meta := rest[:idx]
	body := strings.TrimPrefix(rest[idx+len("\n---"):], "\n")
	return meta, body, nil
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
