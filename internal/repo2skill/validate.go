package repo2skill

import (
	"fmt"
	"strings"
)

// NormalizeSkillModel turns analyzer output into a safer, renderable model.
// It fills missing sections, removes empty/duplicate values, and records notes
// when agent output cannot be traced back to collected repository evidence.
func NormalizeSkillModel(m *SkillModel) *SkillModel {
	if m == nil {
		m = &SkillModel{}
	}
	p := m.Profile
	if p == nil {
		p = &RepoProfile{Name: "repo", Intent: "use-library"}
		m.Profile = p
	}
	fallback, _ := HeuristicAnalyzer{}.Analyze(p)
	if strings.TrimSpace(m.Purpose) == "" {
		m.Purpose = fallback.Purpose
	}
	m.Install = cleanInstallHints(m.Install)
	if len(m.Install) == 0 {
		m.Install = cleanInstallHints(p.InstallHints)
	}
	m.PublicAPI = normalizeAPIs(m.PublicAPI, p)
	if len(m.PublicAPI) == 0 {
		m.PublicAPI = normalizeAPIs(p.PublicAPIs, p)
	}
	m.Recipes = normalizeRecipes(m.Recipes)
	if len(m.Recipes) == 0 {
		m.Recipes = fallback.Recipes
	}
	m.BestPractices = cleanStrings(m.BestPractices)
	if len(m.BestPractices) == 0 {
		m.BestPractices = fallback.BestPractices
	}
	m.WhenToUse = cleanStrings(m.WhenToUse)
	m.WhenNotToUse = cleanStrings(m.WhenNotToUse)
	m.Gotchas = cleanStrings(append(m.Gotchas, p.Warnings...))
	return m
}

func cleanInstallHints(values []string) []string {
	out := cleanStrings(values)
	kept := out[:0]
	for _, v := range out {
		parts := strings.Fields(v)
		if len(parts) < 3 && (strings.HasPrefix(v, "npm install") || strings.HasPrefix(v, "go get") || strings.HasPrefix(v, "pip install") || strings.HasPrefix(v, "cargo add")) {
			continue
		}
		kept = append(kept, v)
	}
	return kept
}

func normalizeAPIs(values []APISymbol, p *RepoProfile) []APISymbol {
	seen := map[string]bool{}
	known := knownAPIEvidence(p)
	var out []APISymbol
	for _, api := range values {
		api.Name = strings.TrimSpace(api.Name)
		if api.Name == "" {
			continue
		}
		api.Kind = strings.TrimSpace(api.Kind)
		if api.Kind == "" {
			api.Kind = "symbol"
		}
		api.Source = strings.TrimSpace(api.Source)
		api.Evidence = strings.TrimSpace(api.Evidence)
		key := strings.ToLower(api.Name + "\x00" + api.Source)
		if seen[key] {
			continue
		}
		seen[key] = true
		if known[api.Name] && api.Evidence == "" {
			api.Evidence = "detected in repository public API evidence"
		}
		if !known[api.Name] && p != nil {
			if api.Evidence == "" {
				api.Evidence = "agent-suggested; not matched by deterministic public API extraction"
			} else if !strings.Contains(strings.ToLower(api.Evidence), "agent-suggested") {
				api.Evidence += "; agent-suggested, not matched by deterministic public API extraction"
			}
			p.Warnings = append(p.Warnings, fmt.Sprintf("API `%s` was suggested by analysis but not matched by deterministic public API extraction; verify upstream docs before use.", api.Name))
		}
		out = append(out, api)
	}
	return out
}

func knownAPIEvidence(p *RepoProfile) map[string]bool {
	known := map[string]bool{}
	if p == nil {
		return known
	}
	for _, api := range p.PublicAPIs {
		if api.Name != "" {
			known[api.Name] = true
		}
	}
	for _, pf := range p.PackageFiles {
		for _, exp := range pf.Exports {
			if exp != "" {
				known[exp] = true
			}
		}
	}
	return known
}

func normalizeRecipes(values []Recipe) []Recipe {
	seen := map[string]bool{}
	var out []Recipe
	for _, r := range values {
		r.Title = strings.TrimSpace(r.Title)
		r.Description = strings.TrimSpace(r.Description)
		r.Example = strings.TrimSpace(r.Example)
		r.Evidence = cleanStrings(r.Evidence)
		if r.Title == "" || r.Description == "" {
			continue
		}
		key := strings.ToLower(r.Title)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}

func cleanStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, v)
	}
	return out
}
