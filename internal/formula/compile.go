package formula

import (
	"context"
	"fmt"
	"regexp"
)

func Compile(_ context.Context, name string, searchPaths []string, vars map[string]string) (*Recipe, error) {
	return compileFormula(name, searchPaths, vars, true)
}

func CompileWithoutRuntimeVarValidation(_ context.Context, name string, searchPaths []string, vars map[string]string) (*Recipe, error) {
	return compileFormula(name, searchPaths, vars, false)
}

func compileFormula(name string, searchPaths []string, vars map[string]string, validateRuntimeVars bool) (*Recipe, error) {
	parser := NewParser(searchPaths...)

	f, err := parser.LoadByName(name)
	if err != nil {
		return nil, fmt.Errorf("loading formula %q: %w", name, err)
	}

	resolved, err := parser.Resolve(f)
	if err != nil {
		return nil, fmt.Errorf("resolving formula %q: %w", name, err)
	}

	if validateRuntimeVars && len(vars) > 0 {
		if err := ValidateVars(resolved, vars); err != nil {
			return nil, err
		}
	}

	compileVars := make(map[string]string)
	for vname, def := range resolved.Vars {
		if def != nil && def.Default != nil {
			compileVars[vname] = *def.Default
		}
	}
	for k, v := range vars {
		compileVars[k] = v
	}

	if err := validateCompileTimeVars(resolved, vars); err != nil {
		return nil, err
	}

	controlFlowSteps, err := ApplyControlFlowWithVars(resolved.Steps, resolved.Compose, compileVars)
	if err != nil {
		return nil, fmt.Errorf("applying control flow to %q: %w", name, err)
	}
	resolved.Steps = controlFlowSteps

	if len(resolved.Advice) > 0 {
		resolved.Steps = ApplyAdvice(resolved.Steps, resolved.Advice)
	}

	inlineExpandedSteps, err := ApplyInlineExpansionsWithVars(resolved.Steps, parser, compileVars)
	if err != nil {
		return nil, fmt.Errorf("applying inline expansions to %q: %w", name, err)
	}
	resolved.Steps = inlineExpandedSteps

	if resolved.Compose != nil && (len(resolved.Compose.Expand) > 0 || len(resolved.Compose.Map) > 0) {
		expandedSteps, err := ApplyExpansionsWithVars(resolved.Steps, resolved.Compose, parser, compileVars)
		if err != nil {
			return nil, fmt.Errorf("applying expansions to %q: %w", name, err)
		}
		resolved.Steps = expandedSteps
	}

	embeddedSteps, err := ApplyEmbedsWithVars(resolved.Steps, parser, compileVars, []string{name})
	if err != nil {
		return nil, fmt.Errorf("applying embeds to %q: %w", name, err)
	}
	resolved.Steps = embeddedSteps

	filteredSteps, err := FilterStepsByCondition(resolved.Steps, compileVars)
	if err != nil {
		return nil, fmt.Errorf("filtering steps by condition: %w", err)
	}
	resolved.Steps = filteredSteps

	return toRecipe(resolved)
}

func validateCompileTimeVars(f *Formula, values map[string]string) error {
	if f == nil || len(f.Vars) == 0 {
		return nil
	}
	refs := make(map[string]bool)
	collectCompileTimeVarRefs(f.Steps, refs)
	collectCompileTimeVarRefs(f.Template, refs)
	if len(refs) == 0 {
		return nil
	}
	defs := make(map[string]*VarDef)
	for name := range refs {
		def := f.Vars[name]
		if def != nil {
			defs[name] = def
		}
	}
	return ValidateVarDefs(defs, ApplyDefaults(f, values))
}

func collectCompileTimeVarRefs(steps []*Step, refs map[string]bool) {
	for _, step := range steps {
		if step == nil {
			continue
		}
		if step.Loop != nil && step.Loop.Range != "" {
			for _, match := range rangeVarPattern.FindAllStringSubmatch(step.Loop.Range, -1) {
				refs[match[1]] = true
			}
		}
		collectStepConditionVarRefs(step.Condition, refs)
		collectCompileTimeVarRefs(step.Children, refs)
		if step.Loop != nil {
			collectCompileTimeVarRefs(step.Loop.Body, refs)
		}
	}
}

func collectStepConditionVarRefs(condition string, refs map[string]bool) {
	if condition == "" {
		return
	}
	for _, pattern := range []*regexp.Regexp{
		stepCondVarPattern,
		stepCondNegatedVarPattern,
		stepCondComparePattern,
	} {
		if match := pattern.FindStringSubmatch(condition); match != nil {
			refs[match[1]] = true
			return
		}
	}
}
