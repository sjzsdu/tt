package steps

import (
	"context"
	"encoding/json"
	"testing"
)

type recordingWorkflowRunner struct {
	request WorkflowRequest
	result  *WorkflowResult
}

func (r *recordingWorkflowRunner) RunWorkflow(_ context.Context, req WorkflowRequest) (*WorkflowResult, error) {
	r.request = req
	return r.result, nil
}

func TestFormulaCallStepPreservesTypedInputAndCollectsOutputs(t *testing.T) {
	runner := &recordingWorkflowRunner{result: &WorkflowResult{
		Status:  StatusCompleted,
		Outputs: map[string]Value{"summary": {Type: "json", Raw: json.RawMessage(`{"ok":true}`)}},
	}}
	step := FormulaCallStep{
		Base:    Base{Metadata: Metadata{ID: "call", Kind: KindFormula}},
		Formula: "child",
		With: map[string]string{
			"payload": "{{source}}",
			"label":   "item={{name}}",
		},
	}
	ctx := mapContextView{
		"source": {Type: "json", Raw: json.RawMessage(`{"items":[1,2]}`)},
		"name":   {Type: "json", Raw: json.RawMessage(`"demo"`)},
	}
	result, err := step.Run(context.Background(), RunRequest{NodeID: "call", Context: ctx, Capabilities: Capabilities{Workflows: runner}})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(runner.request.Inputs["payload"].Raw); got != `{"items":[1,2]}` {
		t.Fatalf("typed payload = %s", got)
	}
	var label string
	if err := json.Unmarshal(runner.request.Inputs["label"].Raw, &label); err != nil || label != "item=demo" {
		t.Fatalf("label = %q, err = %v", label, err)
	}
	if got := string(result.Outputs["summary"].Raw); got != `{"ok":true}` {
		t.Fatalf("summary output = %s", got)
	}
}

func TestFormulaCallStepUsesReportAsPrimaryOutput(t *testing.T) {
	runner := &recordingWorkflowRunner{result: &WorkflowResult{Status: StatusCompleted, Outputs: map[string]Value{
		"data":   {Type: "json", Raw: json.RawMessage(`{"count":2}`)},
		"report": {Type: "markdown", Raw: json.RawMessage(`"# Final report"`)},
	}}}
	step := FormulaCallStep{Base: Base{Metadata: Metadata{ID: "call", Kind: KindFormula}}, Formula: "child"}
	result, err := step.Run(context.Background(), RunRequest{Capabilities: Capabilities{Workflows: runner}})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(result.Output.Raw); got != `"# Final report"` {
		t.Fatalf("primary output = %s", got)
	}
	if len(result.Outputs) != 2 {
		t.Fatalf("outputs = %#v", result.Outputs)
	}
}

func TestFormulaCallStepPropagatesWaiting(t *testing.T) {
	wait := &AwaitRequest{Type: string(KindHumanInput), Reason: "approval"}
	runner := &recordingWorkflowRunner{result: &WorkflowResult{Status: StatusWaiting, Await: wait}}
	step := FormulaCallStep{Base: Base{Metadata: Metadata{ID: "call", Kind: KindFormula}}, Formula: "child"}
	result, err := step.Run(context.Background(), RunRequest{Capabilities: Capabilities{Workflows: runner}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusWaiting || result.Await != wait {
		t.Fatalf("result = %+v", result)
	}
}
