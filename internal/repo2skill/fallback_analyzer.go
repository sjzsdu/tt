package repo2skill

import (
	"fmt"
	"io"
)

type FallbackAnalyzer struct {
	Primary  Analyzer
	Fallback Analyzer
	Log      io.Writer
}

func (a FallbackAnalyzer) Analyze(p *RepoProfile) (*SkillModel, error) {
	if a.Primary != nil {
		m, err := a.Primary.Analyze(p)
		if err == nil {
			return m, nil
		}
		if a.Log != nil {
			fmt.Fprintf(a.Log, "repo2skill: agent analysis failed, falling back to heuristic analyzer: %v\n", err)
		}
	}
	fallback := a.Fallback
	if fallback == nil {
		fallback = HeuristicAnalyzer{}
	}
	return fallback.Analyze(p)
}
