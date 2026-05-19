package repo2skill

import "fmt"

type Analyzer interface {
	Analyze(*RepoProfile) (*SkillModel, error)
}

type HeuristicAnalyzer struct{}

func (HeuristicAnalyzer) Analyze(p *RepoProfile) (*SkillModel, error) {
	m := &SkillModel{Profile: p, PublicAPI: p.PublicAPIs, Install: p.InstallHints}
	m.Purpose = inferPurpose(p)
	m.WhenToUse = []string{"Use this library when a project needs the capability described by its README, package metadata, or examples.", "Prefer public package entrypoints, documented imports, and examples over internal source files."}
	m.WhenNotToUse = []string{"Do not depend on files under internal, private, test, fixture, or build directories unless the upstream docs explicitly expose them."}
	m.BestPractices = []string{"Start from README installation and quick-start snippets.", "Prefer APIs exported from package entrypoints or documented examples.", "Validate generated code against the installed package version and type checker/tests.", "When unsure, inspect upstream docs or examples before inventing usage."}
	m.Gotchas = p.Warnings
	for _, s := range p.UsageSnippets {
		if len(m.Recipes) >= 8 {
			break
		}
		m.Recipes = append(m.Recipes, Recipe{Title: fmt.Sprintf("Use snippet from %s", s.Source), Description: "A documented usage pattern extracted from repository documentation.", Example: s.Code, Evidence: []string{s.Source}})
	}
	if len(m.Recipes) == 0 {
		m.Recipes = append(m.Recipes, Recipe{Title: "Inspect public API", Description: "Use the package metadata and entrypoint exports to identify supported APIs before coding.", Evidence: []string{"package metadata", "entrypoints"}})
	}
	return m, nil
}

func inferPurpose(p *RepoProfile) string {
	for _, pf := range p.PackageFiles {
		if pf.Description != "" {
			return pf.Description
		}
	}
	for _, r := range p.Readmes {
		if r.Summary != "" {
			return r.Summary
		}
		if r.Title != "" {
			return r.Title
		}
	}
	return "A software repository/library. Use collected docs, examples, and public entrypoints as source of truth."
}
