package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

const maxFormulaCallDepth = 16

type executorWorkflowRunner struct {
	executor *Executor
}

func (r executorWorkflowRunner) RunWorkflow(ctx context.Context, req steps.WorkflowRequest) (*steps.WorkflowResult, error) {
	parent := r.executor
	if parent == nil || parent.ResolveWorkflow == nil {
		return nil, fmt.Errorf("formula workflow resolver is required")
	}
	formulaName := strings.TrimSpace(req.Formula)
	stack := append([]string(nil), parent.CallStack...)
	if len(stack) == 0 && parent.Workflow != nil {
		name := strings.TrimSpace(parent.Workflow.Name)
		if name == "" {
			name = string(parent.Workflow.ID)
		}
		stack = append(stack, name)
	}
	if len(stack) >= maxFormulaCallDepth {
		return nil, fmt.Errorf("formula call depth exceeds %d: %s", maxFormulaCallDepth, strings.Join(append(stack, formulaName), " -> "))
	}
	for _, ancestor := range stack {
		if ancestor == formulaName {
			return nil, fmt.Errorf("recursive formula call detected: %s", strings.Join(append(stack, formulaName), " -> "))
		}
	}

	workflow, err := parent.ResolveWorkflow(ctx, formulaName, req.Inputs)
	if err != nil {
		return nil, fmt.Errorf("resolve child formula %q: %w", formulaName, err)
	}
	capabilities := parent.Capabilities
	capabilities.Workflows = nil
	child := NewExecutor(workflow, capabilities)
	child.Mode = parent.Mode
	child.ResolveWorkflow = parent.ResolveWorkflow
	child.Store = parent.Store
	child.Events = parent.Events
	child.Nested = true
	child.StateWorkflowID = parent.stateWorkflowID()
	child.AddressPrefix = parent.executionPath(ir.NodeID(req.NodeID)).Formula(formulaName)
	child.CallStack = append(stack, formulaName)
	child.runID = parent.runID
	child.formulaRunDir = parent.formulaRunDir
	child.SeedWorkflowVars(workflow)
	child.SeedValues(req.Inputs)
	if env, ok := parent.Context.Get(EnvironmentContextKey); ok {
		_ = child.Context.Set(EnvironmentContextKey, env)
	}

	runResult, runErr := child.Run(ctx)
	result := workflowResultFromRun(runResult)
	if result.Await != nil && strings.TrimSpace(result.Await.StepID) == "" && runResult != nil {
		for nodeID, nodeResult := range runResult.Nodes {
			if nodeResult == nil || nodeResult.Status != steps.StatusWaiting || nodeResult.Await == nil {
				continue
			}
			await := *nodeResult.Await
			await.StepID = string(child.runtimeNodeID(nodeID))
			result.Await = &await
			break
		}
	}
	if runErr != nil && result.Error == nil {
		result.Error = &steps.StepError{Message: "child formula failed", Cause: runErr}
	}
	return result, runErr
}

func workflowResultFromRun(result *RunResult) *steps.WorkflowResult {
	out := &steps.WorkflowResult{Status: steps.StatusFailed}
	if result == nil {
		out.Error = &steps.StepError{Message: "child formula returned no result"}
		return out
	}
	out.Status = result.Status
	out.Outputs = result.Outputs
	ids := make([]string, 0, len(result.Nodes))
	for id := range result.Nodes {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, id := range ids {
		node := result.Nodes[ir.NodeID(id)]
		if node == nil {
			continue
		}
		if out.Await == nil && node.Await != nil {
			out.Await = node.Await
		}
		if out.Error == nil && node.Error != nil {
			out.Error = node.Error
		}
	}
	return out
}
