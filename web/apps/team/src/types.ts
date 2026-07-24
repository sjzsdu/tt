export type TeamInfo = {
  team: string;
  title?: string;
  description?: string;
};

export type TeamThread = {
  id: string;
  team: string;
  title?: string;
  status: string;
  created_at: string;
  updated_at: string;
  workspace: string;
  current_round: number;
  last_answer?: string;
  error?: string;
};

export type TeamRound = {
  number: number;
  status: string;
  phase: string;
  review_wave?: number;
  question: string;
  started_at: string;
  finished_at?: string;
  final_answer?: string;
  memory_version?: number;
  error?: string;
  collaboration?: TeamCollaboration;
};

export type TeamActivation = {
  member_id: string;
  reason: string;
  source_event_id?: number;
};

export type TeamObjection = {
  event_id: number;
  from: string;
  targets?: string[];
  content?: string;
  resolved?: boolean;
  resolved_by_event_id?: number;
};

export type TeamCollaboration = {
  turn_count: number;
  cycle: number;
  broad_review_waves: number;
  pending?: TeamActivation[];
  objections?: TeamObjection[];
  proposal_by?: string;
  converged?: boolean;
  stop_reason?: string;
};

export type TeamAgent = {
  id: string;
  role?: string;
  agent?: string;
  model?: string;
  facilitator?: boolean;
  finalizer?: boolean;
  memory_maintainer?: boolean;
};

export type TeamEvent = {
  id: number;
  type: string;
  at: string;
  thread_id: string;
  round: number;
  phase?: string;
  wave?: number;
  from?: string;
  to?: string[];
  signal?: string;
  ref?: number;
  content?: string;
  error?: string;
};

export type TeamMemory = {
  team: string;
  version: number;
  updated_at?: string;
  source_thread?: string;
  source_round?: number;
  content: string;
  path?: string;
};

export type TeamDashboardState = {
  team: TeamInfo;
  thread: TeamThread;
  round?: TeamRound;
  agents: TeamAgent[];
  events: TeamEvent[];
  memory: TeamMemory;
};
