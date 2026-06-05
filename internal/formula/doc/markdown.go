package doc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/spec"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

func GenerateMarkdown(f *spec.Formula, workflow *ir.Workflow) string {
	if f == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("title: \"%s\"\n", escapeYAML(f.Formula)))
	if f.Description != "" {
		b.WriteString(fmt.Sprintf("description: \"%s\"\n", escapeYAML(f.Description)))
	}
	b.WriteString("---\n\n")
	b.WriteString(fmt.Sprintf("# %s\n\n", f.Formula))
	if f.Description != "" {
		b.WriteString(fmt.Sprintf("> %s\n\n", f.Description))
	}

	b.WriteString("## Formula Details\n\n")
	b.WriteString(fmt.Sprintf("- **Version:** `%d`\n", f.Version))
	b.WriteString(fmt.Sprintf("- **Type:** `%s`\n", f.Type))
	if f.Phase != "" {
		b.WriteString(fmt.Sprintf("- **Phase:** `%s`\n", f.Phase))
	}
	stepCount, loopBodyCount := formulaAuthoredStepCounts(f.Steps)
	b.WriteString(fmt.Sprintf("- **Steps:** `%d`\n", stepCount))
	if loopBodyCount > 0 {
		b.WriteString(fmt.Sprintf("- **Loop body steps:** `%d`\n", loopBodyCount))
	}
	b.WriteString("\n")

	if len(f.Vars) > 0 {
		b.WriteString("## Variables\n\n")
		b.WriteString("| Name | Description | Default | Required |\n")
		b.WriteString("|------|-------------|---------|----------|\n")
		for _, vname := range sortedVarNames(f.Vars) {
			def := f.Vars[vname]
			if def == nil {
				continue
			}
			desc := def.Description
			if desc == "" {
				desc = "-"
			}
			defVal := "-"
			if def.Default != nil {
				defVal = fmt.Sprintf("`%s`", *def.Default)
			}
			req := ""
			if def.Required {
				req = "✅"
			}
			b.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s |\n", vname, desc, defVal, req))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Dependency Graph\n\n")
	b.WriteString("```mermaid\n")
	b.WriteString(GenerateMermaidGraph(workflow))
	b.WriteString("\n```\n\n")

	b.WriteString("## Quick Start\n\n")
	b.WriteString(generateQuickStart(f))
	b.WriteString("\n")

	b.WriteString("## Steps\n\n")
	stepOrder := authoredStepOrder(f.Steps)
	for i, step := range orderedWorkflowSteps(workflow, stepOrder) {
		meta := step.Meta()
		b.WriteString(fmt.Sprintf("### %d. `%s`\n\n", i+1, meta.ID))
		if meta.Title != "" {
			b.WriteString(fmt.Sprintf("**%s**\n\n", meta.Title))
		}
		switch s := step.(type) {
		case steps.AgentStep:
			if s.Prompt != "" {
				b.WriteString(fmt.Sprintf("%s\n\n", s.Prompt))
			}
			if s.Agent != "" || s.Model != "" {
				b.WriteString(fmt.Sprintf("**Agent:** `%s`  **Model:** `%s`\n\n", emptyDefault(s.Agent, "default"), emptyDefault(s.Model, "default")))
			}
			if s.DynamicForm {
				b.WriteString("**Dynamic form:** enabled\n\n")
			}
		case steps.ScriptStep:
			writeScriptMarkdown(&b, s)
		case steps.HumanInputStep:
			b.WriteString("**Execution:** human input\n\n")
			if s.Reason != "" {
				b.WriteString(fmt.Sprintf("%s\n\n", s.Reason))
			}
		case steps.LoopStep:
			writeLoopMarkdown(&b, s)
		case *steps.LoopStep:
			writeLoopMarkdown(&b, *s)
		}
		deps := workflowDepsForStep(workflow, ir.NodeID(meta.ID))
		if len(deps) > 0 {
			b.WriteString(fmt.Sprintf("**Dependencies:** `%s`\n\n", strings.Join(deps, "`, `")))
		}
		if len(meta.Labels) > 0 {
			b.WriteString(fmt.Sprintf("**Labels:** %s\n\n", strings.Join(meta.Labels, ", ")))
		}
	}
	return b.String()
}

func writeScriptMarkdown(b *strings.Builder, step steps.ScriptStep) {
	if b == nil {
		return
	}
	b.WriteString("**Execution:** script\n\n")
	if step.Cwd != "" {
		b.WriteString(fmt.Sprintf("**Working directory:** `%s`\n\n", step.Cwd))
	}
	if step.OutputKey != "" {
		b.WriteString(fmt.Sprintf("**Output key:** `%s`\n\n", step.OutputKey))
	}
	if step.Validation != nil {
		b.WriteString(fmt.Sprintf("**Validation:** `%s`", emptyDefault(step.Validation.Format, "configured")))
		if len(step.Validation.Required) > 0 {
			b.WriteString(fmt.Sprintf("; required: `%s`", strings.Join(step.Validation.Required, "`, `")))
		}
		b.WriteString("\n\n")
	}
	if len(step.Command) == 0 {
		return
	}
	summary := scriptCommandSummary(step.Command)
	b.WriteString(fmt.Sprintf("**Command summary:** `%s`\n\n", summary))
	b.WriteString("<details>\n<summary>Show script command</summary>\n\n")
	b.WriteString("```bash\n")
	b.WriteString(scriptCommandBody(step.Command))
	b.WriteString("\n```\n\n")
	b.WriteString("</details>\n\n")
}

func writeLoopMarkdown(b *strings.Builder, loop steps.LoopStep) {
	if b == nil {
		return
	}
	b.WriteString("**Execution:** loop\n\n")
	if loop.Until != "" {
		b.WriteString(fmt.Sprintf("**Until:** `%s`\n\n", loop.Until))
	}
	if loop.Max > 0 {
		b.WriteString(fmt.Sprintf("**Max iterations:** `%d`\n\n", loop.Max))
	}
	if loop.Parallel {
		b.WriteString("**Parallel:** enabled\n\n")
	}
	if loop.MaxConcurrency > 0 {
		b.WriteString(fmt.Sprintf("**Max concurrency:** `%d`\n\n", loop.MaxConcurrency))
	}
	if len(loop.Body) == 0 {
		return
	}
	b.WriteString("#### Loop body\n\n")
	for i, child := range loop.Body {
		if child == nil {
			continue
		}
		meta := child.Meta()
		b.WriteString(fmt.Sprintf("%d. `%s`", i+1, meta.ID))
		if meta.Title != "" {
			b.WriteString(fmt.Sprintf(" - %s", meta.Title))
		}
		b.WriteString("\n")
		if len(meta.DependsOn) > 0 {
			deps := make([]string, 0, len(meta.DependsOn))
			for _, dep := range meta.DependsOn {
				deps = append(deps, string(dep))
			}
			b.WriteString(fmt.Sprintf("   - depends on: `%s`\n", strings.Join(deps, "`, `")))
		}
		if meta.Condition != "" {
			b.WriteString(fmt.Sprintf("   - condition: `%s`\n", meta.Condition))
		}
		switch typed := child.(type) {
		case steps.AgentStep:
			if typed.Agent != "" || typed.Model != "" {
				b.WriteString(fmt.Sprintf("   - agent: `%s`, model: `%s`\n", emptyDefault(typed.Agent, "default"), emptyDefault(typed.Model, "default")))
			}
			if len(typed.InputCtx) > 0 {
				b.WriteString(fmt.Sprintf("   - input context: `%s`\n", strings.Join(typed.InputCtx, "`, `")))
			}
		case steps.ScriptStep:
			b.WriteString("   - execution: `script`\n")
			if len(typed.Command) > 0 {
				b.WriteString(fmt.Sprintf("   - command: `%s`\n", scriptCommandSummary(typed.Command)))
			}
		case steps.HumanInputStep:
			b.WriteString("   - execution: `human_input`\n")
		}
	}
	b.WriteString("\n")
}

func GenerateMermaidGraph(workflow *ir.Workflow) string {
	var b strings.Builder
	b.WriteString("graph TD\n")
	stepsList := orderedWorkflowSteps(workflow, nil)
	if len(stepsList) == 0 {
		b.WriteString("  empty[No steps]\n")
		return b.String()
	}
	ids := map[ir.NodeID]struct{}{}
	for _, step := range stepsList {
		meta := step.Meta()
		id := ir.NodeID(meta.ID)
		ids[id] = struct{}{}
		b.WriteString(fmt.Sprintf("  %s[%s]\n", mermaidNodeID(string(id)), mermaidLabel(nonEmpty(meta.Title, string(meta.ID)))))
		writeLoopMermaidSubgraph(&b, step)
	}
	if workflow != nil {
		for _, edge := range workflow.Graph.Edges {
			if _, ok := ids[edge.From]; !ok {
				continue
			}
			if _, ok := ids[edge.To]; !ok {
				continue
			}
			b.WriteString(fmt.Sprintf("  %s --> %s\n", mermaidNodeID(string(edge.From)), mermaidNodeID(string(edge.To))))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeLoopMermaidSubgraph(b *strings.Builder, step steps.Step) {
	if b == nil || step == nil {
		return
	}
	var loop steps.LoopStep
	switch typed := step.(type) {
	case steps.LoopStep:
		loop = typed
	case *steps.LoopStep:
		loop = *typed
	default:
		return
	}
	if len(loop.Body) == 0 {
		return
	}
	parentID := string(step.Meta().ID)
	b.WriteString(fmt.Sprintf("  subgraph %s [%s]\n", mermaidNodeID(parentID)+"_loop", mermaidLabel(parentID+" loop body")))
	bodyIDs := map[string]struct{}{}
	for _, child := range loop.Body {
		if child == nil {
			continue
		}
		meta := child.Meta()
		bodyIDs[string(meta.ID)] = struct{}{}
		b.WriteString(fmt.Sprintf("    %s[%s]\n", mermaidNodeID(parentID+"__"+string(meta.ID)), mermaidLabel(nonEmpty(meta.Title, string(meta.ID)))))
	}
	for i, child := range loop.Body {
		if child == nil {
			continue
		}
		meta := child.Meta()
		to := mermaidNodeID(parentID + "__" + string(meta.ID))
		deps := meta.DependsOn
		if len(deps) == 0 && i > 0 && loop.Body[i-1] != nil {
			deps = []steps.ID{loop.Body[i-1].Meta().ID}
		}
		for _, dep := range deps {
			if _, ok := bodyIDs[string(dep)]; !ok {
				continue
			}
			b.WriteString(fmt.Sprintf("    %s --> %s\n", mermaidNodeID(parentID+"__"+string(dep)), to))
		}
	}
	b.WriteString("  end\n")
	first := firstLoopBodyID(loop)
	if first != "" {
		b.WriteString(fmt.Sprintf("  %s -. enters .-> %s\n", mermaidNodeID(parentID), mermaidNodeID(parentID+"__"+first)))
	}
}

func firstLoopBodyID(loop steps.LoopStep) string {
	for _, child := range loop.Body {
		if child != nil {
			return string(child.Meta().ID)
		}
	}
	return ""
}

func sortedVarNames(vars map[string]*spec.VarDef) []string {
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func formulaAuthoredStepCounts(items []*spec.Step) (int, int) {
	stepCount := 0
	loopBodyCount := 0
	var walk func([]*spec.Step)
	walk = func(items []*spec.Step) {
		for _, step := range items {
			if step == nil {
				continue
			}
			stepCount++
			if step.Loop != nil {
				loopBodyCount += len(step.Loop.Body)
			}
			walk(step.Children)
		}
	}
	walk(items)
	return stepCount, loopBodyCount
}

func realWorkflowSteps(workflow *ir.Workflow) []steps.Step {
	return orderedWorkflowSteps(workflow, nil)
}

func orderedWorkflowSteps(workflow *ir.Workflow, order map[string]int) []steps.Step {
	if workflow == nil {
		return nil
	}
	nodes := make(map[ir.NodeID]steps.Step, len(workflow.Graph.Nodes))
	for id, node := range workflow.Graph.Nodes {
		if node == nil || node.Step == nil || node.Step.Meta().Kind == steps.KindNoop {
			continue
		}
		nodes[id] = node.Step
	}
	if len(nodes) == 0 {
		return nil
	}
	inDegree := make(map[ir.NodeID]int, len(nodes))
	outgoing := make(map[ir.NodeID][]ir.NodeID, len(nodes))
	for id := range nodes {
		inDegree[id] = 0
	}
	for _, edge := range workflow.Graph.Edges {
		if _, ok := nodes[edge.From]; !ok {
			continue
		}
		if _, ok := nodes[edge.To]; !ok {
			continue
		}
		outgoing[edge.From] = append(outgoing[edge.From], edge.To)
		inDegree[edge.To]++
	}
	ready := make([]ir.NodeID, 0, len(nodes))
	for id, degree := range inDegree {
		if degree == 0 {
			ready = append(ready, id)
		}
	}
	sortNodeIDs(ready, order)
	out := make([]steps.Step, 0, len(nodes))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		out = append(out, nodes[id])
		children := outgoing[id]
		sortNodeIDs(children, order)
		for _, child := range children {
			inDegree[child]--
			if inDegree[child] == 0 {
				ready = append(ready, child)
			}
		}
		sortNodeIDs(ready, order)
	}
	if len(out) == len(nodes) {
		return out
	}
	// Fall back to deterministic ID ordering if the graph is cyclic or malformed.
	fallback := make([]steps.Step, 0, len(nodes))
	for _, step := range nodes {
		fallback = append(fallback, step)
	}
	sort.Slice(fallback, func(i, j int) bool { return fallback[i].Meta().ID < fallback[j].Meta().ID })
	return fallback
}

func authoredStepOrder(items []*spec.Step) map[string]int {
	order := map[string]int{}
	var index int
	var walk func([]*spec.Step)
	walk = func(items []*spec.Step) {
		for _, item := range items {
			if item == nil {
				continue
			}
			if item.ID != "" {
				if _, exists := order[item.ID]; !exists {
					order[item.ID] = index
					index++
				}
			}
			if item.Loop != nil {
				walk(item.Loop.Body)
			}
			walk(item.Children)
		}
	}
	walk(items)
	return order
}

func sortNodeIDs(ids []ir.NodeID, order map[string]int) {
	sort.Slice(ids, func(i, j int) bool {
		left := string(ids[i])
		right := string(ids[j])
		leftOrder, leftOK := order[left]
		rightOrder, rightOK := order[right]
		if leftOK && rightOK && leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		if leftOK != rightOK {
			return leftOK
		}
		return left < right
	})
}

func scriptCommandSummary(command []string) string {
	if len(command) == 0 {
		return "empty"
	}
	if len(command) >= 3 && (command[0] == "bash" || command[0] == "sh") && command[1] == "-lc" {
		lineCount := len(strings.Split(strings.TrimSpace(command[2]), "\n"))
		return fmt.Sprintf("%s -lc (%d lines)", command[0], lineCount)
	}
	joined := strings.Join(command, " ")
	const maxSummaryLength = 96
	if len(joined) <= maxSummaryLength {
		return joined
	}
	return joined[:maxSummaryLength-1] + "…"
}

func scriptCommandBody(command []string) string {
	if len(command) >= 3 && (command[0] == "bash" || command[0] == "sh") && command[1] == "-lc" {
		return strings.TrimRight(command[2], "\n")
	}
	return strings.TrimRight(strings.Join(command, " "), "\n")
}

func workflowDepsForStep(workflow *ir.Workflow, id ir.NodeID) []string {
	if workflow == nil {
		return nil
	}
	deps := []string{}
	for _, edge := range workflow.Graph.Edges {
		if edge.To == id {
			deps = append(deps, string(edge.From))
		}
	}
	sort.Strings(deps)
	return deps
}

func generateQuickStart(f *spec.Formula) string {
	var b strings.Builder
	b.WriteString("```bash\n")
	b.WriteString(fmt.Sprintf("tt formula show %s\n", f.Formula))
	b.WriteString(fmt.Sprintf("tt formula compile %s\n", f.Formula))
	b.WriteString(fmt.Sprintf("tt formula run %s", f.Formula))
	for _, vname := range sortedVarNames(f.Vars) {
		def := f.Vars[vname]
		if def != nil && def.Required && def.Default == nil {
			b.WriteString(fmt.Sprintf(" --var %s=<value>", vname))
		}
	}
	b.WriteString("\n```\n")
	return b.String()
}

func escapeYAML(s string) string   { return strings.ReplaceAll(s, "\"", "\\\"") }
func mermaidLabel(s string) string { return fmt.Sprintf("\"%s\"", escapeMermaidLabel(s)) }
func escapeMermaidLabel(s string) string {
	s = strings.NewReplacer(
		"\n", " ",
		"\r", " ",
		"\"", "'",
		"[", "(",
		"]", ")",
		"{", "(",
		"}", ")",
	).Replace(s)
	return strings.TrimSpace(s)
}
func mermaidNodeID(id string) string {
	id = strings.NewReplacer(".", "_", "-", "_", ":", "_").Replace(id)
	if id == "" {
		return "node"
	}
	return id
}
func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
func emptyDefault(value, fallback string) string { return nonEmpty(value, fallback) }
