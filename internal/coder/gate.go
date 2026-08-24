package coder

const (
	GateTypeProductIntent      = "product_intent"
	GateTypeFeatureScope       = "feature_scope"
	GateTypeArchitecture       = "architecture"
	GateTypeDesign             = "design"
	GateTypeImplementationPlan = "implementation_plan"
	GateTypeRelease            = "release"
)

const (
	GateStatusPending             = "pending"
	GateStatusWaitingHuman        = "waiting_human"
	GateStatusApproved            = "approved"
	GateStatusApprovedWithChanges = "approved_with_changes"
	GateStatusRejected            = "rejected"
	GateStatusSuperseded          = "superseded"
)

const (
	ReviewDecisionApprove            = "approve"
	ReviewDecisionApproveWithChanges = "approve_with_changes"
	ReviewDecisionRequestRevision    = "request_revision"
	ReviewDecisionReject             = "reject"
)

type ReviewGate struct {
	SchemaVersion   int      `json:"schema_version"`
	ID              string   `json:"id"`
	ProjectID       string   `json:"project_id"`
	Type            string   `json:"type"`
	Status          string   `json:"status"`
	Title           string   `json:"title"`
	Summary         string   `json:"summary,omitempty"`
	FormSpecID      string   `json:"form_spec_id,omitempty"`
	ResponseID      string   `json:"response_id,omitempty"`
	CreatedByAgent  string   `json:"created_by_agent,omitempty"`
	ApprovedBy      string   `json:"approved_by,omitempty"`
	CreatedAt       string   `json:"created_at"`
	ResolvedAt      string   `json:"resolved_at,omitempty"`
	LinkedDecisions []string `json:"linked_decisions,omitempty"`
	LinkedTasks     []string `json:"linked_tasks,omitempty"`
	LinkedArtifacts []string `json:"linked_artifacts,omitempty"`
}

type DynamicFormSpec struct {
	SchemaVersion int            `json:"schema_version"`
	ID            string         `json:"id"`
	GateID        string         `json:"gate_id"`
	Title         string         `json:"title"`
	Description   string         `json:"description,omitempty"`
	Fields        []FormField    `json:"fields,omitempty"`
	SubmitActions []string       `json:"submit_actions,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	CreatedAt     string         `json:"created_at"`
}

type FormField struct {
	ID       string         `json:"id"`
	Label    string         `json:"label"`
	Type     string         `json:"type"`
	Required bool           `json:"required,omitempty"`
	Options  []string       `json:"options,omitempty"`
	Default  any            `json:"default,omitempty"`
	Help     string         `json:"help,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type HumanReviewResponse struct {
	SchemaVersion   int            `json:"schema_version"`
	ID              string         `json:"id"`
	GateID          string         `json:"gate_id"`
	ProjectID       string         `json:"project_id"`
	Decision        string         `json:"decision"`
	Answers         map[string]any `json:"answers,omitempty"`
	FreeformComment string         `json:"freeform_comment,omitempty"`
	Reviewer        string         `json:"reviewer"`
	CreatedAt       string         `json:"created_at"`
}

type Decision struct {
	SchemaVersion int      `json:"schema_version"`
	ID            string   `json:"id"`
	ProjectID     string   `json:"project_id"`
	Source        string   `json:"source,omitempty"`
	Content       string   `json:"content"`
	Reason        string   `json:"reason,omitempty"`
	Alternatives  []string `json:"alternatives,omitempty"`
	CreatedAt     string   `json:"created_at"`
}

type Task struct {
	SchemaVersion int         `json:"schema_version"`
	ID            string      `json:"id"`
	ProjectID     string      `json:"project_id"`
	Title         string      `json:"title"`
	Status        string      `json:"status"`
	OwnerAgent    string      `json:"owner_agent,omitempty"`
	Dependencies  []string    `json:"dependencies,omitempty"`
	Inputs        TaskInputs  `json:"inputs,omitempty"`
	Outputs       TaskOutputs `json:"outputs,omitempty"`
	CreatedAt     string      `json:"created_at"`
	UpdatedAt     string      `json:"updated_at"`
}

type TaskInputs struct {
	ContextPacketVersion int      `json:"context_packet_version,omitempty"`
	Decisions            []string `json:"decisions,omitempty"`
	ReviewGates          []string `json:"review_gates,omitempty"`
}

type TaskOutputs struct {
	Artifacts    []string `json:"artifacts,omitempty"`
	Commits      []string `json:"commits,omitempty"`
	TestEvidence []string `json:"test_evidence,omitempty"`
}

type Artifact struct {
	SchemaVersion int    `json:"schema_version"`
	ID            string `json:"id"`
	ProjectID     string `json:"project_id"`
	Type          string `json:"type"`
	PathOrURL     string `json:"path_or_url,omitempty"`
	ProducerAgent string `json:"producer_agent,omitempty"`
	LinkedTask    string `json:"linked_task,omitempty"`
	Version       int    `json:"version,omitempty"`
	Summary       string `json:"summary,omitempty"`
	CreatedAt     string `json:"created_at"`
}
