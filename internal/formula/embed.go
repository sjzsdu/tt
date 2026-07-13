package formula

import (
	"fmt"
	spec "github.com/sjzsdu/tt/internal/formula/spec"
	"sort"
	"strings"
)

const DefaultMaxEmbedDepth = 10

func ApplyEmbedsWithVars(steps []*spec.Step, parser *Parser, parentVars map[string]string, stack []string) ([]*spec.Step, error) {
	if parser == nil {
		return steps, nil
	}
	return applyEmbedsRecursive(steps, parser, parentVars, stack, 0)
}

func applyEmbedsRecursive(steps []*spec.Step, parser *Parser, parentVars map[string]string, stack []string, depth int) ([]*spec.Step, error) {
	if depth > DefaultMaxEmbedDepth {
		return nil, fmt.Errorf("max embed depth (%d) exceeded", DefaultMaxEmbedDepth)
	}

	result := make([]*spec.Step, 0, len(steps))
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
			if step.Loop != nil && len(step.Loop.Body) > 0 {
				loop := *step.Loop
				body, err := applyEmbedsRecursive(step.Loop.Body, parser, parentVars, stack, depth+1)
				if err != nil {
					return nil, err
				}
				loop.Body = body
				clone.Loop = &loop
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

func rewriteDepsFromEmbedBoundaries(steps []*spec.Step) {
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

func embeddedStepsForBoundary(steps []*spec.Step, boundaryID string) []*spec.Step {
	var out []*spec.Step
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

func expandEmbeddedStep(step *spec.Step, parser *Parser, parentVars map[string]string, stack []string, depth int) ([]*spec.Step, error) {
	name := strings.TrimSpace(step.Embed)
	if name == "" {
		return []*spec.Step{cloneStep(step)}, nil
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
	if resolved.Type != spec.TypeWorkflow && resolved.Type != spec.TypeAtomic {
		return nil, fmt.Errorf("embed %q on step %q: %q is not an embeddable formula (type=%s)", step.ID, step.ID, name, resolved.Type)
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
		childVars[k] = spec.Substitute(v, parentVars)
	}
	if err := spec.ValidateVars(resolved, childVars); err != nil {
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
	filteredSteps = substituteStepTemplateVars(filteredSteps, childVars)

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

	protectedTemplateRoots := collectTemplateRootsFromVars(childVars)
	namespaced := namespaceEmbeddedSteps(filteredSteps, step.ID, name, step.ID, protectedTemplateRoots)
	if strings.TrimSpace(step.Condition) != "" {
		for _, childStep := range namespaced {
			if childStep == nil {
				continue
			}
			childStep.Condition = combineConditions(step.Condition, childStep.Condition)
		}
	}
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

	result := make([]*spec.Step, 0, 1+len(namespaced))
	result = append(result, boundary)
	result = append(result, namespaced...)
	return result, nil
}

func substituteStepTemplateVars(steps []*spec.Step, vars map[string]string) []*spec.Step {
	if len(steps) == 0 || len(vars) == 0 {
		return steps
	}
	result := make([]*spec.Step, 0, len(steps))
	for _, step := range steps {
		if step == nil {
			continue
		}
		clone := cloneStep(step)
		clone.Title = spec.Substitute(clone.Title, vars)
		clone.Description = spec.Substitute(clone.Description, vars)
		clone.Condition = spec.Substitute(clone.Condition, vars)
		clone.Timeout = spec.Substitute(clone.Timeout, vars)
		clone.OutputKey = spec.Substitute(clone.OutputKey, vars)
		clone.InputCtx = substituteStringSliceVars(clone.InputCtx, vars)
		clone.DependsOn = substituteStringSliceVars(clone.DependsOn, vars)
		clone.Needs = substituteStringSliceVars(clone.Needs, vars)
		clone.Labels = substituteStringSliceVars(clone.Labels, vars)
		clone.Metadata = substituteStringMapVars(clone.Metadata, vars)
		if clone.Agent != nil {
			agent := *clone.Agent
			agent.Name = spec.Substitute(agent.Name, vars)
			agent.Model = spec.Substitute(agent.Model, vars)
			agent.Cwd = spec.Substitute(agent.Cwd, vars)
			clone.Agent = &agent
		}
		if clone.ExternalAgent != nil {
			agent := *clone.ExternalAgent
			agent.Driver = spec.Substitute(agent.Driver, vars)
			agent.Provider = spec.Substitute(agent.Provider, vars)
			agent.Model = spec.Substitute(agent.Model, vars)
			agent.Mode = spec.Substitute(agent.Mode, vars)
			agent.Resume = spec.Substitute(agent.Resume, vars)
			agent.Cwd = spec.Substitute(agent.Cwd, vars)
			agent.Timeout = spec.Substitute(agent.Timeout, vars)
			agent.ExtraArgs = substituteStringSliceVars(agent.ExtraArgs, vars)
			clone.ExternalAgent = &agent
		}
		if clone.Script != nil {
			script := *clone.Script
			script.Command = substituteStringSliceVars(script.Command, vars)
			script.Cwd = spec.Substitute(script.Cwd, vars)
			script.Env = substituteStringMapVars(script.Env, vars)
			clone.Script = &script
		}
		if clone.Tool != nil {
			tool := *clone.Tool
			tool.Name = spec.Substitute(tool.Name, vars)
			clone.Tool = &tool
		}
		if len(clone.Children) > 0 {
			clone.Children = substituteStepTemplateVars(clone.Children, vars)
		}
		if clone.Loop != nil && len(clone.Loop.Body) > 0 {
			loop := *clone.Loop
			loop.ForEach = spec.Substitute(loop.ForEach, vars)
			loop.Var = spec.Substitute(loop.Var, vars)
			loop.Until = spec.Substitute(loop.Until, vars)
			loop.Range = spec.Substitute(loop.Range, vars)
			loop.Body = substituteStepTemplateVars(loop.Body, vars)
			clone.Loop = &loop
		}
		result = append(result, clone)
	}
	return result
}

func substituteStringSliceVars(values []string, vars map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = spec.Substitute(value, vars)
	}
	return out
}

func substituteStringMapVars(values map[string]string, vars map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = spec.Substitute(value, vars)
	}
	return out
}

func combineConditions(parent, child string) string {
	parent = strings.TrimSpace(parent)
	child = strings.TrimSpace(child)
	switch {
	case parent == "":
		return child
	case child == "":
		return parent
	default:
		return parent + " && " + child
	}
}

func namespaceEmbeddedSteps(steps []*spec.Step, prefix, sourceFormula, embeddedBy string, protectedTemplateRoots map[string]bool) []*spec.Step {
	mapping := make(map[string]string)
	collectStepNamespaceMapping(steps, prefix, mapping)
	result := make([]*spec.Step, 0, len(steps))
	for _, step := range steps {
		result = append(result, rewriteEmbeddedStep(step, mapping, sourceFormula, embeddedBy, protectedTemplateRoots))
	}
	return result
}

func collectStepNamespaceMapping(steps []*spec.Step, prefix string, mapping map[string]string) {
	for _, step := range steps {
		if step == nil {
			continue
		}
		mapping[step.ID] = prefix + "." + step.ID
		if len(step.Children) > 0 {
			collectStepNamespaceMapping(step.Children, prefix+"."+step.ID, mapping)
		}
		if step.Loop != nil && len(step.Loop.Body) > 0 {
			// Loop bodies are executed dynamically by LoopStep, which already scopes
			// emitted event node IDs under the loop iteration. Keep their context keys
			// as siblings under the embedded formula prefix so loop until/condition
			// expressions like `review.approved == true` rewrite to the same key that
			// LoopStep stores after each body execution.
			collectStepNamespaceMapping(step.Loop.Body, prefix, mapping)
		}
	}
}

func rewriteEmbeddedStep(step *spec.Step, mapping map[string]string, sourceFormula, embeddedBy string, protectedTemplateRoots map[string]bool) *spec.Step {
	clone := cloneStep(step)
	origID := step.ID
	if mapped, ok := mapping[origID]; ok {
		clone.ID = mapped
	}
	clone.DependsOn = rewriteStepRefs(step.DependsOn, mapping)
	clone.Needs = rewriteStepRefs(step.Needs, mapping)
	clone.Condition = rewriteContextRefsInString(step.Condition, mapping, nil)
	clone.InputCtx = rewriteContextKeys(step.InputCtx, mapping, embeddedBy)
	clone.OutputKey = rewriteContextKey(step.OutputKey, mapping, embeddedBy)
	rewriteEmbeddedStepTemplates(clone, mapping, protectedTemplateRoots)
	clone.Metadata = copyStringMap(clone.Metadata)
	if clone.Metadata == nil {
		clone.Metadata = map[string]string{}
	}
	clone.Metadata["embedded_formula"] = sourceFormula
	clone.Metadata["embedded_by"] = embeddedBy
	clone.Metadata["source_formula"] = sourceFormula
	clone.Metadata["source_step_id"] = origID
	if len(step.Children) > 0 {
		clone.Children = make([]*spec.Step, len(step.Children))
		for i, child := range step.Children {
			clone.Children[i] = rewriteEmbeddedStep(child, mapping, sourceFormula, embeddedBy, protectedTemplateRoots)
		}
	}
	if clone.Loop != nil {
		clone.Loop = cloneLoopSpec(clone.Loop, mapping, sourceFormula, embeddedBy, protectedTemplateRoots)
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

func rewriteContextKeys(in []string, mapping map[string]string, prefix string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = rewriteContextKey(v, mapping, prefix)
	}
	return out
}

func rewriteContextKey(v string, mapping map[string]string, prefix string) string {
	if strings.TrimSpace(v) == "" {
		return v
	}
	if rewritten, ok := rewriteContextRef(v, mapping); ok {
		return rewritten
	}
	if strings.Contains(v, ".") {
		return v
	}
	return prefix + "." + v
}

func rewriteEmbeddedStepTemplates(step *spec.Step, mapping map[string]string, protectedTemplateRoots map[string]bool) {
	step.Title = rewriteContextRefsInString(step.Title, mapping, protectedTemplateRoots)
	step.Description = rewriteContextRefsInString(step.Description, mapping, protectedTemplateRoots)
	step.Timeout = rewriteContextRefsInString(step.Timeout, mapping, protectedTemplateRoots)
	step.Metadata = rewriteStringMapContextRefs(step.Metadata, mapping, protectedTemplateRoots)
	if step.Agent != nil {
		agent := *step.Agent
		agent.Name = rewriteContextRefsInString(agent.Name, mapping, protectedTemplateRoots)
		agent.Model = rewriteContextRefsInString(agent.Model, mapping, protectedTemplateRoots)
		agent.Cwd = rewriteContextRefsInString(agent.Cwd, mapping, protectedTemplateRoots)
		step.Agent = &agent
	}
	if step.Script != nil {
		script := *step.Script
		script.Command = rewriteStringSliceContextRefs(script.Command, mapping, protectedTemplateRoots)
		script.Cwd = rewriteContextRefsInString(script.Cwd, mapping, protectedTemplateRoots)
		script.Env = rewriteStringMapContextRefs(script.Env, mapping, protectedTemplateRoots)
		step.Script = &script
	}
	if step.Tool != nil {
		tool := *step.Tool
		tool.Name = rewriteContextRefsInString(tool.Name, mapping, protectedTemplateRoots)
		step.Tool = &tool
	}
	if step.Aggregate != nil {
		aggregate := *step.Aggregate
		if rewritten, ok := rewriteContextRef(aggregate.Source, mapping); ok {
			aggregate.Source = rewritten
		}
		step.Aggregate = &aggregate
	}
}

func rewriteStringSliceContextRefs(values []string, mapping map[string]string, protectedTemplateRoots map[string]bool) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = rewriteContextRefsInString(value, mapping, protectedTemplateRoots)
	}
	return out
}

func rewriteStringMapContextRefs(values map[string]string, mapping map[string]string, protectedTemplateRoots map[string]bool) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = rewriteContextRefsInString(value, mapping, protectedTemplateRoots)
	}
	return out
}

func rewriteContextRefsInString(value string, mapping map[string]string, protectedTemplateRoots map[string]bool) string {
	if value == "" || len(mapping) == 0 {
		return value
	}
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	out := value
	for _, key := range keys {
		if protectedTemplateRoots[key] {
			continue
		}
		mapped := mapping[key]
		out = strings.ReplaceAll(out, "{{"+key+"}}", "{{"+mapped+"}}")
		out = strings.ReplaceAll(out, "{{"+key+".", "{{"+mapped+".")
		out = replaceBareContextRef(out, key, mapped)
	}
	return out
}

func replaceBareContextRef(value, key, mapped string) string {
	needle := key + "."
	if !strings.Contains(value, needle) {
		return value
	}
	var b strings.Builder
	start := 0
	for {
		idx := strings.Index(value[start:], needle)
		if idx < 0 {
			b.WriteString(value[start:])
			break
		}
		idx += start
		if idx > 0 && isContextIdentByte(value[idx-1]) {
			b.WriteString(value[start : idx+len(needle)])
			start = idx + len(needle)
			continue
		}
		b.WriteString(value[start:idx])
		b.WriteString(mapped)
		b.WriteByte('.')
		start = idx + len(needle)
	}
	return b.String()
}

func isContextIdentByte(ch byte) bool {
	return ch == '_' || ch == '-' || ch == '.' || (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func rewriteContextRef(value string, mapping map[string]string) (string, bool) {
	if len(mapping) == 0 {
		return "", false
	}
	for key, mapped := range mapping {
		if value == key {
			return mapped, true
		}
		if strings.HasPrefix(value, key+".") {
			return mapped + strings.TrimPrefix(value, key), true
		}
	}
	return "", false
}

func embeddedBoundaryIDs(steps []*spec.Step) (entries []string, exits []string) {
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

func cloneLoopSpec(loop *spec.LoopSpec, mapping map[string]string, sourceFormula, prefix string, protectedTemplateRoots map[string]bool) *spec.LoopSpec {
	if loop == nil {
		return nil
	}
	cp := *loop
	cp.ForEach = rewriteContextRefsInString(cp.ForEach, mapping, nil)
	cp.Until = rewriteContextRefsInString(cp.Until, mapping, nil)
	if len(loop.Body) > 0 {
		cp.Body = make([]*spec.Step, len(loop.Body))
		for i, step := range loop.Body {
			cp.Body[i] = rewriteEmbeddedStep(step, mapping, sourceFormula, prefix, protectedTemplateRoots)
		}
	}
	return &cp
}

func collectTemplateRootsFromVars(vars map[string]string) map[string]bool {
	roots := map[string]bool{}
	for _, value := range vars {
		for _, root := range collectTemplateRoots(value) {
			roots[root] = true
		}
	}
	return roots
}

func collectTemplateRoots(value string) []string {
	var roots []string
	start := 0
	for {
		open := strings.Index(value[start:], "{{")
		if open < 0 {
			break
		}
		open += start + 2
		close := strings.Index(value[open:], "}}")
		if close < 0 {
			break
		}
		inner := strings.TrimSpace(value[open : open+close])
		root := inner
		if dot := strings.Index(root, "."); dot >= 0 {
			root = root[:dot]
		}
		if root != "" {
			roots = append(roots, root)
		}
		start = open + close + 2
	}
	return roots
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
