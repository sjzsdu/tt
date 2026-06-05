package spec

import (
	"fmt"
	"regexp"
	"strings"
)

// varPattern matches {{variable}} placeholders.
var varPattern = regexp.MustCompile(`\{\{([a-zA-Z_][a-zA-Z0-9_]*)\}\}`)

// ExtractVariables finds all {{variable}} references in a formula.
func ExtractVariables(formula *Formula) []string {
	seen := make(map[string]bool)
	var vars []string

	extract := func(s string) {
		matches := varPattern.FindAllStringSubmatch(s, -1)
		for _, match := range matches {
			if len(match) >= 2 && !seen[match[1]] {
				seen[match[1]] = true
				vars = append(vars, match[1])
			}
		}
	}

	extract(formula.Description)

	var extractFromStep func(*Step)
	extractFromStep = func(step *Step) {
		extract(step.Title)
		extract(step.Description)
		extract(step.Assignee)
		extract(step.Condition)
		for _, l := range step.Labels {
			extract(l)
		}
		for _, child := range step.Children {
			extractFromStep(child)
		}
	}

	for _, step := range formula.Steps {
		extractFromStep(step)
	}

	return vars
}

// Substitute replaces {{variable}} placeholders with values.
func Substitute(s string, vars map[string]string) string {
	return varPattern.ReplaceAllStringFunc(s, func(match string) string {
		name := match[2 : len(match)-2]
		if val, ok := vars[name]; ok {
			return val
		}
		return match
	})
}

// CheckResidualVars returns the names of any {{...}} placeholders remaining
// after substitution.
func CheckResidualVars(s string) []string {
	matches := varPattern.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		if seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		names = append(names, m[1])
	}
	return names
}

// ValidateVars checks that all required variables are provided
// and all values pass their constraints.
func ValidateVars(formula *Formula, values map[string]string) error {
	errs, _ := CollectVarValidationErrors(formula.Vars, values)
	return formatVarValidationErrors(errs)
}

// ValidateVarDefs validates explicit var definitions against provided values.
func ValidateVarDefs(defs map[string]*VarDef, values map[string]string) error {
	errs, _ := CollectVarValidationErrors(defs, values)
	return formatVarValidationErrors(errs)
}

// CollectVarValidationErrors validates explicit var definitions against the
// provided values and returns raw error strings plus the set of missing
// required vars.
func CollectVarValidationErrors(defs map[string]*VarDef, values map[string]string) ([]string, map[string]bool) {
	return collectVarValidationErrors(defs, values, true)
}

func collectVarValidationErrors(defs map[string]*VarDef, values map[string]string, requireMissing bool) ([]string, map[string]bool) {
	var errs []string
	missingRequired := make(map[string]bool)
	names := make([]string, 0, len(defs))
	for name := range defs {
		names = append(names, name)
	}

	for _, name := range names {
		def := defs[name]
		if def == nil {
			continue
		}
		val, provided := values[name]

		if requireMissing && def.Required && !provided {
			errs = append(errs, fmt.Sprintf("variable %q is required", name))
			missingRequired[name] = true
			continue
		}

		if !provided && def.Default != nil {
			val = *def.Default
		}

		if val == "" {
			continue
		}

		if len(def.Enum) > 0 {
			found := false
			for _, allowed := range def.Enum {
				if val == allowed {
					found = true
					break
				}
			}
			if !found {
				errs = append(errs, fmt.Sprintf("variable %q: value %q not in allowed values %v", name, val, def.Enum))
			}
		}

		if def.Pattern != "" {
			re, err := regexp.Compile(def.Pattern)
			if err != nil {
				errs = append(errs, fmt.Sprintf("variable %q: invalid pattern %q: %v", name, def.Pattern, err))
			} else if !re.MatchString(val) {
				errs = append(errs, fmt.Sprintf("variable %q: value %q does not match pattern %q", name, val, def.Pattern))
			}
		}
	}

	if len(missingRequired) == 0 {
		missingRequired = nil
	}
	return errs, missingRequired
}

func formatVarValidationErrors(errs []string) error {
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("variable validation failed:\n  - %s", strings.Join(errs, "\n  - "))
}

// ApplyDefaults returns a new map with default values filled in.
func ApplyDefaults(formula *Formula, values map[string]string) map[string]string {
	result := make(map[string]string)

	for k, v := range values {
		result[k] = v
	}

	for name, def := range formula.Vars {
		if _, exists := result[name]; !exists && def != nil && def.Default != nil {
			result[name] = *def.Default
		}
	}

	return result
}
