package formuladoc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sjzsdu/tt/internal/formula"
)

func sortedVarNames(vars map[string]*formula.VarDef) []string {
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func GenerateMarkdown(f *formula.Formula, recipe *formula.Recipe) string {
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
	b.WriteString(GenerateMermaidGraph(recipe))
	b.WriteString("\n```\n\n")

	b.WriteString("## Quick Start\n\n")
	b.WriteString(generateQuickStart(f, recipe))
	b.WriteString("\n")

	b.WriteString("## Steps\n\n")
	displayIndex := 1
	for _, step := range recipe.Steps {
		if step.IsRoot || isGeneratedBoundaryRecipeStep(step) {
			continue
		}
		priority := ""
		if step.Priority != nil {
			priority = fmt.Sprintf(" [P%d]", *step.Priority)
		}
		b.WriteString(fmt.Sprintf("### %d. `%s`%s\n\n", displayIndex, step.ID, priority))
		displayIndex++
		b.WriteString(fmt.Sprintf("**%s**\n\n", step.Title))
		if step.Description != "" {
			b.WriteString(fmt.Sprintf("%s\n\n", step.Description))
		}
		if step.Notes != "" {
			b.WriteString(fmt.Sprintf("> %s\n\n", step.Notes))
		}

		deps := findDepsForStep(recipe, step.ID)
		if len(deps) > 0 {
			b.WriteString(fmt.Sprintf("**Dependencies:** %s\n\n", strings.Join(deps, ", ")))
		}

		if len(step.Labels) > 0 {
			b.WriteString(fmt.Sprintf("**Labels:** %s\n\n", strings.Join(step.Labels, ", ")))
		}

		if step.Gate != nil {
			b.WriteString(fmt.Sprintf("**Gate:** %s (type: %s)\n\n", step.Gate.ID, step.Gate.Type))
		}

		if step.Loop != nil {
			b.WriteString(generateLoopMarkdown(step))
		}
	}

	return b.String()
}

func formulaAuthoredStepCounts(steps []*formula.Step) (int, int) {
	stepCount := 0
	loopBodyCount := 0
	var walk func([]*formula.Step)
	walk = func(items []*formula.Step) {
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
	walk(steps)
	return stepCount, loopBodyCount
}

func generateLoopMarkdown(step formula.RecipeStep) string {
	if step.Loop == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("#### Runtime Loop\n\n")
	if summary := loopSummary(step.Loop); summary != "" {
		b.WriteString(fmt.Sprintf("- **Control:** %s\n", summary))
	}
	if step.Condition != "" {
		b.WriteString(fmt.Sprintf("- **Step condition:** `%s`\n", step.Condition))
	}
	b.WriteString(fmt.Sprintf("- **Body steps:** `%d`\n\n", len(step.Loop.Body)))

	if len(step.Loop.Body) == 0 {
		return b.String()
	}
	b.WriteString("| # | Body Step | Title | Input | Output | Condition | Agent |\n")
	b.WriteString("|---|-----------|-------|-------|--------|-----------|-------|\n")
	for i, body := range step.Loop.Body {
		if body == nil {
			continue
		}
		input := "-"
		if len(body.InputCtx) > 0 {
			input = "`" + strings.Join(body.InputCtx, "`, `") + "`"
		}
		output := "-"
		if body.OutputKey != "" {
			output = "`" + body.OutputKey + "`"
		}
		condition := "-"
		if body.Condition != "" {
			condition = "`" + body.Condition + "`"
		}
		agent := "default"
		if body.Agent != nil && body.Agent.Name != "" {
			agent = "`" + body.Agent.Name + "`"
		}
		b.WriteString(fmt.Sprintf("| %d | `%s` | %s | %s | %s | %s | %s |\n",
			i+1,
			markdownCell(body.ID),
			markdownCell(body.Title),
			markdownCell(input),
			markdownCell(output),
			markdownCell(condition),
			markdownCell(agent),
		))
	}
	b.WriteString("\n")
	b.WriteString("> During execution, each loop body result is saved as `parent.iterN.body`, for example `")
	b.WriteString(step.ID)
	b.WriteString(".iter1.<body>`.\n\n")
	return b.String()
}

func GenerateMermaidGraph(recipe *formula.Recipe) string {
	var b strings.Builder
	b.WriteString("graph TD\n")

	parallelSteps := findParallelSteps(recipe)
	stepByID := recipeStepMap(recipe)
	depths := computeStepDepths(recipe)
	maxDepth := 1
	for _, d := range depths {
		if d > maxDepth {
			maxDepth = d
		}
	}

	for _, step := range recipe.Steps {
		if step.IsRoot || isGeneratedBoundaryRecipeStep(step) {
			continue
		}
		nodeID := mermaidNodeID(step.ID)
		label := mermaidLabel(step)
		shape := mermaidShape(step, nil, parallelSteps)
		b.WriteString(fmt.Sprintf("    %s%s%s\n", nodeID, shape.open, label+shape.close))

		depth := depths[step.ID]
		color := depthColor(depth, maxDepth)

		if isMermaidBoundaryStep(step) {
			b.WriteString(fmt.Sprintf("    class %s nodeBoundary\n", nodeID))
		} else if step.Gate != nil {
			b.WriteString(fmt.Sprintf("    class %s nodeGate\n", nodeID))
		} else if step.Condition != "" {
			b.WriteString(fmt.Sprintf("    class %s nodeCondition\n", nodeID))
		} else if step.Loop != nil {
			b.WriteString(fmt.Sprintf("    class %s nodeLoop\n", nodeID))
		} else {
			b.WriteString(fmt.Sprintf("    classDef c%s fill:%s,stroke:%s,stroke-width:2px\n", nodeID, color.Fill, color.Stroke))
			b.WriteString(fmt.Sprintf("    class %s c%s\n", nodeID, nodeID))
		}
	}

	for _, step := range recipe.Steps {
		if step.IsRoot || isGeneratedBoundaryRecipeStep(step) {
			continue
		}
		appendMermaidLoopBody(&b, step)
	}

	b.WriteString("    classDef nodeBoundary fill:#e8eaf6,stroke:#3f51b5,stroke-width:3px\n")
	b.WriteString("    classDef nodeGate fill:#fce4ec,stroke:#c2185b,stroke-width:2px,stroke-dasharray: 5 5\n")
	b.WriteString("    classDef nodeCondition fill:#e1f5fe,stroke:#0277bd,stroke-width:2px\n")
	b.WriteString("    classDef nodeLoop fill:#fff3e0,stroke:#ef6c00,stroke-width:2px\n")
	b.WriteString("    classDef nodeLoopBody fill:#fff8e1,stroke:#f9a825,stroke-width:1px,stroke-dasharray: 3 3\n")

	for _, dep := range recipe.Deps {
		if dep.Type == "parent-child" {
			continue
		}
		from := mermaidNodeID(dep.DependsOnID)
		to := mermaidNodeID(dep.StepID)
		if dep.StepID == "" || stepByID[dep.StepID].ID == "" || stepByID[dep.StepID].IsRoot || isGeneratedBoundaryRecipeStep(stepByID[dep.StepID]) {
			continue
		}
		if dep.DependsOnID == "" || stepByID[dep.DependsOnID].ID == "" || stepByID[dep.DependsOnID].IsRoot || isGeneratedBoundaryRecipeStep(stepByID[dep.DependsOnID]) {
			continue
		}
		edgeStyle := " -->"
		if dep.Type == "waits-for" {
			edgeStyle = " -.-> |wait|"
		}
		b.WriteString(fmt.Sprintf("    %s%s %s\n", from, edgeStyle, to))
	}

	return b.String()
}

func recipeStepMap(recipe *formula.Recipe) map[string]formula.RecipeStep {
	steps := make(map[string]formula.RecipeStep, len(recipe.Steps))
	for _, step := range recipe.Steps {
		steps[step.ID] = step
	}
	return steps
}

func realRecipeSteps(recipe *formula.Recipe) []formula.RecipeStep {
	steps := make([]formula.RecipeStep, 0, len(recipe.Steps))
	for _, step := range recipe.Steps {
		if !step.IsRoot {
			steps = append(steps, step)
		}
	}
	return steps
}

func isMermaidBoundaryStep(step formula.RecipeStep) bool {
	return step.Metadata != nil && step.Metadata["formula_boundary"] != ""
}

func isGeneratedBoundaryRecipeStep(step formula.RecipeStep) bool {
	return isMermaidBoundaryStep(step) && step.Execution == "noop"
}

func appendMermaidLoopBody(b *strings.Builder, step formula.RecipeStep) {
	if step.Loop == nil || len(step.Loop.Body) == 0 {
		return
	}

	loopID := mermaidNodeID(step.ID)
	loopBodyGraphID := loopID + "_loop_body"
	b.WriteString(fmt.Sprintf("    subgraph %s[\"loop body: %s\"]\n", loopBodyGraphID, mermaidEscapeLabel(shortStepID(step.ID))))
	b.WriteString("        direction TB\n")
	var previous string
	for i, bodyStep := range step.Loop.Body {
		bodyID := mermaidNodeID(fmt.Sprintf("%s.loop.%d.%s", step.ID, i+1, bodyStep.ID))
		label := mermaidLoopBodyLabel(bodyStep)
		b.WriteString(fmt.Sprintf("        %s[\"%s\"]\n", bodyID, label))
		b.WriteString(fmt.Sprintf("    class %s nodeLoopBody\n", bodyID))
		if i > 0 {
			b.WriteString(fmt.Sprintf("        %s -.-> %s\n", previous, bodyID))
		}
		previous = bodyID
	}
	b.WriteString("    end\n")
	if first := firstLoopBodyNodeID(step); first != "" {
		b.WriteString(fmt.Sprintf("    %s -.-> |iterate| %s\n", loopID, first))
	}
	if previous != "" {
		b.WriteString(fmt.Sprintf("    %s -.-> |%s| %s\n", previous, mermaidLoopEdgeLabel(step.Loop), loopID))
	}
}

func firstLoopBodyNodeID(step formula.RecipeStep) string {
	if step.Loop == nil {
		return ""
	}
	for i, bodyStep := range step.Loop.Body {
		if bodyStep != nil {
			return mermaidNodeID(fmt.Sprintf("%s.loop.%d.%s", step.ID, i+1, bodyStep.ID))
		}
	}
	return ""
}

func mermaidLoopBodyLabel(step *formula.Step) string {
	if step == nil {
		return "loop body"
	}
	parts := []string{fmt.Sprintf("body: %s", mermaidEscapeLabel(step.ID))}
	if step.Title != "" {
		parts = append(parts, mermaidEscapeLabel(step.Title))
	}
	if step.OutputKey != "" {
		parts = append(parts, fmt.Sprintf("out: %s", mermaidEscapeLabel(step.OutputKey)))
	}
	return strings.Join(parts, "<br/>")
}

func mermaidLoopEdgeLabel(loop *formula.LoopSpec) string {
	if loop == nil {
		return "next"
	}
	var parts []string
	if loop.Until != "" {
		parts = append(parts, "until "+loop.Until)
	}
	if loop.Max > 0 {
		parts = append(parts, fmt.Sprintf("max %d", loop.Max))
	}
	if loop.Count > 0 {
		parts = append(parts, fmt.Sprintf("count %d", loop.Count))
	}
	if loop.Range != "" {
		parts = append(parts, "range "+loop.Range)
	}
	if len(parts) == 0 {
		return "next"
	}
	return mermaidEscapeLabel(strings.Join(parts, "; "))
}

func loopSummary(loop *formula.LoopSpec) string {
	if loop == nil {
		return ""
	}
	var parts []string
	if loop.Until != "" {
		parts = append(parts, fmt.Sprintf("until `%s`", loop.Until))
	}
	if loop.Max > 0 {
		parts = append(parts, fmt.Sprintf("max `%d`", loop.Max))
	}
	if loop.Count > 0 {
		parts = append(parts, fmt.Sprintf("count `%d`", loop.Count))
	}
	if loop.Range != "" {
		parts = append(parts, fmt.Sprintf("range `%s`", loop.Range))
	}
	if loop.Var != "" {
		parts = append(parts, fmt.Sprintf("var `%s`", loop.Var))
	}
	return strings.Join(parts, "; ")
}

type shapeDef struct {
	open  string
	close string
}

func mermaidShape(step formula.RecipeStep, endSteps, parallelSteps map[string]bool) shapeDef {
	if step.IsRoot {
		return shapeDef{open: "([\"", close: "\"])"}
	}
	if isMermaidBoundaryStep(step) {
		return shapeDef{open: "([\"", close: "\"])"}
	}
	if step.Gate != nil {
		return shapeDef{open: "{\"", close: "\"}"}
	}
	if step.Condition != "" {
		return shapeDef{open: "{\"", close: "\"}"}
	}
	if endSteps != nil && endSteps[step.ID] {
		return shapeDef{open: "([\"", close: "\"])"}
	}
	if parallelSteps[step.ID] {
		return shapeDef{open: "(\"", close: "\")"}
	}
	return shapeDef{open: "[\"", close: "\"]"}
}

type nodeColor struct {
	Fill   string
	Stroke string
}

func depthColor(depth, maxDepth int) nodeColor {
	ratio := float64(depth) / float64(maxDepth)

	stops := []struct {
		ratio  float64
		fill   string
		stroke string
	}{
		{0.0, "#e3f2fd", "#1976d2"},
		{0.25, "#e0f2f1", "#00796b"},
		{0.5, "#e8f5e9", "#388e3c"},
		{0.75, "#fff8e1", "#f57f17"},
		{1.0, "#fbe9e7", "#d84315"},
	}

	if ratio <= stops[0].ratio {
		return nodeColor{Fill: stops[0].fill, Stroke: stops[0].stroke}
	}
	for i := 1; i < len(stops); i++ {
		if ratio <= stops[i].ratio {
			return nodeColor{Fill: stops[i].fill, Stroke: stops[i].stroke}
		}
	}
	last := stops[len(stops)-1]
	return nodeColor{Fill: last.fill, Stroke: last.stroke}
}

func findEndSteps(recipe *formula.Recipe) map[string]bool {
	hasDependent := make(map[string]bool)
	for _, dep := range recipe.Deps {
		if dep.Type == "parent-child" {
			continue
		}
		hasDependent[dep.DependsOnID] = true
	}

	ends := make(map[string]bool)
	for _, step := range recipe.Steps {
		if step.IsRoot {
			continue
		}
		if !hasDependent[step.ID] {
			ends[step.ID] = true
		}
	}
	return ends
}

func computeStepDepths(recipe *formula.Recipe) map[string]int {
	depths := make(map[string]int)
	for _, step := range recipe.Steps {
		depths[step.ID] = 0
	}

	for _, step := range recipe.Steps {
		d := stepDepth(step.ID, recipe, depths, make(map[string]bool))
		depths[step.ID] = d
	}
	return depths
}

func stepDepth(id string, recipe *formula.Recipe, depths map[string]int, visiting map[string]bool) int {
	if d, ok := depths[id]; ok && d > 0 {
		return d
	}
	if visiting[id] {
		return 0
	}
	visiting[id] = true

	maxParentDepth := 0
	for _, dep := range recipe.Deps {
		if dep.StepID == id && dep.Type != "parent-child" {
			parentD := stepDepth(dep.DependsOnID, recipe, depths, visiting)
			if parentD+1 > maxParentDepth {
				maxParentDepth = parentD + 1
			}
		}
	}

	depths[id] = maxParentDepth
	return maxParentDepth
}

func findParallelSteps(recipe *formula.Recipe) map[string]bool {
	parallel := make(map[string]bool)
	depMap := make(map[string][]string)
	for _, dep := range recipe.Deps {
		if dep.Type == "parent-child" {
			continue
		}
		depMap[dep.StepID] = append(depMap[dep.StepID], dep.DependsOnID)
	}

	sourceTargets := make(map[string][]string)
	for stepID, deps := range depMap {
		for _, dep := range deps {
			sourceTargets[dep] = append(sourceTargets[dep], stepID)
		}
	}

	for _, targets := range sourceTargets {
		if len(targets) > 1 {
			for _, t := range targets {
				parallel[t] = true
			}
		}
	}

	return parallel
}

func findDepsForStep(recipe *formula.Recipe, stepID string) []string {
	var deps []string
	for _, dep := range recipe.Deps {
		if dep.StepID == stepID && dep.Type != "parent-child" {
			deps = append(deps, dep.DependsOnID)
		}
	}
	return deps
}

func mermaidNodeID(id string) string {
	result := strings.ReplaceAll(id, ".", "_")
	result = strings.ReplaceAll(result, "-", "_")
	return result
}

func mermaidLabel(step formula.RecipeStep) string {
	safeTitle := mermaidEscapeLabel(step.Title)
	prefix := ""
	if step.Priority != nil {
		prefix = fmt.Sprintf("[P%d] ", *step.Priority)
	}
	shortID := shortStepID(step.ID)
	parts := []string{fmt.Sprintf("%s: %s%s", mermaidEscapeLabel(shortID), prefix, safeTitle)}
	if step.Condition != "" {
		parts = append(parts, "if: "+mermaidEscapeLabel(step.Condition))
	}
	if step.OutputKey != "" {
		parts = append(parts, "out: "+mermaidEscapeLabel(step.OutputKey))
	}
	if len(step.InputCtx) > 0 {
		parts = append(parts, "in: "+mermaidEscapeLabel(strings.Join(step.InputCtx, ", ")))
	}
	if step.Loop != nil {
		parts = append(parts, "loop: "+mermaidLoopEdgeLabel(step.Loop))
	}
	return strings.Join(parts, "<br/>")
}

func shortStepID(id string) string {
	if idx := strings.LastIndex(id, "."); idx >= 0 {
		return id[idx+1:]
	}
	return id
}

func markdownCell(s string) string {
	s = strings.ReplaceAll(s, "\n", "<br/>")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "|", "\\|")
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func mermaidEscapeLabel(s string) string {
	s = strings.ReplaceAll(s, "\"", "'")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}

func generateQuickStart(f *formula.Formula, recipe *formula.Recipe) string {
	var b strings.Builder
	b.WriteString("```bash\n")
	b.WriteString(fmt.Sprintf("# 查看公式详情（带 Mermaid 流程图）\n"))
	b.WriteString(fmt.Sprintf("tt formula show %s --markdown\n\n", f.Formula))
	b.WriteString(fmt.Sprintf("# 编译公式，查看任务依赖\n"))
	b.WriteString(fmt.Sprintf("tt formula compile %s\n\n", f.Formula))
	b.WriteString(fmt.Sprintf("# 实例化为 JSON 任务树\n"))
	b.WriteString(fmt.Sprintf("tt formula instantiate %s -o json\n\n", f.Formula))
	b.WriteString(fmt.Sprintf("# 实例化为中文任务提示（适合给 AI agent）\n"))
	b.WriteString(fmt.Sprintf("tt formula instantiate %s -o prompt\n\n", f.Formula))
	b.WriteString(fmt.Sprintf("# 试运行执行计划（不调用大模型）\n"))
	b.WriteString(fmt.Sprintf("tt formula run %s --dry-run\n\n", f.Formula))
	b.WriteString(fmt.Sprintf("# 执行公式\n"))
	b.WriteString(fmt.Sprintf("tt formula run %s\n\n", f.Formula))

	requiredVars := f.RequiredVarNames()
	if len(requiredVars) > 0 {
		vars := make([]string, len(requiredVars))
		for i, v := range requiredVars {
			vars[i] = fmt.Sprintf("--var %s=<value>", v)
		}
		if len(requiredVars) == 1 {
			b.WriteString(fmt.Sprintf("# 带必填变量执行: %s（位置参数简写）\n", requiredVars[0]))
			b.WriteString(fmt.Sprintf("tt formula run %s <value>\n", f.Formula))
		} else {
			b.WriteString(fmt.Sprintf("# 带必填变量执行: %s\n", strings.Join(requiredVars, ", ")))
			b.WriteString(fmt.Sprintf("tt formula run %s %s\n", f.Formula, strings.Join(vars, " ")))
		}
	} else {
		b.WriteString(fmt.Sprintf("# 传入变量值\n"))
		b.WriteString(fmt.Sprintf("tt formula run %s --var key=value\n", f.Formula))
	}
	b.WriteString("```\n")
	return b.String()
}

func escapeYAML(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}
