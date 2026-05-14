package agents

import (
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	"gopkg.in/yaml.v3"
)

const (
	TranslateMasterID     = "translate-master"
	StockBeginnerID       = "stock-growth-investor"
	StockOldHandID        = "stock-risk-investor"
	StockDiscussionHostID = "stock-discussion-host"
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

func StockDiscussion() []pcwrap.EmbeddedAgent {
	return mustGetMany(StockBeginnerID, StockOldHandID, StockDiscussionHostID)
}

func Get(id string) (pcwrap.EmbeddedAgent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("agent id is required")
	}
	entries, err := embeddedFS.ReadDir("embedded")
	if err != nil {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("read embedded agents failed: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		agent, err := loadMarkdownAgent("embedded/" + entry.Name())
		if err != nil {
			return pcwrap.EmbeddedAgent{}, err
		}
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
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		agent, err := loadMarkdownAgent("embedded/" + entry.Name())
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })
	return agents, nil
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
	def.ID = strings.TrimSpace(def.ID)
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
