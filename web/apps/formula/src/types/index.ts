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
  session?: string;
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
  var_refs?: string[];
  condition?: string;
  depends_on?: string[];
};

export type FormulaDashboardLoop = {
  count?: number;
  until?: string;
  max?: number;
  range?: string;
  for_each?: string;
  var?: string;
  parallel?: boolean;
  max_concurrency?: number;
  summary?: string;
  body?: FormulaDashboardLoopBody[];
};

export type FormulaFormField = {
  name: string;
  label: string;
  type: 'input' | 'textarea' | 'radio' | 'checkbox' | 'select' | string;
  required?: boolean;
  placeholder?: string;
  default?: string;
  options?: string[];
  help?: string;
};

export type FormulaFormSpec = {
  title?: string;
  description?: string;
  submit_label?: string;
  fields?: FormulaFormField[];
};

export type HumanInputRequest = {
  reason?: string;
  form?: FormulaFormSpec;
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
  var_refs?: string[];
  execution?: string;
  condition?: string;
  metadata?: Record<string, string>;
  gate?: FormulaDashboardGate;
  loop?: FormulaDashboardLoop;
  depends_on?: string[];
  activities?: FormulaStepActivity[];
  human_input_request?: HumanInputRequest;
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

export type FinalReportChatMessage = {
  role: 'user' | 'assistant' | string;
  content: string;
  at?: string;
  error?: string;
};

export type FinalReportChat = {
  session_id?: string;
  agent?: string;
  status?: string;
  error?: string;
  messages?: FinalReportChatMessage[];
};

export type FormulaRepairRecord = {
  step_id: string;
  kind?: string;
  attempt?: number;
  status?: string;
  reason?: string;
  formula_update_hint?: string;
  next_attempt_hint?: string;
  advice?: string;
  original_command?: string[];
  fixed_command?: string[];
  error?: string;
  recorded_at?: string;
  confirmed_at?: string;
  confirmation_status?: string;
};

export type FormulaDashboardSnapshot = {
  recipe_name: string;
  description?: string;
  phase?: string;
  status: string;
  final_output?: string;
  final_report_chat?: FinalReportChat;
  repairs?: FormulaRepairRecord[];
  error?: string;
  vars?: Record<string, unknown>;
  steps: FormulaDashboardStep[];
  edges: FormulaDashboardEdge[];
  logs: FormulaDashboardLogEntry[];
  workspace_dir?: string;
  run_id?: string;
};

export type FormulaDashboardMessage = {
  type: 'state';
  state: FormulaDashboardSnapshot;
};

export type DashboardView = 'list' | 'graph';
