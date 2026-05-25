package formula

import (
	"fmt"
	"sort"
	"strings"
)

const DefaultMaxEmbedDepth = 10

func ApplyEmbedsWithVars(steps []*Step, parser *Parser, parentVars map[string]string, stack []string) ([]*Step, error) {
	if parser == nil {
		return steps, nil
	}
	return applyEmbedsRecursive(steps, parser, parentVars, stack, 0)
}

func applyEmbedsRecursive(steps []*Step, parser *Parser, parentVars map[string]string, stack []string, depth int) ([]*Step, error) {
	if depth > DefaultMaxEmbedDepth {
		return nil, fmt.Errorf("max embed depth (%d) exceeded", DefaultMaxEmbedDepth)
	}

	result := make([]*Step, 0, len(steps))
	for _, step := range steps {
		if step == nil {
			continue
		}
		if strings.TrimSpace(step.Embed) == "" {
			clone := cloneStep(step)
			if len(step.Children) > 0 {
				children, err := applyEmbedsRecursive(step.Children, parser, parentVars, stack, depth+1)
				if err != nil {
					return nil, err
				}
				clone.Children = children
			}
			result = append(result, clone)
			continue
		}

		embedded, err := expandEmbeddedStep(step, parser, parentVars, stack, depth)
		if err != nil {
			return nil, err
		}
		result = append(result, embedded...)
	}
	rewriteDepsFromEmbedBoundaries(result)
	return result, nil
}

func rewriteDepsFromEmbedBoundaries(steps []*Step) {
	exitsByBoundary := map[string][]string{}
	for _, step := range steps {
		if step == nil || step.Metadata["step_kind"] != "embed_boundary" {
			continue
		}
		embedded := embeddedStepsForBoundary(steps, step.ID)
		_, exits := embeddedBoundaryIDs(embedded)
		if len(exits) > 0 {
			exitsByBoundary[step.ID] = exits
		}
	}
	if len(exitsByBoundary) == 0 {
		return
	}
	for _, step := range steps {
		if step == nil {
			continue
		}
		for boundaryID, exits := range exitsByBoundary {
			// Embedded entry steps intentionally depend on their boundary so the
			// boundary is visible/runnable before the sub-flow starts. External
			// downstream steps, however, must wait for the embedded workflow exits
			// rather than the noop boundary itself.
			if step.ID == boundaryID || step.Metadata["embedded_by"] == boundaryID {
				continue
			}
			step.DependsOn = replaceRefWithRefs(step.DependsOn, boundaryID, exits)
			step.Needs = replaceRefWithRefs(step.Needs, boundaryID, exits)
		}
	}
}

func embeddedStepsForBoundary(steps []*Step, boundaryID string) []*Step {
	var out []*Step
	for _, step := range steps {
		if step != nil && step.Metadata["embedded_by"] == boundaryID {
			out = append(out, step)
		}
	}
	return out
}

func replaceRefWithRefs(in []string, target string, replacements []string) []string {
	if len(in) == 0 || len(replacements) == 0 {
		return in
	}
	out := make([]string, 0, len(in)+len(replacements)-1)
	changed := false
	for _, ref := range in {
		if ref != target {
			out = appendUnique(out, ref)
			continue
		}
		changed = true
		for _, replacement := range replacements {
			out = appendUnique(out, replacement)
		}
	}
	if !changed {
		return in
	}
	return out
}

func expandEmbeddedStep(step *Step, parser *Parser, parentVars map[string]string, stack []string, depth int) ([]*Step, error) {
	name := strings.TrimSpace(step.Embed)
	if name == "" {
		return []*Step{cloneStep(step)}, nil
	}
	if containsString(stack, name) {
		cycle := append(append([]string(nil), stack...), name)
		return nil, fmt.Errorf("circular formula embedding detected: %s", strings.Join(cycle, " -> "))
	}

	child, err := parser.LoadByName(name)
	if err != nil {
		return nil, fmt.Errorf("embed %q on step %q: loading formula %q: %w", step.ID, step.ID, name, err)
	}
	resolved, err := parser.Resolve(child)
	if err != nil {
		return nil, fmt.Errorf("embed %q on step %q: resolving formula %q: %w", step.ID, step.ID, name, err)
	}
	if resolved.Type != TypeWorkflow {
		return nil, fmt.Errorf("embed %q on step %q: %q is not a workflow formula (type=%s)", step.ID, step.ID, name, resolved.Type)
	}

	childVars := make(map[string]string)
	for vname, def := range resolved.Vars {
		if def != nil && def.Default != nil {
			childVars[vname] = *def.Default
		}
	}
	for k, v := range parentVars {
		childVars[k] = v
	}
	for k, v := range step.EmbedVars {
		childVars[k] = Substitute(v, parentVars)
	}
	if err := ValidateVars(resolved, childVars); err != nil {
		return nil, fmt.Errorf("embed %q on step %q: %w", name, step.ID, err)
	}
	if err := validateCompileTimeVars(resolved, childVars); err != nil {
		return nil, fmt.Errorf("embed %q on step %q: %w", name, step.ID, err)
	}

	controlFlowSteps, err := ApplyControlFlowWithVars(resolved.Steps, resolved.Compose, childVars)
	if err != nil {
		return nil, fmt.Errorf("embed %q on step %q: applying control flow: %w", name, step.ID, err)
	}
	resolved.Steps = controlFlowSteps
	if len(resolved.Advice) > 0 {
		resolved.Steps = ApplyAdvice(resolved.Steps, resolved.Advice)
	}
	inlineExpandedSteps, err := ApplyInlineExpansionsWithVars(resolved.Steps, parser, childVars)
	if err != nil {
		return nil, fmt.Errorf("embed %q on step %q: applying inline expansions: %w", name, step.ID, err)
	}
	resolved.Steps = inlineExpandedSteps
	if resolved.Compose != nil && (len(resolved.Compose.Expand) > 0 || len(resolved.Compose.Map) > 0) {
		expandedSteps, err := ApplyExpansionsWithVars(resolved.Steps, resolved.Compose, parser, childVars)
		if err != nil {
			return nil, fmt.Errorf("embed %q on step %q: applying expansions: %w", name, step.ID, err)
		}
		resolved.Steps = expandedSteps
	}

	nestedSteps, err := applyEmbedsRecursive(resolved.Steps, parser, childVars, append(stack, name), depth+1)
	if err != nil {
		return nil, fmt.Errorf("embed %q on step %q: %w", name, step.ID, err)
	}
	filteredSteps, err := FilterStepsByCondition(nestedSteps, childVars)
	if err != nil {
		return nil, fmt.Errorf("embed %q on step %q: filtering steps by condition: %w", name, step.ID, err)
	}

	boundary := cloneStep(step)
	boundary.Expand = ""
	boundary.ExpandVars = nil
	boundary.Embed = ""
	boundary.EmbedVars = nil
	boundary.Children = nil
	boundary.Loop = nil
	boundary.Gate = nil
	boundary.Agent = nil
	boundary.Script = nil
	boundary.Form = nil
	boundary.OutputKey = ""
	boundary.InputCtx = nil
	if strings.TrimSpace(boundary.Title) == "" {
		boundary.Title = fmt.Sprintf("Embed %s", name)
	}
	if boundary.Metadata == nil {
		boundary.Metadata = map[string]string{}
	}
	boundary.Metadata["embedded_formula"] = name
	boundary.Metadata["step_kind"] = "embed_boundary"
	if boundary.Execution == "" {
		boundary.Execution = "noop"
	}
	if boundary.Type == "" {
		boundary.Type = "boundary"
	}

	namespaced := namespaceEmbeddedSteps(filteredSteps, step.ID, name, step.ID)
	if len(namespaced) > 0 {
		entryIDs, exitIDs := embeddedBoundaryIDs(namespaced)
		for _, childStep := range namespaced {
			for _, dep := range step.DependsOn {
				if containsString(entryIDs, childStep.ID) {
					childStep.DependsOn = appendUnique(childStep.DependsOn, dep)
				}
			}
			for _, need := range step.Needs {
				if containsString(entryIDs, childStep.ID) {
					childStep.Needs = appendUnique(childStep.Needs, need)
				}
			}
			for _, dep := range childStep.DependsOn {
				if containsString(exitIDs, dep) {
					dep = dep
				}
			}
		}
		for _, childStep := range namespaced {
			if containsString(entryIDs, childStep.ID) {
				childStep.DependsOn = appendUnique(childStep.DependsOn, step.ID)
			}
		}
		for _, childStep := range namespaced {
			for i, dep := range childStep.DependsOn {
				if dep == step.ID {
					childStep.DependsOn[i] = step.ID
				}
			}
		}
	}

	result := make([]*Step, 0, 1+len(namespaced))
	result = append(result, boundary)
	result = append(result, namespaced...)
	return result, nil
}

func namespaceEmbeddedSteps(steps []*Step, prefix, sourceFormula, embeddedBy string) []*Step {
	mapping := make(map[string]string)
	collectStepNamespaceMapping(steps, prefix, mapping)
	result := make([]*Step, 0, len(steps))
	for _, step := range steps {
		result = append(result, rewriteEmbeddedStep(step, mapping, sourceFormula, embeddedBy))
	}
	return result
}

func collectStepNamespaceMapping(steps []*Step, prefix string, mapping map[string]string) {
	for _, step := range steps {
		if step == nil {
			continue
		}
		mapping[step.ID] = prefix + "." + step.ID
		if len(step.Children) > 0 {
			collectStepNamespaceMapping(step.Children, prefix+"."+step.ID, mapping)
		}
	}
}

func rewriteEmbeddedStep(step *Step, mapping map[string]string, sourceFormula, embeddedBy string) *Step {
	clone := cloneStep(step)
	origID := step.ID
	clone.ID = mapping[origID]
	clone.DependsOn = rewriteStepRefs(step.DependsOn, mapping)
	clone.Needs = rewriteStepRefs(step.Needs, mapping)
	clone.InputCtx = prefixContextKeys(step.InputCtx, embeddedBy)
	clone.OutputKey = prefixContextKey(step.OutputKey, embeddedBy)
	clone.Metadata = copyStringMap(clone.Metadata)
	if clone.Metadata == nil {
		clone.Metadata = map[string]string{}
	}
	clone.Metadata["embedded_formula"] = sourceFormula
	clone.Metadata["embedded_by"] = embeddedBy
	clone.Metadata["source_formula"] = sourceFormula
	clone.Metadata["source_step_id"] = origID
	if len(step.Children) > 0 {
		clone.Children = make([]*Step, len(step.Children))
		for i, child := range step.Children {
			clone.Children[i] = rewriteEmbeddedStep(child, mapping, sourceFormula, embeddedBy)
		}
	}
	if clone.Loop != nil {
		clone.Loop = cloneLoopSpec(clone.Loop, embeddedBy)
	}
	return clone
}

func rewriteStepRefs(in []string, mapping map[string]string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, v := range in {
		if mapped, ok := mapping[v]; ok {
			out[i] = mapped
		} else {
			out[i] = v
		}
	}
	return out
}

func prefixContextKeys(in []string, prefix string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = prefixContextKey(v, prefix)
	}
	return out
}

func prefixContextKey(v, prefix string) string {
	if strings.TrimSpace(v) == "" {
		return v
	}
	if strings.Contains(v, ".") {
		return v
	}
	return prefix + "." + v
}

func embeddedBoundaryIDs(steps []*Step) (entries []string, exits []string) {
	inDegree := map[string]int{}
	outDegree := map[string]int{}
	ids := map[string]struct{}{}
	for _, step := range steps {
		ids[step.ID] = struct{}{}
		if _, ok := inDegree[step.ID]; !ok {
			inDegree[step.ID] = 0
		}
		if _, ok := outDegree[step.ID]; !ok {
			outDegree[step.ID] = 0
		}
	}
	for _, step := range steps {
		for _, dep := range step.DependsOn {
			if _, ok := ids[dep]; ok {
				inDegree[step.ID]++
				outDegree[dep]++
			}
		}
	}
	for id := range ids {
		if inDegree[id] == 0 {
			entries = append(entries, id)
		}
		if outDegree[id] == 0 {
			exits = append(exits, id)
		}
	}
	sort.Strings(entries)
	sort.Strings(exits)
	return entries, exits
}

func cloneLoopSpec(loop *LoopSpec, prefix string) *LoopSpec {
	if loop == nil {
		return nil
	}
	cp := *loop
	if len(loop.Body) > 0 {
		cp.Body = make([]*Step, len(loop.Body))
		for i, step := range loop.Body {
			cp.Body[i] = cloneStepDeep(step)
			cp.Body[i].InputCtx = prefixContextKeys(cp.Body[i].InputCtx, prefix)
			cp.Body[i].OutputKey = prefixContextKey(cp.Body[i].OutputKey, prefix)
		}
	}
	return &cp
}

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func containsString(xs []string, target string) bool {
	for _, x := range xs {
		if x == target {
			return true
		}
	}
	return false
}
