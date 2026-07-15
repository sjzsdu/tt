package compile

import (
	"fmt"
	"strings"

	"github.com/sjzsdu/tt/internal/formula/ast"
	"github.com/sjzsdu/tt/internal/formula/ir"
	spec "github.com/sjzsdu/tt/internal/formula/spec"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

type Compiler struct{ Registry *steps.Registry }

func New(registry *steps.Registry) *Compiler {
	if registry == nil {
		registry = steps.NewDefaultRegistry()
	}
	return &Compiler{Registry: registry}
}

func (c *Compiler) Compile(doc *ast.Document) (*ir.Workflow, error) {
	if doc == nil {
		return nil, fmt.Errorf("formula document is required")
	}
	wf := &ir.Workflow{ID: ir.WorkflowID(doc.Name), Name: doc.Name, Description: doc.Description, Vars: map[string]ir.VarSchema{}, Outputs: map[string]ir.OutputSchema{}, Workspace: workspacePolicyFromDocument(doc), Graph: ir.NewGraph()}
	for name, v := range doc.Vars {
		wf.Vars[name] = ir.VarSchema{Type: v.Type, Required: v.Required, Default: v.Default}
	}
	for name, output := range doc.Outputs {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("workflow output name is required")
		}
		if strings.TrimSpace(output.From) == "" {
			return nil, fmt.Errorf("workflow output %q source is required", name)
		}
		wf.Outputs[name] = ir.OutputSchema{From: output.From, Type: output.Type, Required: output.Required, Description: output.Description}
	}
	seen := map[string]bool{}
	for _, decl := range doc.Steps {
		if seen[decl.ID] {
			return nil, fmt.Errorf("duplicate step id %q", decl.ID)
		}
		seen[decl.ID] = true
		step, err := c.Registry.Decode(decl)
		if err != nil {
			return nil, fmt.Errorf("decode step %q: %w", decl.ID, err)
		}
		wf.Graph.AddNode(&ir.Node{ID: ir.NodeID(decl.ID), Step: step})
	}
	ir.ApplyOutputConventions(wf)
	for _, decl := range doc.Steps {
		for _, dep := range decl.DependsOn {
			if !seen[dep] {
				return nil, fmt.Errorf("step %q depends on unknown step %q", decl.ID, dep)
			}
			wf.Graph.AddEdge(ir.NodeID(dep), ir.NodeID(decl.ID), "blocks")
		}
	}
	if _, err := validateAcyclic(wf.Graph); err != nil {
		return nil, err
	}
	return wf, nil
}

func workspacePolicyFromDocument(doc *ast.Document) *ir.WorkspacePolicy {
	if doc == nil {
		return nil
	}
	ws := doc.Workspace
	if ws == nil && doc.Worktree {
		ws = &spec.WorkspaceSpec{Kind: "worktree"}
	}
	if ws == nil {
		return nil
	}
	kind := strings.TrimSpace(ws.Kind)
	if kind == "" {
		kind = "worktree"
	}
	cleanup := true
	if ws.Cleanup != nil {
		cleanup = *ws.Cleanup
	}
	return &ir.WorkspacePolicy{Kind: kind, Path: strings.TrimSpace(ws.Path), Cleanup: cleanup, Branch: strings.TrimSpace(ws.Branch), Base: strings.TrimSpace(ws.Base), BranchSlugFrom: strings.TrimSpace(ws.BranchSlugFrom), BranchPrefix: strings.TrimSpace(ws.BranchPrefix)}
}

func validateAcyclic(graph ir.Graph) ([]ir.NodeID, error) {
	inDegree := map[ir.NodeID]int{}
	adj := map[ir.NodeID][]ir.NodeID{}
	for id := range graph.Nodes {
		inDegree[id] = 0
	}
	for _, edge := range graph.Edges {
		adj[edge.From] = append(adj[edge.From], edge.To)
		inDegree[edge.To]++
	}
	var order []ir.NodeID
	for len(inDegree) > 0 {
		var ready []ir.NodeID
		for id, deg := range inDegree {
			if deg == 0 {
				ready = append(ready, id)
			}
		}
		if len(ready) == 0 {
			return nil, fmt.Errorf("workflow graph contains a cycle")
		}
		for _, id := range ready {
			order = append(order, id)
			delete(inDegree, id)
			for _, next := range adj[id] {
				if _, ok := inDegree[next]; ok {
					inDegree[next]--
				}
			}
		}
	}
	return order, nil
}
