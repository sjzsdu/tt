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
  blackboard?: TeamBlackboardOperation;
  memory_proposal?: string;
  metrics?: TeamEventMetrics;
  content?: string;
  error?: string;
};

export type TeamEventMetrics = {
  duration_ms?: number;
  turn?: number;
  model?: string;
  input_chars?: number;
  output_chars?: number;
};

export type TeamBlackboardOperation = {
  action: 'upsert' | 'resolve';
  kind: TeamBlackboardKind;
  key: string;
  content?: string;
};

export type TeamBlackboardKind = 'fact' | 'proposal' | 'question' | 'decision' | 'objection' | 'artifact';

export type TeamBlackboardRevision = {
  event_id: number;
  ref?: number;
  action: 'upsert' | 'resolve';
  by?: string;
  at?: string;
  content?: string;
};

export type TeamBlackboardEntry = {
  kind: TeamBlackboardKind;
  key: string;
  content?: string;
  status: 'active' | 'resolved';
  updated_by?: string;
  updated_at_event_id: number;
  revisions: TeamBlackboardRevision[];
};

export type TeamBlackboard = {
  round: number;
  entries: TeamBlackboardEntry[];
};

export type TeamMemory = {
  team: string;
  version: number;
  updated_at?: string;
  source_thread?: string;
  source_round?: number;
  source_events?: number[];
  restored_from?: number;
  content: string;
  path?: string;
};

export type TeamMemoryProposal = {
  id: string;
  base_version: number;
  proposed_version: number;
  created_at: string;
  source_thread: string;
  source_round: number;
  source_events?: number[];
  maintainer?: string;
  content: string;
  diff: string;
  status: string;
  error?: string;
};

export type TeamMemoryReview = {
  current: TeamMemory;
  versions: TeamMemory[];
  proposals: TeamMemoryProposal[];
};

export type TeamDashboardState = {
  team: TeamInfo;
  thread: TeamThread;
  round?: TeamRound;
  agents: TeamAgent[];
  events: TeamEvent[];
  blackboard: TeamBlackboard;
  memory: TeamMemory;
  memory_review: TeamMemoryReview;
  controls: {
    busy: boolean;
    can_follow_up: boolean;
    can_resume: boolean;
    can_stop: boolean;
    can_retry_memory: boolean;
    can_rollback_memory: boolean;
  };
};
