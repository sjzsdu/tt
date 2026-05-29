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

func TestDashboardFinalReportChatLifecycle(t *testing.T) {
	dashboard := newFormulaDashboardServer(nil)
	dashboard.state.RunID = "run-123"
	dashboard.markWorkflowWorkspaceReady("/tmp/gongbu-worktree")
	dashboard.markWorkflowCompleted("final report")
	chat, err := dashboard.ensureFinalReportChat()
	if err != nil {
		t.Fatal(err)
	}
	if chat.Agent != finalReportChatAgent || chat.SessionID != "run-123:final-report-chat" {
		t.Fatalf("chat = %+v", chat)
	}
	prompt := dashboard.buildFinalReportChatPrompt("improve it")
	if !strings.Contains(prompt, "final report") || !strings.Contains(prompt, "improve it") || !strings.Contains(prompt, "/tmp/gongbu-worktree") {
		t.Fatalf("prompt = %q", prompt)
	}
}

func TestDashboardWorkspaceReadyUpdatesSnapshot(t *testing.T) {
	dashboard := newFormulaDashboardServer(nil)
	dashboard.state.WorkspaceDir = "/repo/.tt"
	dashboard.markWorkflowWorkspaceReady("/repo/.tt/worktrees/gongbu-run")
	if dashboard.state.WorkspaceDir != "/repo/.tt/worktrees/gongbu-run" {
		t.Fatalf("workspace = %q", dashboard.state.WorkspaceDir)
	}
	if len(dashboard.state.Logs) == 0 || !strings.Contains(dashboard.state.Logs[len(dashboard.state.Logs)-1].Text, "Workspace ready") {
		t.Fatalf("logs = %+v", dashboard.state.Logs)
	}
}

func structStepForTest(id, title string) formulaui.Step {
	return formulaui.Step{ID: id, Title: title, Status: "pending"}
}
