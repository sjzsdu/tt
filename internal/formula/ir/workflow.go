package ir

import "github.com/sjzsdu/tt/internal/formula/steps"

type WorkflowID string
type NodeID string

type Workflow struct {
	ID          WorkflowID
	Name        string
	Description string
	Vars        map[string]VarSchema
	Workspace   *WorkspacePolicy
	Graph       Graph
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
