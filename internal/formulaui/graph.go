package formulaui

import (
	"fmt"

	"github.com/sjzsdu/tt/internal/formula"
	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

func BuildWorkflowGraph(workflow *ir.Workflow) ([]Step, []Edge) {
	if workflow == nil {
		return nil, nil
	}
	depths := computeWorkflowDepths(workflow)
	dependsOnMap := map[string][]string{}
	stepIDs := map[string]struct{}{}

	uiSteps := make([]Step, 0, len(workflow.Graph.Nodes))
	index := 0
	for id, node := range workflow.Graph.Nodes {
		if node == nil || node.Step == nil {
			continue
		}
		meta := node.Step.Meta()
		if meta.Kind == steps.KindNoop {
			continue
		}
		stepIDs[string(id)] = struct{}{}
		uiStep := Step{
			ID:        string(id),
			Title:     meta.Title,
			Type:      string(meta.Kind),
			Status:    StatusPending,
			Labels:    append([]string(nil), meta.Labels...),
			Condition: meta.Condition,
			Depth:     depths[id],
			Index:     index,
		}
		index++
		switch typed := node.Step.(type) {
		case steps.AgentStep:
			uiStep.Agent = typed.Agent
			uiStep.Model = typed.Model
			uiStep.Description = typed.Prompt
			uiStep.InputCtx = append([]string(nil), typed.InputCtx...)
		case steps.ScriptStep:
			uiStep.Execution = string(steps.KindScript)
		case steps.HumanInputStep:
			uiStep.Execution = string(steps.KindHumanInput)
			uiStep.Description = typed.Reason
			request := &HumanInputRequest{Reason: typed.Reason}
			if form, ok := typed.Form.(*formula.FormSpec); ok {
				request.Form = form
			}
			uiStep.HumanInputRequest = request
		case steps.LoopStep:
			uiStep.Loop = BuildLoopFromStep(typed)
		case *steps.LoopStep:
			uiStep.Loop = BuildLoopFromStep(*typed)
		}
		uiSteps = append(uiSteps, uiStep)
	}

	uiEdges := make([]Edge, 0, len(workflow.Graph.Edges))
	for _, dep := range workflow.Graph.Edges {
		from := string(dep.From)
		to := string(dep.To)
		if _, ok := stepIDs[from]; !ok {
			continue
		}
		if _, ok := stepIDs[to]; !ok {
			continue
		}
		dependsOnMap[to] = append(dependsOnMap[to], from)
		uiEdges = append(uiEdges, Edge{From: from, To: to, Type: dep.Type})
	}
	for i := range uiSteps {
		uiSteps[i].DependsOn = append([]string(nil), dependsOnMap[uiSteps[i].ID]...)
	}
	return uiSteps, uiEdges
}

func BuildLoopFromStep(loop steps.LoopStep) *Loop {
	dashboardLoop := &Loop{
		ForEach:        loop.ForEach,
		Var:            loop.Var,
		Until:          loop.Until,
		Max:            loop.Max,
		Parallel:       loop.Parallel,
		MaxConcurrency: loop.MaxConcurrency,
		Summary:        typedLoopSummary(loop),
		Body:           make([]LoopBody, 0, len(loop.Body)),
	}
	for _, child := range loop.Body {
		if child == nil {
			continue
		}
		meta := child.Meta()
		body := LoopBody{
			ID:        string(meta.ID),
			Title:     meta.Title,
			Condition: meta.Condition,
			DependsOn: metadataDependsOn(meta),
		}
		switch typed := child.(type) {
		case steps.AgentStep:
			body.Description = typed.Prompt
			body.Agent = typed.Agent
			body.Model = typed.Model
			body.OutputKey = typed.OutputKey
			body.InputCtx = append([]string(nil), typed.InputCtx...)
		case *steps.AgentStep:
			body.Description = typed.Prompt
			body.Agent = typed.Agent
			body.Model = typed.Model
			body.OutputKey = typed.OutputKey
			body.InputCtx = append([]string(nil), typed.InputCtx...)
		case steps.ScriptStep:
			body.OutputKey = typed.OutputKey
		case *steps.ScriptStep:
			body.OutputKey = typed.OutputKey
		case steps.HumanInputStep:
			body.Description = typed.Reason
			body.OutputKey = typed.OutputKey
		case *steps.HumanInputStep:
			body.Description = typed.Reason
			body.OutputKey = typed.OutputKey
		}
		dashboardLoop.Body = append(dashboardLoop.Body, body)
	}
	return dashboardLoop
}

func metadataDependsOn(meta steps.Metadata) []string {
	out := make([]string, 0, len(meta.DependsOn))
	for _, dep := range meta.DependsOn {
		out = append(out, string(dep))
	}
	return out
}

func typedLoopSummary(loop steps.LoopStep) string {
	if loop.ForEach != "" {
		mode := "sequential"
		if loop.Parallel {
			mode = "parallel"
		}
		if loop.MaxConcurrency > 0 {
			return fmt.Sprintf("foreach %s as %s · %s · max concurrency %d", loop.ForEach, loop.Var, mode, loop.MaxConcurrency)
		}
		return fmt.Sprintf("foreach %s as %s · %s", loop.ForEach, loop.Var, mode)
	}
	if loop.Until != "" {
		max := loop.Max
		if max <= 0 {
			max = 1
		}
		return fmt.Sprintf("until %s · max %d", loop.Until, max)
	}
	return fmt.Sprintf("%d body step(s)", len(loop.Body))
}

func computeWorkflowDepths(workflow *ir.Workflow) map[ir.NodeID]int {
	stepIDs := map[ir.NodeID]struct{}{}
	for id, node := range workflow.Graph.Nodes {
		if node == nil || node.Step == nil || node.Step.Meta().Kind == steps.KindNoop {
			continue
		}
		stepIDs[id] = struct{}{}
	}
	parents := map[ir.NodeID][]ir.NodeID{}
	for _, dep := range workflow.Graph.Edges {
		if _, ok := stepIDs[dep.From]; !ok {
			continue
		}
		if _, ok := stepIDs[dep.To]; !ok {
			continue
		}
		parents[dep.To] = append(parents[dep.To], dep.From)
	}
	depths := map[ir.NodeID]int{}
	var visit func(ir.NodeID, map[ir.NodeID]bool) int
	visit = func(id ir.NodeID, visiting map[ir.NodeID]bool) int {
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
		visit(id, map[ir.NodeID]bool{})
	}
	return depths
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
