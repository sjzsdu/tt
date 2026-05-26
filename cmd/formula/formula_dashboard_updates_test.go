package formulacmd

import (
	"strings"
	"testing"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formula/steps"
	"github.com/sjzsdu/tt/internal/formulaui"
)

func TestDashboardStepUpdatesPopulateTimelineLogs(t *testing.T) {
	workflow := &ir.Workflow{ID: "demo", Name: "demo", Graph: ir.NewGraph()}
	workflow.Graph.AddNode(&ir.Node{ID: "work", Step: steps.NoopStep{Base: steps.Base{Metadata: steps.Metadata{ID: "work", Kind: steps.KindNoop, Title: "Work"}}}})
	dashboard := newFormulaDashboardServer(workflow)
	dashboard.state.Steps = append(dashboard.state.Steps, structStepForTest("work", "Work"))

	dashboard.markStepRunning("work", "Work", "", "", "")
	dashboard.markStepCompleted("work", "ok")

	if len(dashboard.state.Logs) < 2 {
		t.Fatalf("logs = %+v, want timeline entries", dashboard.state.Logs)
	}
	joined := dashboard.state.Logs[0].Text + "\n" + dashboard.state.Logs[1].Text
	if !strings.Contains(joined, "Step work started") || !strings.Contains(joined, "Step work completed") {
		t.Fatalf("unexpected logs: %+v", dashboard.state.Logs)
	}
}

func structStepForTest(id, title string) formulaui.Step {
	return formulaui.Step{ID: id, Title: title, Status: "pending"}
}
