package run

import (
	"fmt"
	"strings"

	"github.com/sjzsdu/tt/internal/formula/ir"
)

// ValidateWorkflowDefinition prevents resume from combining persisted step
// state with a different root or transitive child Formula definition. Runs
// created before workflow hashes were introduced remain readable/resumable.
func ValidateWorkflowDefinition(metadata Metadata, workflow *ir.Workflow) error {
	expected := strings.TrimSpace(metadata.WorkflowHash)
	if expected == "" {
		return nil
	}
	if workflow == nil {
		return fmt.Errorf("cannot verify formula definition for resume: workflow is unavailable")
	}
	actual := strings.TrimSpace(workflow.DefinitionHash)
	if actual == "" {
		return fmt.Errorf("cannot resume formula %q: current workflow has no definition hash", metadata.Formula)
	}
	if expected != actual {
		return fmt.Errorf("cannot resume formula %q because its definition or a called Formula changed; start a new run", metadata.Formula)
	}
	return nil
}
