package formula

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	spec "github.com/sjzsdu/tt/internal/formula/spec"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

type Parser struct {
	searchPaths    []string
	cache          map[string]*spec.Formula
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
		cache:          make(map[string]*spec.Formula),
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

func (p *Parser) ParseFile(path string) (*spec.Formula, error) {
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

	var formula *spec.Formula
	if IsTOMLFilename(path) {
		formula, err = p.ParseTOML(data)
	} else {
		formula, err = p.Parse(data)
	}
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	formula.Source = absPath
	spec.SetSourceInfo(formula)
	if err := resolveFormulaScriptCommands(formula); err != nil {
		return nil, err
	}

	formulaDir := filepath.Dir(absPath)
	spec.ResolveDescriptionFiles(formula.Steps, formulaDir)
	spec.ResolveDescriptionFiles(formula.Template, formulaDir)

	p.cache[absPath] = formula
	p.cache[formula.Formula] = formula

	return formula, nil
}

func (p *Parser) Parse(data []byte) (*spec.Formula, error) {
	var formula spec.Formula
	if err := json.Unmarshal(data, &formula); err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}

	if formula.Version == 0 {
		formula.Version = 1
	}
	if formula.Type == "" {
		formula.Type = spec.TypeWorkflow
	}
	normalizeFormulaWorkspace(&formula)

	return &formula, nil
}

func (p *Parser) ParseTOML(data []byte) (*spec.Formula, error) {
	var formula spec.Formula
	if err := toml.Unmarshal(data, &formula); err != nil {
		return nil, fmt.Errorf("toml: %w", err)
	}

	if formula.Version == 0 {
		formula.Version = 1
	}
	if formula.Type == "" {
		formula.Type = spec.TypeWorkflow
	}
	normalizeFormulaWorkspace(&formula)

	return &formula, nil
}

func resolveFormulaScriptCommands(formula *spec.Formula) error {
	if formula == nil {
		return nil
	}
	return resolveStepScriptCommands(formula.Steps, formula.Source)
}

func resolveStepScriptCommands(items []*spec.Step, source string) error {
	for _, step := range items {
		if step == nil {
			continue
		}
		if step.Script != nil && len(step.Script.Command) == 0 && strings.TrimSpace(step.Script.Script) != "" {
			command, err := commandForFormulaScript(source, step.Script.Script)
			if err != nil {
				return fmt.Errorf("step %q script %q: %w", step.ID, step.Script.Script, err)
			}
			step.Script.Command = command
		}
		if err := resolveStepScriptCommands(step.Children, source); err != nil {
			return err
		}
		if step.Loop != nil {
			if err := resolveStepScriptCommands(step.Loop.Body, source); err != nil {
				return err
			}
		}
	}
	return nil
}

func commandForFormulaScript(source, scriptPath string) ([]string, error) {
	if strings.HasPrefix(source, "builtin:") {
		base := strings.TrimPrefix(source, "builtin:")
		fullPath := scriptPath
		if !filepath.IsAbs(scriptPath) {
			fullPath = filepath.ToSlash(filepath.Join(filepath.Dir(base), scriptPath))
		}
		data, err := builtinFormulaFS.ReadFile(fullPath)
		if err != nil && strings.HasPrefix(scriptPath, "./scripts/") {
			fallback := filepath.ToSlash(filepath.Join(filepath.Dir(filepath.Dir(base)), strings.TrimPrefix(scriptPath, "./")))
			if fallbackData, fallbackErr := builtinFormulaFS.ReadFile(fallback); fallbackErr == nil {
				fullPath = fallback
				data = fallbackData
				err = nil
			}
		}
		if err != nil {
			return nil, fmt.Errorf("read builtin script: %w", err)
		}
		return steps.CommandForInlineScript(fullPath, string(data)), nil
	}
	baseDir := ""
	if strings.TrimSpace(source) != "" {
		baseDir = filepath.Dir(source)
	}
	return steps.LoadExternalScript(scriptPath, baseDir)
}

func (p *Parser) Resolve(formula *spec.Formula) (*spec.Formula, error) {
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

	merged := &spec.Formula{
		Formula:     formula.Formula,
		Description: formula.Description,
		Version:     formula.Version,
		Contract:    formula.Contract,
		Type:        formula.Type,
		Source:      formula.Source,
		Phase:       formula.Phase,
		Pour:        formula.Pour,
		Worktree:    formula.Worktree,
		Workspace:   formula.Workspace,
		Vars:        make(map[string]*spec.VarDef),
		Outputs:     make(map[string]*spec.OutputDef),
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
		if merged.Workspace == nil {
			merged.Workspace = parent.Workspace
		}
		merged.Preflight = mergePreflightSpec(merged.Preflight, parent.Preflight)

		for name, varDef := range parent.Vars {
			if _, exists := merged.Vars[name]; !exists {
				merged.Vars[name] = varDef
			}
		}
		for name, output := range parent.Outputs {
			if _, exists := merged.Outputs[name]; !exists {
				merged.Outputs[name] = output
			}
		}

		merged.Steps = append(merged.Steps, parent.Steps...)
		merged.Template = append(merged.Template, parent.Template...)
		merged.Compose = mergeComposeRules(merged.Compose, parent.Compose)
	}

	for name, varDef := range formula.Vars {
		merged.Vars[name] = varDef
	}
	for name, output := range formula.Outputs {
		merged.Outputs[name] = output
	}

	merged.Steps = mergeSteps(merged.Steps, formula.Steps)
	merged.Template = mergeSteps(merged.Template, formula.Template)
	merged.Compose = mergeComposeRules(merged.Compose, formula.Compose)
	merged.Preflight = mergePreflightSpec(merged.Preflight, formula.Preflight)

	if merged.Workspace == nil && merged.Worktree {
		merged.Workspace = &spec.WorkspaceSpec{Kind: "worktree"}
	}

	if formula.Description != "" {
		merged.Description = formula.Description
	}

	if err := merged.Validate(); err != nil {
		return nil, err
	}

	return merged, nil
}

func mergePreflightSpec(base, overlay *spec.PreflightSpec) *spec.PreflightSpec {
	if overlay == nil || len(overlay.Checks) == 0 {
		return base
	}
	if base == nil || len(base.Checks) == 0 {
		return &spec.PreflightSpec{Checks: append([]*spec.PreflightCheck{}, overlay.Checks...)}
	}
	return &spec.PreflightSpec{Checks: append(append([]*spec.PreflightCheck{}, base.Checks...), overlay.Checks...)}
}

func (p *Parser) loadFormula(name string) (*spec.Formula, error) {
	if cached, ok := p.cache[name]; ok {
		return cached, nil
	}

	extensions := []string{CanonicalTOMLExt, FormulaExtJSON}
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

func (p *Parser) LoadByName(name string) (*spec.Formula, error) {
	return p.loadFormula(name)
}

func mergeSteps(parent, child []*spec.Step) []*spec.Step {
	parentIdx := make(map[string]int, len(parent))
	for i, s := range parent {
		parentIdx[s.ID] = i
	}

	result := make([]*spec.Step, len(parent))
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

func mergeComposeRules(base, overlay *spec.ComposeRules) *spec.ComposeRules {
	if overlay == nil {
		return base
	}
	if base == nil {
		return overlay
	}

	result := &spec.ComposeRules{
		BondPoints: append([]*spec.BondPoint{}, base.BondPoints...),
		Hooks:      append([]*spec.Hook{}, base.Hooks...),
		Expand:     append([]*spec.ExpandRule{}, base.Expand...),
		Map:        append([]*spec.MapRule{}, base.Map...),
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
