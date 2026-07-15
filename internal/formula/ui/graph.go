package ui

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/spec"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

func BuildWorkflowGraph(workflow *ir.Workflow) ([]Step, []Edge) {
	if workflow == nil {
		return nil, nil
	}
	depths := computeWorkflowDepths(workflow)
	allowedVars := workflowVarNames(workflow)
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
			uiStep.VarRefs = templateVarRefs(allowedVars, nil, typed.Prompt, typed.Cwd, typed.Agent, typed.Model)
		case *steps.AgentStep:
			uiStep.Agent = typed.Agent
			uiStep.Model = typed.Model
			uiStep.Description = typed.Prompt
			uiStep.InputCtx = append([]string(nil), typed.InputCtx...)
			uiStep.VarRefs = templateVarRefs(allowedVars, nil, typed.Prompt, typed.Cwd, typed.Agent, typed.Model)
		case steps.ScriptStep:
			uiStep.Execution = string(steps.KindScript)
			if typed.ScriptPath != "" {
				uiStep.Description = fmt.Sprintf("script: %s", typed.ScriptPath)
				uiStep.ScriptPath = typed.ScriptPath
				// Use resolved path for reading content
				readPath := typed.ResolvedScriptPath
				if readPath == "" {
					readPath = typed.ScriptPath
				}
				if content, err := os.ReadFile(readPath); err == nil {
					uiStep.ScriptContent = string(content)
				}
			} else {
				uiStep.Description = scriptSummary(typed.Command)
			}
			uiStep.VarRefs = templateVarRefs(allowedVars, nil, typed.Command, typed.Cwd, typed.Env)
		case *steps.ScriptStep:
			uiStep.Execution = string(steps.KindScript)
			if typed.ScriptPath != "" {
				uiStep.Description = fmt.Sprintf("script: %s", typed.ScriptPath)
				uiStep.ScriptPath = typed.ScriptPath
				// Use resolved path for reading content
				readPath := typed.ResolvedScriptPath
				if readPath == "" {
					readPath = typed.ScriptPath
				}
				if content, err := os.ReadFile(readPath); err == nil {
					uiStep.ScriptContent = string(content)
				}
			} else {
				uiStep.Description = scriptSummary(typed.Command)
			}
			uiStep.VarRefs = templateVarRefs(allowedVars, nil, typed.Command, typed.Cwd, typed.Env)
		case steps.ExternalAgentStep:
			uiStep.Execution = string(steps.KindExternalAgent)
			uiStep.Agent = typed.Driver
			uiStep.Model = typed.Model
			uiStep.Description = typed.Prompt
			uiStep.InputCtx = append([]string(nil), typed.InputCtx...)
			uiStep.VarRefs = templateVarRefs(allowedVars, nil, typed.Prompt, typed.Cwd, typed.Driver, typed.Provider, typed.Model, typed.Mode, typed.Resume, typed.ExtraArgs)
		case *steps.ExternalAgentStep:
			uiStep.Execution = string(steps.KindExternalAgent)
			uiStep.Agent = typed.Driver
			uiStep.Model = typed.Model
			uiStep.Description = typed.Prompt
			uiStep.InputCtx = append([]string(nil), typed.InputCtx...)
			uiStep.VarRefs = templateVarRefs(allowedVars, nil, typed.Prompt, typed.Cwd, typed.Driver, typed.Provider, typed.Model, typed.Mode, typed.Resume, typed.ExtraArgs)
		case steps.HumanInputStep:
			uiStep.Execution = string(steps.KindHumanInput)
			uiStep.Description = typed.Reason
			uiStep.VarRefs = templateVarRefs(allowedVars, nil, typed.Reason)
			request := &HumanInputRequest{Reason: typed.Reason}
			if form, ok := typed.Form.(*spec.FormSpec); ok {
				request.Form = form
			}
			uiStep.HumanInputRequest = request
		case *steps.HumanInputStep:
			uiStep.Execution = string(steps.KindHumanInput)
			uiStep.Description = typed.Reason
			uiStep.VarRefs = templateVarRefs(allowedVars, nil, typed.Reason)
			request := &HumanInputRequest{Reason: typed.Reason}
			if form, ok := typed.Form.(*spec.FormSpec); ok {
				request.Form = form
			}
			uiStep.HumanInputRequest = request
		case steps.FormulaCallStep:
			uiStep.Execution = string(steps.KindFormula)
			uiStep.Formula = typed.Formula
			uiStep.Description = fmt.Sprintf("Formula: %s", typed.Formula)
			uiStep.VarRefs = templateVarRefs(allowedVars, nil, typed.With)
		case *steps.FormulaCallStep:
			uiStep.Execution = string(steps.KindFormula)
			uiStep.Formula = typed.Formula
			uiStep.Description = fmt.Sprintf("Formula: %s", typed.Formula)
			uiStep.VarRefs = templateVarRefs(allowedVars, nil, typed.With)
		case steps.LoopStep:
			uiStep.VarRefs = templateVarRefs(allowedVars, localVarSet(typed.Var), typed.ForEach, typed.Until)
			uiStep.Loop = BuildLoopFromStep(typed, allowedVars)
		case *steps.LoopStep:
			uiStep.VarRefs = templateVarRefs(allowedVars, localVarSet(typed.Var), typed.ForEach, typed.Until)
			uiStep.Loop = BuildLoopFromStep(*typed, allowedVars)
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

func BuildLoopFromStep(loop steps.LoopStep, allowedVars map[string]struct{}) *Loop {
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
		localVars := map[string]struct{}{loop.Var: {}}
		switch typed := child.(type) {
		case steps.AgentStep:
			body.Description = typed.Prompt
			body.Agent = typed.Agent
			body.Model = typed.Model
			body.OutputKey = typed.OutputKey
			body.InputCtx = append([]string(nil), typed.InputCtx...)
			body.VarRefs = templateVarRefs(allowedVars, localVars, typed.Prompt, typed.Cwd, typed.Agent, typed.Model)
		case *steps.AgentStep:
			body.Description = typed.Prompt
			body.Agent = typed.Agent
			body.Model = typed.Model
			body.OutputKey = typed.OutputKey
			body.InputCtx = append([]string(nil), typed.InputCtx...)
			body.VarRefs = templateVarRefs(allowedVars, localVars, typed.Prompt, typed.Cwd, typed.Agent, typed.Model)
		case steps.ScriptStep:
			body.OutputKey = typed.OutputKey
			if typed.ScriptPath != "" {
				body.Description = fmt.Sprintf("script: %s", typed.ScriptPath)
			} else {
				body.Description = scriptSummary(typed.Command)
			}
			body.VarRefs = templateVarRefs(allowedVars, localVars, typed.Command, typed.Cwd, typed.Env)
		case *steps.ScriptStep:
			body.OutputKey = typed.OutputKey
			if typed.ScriptPath != "" {
				body.Description = fmt.Sprintf("script: %s", typed.ScriptPath)
			} else {
				body.Description = scriptSummary(typed.Command)
			}
			body.VarRefs = templateVarRefs(allowedVars, localVars, typed.Command, typed.Cwd, typed.Env)
		case steps.HumanInputStep:
			body.Description = typed.Reason
			body.OutputKey = typed.OutputKey
			body.VarRefs = templateVarRefs(allowedVars, localVars, typed.Reason)
		case *steps.HumanInputStep:
			body.Description = typed.Reason
			body.OutputKey = typed.OutputKey
			body.VarRefs = templateVarRefs(allowedVars, localVars, typed.Reason)
		case steps.LoopStep:
			body.Description = typedLoopSummary(typed)
			body.VarRefs = templateVarRefs(allowedVars, localVars, typed.ForEach, typed.Until)
			body.Loop = BuildLoopFromStep(typed, allowedVars)
		case *steps.LoopStep:
			body.Description = typedLoopSummary(*typed)
			body.VarRefs = templateVarRefs(allowedVars, localVars, typed.ForEach, typed.Until)
			body.Loop = BuildLoopFromStep(*typed, allowedVars)
		}
		dashboardLoop.Body = append(dashboardLoop.Body, body)
	}
	return dashboardLoop
}

var templateVarPattern = regexp.MustCompile(`{{\s*([A-Za-z_][A-Za-z0-9_.-]*)\s*}}`)

func workflowVarNames(workflow *ir.Workflow) map[string]struct{} {
	out := map[string]struct{}{}
	if workflow == nil {
		return out
	}
	for key := range workflow.Vars {
		key = strings.TrimSpace(key)
		if key != "" {
			out[key] = struct{}{}
		}
	}
	return out
}

func templateVarRefs(allowed map[string]struct{}, local map[string]struct{}, values ...any) []string {
	seen := map[string]struct{}{}
	var scan func(any)
	scan = func(value any) {
		switch typed := value.(type) {
		case string:
			for _, match := range templateVarPattern.FindAllStringSubmatch(typed, -1) {
				root := strings.Split(strings.TrimSpace(match[1]), ".")[0]
				if root == "" {
					continue
				}
				if _, ok := allowed[root]; !ok {
					continue
				}
				if _, ok := local[root]; ok {
					continue
				}
				seen[root] = struct{}{}
			}
		case []string:
			for _, item := range typed {
				scan(item)
			}
		case map[string]string:
			for key, item := range typed {
				scan(key)
				scan(item)
			}
		}
	}
	for _, value := range values {
		scan(value)
	}
	out := make([]string, 0, len(seen))
	for key := range seen {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func localVarSet(names ...string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			out[name] = struct{}{}
		}
	}
	return out
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

func BuildLoop(loop *spec.LoopSpec) *Loop {
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
			Loop:        BuildLoop(body.Loop),
		})
	}
	return dashboardLoop
}

func loopSummary(loop *spec.LoopSpec) string {
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
		cp.Body[i].VarRefs = append([]string(nil), src.Body[i].VarRefs...)
		cp.Body[i].DependsOn = append([]string(nil), src.Body[i].DependsOn...)
		cp.Body[i].Loop = CloneLoop(src.Body[i].Loop)
	}
	return &cp
}

func scriptSummary(command []string) string {
	if len(command) == 0 {
		return "empty script"
	}
	if len(command) == 1 {
		return command[0]
	}
	return fmt.Sprintf("%s ...", command[0])
}
