package formula

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

type Parser struct {
	searchPaths    []string
	cache          map[string]*Formula
	resolvingSet   map[string]bool
	resolvingChain []string
}

func NewParser(searchPaths ...string) *Parser {
	paths := searchPaths
	if len(paths) == 0 {
		paths = defaultSearchPaths()
	}
	return &Parser{
		searchPaths:    paths,
		cache:          make(map[string]*Formula),
		resolvingSet:   make(map[string]bool),
		resolvingChain: nil,
	}
}

func defaultSearchPaths() []string {
	var paths []string
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(cwd, ".tt", "formulas"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".tt", "formulas"))
	}
	return paths
}

func (p *Parser) ParseFile(path string) (*Formula, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	if cached, ok := p.cache[absPath]; ok {
		return cached, nil
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var formula *Formula
	if IsTOMLFilename(path) {
		formula, err = p.ParseTOML(data)
	} else {
		formula, err = p.Parse(data)
	}
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	formula.Source = absPath
	SetSourceInfo(formula)

	formulaDir := filepath.Dir(absPath)
	ResolveDescriptionFiles(formula.Steps, formulaDir)
	ResolveDescriptionFiles(formula.Template, formulaDir)

	p.cache[absPath] = formula
	p.cache[formula.Formula] = formula

	return formula, nil
}

func (p *Parser) Parse(data []byte) (*Formula, error) {
	var formula Formula
	if err := json.Unmarshal(data, &formula); err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}

	if formula.Version == 0 {
		formula.Version = 1
	}
	if formula.Type == "" {
		formula.Type = TypeWorkflow
	}

	return &formula, nil
}

func (p *Parser) ParseTOML(data []byte) (*Formula, error) {
	var formula Formula
	if err := toml.Unmarshal(data, &formula); err != nil {
		return nil, fmt.Errorf("toml: %w", err)
	}

	if formula.Version == 0 {
		formula.Version = 1
	}
	if formula.Type == "" {
		formula.Type = TypeWorkflow
	}

	return &formula, nil
}

func (p *Parser) Resolve(formula *Formula) (*Formula, error) {
	if p.resolvingSet[formula.Formula] {
		chain := append(slices.Clone(p.resolvingChain), formula.Formula)
		return nil, fmt.Errorf("circular extends detected: %s", strings.Join(chain, " -> "))
	}
	p.resolvingSet[formula.Formula] = true
	p.resolvingChain = append(p.resolvingChain, formula.Formula)
	defer func() {
		delete(p.resolvingSet, formula.Formula)
		p.resolvingChain = p.resolvingChain[:len(p.resolvingChain)-1]
	}()

	if len(formula.Extends) == 0 {
		if err := formula.Validate(); err != nil {
			return nil, err
		}
		return formula, nil
	}

	merged := &Formula{
		Formula:     formula.Formula,
		Description: formula.Description,
		Version:     formula.Version,
		Contract:    formula.Contract,
		Type:        formula.Type,
		Source:      formula.Source,
		Phase:       formula.Phase,
		Pour:        formula.Pour,
		Vars:        make(map[string]*VarDef),
		Steps:       nil,
		Template:    nil,
		Compose:     nil,
	}

	for _, parentName := range formula.Extends {
		parent, err := p.loadFormula(parentName)
		if err != nil {
			return nil, fmt.Errorf("extends %s: %w", parentName, err)
		}

		parent, err = p.Resolve(parent)
		if err != nil {
			return nil, fmt.Errorf("resolve parent %s: %w", parentName, err)
		}

		if merged.Contract == "" {
			merged.Contract = parent.Contract
		}
		if merged.Phase == "" {
			merged.Phase = parent.Phase
		}
		if !merged.Pour {
			merged.Pour = parent.Pour
		}

		for name, varDef := range parent.Vars {
			if _, exists := merged.Vars[name]; !exists {
				merged.Vars[name] = varDef
			}
		}

		merged.Steps = append(merged.Steps, parent.Steps...)
		merged.Template = append(merged.Template, parent.Template...)
		merged.Compose = mergeComposeRules(merged.Compose, parent.Compose)
	}

	for name, varDef := range formula.Vars {
		merged.Vars[name] = varDef
	}

	merged.Steps = mergeSteps(merged.Steps, formula.Steps)
	merged.Template = mergeSteps(merged.Template, formula.Template)
	merged.Compose = mergeComposeRules(merged.Compose, formula.Compose)

	if formula.Description != "" {
		merged.Description = formula.Description
	}

	if err := merged.Validate(); err != nil {
		return nil, err
	}

	return merged, nil
}

func (p *Parser) loadFormula(name string) (*Formula, error) {
	if cached, ok := p.cache[name]; ok {
		return cached, nil
	}

	extensions := []string{CanonicalTOMLExt, LegacyTOMLExt, FormulaExtJSON}
	for _, dir := range p.searchPaths {
		for _, ext := range extensions {
			path := filepath.Join(dir, name+ext)
			if _, err := os.Stat(path); err == nil {
				return p.ParseFile(path)
			}
		}
	}

	if f, err := p.ParseBuiltin(name); err == nil {
		return f, nil
	} else if !strings.Contains(err.Error(), "not found") {
		return nil, err
	}

	return nil, fmt.Errorf("formula %q not found in search paths", name)
}

func (p *Parser) LoadByName(name string) (*Formula, error) {
	return p.loadFormula(name)
}

func mergeSteps(parent, child []*Step) []*Step {
	parentIdx := make(map[string]int, len(parent))
	for i, s := range parent {
		parentIdx[s.ID] = i
	}

	result := make([]*Step, len(parent))
	copy(result, parent)

	for _, cs := range child {
		if idx, exists := parentIdx[cs.ID]; exists {
			result[idx] = cs
		} else {
			result = append(result, cs)
		}
	}

	return result
}

func mergeComposeRules(base, overlay *ComposeRules) *ComposeRules {
	if overlay == nil {
		return base
	}
	if base == nil {
		return overlay
	}

	result := &ComposeRules{
		BondPoints: append([]*BondPoint{}, base.BondPoints...),
		Hooks:      append([]*Hook{}, base.Hooks...),
		Expand:     append([]*ExpandRule{}, base.Expand...),
		Map:        append([]*MapRule{}, base.Map...),
	}

	existingBP := make(map[string]int)
	for i, bp := range result.BondPoints {
		existingBP[bp.ID] = i
	}
	for _, bp := range overlay.BondPoints {
		if idx, exists := existingBP[bp.ID]; exists {
			result.BondPoints[idx] = bp
		} else {
			result.BondPoints = append(result.BondPoints, bp)
		}
	}

	result.Hooks = append(result.Hooks, overlay.Hooks...)
	result.Expand = append(result.Expand, overlay.Expand...)
	result.Map = append(result.Map, overlay.Map...)

	return result
}
