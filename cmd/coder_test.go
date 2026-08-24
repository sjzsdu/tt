package cmd

import (
	"bytes"
	"strings"
	"testing"

	coderruntime "github.com/sjzsdu/tt/internal/coder"
	"github.com/spf13/cobra"
)

func TestCoderCommandRegistersPhaseTwoSubcommands(t *testing.T) {
	want := map[string]bool{"start": false, "status": false, "show": false, "approve": false}
	for _, command := range coderCmd.Commands() {
		if _, ok := want[command.Name()]; ok {
			want[command.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("tt coder %s is not registered", name)
		}
	}
}

func TestCoderStartStatusShowApproveFlow(t *testing.T) {
	root := t.TempDir()
	resetCoderTestGlobals(root)
	coderID = "ai"

	startOut := runCoderCommandForTest(t, runCoderStart, []string{"我想做一个 AI 招聘助手"})
	if !strings.Contains(startOut, "Created coder project ai") || !strings.Contains(startOut, "Waiting review gate: product-intent") {
		t.Fatalf("start output = %s", startOut)
	}

	statusOut := runCoderCommandForTest(t, runCoderStatus, nil)
	if !strings.Contains(statusOut, "Status: planning") || !strings.Contains(statusOut, "Waiting gate: product-intent") {
		t.Fatalf("status output = %s", statusOut)
	}

	showOut := runCoderCommandForTest(t, runCoderShow, nil)
	if !strings.Contains(showOut, "Form fields:") || !strings.Contains(showOut, "target_users") {
		t.Fatalf("show output = %s", showOut)
	}

	coderDecision = coderruntime.ReviewDecisionApproveWithChanges
	coderComment = "先做最小 MVP，不做权限"
	coderSetValues = []string{"target_users=small teams", "priority=最快可用"}
	approveOut := runCoderCommandForTest(t, runCoderApprove, []string{"product-intent"})
	if !strings.Contains(approveOut, "Submitted review for gate product-intent") || !strings.Contains(approveOut, "Current gate: none") {
		t.Fatalf("approve output = %s", approveOut)
	}

	store, err := coderruntime.OpenStore(root, "ai")
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.LoadProject()
	if err != nil {
		t.Fatal(err)
	}
	if project.CurrentGate != "" || project.CurrentContext != 2 {
		t.Fatalf("project after approve = %+v", project)
	}
	gate, err := store.LoadReviewGate("product-intent")
	if err != nil {
		t.Fatal(err)
	}
	if gate.Status != coderruntime.GateStatusApprovedWithChanges || gate.ResponseID == "" || gate.ResolvedAt == "" {
		t.Fatalf("gate after approve = %+v", gate)
	}
	response, err := store.LoadHumanReviewResponse("product-intent")
	if err != nil {
		t.Fatal(err)
	}
	if response.Answers["priority"] != "最快可用" || response.FreeformComment != coderComment {
		t.Fatalf("response = %+v", response)
	}
	decisions, err := store.Decisions()
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 || decisions[0].Source != "human_review" {
		t.Fatalf("decisions = %+v", decisions)
	}
}

func TestCoderStatusWithoutProjectExplainsStart(t *testing.T) {
	resetCoderTestGlobals(t.TempDir())
	cmd := &cobra.Command{}
	err := runCoderStatus(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "tt coder start") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseCoderSetValuesRejectsInvalidInput(t *testing.T) {
	if _, err := parseCoderSetValues([]string{"missing-equals"}); err == nil {
		t.Fatal("expected invalid --set error")
	}
}

func runCoderCommandForTest(t *testing.T, fn func(*cobra.Command, []string) error, args []string) string {
	t.Helper()
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := fn(cmd, args); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func resetCoderTestGlobals(root string) {
	coderRoot = root
	coderProject = ""
	coderName = ""
	coderID = ""
	coderDecision = coderruntime.ReviewDecisionApprove
	coderReviewer = "human"
	coderComment = ""
	coderSetValues = nil
	coderOutputJSON = false
}
