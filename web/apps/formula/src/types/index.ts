export type FormulaDashboardGate = {
  type?: string;
  id?: string;
  timeout?: string;
};

export type FormulaDashboardStep = {
  id: string;
  title: string;
  description?: string;
  notes?: string;
  type?: string;
  agent: string;
  model?: string;
  session?: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'skipped' | string;
  output?: string;
  error?: string;
  started_at?: string;
  finished_at?: string;
  duration_ms?: number;
  priority?: number;
  labels?: string[];
  assignee?: string;
  output_key?: string;
  input_ctx?: string[];
  execution?: string;
  condition?: string;
  metadata?: Record<string, string>;
  gate?: FormulaDashboardGate;
  depends_on?: string[];
  depth?: number;
  index: number;
};

export type FormulaDashboardEdge = {
  from: string;
  to: string;
  type?: string;
};

export type FormulaDashboardLogEntry = {
  at: string;
  text: string;
};

export type FormulaDashboardSnapshot = {
  recipe_name: string;
  description?: string;
  phase?: string;
  status: string;
  started_at?: string;
  finished_at?: string;
  final_output?: string;
  error?: string;
  steps: FormulaDashboardStep[];
  edges: FormulaDashboardEdge[];
  logs: FormulaDashboardLogEntry[];
  workspace_dir?: string;
};

export type FormulaDashboardMessage = {
  type: 'state';
  state: FormulaDashboardSnapshot;
};

export type DashboardView = 'list' | 'graph';
