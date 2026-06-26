package agents

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	"github.com/sjzsdu/tt/internal/util"
	"gopkg.in/yaml.v3"
)

const (
	AgentOptimizerID        = "agent-optimizer"
	TranslateMasterID       = "translate-master"
	CoderID                 = "coder"
	ReporterID              = "reporter"
	legacyReporterID        = "nvwa-agent"
	PlannerID               = "planner"
	TesterID                = "tester"
	StockBeginnerID         = "stock-growth-investor"
	StockOldHandID          = "stock-risk-investor"
	StockMacroStrategistID  = "stock-macro-strategist"
	StockQuantTechnicianID  = "stock-quant-technician"
	StockNewsEventAnalystID = "stock-news-event-analyst"
	StockSectorSpecialistID = "stock-sector-specialist"
	StockDiscussionHostID   = "stock-discussion-host"
	Repo2SkillID            = "repo2skill"
	CodeResearchID          = "code-research"
	FormulaWriterID         = "formula-writer"
	DocsAnalystID           = "docs-analyst"
	BeadManagerID           = "bead-manager"
)

//go:embed embedded
var embeddedFS embed.FS

type definition struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Soul        string   `yaml:"soul"`
	Skills      []string `yaml:"skills"`
	Tools       []string `yaml:"tools"`
	NoHistory   bool     `yaml:"no_history"`
}

func TranslateMaster() (pcwrap.EmbeddedAgent, error) {
	return Get(TranslateMasterID)
}

func Coder() (pcwrap.EmbeddedAgent, error) {
	return Get(CoderID)
}

func Reporter() (pcwrap.EmbeddedAgent, error) {
	return Get(ReporterID)
}

func Planner() (pcwrap.EmbeddedAgent, error) {
	return Get(PlannerID)
}

func Tester() (pcwrap.EmbeddedAgent, error) {
	return Get(TesterID)
}

func Repo2Skill() (pcwrap.EmbeddedAgent, error) {
	return Get(Repo2SkillID)
}

func FormulaWriter() (pcwrap.EmbeddedAgent, error) {
	return Get(FormulaWriterID)
}

func DocsAnalyst() (pcwrap.EmbeddedAgent, error) {
	return Get(DocsAnalystID)
}

func AgentOptimizer() (pcwrap.EmbeddedAgent, error) {
	return Get(AgentOptimizerID)
}

func Core() ([]pcwrap.EmbeddedAgent, error) {
	return getMany(CoderID, CodeResearchID, PlannerID, TesterID)
}

func All() ([]pcwrap.EmbeddedAgent, error) {
	return List()
}

func Embedded() ([]pcwrap.EmbeddedAgent, error) {
	paths, err := embeddedAgentPaths()
	if err != nil {
		return nil, err
	}
	agents := make([]pcwrap.EmbeddedAgent, 0, len(paths))
	indexByID := map[string]int{}
	for _, path := range paths {
		agent, err := loadMarkdownAgent(path)
		if err != nil {
			return nil, err
		}
		agents = upsertAgent(agents, indexByID, agent)
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })
	return agents, nil
}

func StockDiscussion() ([]pcwrap.EmbeddedAgent, error) {
	return getMany(
		StockBeginnerID,
		StockOldHandID,
		StockDiscussionHostID,
		StockMacroStrategistID,
		StockQuantTechnicianID,
		StockNewsEventAnalystID,
		StockSectorSpecialistID,
	)
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

func FilePathForID(id string) (string, error) {
	id = canonicalAgentID(strings.TrimSpace(id))
	if id == "" {
		return "", fmt.Errorf("agent id is required")
	}
	searchRoot := strings.TrimSpace(os.Getenv("TT_AGENT_DIR"))
	if searchRoot == "" {
		searchRoot = ".tt/agents"
	}
	err := filepath.WalkDir(searchRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			return nil
		}
		agent, err := LoadFromFile(path)
		if err != nil {
			return err
		}
		if agent.ID == id {
			return foundAgentPath(path)
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("agent %q is embedded or has no local markdown file", id)
		}
		if found, ok := err.(foundAgentPath); ok {
			return string(found), nil
		}
		return "", fmt.Errorf("read embedded agents from %s failed: %w", searchRoot, err)
	}
	return "", fmt.Errorf("agent %q is embedded or has no local markdown file", id)
}

func List() ([]pcwrap.EmbeddedAgent, error) {
	paths, err := embeddedAgentPaths()
	if err != nil {
		return nil, err
	}
	agents := make([]pcwrap.EmbeddedAgent, 0, len(paths))
	indexByID := map[string]int{}
	for _, path := range paths {
		agent, err := loadMarkdownAgent(path)
		if err != nil {
			return nil, err
		}
		agents = upsertAgent(agents, indexByID, agent)
	}

	fsAgents, err := loadFilesystemAgents()
	if err != nil {
		return nil, err
	}
	for _, agent := range fsAgents {
		agents = upsertAgent(agents, indexByID, agent)
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })
	return agents, nil
}

func embeddedAgentPaths() ([]string, error) {
	paths := make([]string, 0)
	err := fs.WalkDir(embeddedFS, "embedded", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read embedded agents failed: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

func upsertAgent(agents []pcwrap.EmbeddedAgent, indexByID map[string]int, agent pcwrap.EmbeddedAgent) []pcwrap.EmbeddedAgent {
	if idx, exists := indexByID[agent.ID]; exists {
		agents[idx] = agent
		return agents
	}
	indexByID[agent.ID] = len(agents)
	return append(agents, agent)
}

func loadFilesystemAgents() ([]pcwrap.EmbeddedAgent, error) {
	searchRoot := strings.TrimSpace(os.Getenv("TT_AGENT_DIR"))
	if searchRoot == "" {
		searchRoot = ".tt/agents"
	}
	searchRoots := []string{searchRoot}
	collected := make([]pcwrap.EmbeddedAgent, 0)
	for _, root := range searchRoots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
				return nil
			}
			agent, err := LoadFromFile(path)
			if err != nil {
				return err
			}
			collected = append(collected, agent)
			return nil
		})
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read embedded agents from %s failed: %w", root, err)
		}
	}
	return collected, nil
}

type foundAgentPath string

func (p foundAgentPath) Error() string { return string(p) }

func getMany(ids ...string) ([]pcwrap.EmbeddedAgent, error) {
	agents := make([]pcwrap.EmbeddedAgent, 0, len(ids))
	for _, id := range ids {
		agent, err := Get(id)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	return agents, nil
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
		ID:          def.ID,
		Name:        def.Name,
		Description: strings.TrimSpace(def.Description),
		Prompt:      strings.TrimSpace(body),
		Soul:        strings.TrimSpace(def.Soul),
		Skills:      compactStrings(def.Skills),
		Tools:       compactStrings(def.Tools),
		NoHistory:   def.NoHistory,
	}, nil
}

func LoadFromFile(path string) (pcwrap.EmbeddedAgent, error) {
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
		ID:          def.ID,
		Name:        def.Name,
		Description: strings.TrimSpace(def.Description),
		Prompt:      strings.TrimSpace(body),
		Soul:        strings.TrimSpace(def.Soul),
		Skills:      compactStrings(def.Skills),
		Tools:       compactStrings(def.Tools),
		NoHistory:   def.NoHistory,
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
	return util.CompactStrings(values)
}
