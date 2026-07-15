package ir

import "github.com/sjzsdu/tt/internal/formula/steps"

type WorkflowID string
type NodeID string

type Workflow struct {
	ID          WorkflowID
	Name        string
	Description string
	Vars        map[string]VarSchema
	Outputs     map[string]OutputSchema
	Workspace   *WorkspacePolicy
	Graph       Graph
}

type OutputSchema struct {
	From        string
	Type        string
	Required    bool
	Description string
}

type VarSchema struct {
	Type     string
	Required bool
	Default  *string
}

type Graph struct {
	Nodes map[NodeID]*Node
	Edges []Edge
}

type Node struct {
	ID   NodeID
	Step steps.Step
}

type Edge struct {
	From NodeID
	To   NodeID
	Type string
}

func NewGraph() Graph { return Graph{Nodes: map[NodeID]*Node{}} }

func (g *Graph) AddNode(node *Node) {
	if g.Nodes == nil {
		g.Nodes = map[NodeID]*Node{}
	}
	g.Nodes[node.ID] = node
}

func (g *Graph) AddEdge(from, to NodeID, typ string) {
	g.Edges = append(g.Edges, Edge{From: from, To: to, Type: typ})
}

// ApplyOutputConventions supplies the stable public report port used by
// reusable Formulas while allowing an explicit declaration to override it.
func ApplyOutputConventions(workflow *Workflow) {
	if workflow == nil {
		return
	}
	if workflow.Outputs == nil {
		workflow.Outputs = map[string]OutputSchema{}
	}
	if _, declared := workflow.Outputs[steps.OutputReport]; declared {
		return
	}
	if _, hasFinalReport := workflow.Graph.Nodes["final-report"]; hasFinalReport {
		workflow.Outputs[steps.OutputReport] = OutputSchema{From: "final-report", Type: "markdown", Required: true, Description: "Human-readable final Formula report"}
	}
}
