package formula

import "regexp"

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
