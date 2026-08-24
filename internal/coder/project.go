package coder

const CurrentSchemaVersion = 1

const (
	ProjectStatusExploring  = "exploring"
	ProjectStatusPlanning   = "planning"
	ProjectStatusDesigning  = "designing"
	ProjectStatusDeveloping = "developing"
	ProjectStatusTesting    = "testing"
	ProjectStatusReleasing  = "releasing"
	ProjectStatusLive       = "live"
	ProjectStatusPaused     = "paused"
)

type Project struct {
	SchemaVersion  int      `json:"schema_version"`
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Vision         string   `json:"vision"`
	OwnerIntent    string   `json:"owner_intent,omitempty"`
	TargetUsers    []string `json:"target_users,omitempty"`
	Status         string   `json:"status"`
	CurrentThread  string   `json:"current_thread_id,omitempty"`
	CurrentGate    string   `json:"current_gate_id,omitempty"`
	CurrentContext int      `json:"current_context_version,omitempty"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

type ContextPacket struct {
	SchemaVersion int              `json:"schema_version"`
	ID            string           `json:"id"`
	ProjectID     string           `json:"project_id"`
	ThreadID      string           `json:"thread_id,omitempty"`
	Version       int              `json:"version"`
	Product       ProductContext   `json:"product"`
	Phase         PhaseContext     `json:"phase"`
	Decisions     DecisionsContext `json:"decisions,omitempty"`
	Constraints   Constraints      `json:"constraints,omitempty"`
	OpenQuestions []OpenQuestion   `json:"open_questions,omitempty"`
	AgentStates   []AgentState     `json:"agent_states,omitempty"`
	TaskGraph     TaskGraphSummary `json:"task_graph,omitempty"`
	Artifacts     []ArtifactRef    `json:"artifacts,omitempty"`
	ReviewGates   ReviewGateLinks  `json:"review_gates,omitempty"`
	CreatedAt     string           `json:"created_at"`
}

type ProductContext struct {
	Vision         string   `json:"vision,omitempty"`
	TargetUsers    []string `json:"target_users,omitempty"`
	CoreProblem    string   `json:"core_problem,omitempty"`
	CurrentStage   string   `json:"current_stage,omitempty"`
	HumanDirection string   `json:"human_direction,omitempty"`
	NonGoals       []string `json:"non_goals,omitempty"`
}

type PhaseContext struct {
	Name            string   `json:"name,omitempty"`
	Objective       string   `json:"objective,omitempty"`
	SuccessCriteria []string `json:"success_criteria,omitempty"`
}

type DecisionsContext struct {
	Accepted []DecisionRef `json:"accepted,omitempty"`
	Pending  []DecisionRef `json:"pending,omitempty"`
}

type DecisionRef struct {
	ID      string `json:"id"`
	Summary string `json:"summary,omitempty"`
}

type Constraints struct {
	TechStack          map[string]string `json:"tech_stack,omitempty"`
	Deployment         map[string]string `json:"deployment,omitempty"`
	QualityBar         map[string]string `json:"quality_bar,omitempty"`
	BudgetOrSimplicity string            `json:"budget_or_simplicity,omitempty"`
}

type OpenQuestion struct {
	ID      string   `json:"id"`
	Prompt  string   `json:"prompt"`
	Options []string `json:"options,omitempty"`
	Owner   string   `json:"owner,omitempty"`
}

type AgentState struct {
	ID          string `json:"id"`
	Role        string `json:"role,omitempty"`
	Status      string `json:"status,omitempty"`
	CurrentWork string `json:"current_work,omitempty"`
	BlockedBy   string `json:"blocked_by,omitempty"`
}

type TaskGraphSummary struct {
	Total      int      `json:"total,omitempty"`
	Pending    int      `json:"pending,omitempty"`
	Active     int      `json:"active,omitempty"`
	Blocked    int      `json:"blocked,omitempty"`
	Done       int      `json:"done,omitempty"`
	Milestones []string `json:"milestones,omitempty"`
}

type ArtifactRef struct {
	ID      string `json:"id"`
	Type    string `json:"type,omitempty"`
	Summary string `json:"summary,omitempty"`
	Path    string `json:"path,omitempty"`
}

type ReviewGateLinks struct {
	Current   string   `json:"current,omitempty"`
	Completed []string `json:"completed,omitempty"`
}
