package repo2skill

import (
	"encoding/json"
	"fmt"
	"strings"
)

type DirectProcessor interface {
	ProcessDirect(message string) (string, error)
}

type AgentAnalyzer struct {
	Processor DirectProcessor
}

type agentSkillModel struct {
	Purpose       string      `json:"purpose"`
	WhenToUse     []string    `json:"when_to_use"`
	WhenNotToUse  []string    `json:"when_not_to_use"`
	Install       []string    `json:"install"`
	PublicAPI     []APISymbol `json:"public_api"`
	Recipes       []Recipe    `json:"recipes"`
	BestPractices []string    `json:"best_practices"`
	Gotchas       []string    `json:"gotchas"`
}

func (a AgentAnalyzer) Analyze(p *RepoProfile) (*SkillModel, error) {
	if a.Processor == nil {
		return nil, fmt.Errorf("agent analyzer requires a direct processor")
	}
	payload, err := json.MarshalIndent(compactProfileForAgent(p), "", "  ")
	if err != nil {
		return nil, err
	}
	message := "Analyze this repository profile and return only the repo2skill JSON object described in your instructions.\n\n" + string(payload)
	resp, err := a.Processor.ProcessDirect(message)
	if err != nil {
		return nil, err
	}
	var out agentSkillModel
	if err := json.Unmarshal([]byte(extractJSONObject(resp)), &out); err != nil {
		return nil, fmt.Errorf("parse agent repo2skill JSON: %w", err)
	}
	m := &SkillModel{Profile: p, Purpose: strings.TrimSpace(out.Purpose), WhenToUse: out.WhenToUse, WhenNotToUse: out.WhenNotToUse, Install: out.Install, PublicAPI: out.PublicAPI, Recipes: out.Recipes, BestPractices: out.BestPractices, Gotchas: out.Gotchas}
	if m.Purpose == "" {
		m.Purpose = inferPurpose(p)
	}
	if len(m.Install) == 0 {
		m.Install = p.InstallHints
	}
	if len(m.PublicAPI) == 0 {
		m.PublicAPI = p.PublicAPIs
	}
	if len(m.BestPractices) == 0 {
		fallback, _ := HeuristicAnalyzer{}.Analyze(p)
		m.BestPractices = fallback.BestPractices
	}
	if len(m.Recipes) == 0 {
		fallback, _ := HeuristicAnalyzer{}.Analyze(p)
		m.Recipes = fallback.Recipes
	}
	return m, nil
}

type agentRepoProfile struct {
	Name          string        `json:"name"`
	Source        string        `json:"source"`
	Intent        string        `json:"intent"`
	Languages     []string      `json:"languages,omitempty"`
	PackageFiles  []PackageFile `json:"package_files,omitempty"`
	Readmes       []DocFile     `json:"readmes,omitempty"`
	Docs          []DocFile     `json:"docs,omitempty"`
	Examples      []CodeFile    `json:"examples,omitempty"`
	Tests         []CodeFile    `json:"tests,omitempty"`
	EntryPoints   []EntryPoint  `json:"entrypoints,omitempty"`
	PublicAPIs    []APISymbol   `json:"public_apis,omitempty"`
	InstallHints  []string      `json:"install_hints,omitempty"`
	UsageSnippets []Snippet     `json:"usage_snippets,omitempty"`
}

func compactProfileForAgent(p *RepoProfile) agentRepoProfile {
	if p == nil {
		return agentRepoProfile{}
	}
	return agentRepoProfile{
		Name:          p.Name,
		Source:        p.Source,
		Intent:        p.Intent,
		Languages:     p.Languages,
		PackageFiles:  limitSlice(p.PackageFiles, 20),
		Readmes:       compactDocs(limitSlice(p.Readmes, 6)),
		Docs:          compactDocs(limitSlice(p.Docs, 12)),
		Examples:      compactCode(limitSlice(p.Examples, 10)),
		Tests:         compactCode(limitSlice(p.Tests, 8)),
		EntryPoints:   limitSlice(p.EntryPoints, 30),
		PublicAPIs:    limitSlice(p.PublicAPIs, 80),
		InstallHints:  limitSlice(p.InstallHints, 20),
		UsageSnippets: limitSlice(p.UsageSnippets, 20),
	}
}

func compactDocs(in []DocFile) []DocFile {
	out := append([]DocFile(nil), in...)
	for i := range out {
		out[i].Content = truncate(out[i].Content, 4000)
	}
	return out
}

func compactCode(in []CodeFile) []CodeFile {
	out := append([]CodeFile(nil), in...)
	for i := range out {
		out[i].Content = truncate(out[i].Content, 2500)
	}
	return out
}

func limitSlice[T any](in []T, n int) []T {
	if len(in) <= n {
		return append([]T(nil), in...)
	}
	return append([]T(nil), in[:n]...)
}

func extractJSONObject(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 3 {
			lines = lines[1:]
			if strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
				lines = lines[:len(lines)-1]
			}
			s = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end >= start {
		return s[start : end+1]
	}
	return s
}
