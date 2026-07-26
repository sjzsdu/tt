package team

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

const DefinitionFilename = "team.toml"

type Definition struct {
	Team           string             `json:"team" toml:"team"`
	Title          string             `json:"title,omitempty" toml:"title,omitempty"`
	Description    string             `json:"description,omitempty" toml:"description,omitempty"`
	Version        int                `json:"version" toml:"version"`
	Language       string             `json:"language,omitempty" toml:"language,omitempty"`
	DefaultModel   string             `json:"default_model,omitempty" toml:"default_model,omitempty"`
	Coordination   CoordinationConfig `json:"coordination" toml:"coordination"`
	Limits         LimitsConfig       `json:"limits" toml:"limits"`
	Memory         MemoryConfig       `json:"memory" toml:"memory"`
	Verification   VerificationConfig `json:"verification,omitempty" toml:"verification,omitempty"`
	Agents         []Agent            `json:"agents" toml:"agents"`
	Source         string             `json:"source,omitempty" toml:"-"`
	DefinitionHash string             `json:"definition_hash,omitempty" toml:"-"`
}

type CoordinationConfig struct {
	Facilitator       string `json:"facilitator,omitempty" toml:"facilitator,omitempty"`
	Finalizer         string `json:"finalizer,omitempty" toml:"finalizer,omitempty"`
	InitialHandoff    string `json:"initial_handoff,omitempty" toml:"initial_handoff,omitempty"`
	DeliveryOwner     string `json:"delivery_owner,omitempty" toml:"delivery_owner,omitempty"`
	MaxHandoffTargets int    `json:"max_handoff_targets,omitempty" toml:"max_handoff_targets,omitempty"`
	ReviewWaves       int    `json:"review_waves" toml:"review_waves"`
	MaxConcurrency    int    `json:"max_concurrency" toml:"max_concurrency"`
}

type LimitsConfig struct {
	MaxAgentTurns          int    `json:"max_agent_turns" toml:"max_agent_turns"`
	MaxReviewTurnsPerAgent int    `json:"max_review_turns_per_agent,omitempty" toml:"max_review_turns_per_agent,omitempty"`
	MaxResponseChars       int    `json:"max_response_chars,omitempty" toml:"max_response_chars,omitempty"`
	MaxWallTime            string `json:"max_wall_time,omitempty" toml:"max_wall_time,omitempty"`
}

type MemoryConfig struct {
	Enabled    *bool  `json:"enabled,omitempty" toml:"enabled,omitempty"`
	Maintainer string `json:"maintainer,omitempty" toml:"maintainer,omitempty"`
	Path       string `json:"path,omitempty" toml:"path,omitempty"`
	MaxChars   int    `json:"max_chars" toml:"max_chars"`
}

type VerificationConfig struct {
	Enabled     bool   `json:"enabled,omitempty" toml:"enabled,omitempty"`
	Verifier    string `json:"verifier,omitempty" toml:"verifier,omitempty"`
	MaxCommands int    `json:"max_commands,omitempty" toml:"max_commands,omitempty"`
	Timeout     string `json:"timeout,omitempty" toml:"timeout,omitempty"`
}

type Agent struct {
	ID          string               `json:"id" toml:"id"`
	Role        string               `json:"role,omitempty" toml:"role,omitempty"`
	Agent       string               `json:"agent,omitempty" toml:"agent,omitempty"`
	Model       string               `json:"model,omitempty" toml:"model,omitempty"`
	Prompt      string               `json:"prompt,omitempty" toml:"prompt,omitempty"`
	CanFinalize bool                 `json:"can_finalize,omitempty" toml:"can_finalize,omitempty"`
	External    *ExternalAgentConfig `json:"external,omitempty" toml:"external,omitempty"`
}

// ExternalAgentConfig routes a team member through a Formula-compatible
// external agent CLI instead of the embedded Picoclaw runtime.
type ExternalAgentConfig struct {
	Driver    string   `json:"driver" toml:"driver"`
	Provider  string   `json:"provider,omitempty" toml:"provider,omitempty"`
	Mode      string   `json:"mode,omitempty" toml:"mode,omitempty"`
	Resume    string   `json:"resume,omitempty" toml:"resume,omitempty"`
	Cwd       string   `json:"cwd,omitempty" toml:"cwd,omitempty"`
	Timeout   string   `json:"timeout,omitempty" toml:"timeout,omitempty"`
	ExtraArgs []string `json:"extra_args,omitempty" toml:"extra_args,omitempty"`
}

func (d *Definition) Normalize() {
	if d == nil {
		return
	}
	d.Team = strings.TrimSpace(d.Team)
	d.Title = strings.TrimSpace(d.Title)
	d.Description = strings.TrimSpace(d.Description)
	d.Language = strings.TrimSpace(d.Language)
	d.DefaultModel = strings.TrimSpace(d.DefaultModel)
	d.Coordination.Facilitator = strings.TrimSpace(d.Coordination.Facilitator)
	d.Coordination.Finalizer = strings.TrimSpace(d.Coordination.Finalizer)
	d.Coordination.InitialHandoff = strings.TrimSpace(d.Coordination.InitialHandoff)
	d.Coordination.DeliveryOwner = strings.TrimSpace(d.Coordination.DeliveryOwner)
	if d.Version == 0 {
		d.Version = 1
	}
	if d.Coordination.ReviewWaves < 0 {
		d.Coordination.ReviewWaves = 0
	}
	if d.Coordination.MaxConcurrency <= 0 {
		d.Coordination.MaxConcurrency = 4
	}
	if d.Coordination.MaxHandoffTargets < 0 {
		d.Coordination.MaxHandoffTargets = 0
	}
	if d.Limits.MaxAgentTurns <= 0 {
		d.Limits.MaxAgentTurns = 24
	}
	if d.Limits.MaxResponseChars < 0 {
		d.Limits.MaxResponseChars = 0
	}
	if d.Limits.MaxReviewTurnsPerAgent <= 0 {
		d.Limits.MaxReviewTurnsPerAgent = 4
	}
	if strings.TrimSpace(d.Limits.MaxWallTime) == "" {
		d.Limits.MaxWallTime = "15m"
	}
	if d.Memory.MaxChars <= 0 {
		d.Memory.MaxChars = 20000
	}
	if d.Verification.Enabled {
		if d.Verification.MaxCommands <= 0 {
			d.Verification.MaxCommands = 8
		}
		if strings.TrimSpace(d.Verification.Timeout) == "" {
			d.Verification.Timeout = "10m"
		}
	}
	for i := range d.Agents {
		d.Agents[i].ID = strings.TrimSpace(d.Agents[i].ID)
		d.Agents[i].Role = strings.TrimSpace(d.Agents[i].Role)
		d.Agents[i].Agent = strings.TrimSpace(d.Agents[i].Agent)
		d.Agents[i].Model = strings.TrimSpace(d.Agents[i].Model)
		d.Agents[i].Prompt = strings.TrimSpace(d.Agents[i].Prompt)
		if external := d.Agents[i].External; external != nil {
			external.Driver = strings.ToLower(strings.TrimSpace(external.Driver))
			external.Provider = strings.TrimSpace(external.Provider)
			external.Mode = strings.TrimSpace(external.Mode)
			external.Resume = strings.TrimSpace(external.Resume)
			external.Cwd = strings.TrimSpace(external.Cwd)
			external.Timeout = strings.TrimSpace(external.Timeout)
		}
	}
	if strings.TrimSpace(d.Coordination.Facilitator) == "" && len(d.Agents) > 0 {
		d.Coordination.Facilitator = d.Agents[0].ID
	}
	if strings.TrimSpace(d.Coordination.Finalizer) == "" {
		d.Coordination.Finalizer = d.Coordination.Facilitator
	}
	if strings.TrimSpace(d.Memory.Maintainer) == "" {
		d.Memory.Maintainer = d.Coordination.Finalizer
	}
}

func (d Definition) MemoryEnabled() bool {
	return d.Memory.Enabled == nil || *d.Memory.Enabled
}

func (d Definition) AgentByID(id string) (Agent, bool) {
	for _, member := range d.Agents {
		if strings.EqualFold(member.ID, strings.TrimSpace(id)) {
			return member, true
		}
	}
	return Agent{}, false
}

func (d *Definition) Validate() error {
	if d == nil {
		return fmt.Errorf("team definition is required")
	}
	d.Normalize()
	if d.Team == "" {
		return fmt.Errorf("team name is required")
	}
	if d.Version < 1 {
		return fmt.Errorf("team version must be greater than zero")
	}
	if len(d.Agents) < 2 {
		return fmt.Errorf("team %q requires at least two agents", d.Team)
	}
	seen := map[string]bool{}
	for i, member := range d.Agents {
		if member.ID == "" {
			return fmt.Errorf("agents[%d].id is required", i)
		}
		key := strings.ToLower(member.ID)
		if seen[key] {
			return fmt.Errorf("duplicate team agent id %q", member.ID)
		}
		seen[key] = true
		if member.External != nil {
			if member.Agent != "" {
				return fmt.Errorf("agents[%d] %q cannot configure both agent and external", i, member.ID)
			}
			if member.External.Driver == "" {
				return fmt.Errorf("agents[%d].external.driver is required", i)
			}
			if !steps.SupportedExternalAgentDrivers[member.External.Driver] {
				return fmt.Errorf("agents[%d].external.driver %q is not supported", i, member.External.Driver)
			}
			if member.External.Timeout != "" {
				timeout, err := time.ParseDuration(member.External.Timeout)
				if err != nil || timeout <= 0 {
					return fmt.Errorf("agents[%d].external.timeout must be a positive duration", i)
				}
			}
		}
	}
	for label, id := range map[string]string{
		"coordination.facilitator": d.Coordination.Facilitator,
		"coordination.finalizer":   d.Coordination.Finalizer,
		"memory.maintainer":        d.Memory.Maintainer,
	} {
		if _, ok := d.AgentByID(id); !ok {
			return fmt.Errorf("%s references unknown agent %q", label, id)
		}
	}
	if d.Verification.Enabled {
		if _, ok := d.AgentByID(d.Verification.Verifier); !ok {
			return fmt.Errorf("verification.verifier references unknown agent %q", d.Verification.Verifier)
		}
		if d.Verification.MaxCommands < 1 {
			return fmt.Errorf("verification.max_commands must be greater than zero")
		}
		timeout, err := time.ParseDuration(d.Verification.Timeout)
		if err != nil || timeout <= 0 {
			return fmt.Errorf("verification.timeout must be a positive duration")
		}
	}
	if id := strings.TrimSpace(d.Coordination.InitialHandoff); id != "" {
		if _, ok := d.AgentByID(id); !ok {
			return fmt.Errorf("coordination.initial_handoff references unknown agent %q", id)
		}
	}
	if id := strings.TrimSpace(d.Coordination.DeliveryOwner); id != "" {
		if _, ok := d.AgentByID(id); !ok {
			return fmt.Errorf("coordination.delivery_owner references unknown agent %q", id)
		}
	}
	if d.Coordination.MaxConcurrency < 1 {
		return fmt.Errorf("coordination.max_concurrency must be greater than zero")
	}
	expectedTurns := len(d.Agents) + d.Coordination.ReviewWaves*(len(d.Agents)-1) + 1
	if d.Limits.MaxAgentTurns < expectedTurns {
		return fmt.Errorf("limits.max_agent_turns must be at least %d for the configured initial, review, and finalization waves", expectedTurns)
	}
	if d.Memory.MaxChars < 1000 {
		return fmt.Errorf("memory.max_chars must be at least 1000")
	}
	return nil
}

func Parse(data []byte) (*Definition, error) {
	var definition Definition
	if err := toml.Unmarshal(data, &definition); err != nil {
		return nil, fmt.Errorf("parse team TOML: %w", err)
	}
	definition.Normalize()
	if err := definition.Validate(); err != nil {
		return nil, err
	}
	hash, err := DefinitionHash(&definition)
	if err != nil {
		return nil, err
	}
	definition.DefinitionHash = hash
	return &definition, nil
}

func ParseFile(path string) (*Definition, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve team definition %q: %w", path, err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read team definition %q: %w", path, err)
	}
	definition, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse team definition %q: %w", path, err)
	}
	definition.Source = abs
	return definition, nil
}

func DefinitionHash(definition *Definition) (string, error) {
	if definition == nil {
		return "", fmt.Errorf("team definition is required")
	}
	copy := *definition
	copy.Source = ""
	copy.DefinitionHash = ""
	data, err := json.Marshal(copy)
	if err != nil {
		return "", fmt.Errorf("marshal team definition: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func DefaultSearchPaths(workspace string) []string {
	var paths []string
	if strings.TrimSpace(workspace) != "" {
		paths = append(paths, filepath.Join(workspace, ".tt", "teams"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".tt", "teams"))
	}
	return uniquePaths(paths)
}

func Load(name string, searchPaths ...string) (*Definition, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("team name or definition path is required")
	}
	if looksLikeDefinitionPath(name) {
		if _, err := os.Stat(name); err == nil {
			return ParseFile(name)
		}
	}
	for _, dir := range searchPaths {
		for _, candidate := range []string{
			filepath.Join(dir, name+".toml"),
			filepath.Join(dir, name, DefinitionFilename),
		} {
			if _, err := os.Stat(candidate); err == nil {
				return ParseFile(candidate)
			}
		}
	}
	if data, ok, err := BuiltinTeamContent(name); err == nil && ok {
		definition, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("parse builtin team %q: %w", name, err)
		}
		definition.Source = "builtin:" + name
		return definition, nil
	}
	return nil, fmt.Errorf("team %q not found in search paths: %s", name, strings.Join(searchPaths, ", "))
}

type DefinitionRecord struct {
	Name   string
	Path   string
	Title  string
	Agents int
}

func List(searchPaths ...string) ([]DefinitionRecord, error) {
	seen := map[string]bool{}
	var records []DefinitionRecord
	for _, dir := range searchPaths {
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("list team definitions in %q: %w", dir, err)
		}
		for _, entry := range entries {
			candidate := ""
			if entry.IsDir() {
				candidate = filepath.Join(dir, entry.Name(), DefinitionFilename)
			} else if strings.EqualFold(filepath.Ext(entry.Name()), ".toml") {
				candidate = filepath.Join(dir, entry.Name())
			}
			if candidate == "" {
				continue
			}
			definition, err := ParseFile(candidate)
			if err != nil {
				continue
			}
			key := strings.ToLower(definition.Team)
			if seen[key] {
				continue
			}
			seen[key] = true
			records = append(records, DefinitionRecord{
				Name:   definition.Team,
				Path:   definition.Source,
				Title:  definition.Title,
				Agents: len(definition.Agents),
			})
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Name < records[j].Name })
	return records, nil
}

func looksLikeDefinitionPath(value string) bool {
	return filepath.IsAbs(value) ||
		strings.Contains(value, string(filepath.Separator)) ||
		strings.EqualFold(filepath.Ext(value), ".toml")
}

func uniquePaths(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "." || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}
