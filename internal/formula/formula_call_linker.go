package formula

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/spec"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

const maxLinkedFormulaDepth = 16

var formulaIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type linkedFormulaDefinition struct {
	formula *spec.Formula
	hash    string
}

type linkedFormulaCall struct {
	step     *spec.Step
	parent   *spec.Formula
	parallel bool
}

type formulaCallLinker struct {
	parser      *Parser
	definitions map[string]*linkedFormulaDefinition
	hashes      map[string]string
}

func linkFormulaCalls(workflow *ir.Workflow, rootName string, searchPaths []string) error {
	if workflow == nil {
		return nil
	}
	linker := &formulaCallLinker{
		parser:      NewParser(searchPaths...),
		definitions: map[string]*linkedFormulaDefinition{},
		hashes:      map[string]string{},
	}
	root, err := linker.load(rootName)
	if err != nil {
		return fmt.Errorf("link formula calls: %w", err)
	}
	if err := linker.visit(root, nil, map[string]bool{}); err != nil {
		return fmt.Errorf("link formula calls: %w", err)
	}
	workflow.DefinitionHash = linkedDefinitionHash(linker.hashes)
	return nil
}

// ValidateFormulaCallGraph resolves and validates every FormulaCall reachable
// from formula. The formula itself is used as the root so validation also works
// when its filename is different from its canonical formula name.
func ValidateFormulaCallGraph(formula *spec.Formula, searchPaths []string) error {
	if formula == nil {
		return fmt.Errorf("formula is required")
	}
	linker := &formulaCallLinker{
		parser:      NewParser(searchPaths...),
		definitions: map[string]*linkedFormulaDefinition{},
		hashes:      map[string]string{},
	}
	resolved, err := linker.parser.Resolve(formula)
	if err != nil {
		return fmt.Errorf("link formula calls: %w", err)
	}
	root, err := linker.register(resolved, formula.Formula)
	if err != nil {
		return fmt.Errorf("link formula calls: %w", err)
	}
	if err := linker.visit(root, nil, map[string]bool{}); err != nil {
		return fmt.Errorf("link formula calls: %w", err)
	}
	return nil
}

func (l *formulaCallLinker) load(requestedName string) (*linkedFormulaDefinition, error) {
	requestedName = strings.TrimSpace(requestedName)
	if !formulaIdentifierPattern.MatchString(requestedName) {
		return nil, fmt.Errorf("invalid formula name %q", requestedName)
	}
	loaded, err := l.parser.LoadByName(requestedName)
	if err != nil {
		return nil, err
	}
	resolved, err := l.parser.Resolve(loaded)
	if err != nil {
		return nil, err
	}
	return l.register(resolved, requestedName)
}

func (l *formulaCallLinker) register(resolved *spec.Formula, requestedName string) (*linkedFormulaDefinition, error) {
	canonical := strings.TrimSpace(resolved.Formula)
	if !formulaIdentifierPattern.MatchString(canonical) {
		return nil, fmt.Errorf("formula %q resolves to invalid canonical name %q", requestedName, canonical)
	}
	raw, err := json.Marshal(resolved)
	if err != nil {
		return nil, fmt.Errorf("hash formula %q: %w", canonical, err)
	}
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	if existing := l.definitions[canonical]; existing != nil {
		if existing.hash != digest {
			return nil, fmt.Errorf("formula name %q resolves to multiple different definitions", canonical)
		}
		return existing, nil
	}
	definition := &linkedFormulaDefinition{formula: resolved, hash: digest}
	l.definitions[canonical] = definition
	l.hashes[canonical] = definition.hash
	return definition, nil
}

func (l *formulaCallLinker) visit(definition *linkedFormulaDefinition, stack []string, included map[string]bool) error {
	canonical := definition.formula.Formula
	if len(stack) >= maxLinkedFormulaDepth {
		return fmt.Errorf("formula call depth exceeds %d: %s", maxLinkedFormulaDepth, strings.Join(append(stack, canonical), " -> "))
	}
	for _, ancestor := range stack {
		if ancestor == canonical {
			return fmt.Errorf("formula call cycle: %s", strings.Join(append(stack, canonical), " -> "))
		}
	}
	if included[canonical] {
		return nil
	}
	included[canonical] = true

	calls, err := l.collectEffectiveCalls(definition.formula, map[string]bool{})
	if err != nil {
		return err
	}
	for _, call := range calls {
		if call.parallel && !call.step.AllowParallel {
			return fmt.Errorf("formula %q step %q is inside a parallel loop; set allow_parallel=true only when the child uses isolated or read-only resources", call.parent.Formula, call.step.ID)
		}
		child, err := l.load(call.step.Formula)
		if err != nil {
			return fmt.Errorf("formula %q step %q target %q: %w", call.parent.Formula, call.step.ID, call.step.Formula, err)
		}
		if err := validateLinkedFormulaContract(call.parent, call.step, child.formula); err != nil {
			return err
		}
		if err := l.visit(child, append(stack, canonical), included); err != nil {
			return err
		}
	}
	return nil
}

func (l *formulaCallLinker) collectEffectiveCalls(formula *spec.Formula, embedded map[string]bool) ([]linkedFormulaCall, error) {
	var calls []linkedFormulaCall
	var walk func([]*spec.Step, bool, string) error
	walk = func(items []*spec.Step, parallel bool, prefix string) error {
		for _, step := range items {
			if step == nil {
				continue
			}
			location := strings.Trim(strings.Join([]string{prefix, step.ID}, "."), ".")
			if strings.EqualFold(strings.TrimSpace(step.Execution), "formula") {
				calls = append(calls, linkedFormulaCall{step: step, parent: formula, parallel: parallel})
			}
			for _, includedName := range []string{step.Embed, step.Expand} {
				includedName = strings.TrimSpace(includedName)
				if includedName == "" || embedded[includedName] {
					continue
				}
				embedded[includedName] = true
				includedDefinition, err := l.load(includedName)
				if err != nil {
					return fmt.Errorf("formula %q step %q includes %q: %w", formula.Formula, step.ID, includedName, err)
				}
				includedCalls, err := l.collectEffectiveCalls(includedDefinition.formula, embedded)
				if err != nil {
					return err
				}
				for i := range includedCalls {
					includedCalls[i].parallel = includedCalls[i].parallel || parallel
				}
				calls = append(calls, includedCalls...)
			}
			if err := walk(step.Children, parallel, location); err != nil {
				return err
			}
			if step.Loop != nil {
				if err := walk(step.Loop.Body, parallel || step.Loop.Parallel, location); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(formula.Steps, false, ""); err != nil {
		return nil, err
	}
	return calls, nil
}

func validateLinkedFormulaContract(parent *spec.Formula, call *spec.Step, child *spec.Formula) error {
	for name := range call.With {
		if _, ok := child.Vars[name]; !ok {
			return fmt.Errorf("formula %q step %q passes unknown input %q to %q", parent.Formula, call.ID, name, child.Formula)
		}
	}
	for name, definition := range child.Vars {
		if definition != nil && definition.Required {
			if _, ok := call.With[name]; !ok {
				return fmt.Errorf("formula %q step %q does not bind required input %q for %q", parent.Formula, call.ID, name, child.Formula)
			}
		}
	}
	for name, binding := range call.With {
		childVar := child.Vars[name]
		if childVar == nil || strings.TrimSpace(childVar.Type) == "" {
			continue
		}
		if parentName, ok := exactTemplateReference(binding); ok {
			parentVar := parent.Vars[parentName]
			if parentVar != nil && !compatibleFormulaTypes(parentVar.Type, childVar.Type) {
				return fmt.Errorf("formula %q step %q input %q type %q is incompatible with child type %q", parent.Formula, call.ID, name, parentVar.Type, childVar.Type)
			}
		}
	}

	outputs := publicFormulaOutputs(child)
	for _, reference := range formulaCallOutputReferences(parent, call.ID) {
		if _, ok := outputs[reference]; !ok {
			return fmt.Errorf("formula %q references undeclared output %q on step %q calling %q", parent.Formula, reference, call.ID, child.Formula)
		}
	}
	return nil
}

func exactTemplateReference(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "{{") || !strings.HasSuffix(value, "}}") {
		return "", false
	}
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "{{"), "}}"))
	if inner == "" || strings.ContainsAny(inner, ". []()") {
		return "", false
	}
	return inner, true
}

func compatibleFormulaTypes(parent, child string) bool {
	normalize := func(value string) string {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "bool":
			return "boolean"
		case "int", "integer", "float":
			return "number"
		case "map":
			return "object"
		default:
			return strings.ToLower(strings.TrimSpace(value))
		}
	}
	parent, child = normalize(parent), normalize(child)
	return parent == "" || child == "" || parent == child
}

func publicFormulaOutputs(formula *spec.Formula) map[string]struct{} {
	outputs := make(map[string]struct{}, len(formula.Outputs)+1)
	for name := range formula.Outputs {
		outputs[name] = struct{}{}
	}
	if _, ok := outputs[steps.OutputReport]; !ok && formula.GetStepByID("final-report") != nil {
		outputs[steps.OutputReport] = struct{}{}
	}
	return outputs
}

func formulaCallOutputReferences(formula *spec.Formula, callID string) []string {
	pattern := regexp.MustCompile(`(?:^|[^A-Za-z0-9_-])` + regexp.QuoteMeta(callID) + `\.([A-Za-z0-9_-]+)`)
	seen := map[string]struct{}{}
	collect := func(value string) {
		for _, match := range pattern.FindAllStringSubmatch(value, -1) {
			seen[match[1]] = struct{}{}
		}
	}
	for _, output := range formula.Outputs {
		if output != nil {
			collect(output.From)
		}
	}
	var walk func([]*spec.Step)
	walk = func(items []*spec.Step) {
		for _, step := range items {
			if step == nil {
				continue
			}
			collect(step.Description)
			collect(step.Condition)
			for _, value := range step.InputCtx {
				collect(value)
			}
			for _, value := range step.With {
				collect(value)
			}
			walk(step.Children)
			if step.Loop != nil {
				collect(step.Loop.ForEach)
				collect(step.Loop.Until)
				walk(step.Loop.Body)
			}
		}
	}
	walk(formula.Steps)
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func linkedDefinitionHash(hashes map[string]string) string {
	names := make([]string, 0, len(hashes))
	for name := range hashes {
		names = append(names, name)
	}
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		_, _ = hash.Write([]byte(name + "\x00" + hashes[name] + "\n"))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
