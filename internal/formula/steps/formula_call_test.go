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
	var output map[string]any
	if err := json.Unmarshal(result.Output.Raw, &output); err != nil {
		t.Fatal(err)
	}
	if summary, ok := output["summary"].(map[string]any); !ok || summary["ok"] != true {
		t.Fatalf("output = %#v", output)
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
