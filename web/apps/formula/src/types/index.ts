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

export type FormulaExecutionPathSegment = {
  kind: 'step' | 'formula' | 'iteration' | string;
  id?: string;
  index?: number;
};

export type FormulaExecutionInstance = {
  address: string;
  path?: FormulaExecutionPathSegment[];
  definition_step_id: string;
  parent_loop_id?: string;
  formula_path?: string[];
  body_step_id?: string;
  iteration_path?: number[];
  title?: string;
  status: string;
  attempt?: number;
  started_at?: string;
  finished_at?: string;
  updated_at?: string;
  duration_ms?: number;
  session?: string;
  detail?: string;
  output?: string;
  error?: string;
};

export type FormulaExecutionEvent = {
  id: string;
  at: string;
  instance_address?: string;
  path?: FormulaExecutionPathSegment[];
  definition_step_id?: string;
  parent_loop_id?: string;
  formula_path?: string[];
  type: string;
  from_status?: string;
  status: string;
  attempt?: number;
  title?: string;
  detail?: string;
  duration_ms?: number;
  session?: string;
  error?: string;
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
  loop?: FormulaDashboardLoop;
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
  step_id?: string;
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
  queued_at?: string;
  started_at?: string;
  finished_at?: string;
  duration_ms?: number;
  priority?: number;
  labels?: string[];
  assignee?: string;
  output_key?: string;
  input_ctx?: string[];
  var_refs?: string[];
  execution?: string;
  formula?: string;
  condition?: string;
  metadata?: Record<string, string>;
  gate?: FormulaDashboardGate;
  loop?: FormulaDashboardLoop;
  depends_on?: string[];
  activities?: FormulaStepActivity[];
  human_input_request?: HumanInputRequest;
  depth?: number;
  index: number;
  script_path?: string;
  script_content?: string;
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
  execution_instances?: FormulaExecutionInstance[];
  execution_events?: FormulaExecutionEvent[];
  workspace_dir?: string;
  run_id?: string;
  stop_requested?: boolean;
  max_concurrency?: number;
  max_agent_concurrency?: number;
  current_concurrency?: number;
  current_agent_concurrency?: number;
};

export type FormulaDashboardMessage = {
  type: 'state';
  state: FormulaDashboardSnapshot;
};

export type DashboardView = 'list' | 'graph';
