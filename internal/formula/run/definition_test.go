package run

import (
	"strings"
	"testing"

	"github.com/sjzsdu/tt/internal/formula/ir"
)

func TestValidateWorkflowDefinition(t *testing.T) {
	workflow := &ir.Workflow{Name: "demo", DefinitionHash: "current"}
	if err := ValidateWorkflowDefinition(Metadata{Formula: "demo", WorkflowHash: "current"}, workflow); err != nil {
		t.Fatalf("matching definition rejected: %v", err)
	}
	if err := ValidateWorkflowDefinition(Metadata{Formula: "demo"}, workflow); err != nil {
		t.Fatalf("legacy run without hash should remain resumable: %v", err)
	}
	if err := ValidateWorkflowDefinition(Metadata{Formula: "demo", WorkflowHash: "old"}, workflow); err == nil || !strings.Contains(err.Error(), "start a new run") {
		t.Fatalf("err = %v, want changed definition rejection", err)
	}
}
