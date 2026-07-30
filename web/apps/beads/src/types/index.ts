export type IssueStatus = 'open' | 'in_progress' | 'blocked' | 'closed' | 'deferred';
export type DependencyType = 'blocks' | 'related' | 'parent' | string;

export type Dependency = {
  issue_id: string;
  depends_on_id: string;
  type: DependencyType;
};

export type Issue = {
  id: string;
  title: string;
  description?: string;
  status: IssueStatus;
  priority: number;
  issue_type?: string;
  assignee?: string;
  labels?: string[];
  dependencies?: Dependency[];
  created_at?: string;
  updated_at?: string;
  started_at?: string;
  closed_at?: string;
  estimated_minutes?: number;
  design?: string;
  acceptance_criteria?: string;
};

export type GraphNode = {
  id: string;
  title: string;
  status: IssueStatus;
  priority: number;
  issue_type?: string;
  labels?: string[];
  assignee?: string;
};

export type GraphEdge = {
  source: string;
  target: string;
  type: DependencyType;
};

export type GraphResult = {
  nodes: GraphNode[];
  edges: GraphEdge[];
};

export type HealthResponse = {
  status: string;
  read_only: boolean;
  port: number;
};

export type WorkspaceResponse = {
  workspace: string;
  bin: string;
};

export type DashboardView = 'list' | 'graph';
