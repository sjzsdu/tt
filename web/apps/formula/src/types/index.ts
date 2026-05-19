export type FormulaDashboardGate = {
  type?: string;
  id?: string;
  timeout?: string;
};

export type FormulaStepActivity = {
  at: string;
  step_id: string;
  title?: string;
  status: string;
  detail?: string;
  output?: string;
  error?: string;
  duration_ms?: number;
};

export type FormulaDashboardLoopBody = {
  id: string;
  title: string;
  description?: string;
  agent?: string;
  model?: string;
  output_key?: string;
  input_ctx?: string[];
  condition?: string;
};

export type FormulaDashboardLoop = {
  count?: number;
  until?: string;
  max?: number;
  range?: string;
  var?: string;
  summary?: string;
  body?: FormulaDashboardLoopBody[];
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
  loop?: FormulaDashboardLoop;
  depends_on?: string[];
  activities?: FormulaStepActivity[];
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
