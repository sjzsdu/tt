package spec

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// rangeVarPattern matches `{name}` placeholders inside Loop.Range / Loop.Until
// expressions. It is shared between validateStepTimeout (which needs to
// distinguish unresolved range variables from real Duration strings) and any
// caller that wants to scan for the same shape.
var rangeVarPattern = regexp.MustCompile(`\{(\w+)\}`)
var formulaNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// RequiredVarNames returns the names of required variables, sorted
// lexicographically.
func (f *Formula) RequiredVarNames() []string {
	var names []string
	for name, def := range f.Vars {
		if def != nil && def.Required {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// GetStepByID finds a step by its ID (searches recursively).
func (f *Formula) GetStepByID(id string) *Step {
	for _, step := range f.Steps {
		if found := findStepByID(step, id); found != nil {
			return found
		}
	}
	return nil
}

func findStepByID(step *Step, id string) *Step {
	if step.ID == id {
		return step
	}
	for _, child := range step.Children {
		if found := findStepByID(child, id); found != nil {
			return found
		}
	}
	return nil
}

// GetBondPoint finds a bond point by ID.
func (f *Formula) GetBondPoint(id string) *BondPoint {
	if f.Compose == nil {
		return nil
	}
	for _, bp := range f.Compose.BondPoints {
		if bp.ID == id {
			return bp
		}
	}
	return nil
}

// Validate checks the formula for structural errors.
func (f *Formula) Validate() error {
	var errs []string

	if f.Formula == "" {
		errs = append(errs, "formula: name is required")
	} else if !formulaNamePattern.MatchString(strings.TrimSpace(f.Formula)) {
		errs = append(errs, "formula: name must contain only letters, numbers, dot, underscore, and hyphen")
	}

	if f.Version < 1 {
		errs = append(errs, "version: must be >= 1")
	}

	if contract := strings.TrimSpace(f.Contract); contract != "" && !strings.EqualFold(contract, "graph.v2") {
		errs = append(errs, fmt.Sprintf("contract: invalid value %q (must be graph.v2)", f.Contract))
	}

	if f.Type != "" && !f.Type.IsValid() {
		errs = append(errs, fmt.Sprintf("type: invalid value %q (must be workflow, expansion, aspect, or atomic)", f.Type))
	}

	if f.Workspace != nil {
		kind := strings.ToLower(strings.TrimSpace(f.Workspace.Kind))
		if kind != "" && kind != "worktree" {
			errs = append(errs, fmt.Sprintf("workspace.kind: invalid value %q (must be worktree)", f.Workspace.Kind))
		}
	}
	if f.Runtime != nil {
		if f.Runtime.MaxConcurrency < 0 {
			errs = append(errs, "runtime.max_concurrency: must be >= 0")
		}
		if f.Runtime.MaxAgentConcurrency < 0 {
			errs = append(errs, "runtime.max_agent_concurrency: must be >= 0")
		}
	}

	if f.Preflight != nil {
		for i, check := range f.Preflight.Checks {
			prefix := fmt.Sprintf("preflight.checks[%d]", i)
			if check == nil {
				errs = append(errs, fmt.Sprintf("%s: check cannot be null", prefix))
				continue
			}
			switch strings.ToLower(strings.TrimSpace(check.Type)) {
			case "command":
				if strings.TrimSpace(check.Command) == "" {
					errs = append(errs, fmt.Sprintf("%s: command check requires command", prefix))
				}
			case "exec":
				if strings.TrimSpace(check.Command) == "" {
					errs = append(errs, fmt.Sprintf("%s: exec check requires command", prefix))
				}
			case "git":
				// git checks are valid with only require_repo/require_remote flags.
			case "env":
				if strings.TrimSpace(check.Env) == "" && strings.TrimSpace(check.Name) == "" {
					errs = append(errs, fmt.Sprintf("%s: env check requires env or name", prefix))
				}
			case "path":
				if strings.TrimSpace(check.Path) == "" {
					errs = append(errs, fmt.Sprintf("%s: path check requires path", prefix))
				}
			default:
				errs = append(errs, fmt.Sprintf("%s: invalid type %q (must be command, exec, git, env, or path)", prefix, check.Type))
			}
		}
	}

	for name, v := range f.Vars {
		if name == "" {
			errs = append(errs, "vars: variable name cannot be empty")
			continue
		}
		if v.Required && v.Default != nil {
			errs = append(errs, fmt.Sprintf("vars.%s: cannot have both required:true and default", name))
		}
	}

	for name, output := range f.Outputs {
		name = strings.TrimSpace(name)
		if name == "" {
			errs = append(errs, "outputs: output name cannot be empty")
			continue
		}
		if output == nil {
			errs = append(errs, fmt.Sprintf("outputs.%s: definition cannot be null", name))
			continue
		}
		if strings.TrimSpace(output.From) == "" {
			errs = append(errs, fmt.Sprintf("outputs.%s.from: is required", name))
		}
	}

	stepIDLocations := make(map[string]string)
	for i, step := range f.Steps {
		prefix := fmt.Sprintf("steps[%d]", i)
		if step.ID == "" {
			errs = append(errs, fmt.Sprintf("%s: id is required", prefix))
			continue
		}
		if firstLoc, exists := stepIDLocations[step.ID]; exists {
			errs = append(errs, fmt.Sprintf("%s: duplicate id %q (first defined at %s)", prefix, step.ID, firstLoc))
		} else {
			stepIDLocations[step.ID] = prefix
		}

		if step.Title == "" && step.Expand == "" && step.Embed == "" {
			errs = append(errs, fmt.Sprintf("%s (%s): title is required (unless using expand/embed)", prefix, step.ID))
		}

		if step.Expand != "" && step.Embed != "" {
			errs = append(errs, fmt.Sprintf("%s (%s): expand and embed cannot be used together", prefix, step.ID))
		}
		if step.Embed != "" {
			if len(step.Children) > 0 {
				errs = append(errs, fmt.Sprintf("%s (%s): embed cannot be combined with children", prefix, step.ID))
			}
			if step.Loop != nil {
				errs = append(errs, fmt.Sprintf("%s (%s): embed cannot be combined with loop", prefix, step.ID))
			}
			if step.Agent != nil || step.Script != nil || step.Form != nil {
				errs = append(errs, fmt.Sprintf("%s (%s): embed cannot be combined with agent/script/form", prefix, step.ID))
			}
		}
		validateFormulaCallStep(step, &errs, prefix)

		if step.Priority != nil && (*step.Priority < 0 || *step.Priority > 4) {
			errs = append(errs, fmt.Sprintf("%s (%s): priority must be 0-4", prefix, step.ID))
		}

		if err := validateStepTimeout(prefix, step.ID, step.Timeout, step.Retry != nil, nil, true); err != "" {
			errs = append(errs, err)
		}
		validateLoopBodyTimeouts(step.Loop, &errs, fmt.Sprintf("%s (%s).loop", prefix, step.ID), nil, true)

		if step.Retry != nil {
			validateRetry(step.Retry, &errs, fmt.Sprintf("%s (%s)", prefix, step.ID), step)
		}

		validateFormSpec(step.Form, &errs, fmt.Sprintf("%s (%s).form", prefix, step.ID))

		collectChildIDs(step.Children, stepIDLocations, &errs, prefix)
	}

	embedStepIDs := map[string]struct{}{}
	for _, step := range f.Steps {
		if step != nil && strings.TrimSpace(step.Embed) != "" {
			embedStepIDs[step.ID] = struct{}{}
		}
	}

	for i, step := range f.Steps {
		for _, dep := range step.DependsOn {
			if _, exists := stepIDLocations[dep]; !exists && !isEmbeddedStepRef(dep, embedStepIDs) {
				errs = append(errs, fmt.Sprintf("steps[%d] (%s): depends_on references unknown step %q", i, step.ID, dep))
			}
		}
		for _, need := range step.Needs {
			if _, exists := stepIDLocations[need]; !exists {
				errs = append(errs, fmt.Sprintf("steps[%d] (%s): needs references unknown step %q", i, step.ID, need))
			}
		}
		if step.WaitsFor != "" {
			if err := validateWaitsFor(step.WaitsFor, stepIDLocations); err != nil {
				errs = append(errs, fmt.Sprintf("steps[%d] (%s): %s", i, step.ID, err.Error()))
			}
		}
		if step.OnComplete != nil {
			validateOnComplete(step.OnComplete, &errs, fmt.Sprintf("steps[%d] (%s)", i, step.ID))
		}
		validateChildDependsOn(step.Children, stepIDLocations, &errs, fmt.Sprintf("steps[%d]", i))
	}

	if f.Compose != nil {
		for i, bp := range f.Compose.BondPoints {
			if bp.ID == "" {
				errs = append(errs, fmt.Sprintf("compose.bond_points[%d]: id is required", i))
			}
			if bp.AfterStep != "" && bp.BeforeStep != "" {
				errs = append(errs, fmt.Sprintf("compose.bond_points[%d] (%s): cannot have both after_step and before_step", i, bp.ID))
			}
		}

		for i, hook := range f.Compose.Hooks {
			if hook.Trigger == "" {
				errs = append(errs, fmt.Sprintf("compose.hooks[%d]: trigger is required", i))
			}
			if hook.Attach == "" {
				errs = append(errs, fmt.Sprintf("compose.hooks[%d]: attach is required", i))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("formula validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}

func validateFormSpec(form *FormSpec, errs *[]string, prefix string) {
	if form == nil {
		return
	}
	if len(form.Fields) == 0 {
		*errs = append(*errs, fmt.Sprintf("%s: fields are required", prefix))
		return
	}
	seen := map[string]struct{}{}
	for i, field := range form.Fields {
		fieldPrefix := fmt.Sprintf("%s.fields[%d]", prefix, i)
		if field == nil {
			*errs = append(*errs, fmt.Sprintf("%s: field cannot be null", fieldPrefix))
			continue
		}
		name := strings.TrimSpace(field.Name)
		if name == "" {
			*errs = append(*errs, fmt.Sprintf("%s: name is required", fieldPrefix))
		} else if _, ok := seen[name]; ok {
			*errs = append(*errs, fmt.Sprintf("%s: duplicate field name %q", fieldPrefix, name))
		} else {
			seen[name] = struct{}{}
		}
		if strings.TrimSpace(field.Label) == "" {
			*errs = append(*errs, fmt.Sprintf("%s (%s): label is required", fieldPrefix, name))
		}
		fieldType := strings.TrimSpace(field.Type)
		if fieldType == "" {
			fieldType = "input"
		}
		if !isValidFormFieldType(fieldType) {
			*errs = append(*errs, fmt.Sprintf("%s (%s): invalid type %q (must be input, textarea, radio, checkbox, or select)", fieldPrefix, name, field.Type))
		}
		if (fieldType == "radio" || fieldType == "checkbox" || fieldType == "select") && len(field.Options) == 0 {
			*errs = append(*errs, fmt.Sprintf("%s (%s): options are required for %s fields", fieldPrefix, name, fieldType))
		}
	}
}

func isEmbeddedStepRef(ref string, embedStepIDs map[string]struct{}) bool {
	parent, child, ok := strings.Cut(ref, ".")
	if !ok || parent == "" || child == "" {
		return false
	}
	_, exists := embedStepIDs[parent]
	return exists
}

func isValidFormFieldType(fieldType string) bool {
	switch fieldType {
	case "input", "textarea", "radio", "checkbox", "select":
		return true
	default:
		return false
	}
}

func validateStepTimeout(prefix, stepID, raw string, hasRetry bool, allowedLoopVars map[string]struct{}, allowUnresolvedVars bool) string {
	if raw == "" {
		return ""
	}
	if err := validatePositiveTimeout(fmt.Sprintf("%s (%s)", prefix, stepID), raw, allowedLoopVars, allowUnresolvedVars); err != "" {
		return err
	}
	if !hasRetry {
		return fmt.Sprintf("%s (%s): timeout requires retry", prefix, stepID)
	}
	return ""
}

func validatePositiveTimeout(prefix, raw string, allowedLoopVars map[string]struct{}, allowUnresolvedVars bool) string {
	if raw == "" {
		return ""
	}
	parseRaw := substituteAllowedTimeoutLoopVars(raw, allowedLoopVars)
	if allowUnresolvedVars && rangeVarPattern.MatchString(parseRaw) {
		return ""
	}
	d, err := time.ParseDuration(parseRaw)
	if err != nil {
		return fmt.Sprintf("%s: invalid timeout %q: %v", prefix, raw, err)
	}
	if d <= 0 {
		return fmt.Sprintf("%s: timeout must be positive, got %v", prefix, d)
	}
	return ""
}

func substituteAllowedTimeoutLoopVars(raw string, allowedLoopVars map[string]struct{}) string {
	if len(allowedLoopVars) == 0 {
		return raw
	}
	return rangeVarPattern.ReplaceAllStringFunc(raw, func(match string) string {
		name := match[1 : len(match)-1]
		if _, ok := allowedLoopVars[name]; ok {
			return "1"
		}
		return match
	})
}

func validateLoopBodyTimeouts(loop *LoopSpec, errs *[]string, prefix string, allowedLoopVars map[string]struct{}, allowUnresolvedVars bool) {
	if loop == nil {
		return
	}
	validateNestedStepTimeoutsWithOptions(loop.Body, errs, prefix+".body", timeoutLoopVarsFor(loop, allowedLoopVars), allowUnresolvedVars)
}

func timeoutLoopVarsFor(loop *LoopSpec, parent map[string]struct{}) map[string]struct{} {
	if loop == nil || loop.Var == "" {
		return parent
	}
	vars := make(map[string]struct{}, len(parent)+1)
	for k, v := range parent {
		vars[k] = v
	}
	vars[loop.Var] = struct{}{}
	return vars
}

func validateNestedStepTimeoutsWithOptions(steps []*Step, errs *[]string, prefix string, allowedLoopVars map[string]struct{}, allowUnresolvedVars bool) {
	for i, step := range steps {
		if step == nil {
			continue
		}
		stepPrefix := fmt.Sprintf("%s[%d]", prefix, i)
		if err := validateStepTimeout(stepPrefix, step.ID, step.Timeout, step.Retry != nil, allowedLoopVars, allowUnresolvedVars); err != "" {
			*errs = append(*errs, err)
		}
		validateNestedStepTimeoutsWithOptions(step.Children, errs, stepPrefix+".children", allowedLoopVars, allowUnresolvedVars)
		validateLoopBodyTimeouts(step.Loop, errs, fmt.Sprintf("%s (%s).loop", stepPrefix, step.ID), allowedLoopVars, allowUnresolvedVars)
	}
}

func collectChildIDs(children []*Step, idLocations map[string]string, errs *[]string, prefix string) {
	for i, child := range children {
		childPrefix := fmt.Sprintf("%s.children[%d]", prefix, i)
		if child.ID == "" {
			*errs = append(*errs, fmt.Sprintf("%s: id is required", childPrefix))
			continue
		}
		if firstLoc, exists := idLocations[child.ID]; exists {
			*errs = append(*errs, fmt.Sprintf("%s: duplicate id %q (first defined at %s)", childPrefix, child.ID, firstLoc))
		} else {
			idLocations[child.ID] = childPrefix
		}

		if child.Title == "" && child.Expand == "" && child.Embed == "" {
			*errs = append(*errs, fmt.Sprintf("%s (%s): title is required", childPrefix, child.ID))
		}

		if child.Expand != "" && child.Embed != "" {
			*errs = append(*errs, fmt.Sprintf("%s (%s): expand and embed cannot be used together", childPrefix, child.ID))
		}
		if child.Embed != "" {
			if len(child.Children) > 0 {
				*errs = append(*errs, fmt.Sprintf("%s (%s): embed cannot be combined with children", childPrefix, child.ID))
			}
			if child.Loop != nil {
				*errs = append(*errs, fmt.Sprintf("%s (%s): embed cannot be combined with loop", childPrefix, child.ID))
			}
			if child.Agent != nil || child.Script != nil || child.Form != nil {
				*errs = append(*errs, fmt.Sprintf("%s (%s): embed cannot be combined with agent/script/form", childPrefix, child.ID))
			}
		}
		validateFormulaCallStep(child, errs, childPrefix)

		if child.Priority != nil && (*child.Priority < 0 || *child.Priority > 4) {
			*errs = append(*errs, fmt.Sprintf("%s (%s): priority must be 0-4", childPrefix, child.ID))
		}

		if err := validateStepTimeout(childPrefix, child.ID, child.Timeout, child.Retry != nil, nil, true); err != "" {
			*errs = append(*errs, err)
		}
		validateLoopBodyTimeouts(child.Loop, errs, fmt.Sprintf("%s (%s).loop", childPrefix, child.ID), nil, true)

		if child.Retry != nil {
			validateRetry(child.Retry, errs, fmt.Sprintf("%s (%s)", childPrefix, child.ID), child)
		}

		collectChildIDs(child.Children, idLocations, errs, childPrefix)
	}
}

func validateFormulaCallStep(step *Step, errs *[]string, prefix string) {
	if step == nil || !strings.EqualFold(strings.TrimSpace(step.Execution), "formula") {
		return
	}
	if strings.TrimSpace(step.Formula) == "" {
		*errs = append(*errs, fmt.Sprintf("%s (%s): execution=formula requires formula", prefix, step.ID))
	} else if !formulaNamePattern.MatchString(strings.TrimSpace(step.Formula)) {
		*errs = append(*errs, fmt.Sprintf("%s (%s): formula target must contain only letters, numbers, dot, underscore, and hyphen", prefix, step.ID))
	}
	if step.Embed != "" || step.Expand != "" || step.Loop != nil || len(step.Children) > 0 || step.Agent != nil || step.Script != nil || step.ExternalAgent != nil || step.Form != nil {
		*errs = append(*errs, fmt.Sprintf("%s (%s): formula cannot be combined with embed/expand/loop/children/agent/script/external_agent/form", prefix, step.ID))
	}
}

func validateWaitsFor(value string, stepIDLocations map[string]string) error {
	if value == "all-children" || value == "any-children" {
		return nil
	}

	if strings.HasPrefix(value, "children-of(") && strings.HasSuffix(value, ")") {
		stepID := value[len("children-of(") : len(value)-1]
		if stepID == "" {
			return fmt.Errorf("waits_for children-of() requires a step ID")
		}
		if _, exists := stepIDLocations[stepID]; !exists {
			return fmt.Errorf("waits_for references unknown step %q in children-of()", stepID)
		}
		return nil
	}

	return fmt.Errorf("waits_for has invalid value %q (must be all-children, any-children, or children-of(step-id))", value)
}

func validateChildDependsOn(children []*Step, idLocations map[string]string, errs *[]string, prefix string) {
	for i, child := range children {
		childPrefix := fmt.Sprintf("%s.children[%d]", prefix, i)
		for _, dep := range child.DependsOn {
			if _, exists := idLocations[dep]; !exists {
				*errs = append(*errs, fmt.Sprintf("%s (%s): depends_on references unknown step %q", childPrefix, child.ID, dep))
			}
		}
		for _, need := range child.Needs {
			if _, exists := idLocations[need]; !exists {
				*errs = append(*errs, fmt.Sprintf("%s (%s): needs references unknown step %q", childPrefix, child.ID, need))
			}
		}
		if child.WaitsFor != "" {
			if err := validateWaitsFor(child.WaitsFor, idLocations); err != nil {
				*errs = append(*errs, fmt.Sprintf("%s (%s): %s", childPrefix, child.ID, err.Error()))
			}
		}
		if child.OnComplete != nil {
			validateOnComplete(child.OnComplete, errs, fmt.Sprintf("%s (%s)", childPrefix, child.ID))
		}
		validateChildDependsOn(child.Children, idLocations, errs, childPrefix)
	}
}

func validateOnComplete(oc *OnCompleteSpec, errs *[]string, prefix string) {
	if oc.ForEach != "" && oc.Bond == "" {
		*errs = append(*errs, fmt.Sprintf("%s.on_complete: bond is required when for_each is set", prefix))
	}
	if oc.ForEach == "" && oc.Bond != "" {
		*errs = append(*errs, fmt.Sprintf("%s.on_complete: for_each is required when bond is set", prefix))
	}
	if oc.ForEach != "" && !strings.HasPrefix(oc.ForEach, "output.") {
		*errs = append(*errs, fmt.Sprintf("%s.on_complete: for_each must start with 'output.' (got %q)", prefix, oc.ForEach))
	}
	if oc.Parallel && oc.Sequential {
		*errs = append(*errs, fmt.Sprintf("%s.on_complete: cannot set both parallel and sequential", prefix))
	}
}

func validateRetry(spec *RetrySpec, errs *[]string, prefix string, step *Step) {
	if spec.MaxAttempts < 1 {
		*errs = append(*errs, fmt.Sprintf("%s.retry: max_attempts must be >= 1", prefix))
	}
	switch spec.OnExhausted {
	case "", "hard_fail", "soft_fail":
	default:
		*errs = append(*errs, fmt.Sprintf("%s.retry: unsupported on_exhausted %q (want hard_fail or soft_fail)", prefix, spec.OnExhausted))
	}

	if step.Loop != nil {
		*errs = append(*errs, fmt.Sprintf("%s: retry cannot be combined with loop", prefix))
	}
	if step.OnComplete != nil {
		*errs = append(*errs, fmt.Sprintf("%s: retry cannot be combined with on_complete", prefix))
	}
	if step.Gate != nil {
		*errs = append(*errs, fmt.Sprintf("%s: retry cannot be combined with gate", prefix))
	}
	if step.Expand != "" {
		*errs = append(*errs, fmt.Sprintf("%s: retry cannot be combined with expand", prefix))
	}
	if len(step.Children) > 0 {
		*errs = append(*errs, fmt.Sprintf("%s: retry cannot be combined with children", prefix))
	}
}
