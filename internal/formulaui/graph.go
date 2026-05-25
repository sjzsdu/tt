package formulaui

import (
	"fmt"

	"github.com/sjzsdu/tt/internal/executor"
	"github.com/sjzsdu/tt/internal/formula"
)

func BuildGraph(recipe *formula.Recipe) ([]Step, []Edge) {
	depths := computeDepths(recipe)
	dependsOnMap := map[string][]string{}
	stepIDs := map[string]struct{}{}

	steps := make([]Step, 0, len(recipe.Steps))
	for index, step := range recipe.Steps {
		if step.IsRoot || isHiddenBoundary(step) {
			continue
		}
		stepIDs[step.ID] = struct{}{}
		agentName := ""
		modelName := ""
		if step.Agent != nil {
			agentName = step.Agent.Name
			modelName = step.Agent.Model
		}
		var gate *Gate
		if step.Gate != nil {
			gate = &Gate{Type: step.Gate.Type, ID: step.Gate.ID, Timeout: step.Gate.Timeout}
		}
		loop := BuildLoop(step.Loop)
		var humanInputRequest *executor.HumanInputRequest
		if step.Execution == executor.HumanInputExecution && step.Form != nil {
			humanInputRequest = &executor.HumanInputRequest{Reason: step.Description, Form: step.Form}
		}
		steps = append(steps, Step{
			ID:                step.ID,
			Title:             step.Title,
			Description:       step.Description,
			Notes:             step.Notes,
			Type:              step.Type,
			Agent:             agentName,
			Model:             modelName,
			Status:            "pending",
			Priority:          step.Priority,
			Labels:            append([]string(nil), step.Labels...),
			Assignee:          step.Assignee,
			OutputKey:         step.OutputKey,
			InputCtx:          append([]string(nil), step.InputCtx...),
			Execution:         step.Execution,
			Condition:         step.Condition,
			Metadata:          CloneStringMap(step.Metadata),
			Gate:              gate,
			Loop:              loop,
			HumanInputRequest: humanInputRequest,
			Depth:             depths[step.ID],
			Index:             index,
		})
	}

	edges := make([]Edge, 0, len(recipe.Deps))
	for _, dep := range recipe.Deps {
		if dep.Type == "parent-child" {
			continue
		}
		if _, ok := stepIDs[dep.StepID]; !ok {
			continue
		}
		if _, ok := stepIDs[dep.DependsOnID]; !ok {
			continue
		}
		dependsOnMap[dep.StepID] = append(dependsOnMap[dep.StepID], dep.DependsOnID)
		edges = append(edges, Edge{From: dep.DependsOnID, To: dep.StepID, Type: dep.Type})
	}

	for i := range steps {
		steps[i].DependsOn = append([]string(nil), dependsOnMap[steps[i].ID]...)
	}

	return steps, edges
}

func computeDepths(recipe *formula.Recipe) map[string]int {
	stepIDs := map[string]struct{}{}
	for _, step := range recipe.Steps {
		if !step.IsRoot && !isHiddenBoundary(step) {
			stepIDs[step.ID] = struct{}{}
		}
	}

	parents := map[string][]string{}
	for _, dep := range recipe.Deps {
		if dep.Type == "parent-child" {
			continue
		}
		if _, ok := stepIDs[dep.StepID]; !ok {
			continue
		}
		if _, ok := stepIDs[dep.DependsOnID]; !ok {
			continue
		}
		parents[dep.StepID] = append(parents[dep.StepID], dep.DependsOnID)
	}

	depths := map[string]int{}
	var visit func(string, map[string]bool) int
	visit = func(id string, visiting map[string]bool) int {
		if depth, ok := depths[id]; ok {
			return depth
		}
		if visiting[id] {
			return 0
		}
		visiting[id] = true
		maxDepth := 0
		for _, parentID := range parents[id] {
			if next := visit(parentID, visiting) + 1; next > maxDepth {
				maxDepth = next
			}
		}
		delete(visiting, id)
		depths[id] = maxDepth
		return maxDepth
	}

	for id := range stepIDs {
		visit(id, map[string]bool{})
	}
	return depths
}

func isHiddenBoundary(step formula.RecipeStep) bool {
	return step.Execution == "noop" && step.Metadata != nil && step.Metadata["formula_boundary"] != ""
}

func CloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func BuildLoop(loop *formula.LoopSpec) *Loop {
	if loop == nil {
		return nil
	}
	dashboardLoop := &Loop{
		Count:          loop.Count,
		Until:          loop.Until,
		Max:            loop.Max,
		Range:          loop.Range,
		ForEach:        loop.ForEach,
		Var:            loop.Var,
		Parallel:       loop.Parallel,
		MaxConcurrency: loop.MaxConcurrency,
		Summary:        loopSummary(loop),
		Body:           make([]LoopBody, 0, len(loop.Body)),
	}
	for _, body := range loop.Body {
		if body == nil {
			continue
		}
		agentName := ""
		modelName := ""
		if body.Agent != nil {
			agentName = body.Agent.Name
			modelName = body.Agent.Model
		}
		dashboardLoop.Body = append(dashboardLoop.Body, LoopBody{
			ID:          body.ID,
			Title:       body.Title,
			Description: body.Description,
			Agent:       agentName,
			Model:       modelName,
			OutputKey:   body.OutputKey,
			InputCtx:    append([]string(nil), body.InputCtx...),
			Condition:   body.Condition,
			DependsOn:   append(append([]string(nil), body.DependsOn...), body.Needs...),
		})
	}
	return dashboardLoop
}

func loopSummary(loop *formula.LoopSpec) string {
	if loop == nil {
		return ""
	}
	switch {
	case loop.ForEach != "":
		mode := "sequential"
		if loop.Parallel {
			mode = "parallel"
		}
		if loop.MaxConcurrency > 0 {
			return fmt.Sprintf("foreach %s as %s · %s · max concurrency %d", loop.ForEach, loop.Var, mode, loop.MaxConcurrency)
		}
		return fmt.Sprintf("foreach %s as %s · %s", loop.ForEach, loop.Var, mode)
	case loop.Until != "":
		max := loop.Max
		if max <= 0 {
			max = 1
		}
		return fmt.Sprintf("until %s · max %d", loop.Until, max)
	case loop.Count > 0:
		return fmt.Sprintf("count %d", loop.Count)
	case loop.Range != "":
		if loop.Var != "" {
			return fmt.Sprintf("for %s in %s", loop.Var, loop.Range)
		}
		return fmt.Sprintf("range %s", loop.Range)
	default:
		return fmt.Sprintf("%d body step(s)", len(loop.Body))
	}
}

func CloneLoop(src *Loop) *Loop {
	if src == nil {
		return nil
	}
	cp := *src
	cp.Body = make([]LoopBody, len(src.Body))
	copy(cp.Body, src.Body)
	for i := range cp.Body {
		cp.Body[i].InputCtx = append([]string(nil), src.Body[i].InputCtx...)
	}
	return &cp
}
