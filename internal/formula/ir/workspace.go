package ir

// WorkspacePolicy describes a workflow-level execution workspace policy.
type WorkspacePolicy struct {
	Kind    string
	Path    string
	Cleanup bool
}
