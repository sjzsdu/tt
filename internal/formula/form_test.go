package formula

import (
	"testing"

	"github.com/sjzsdu/tt/internal/formula/ir"
	spec "github.com/sjzsdu/tt/internal/formula/spec"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

func TestHumanInputFormParsesValidatesAndCompiles(t *testing.T) {
	const src = `
formula = "human-demo"
version = 1
type = "workflow"

[[steps]]
id = "profile"
title = "收集背景"
execution = "human_input"
output_key = "profile"

[steps.form]
title = "请补充背景"
description = "用于后续步骤。"
submit_label = "继续"

[[steps.form.fields]]
name = "level"
label = "当前水平"
type = "radio"
required = true
options = ["新手", "了解基础", "熟练"]

[[steps.form.fields]]
name = "goal"
label = "学习目标"
type = "textarea"
placeholder = "描述你的目标"

[[steps]]
id = "plan"
title = "制定计划"
depends_on = ["profile"]
input_context = ["profile"]
`
	p := NewParser()
	f, err := p.ParseTOML([]byte(src))
	if err != nil {
		t.Fatalf("ParseTOML() error = %v", err)
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if f.Steps[0].Form == nil || len(f.Steps[0].Form.Fields) != 2 {
		t.Fatalf("form not parsed: %+v", f.Steps[0].Form)
	}
	workflow := WorkflowFromFormula(f)
	node := workflow.Graph.Nodes[ir.NodeID("profile")]
	if node == nil {
		t.Fatal("compiled human input step not found")
	}
	step, ok := node.Step.(steps.HumanInputStep)
	if !ok {
		t.Fatalf("step type = %T, want HumanInputStep", node.Step)
	}
	form, ok := step.Form.(*spec.FormSpec)
	if !ok || form.Title != "请补充背景" || len(form.Fields) != 2 {
		t.Fatalf("compiled form mismatch: %+v", step.Form)
	}
	if form.Fields[0].Type != "radio" || len(form.Fields[0].Options) != 3 {
		t.Fatalf("compiled first field mismatch: %+v", form.Fields[0])
	}
}

func TestLoopMaxParsesTemplatedString(t *testing.T) {
	const src = `
formula = "loop-demo"
version = 1
type = "workflow"

[vars]
max_cycles = { default = "3" }

[[steps]]
id = "cycle"
title = "Cycle"

[steps.loop]
max = "{{max_cycles}}"
until = "done == true"

[[steps.loop.body]]
id = "tick"
title = "Tick"
execution = "noop"
`
	p := NewParser()
	f, err := p.ParseTOML([]byte(src))
	if err != nil {
		t.Fatalf("ParseTOML() error = %v", err)
	}
	if got := f.Steps[0].Loop.MaxExpr; got != "{{max_cycles}}" {
		t.Fatalf("Loop.MaxExpr = %q", got)
	}
	wf := WorkflowFromFormula(f)
	step, ok := wf.Graph.Nodes[ir.NodeID("cycle")].Step.(steps.LoopStep)
	if !ok {
		t.Fatalf("step type = %T", wf.Graph.Nodes[ir.NodeID("cycle")].Step)
	}
	if step.MaxExpr != "{{max_cycles}}" {
		t.Fatalf("compiled MaxExpr = %q", step.MaxExpr)
	}
}

func TestFormValidationRejectsInvalidFields(t *testing.T) {
	f := &spec.Formula{Formula: "bad", Version: 1, Type: spec.TypeWorkflow, Steps: []*spec.Step{{
		ID:        "input",
		Title:     "Input",
		Execution: "human_input",
		Form: &spec.FormSpec{Fields: []*spec.FormField{{
			Name:  "choice",
			Label: "Choice",
			Type:  "select",
		}}},
	}}}
	if err := f.Validate(); err == nil {
		t.Fatal("expected validation error for select field without options")
	}
}

func TestDynamicFormAndValidateParseCompile(t *testing.T) {
	const src = `
formula = "dynamic-demo"
version = 1
type = "workflow"

[[steps]]
id = "triage"
title = "Triage"
form = true
output_key = "triage"

[steps.validate]
format = "json"
required = ["ok", "nested.value"]
`
	p := NewParser()
	f, err := p.ParseTOML([]byte(src))
	if err != nil {
		t.Fatalf("ParseTOML() error = %v", err)
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !f.Steps[0].DynamicForm || f.Steps[0].Form != nil {
		t.Fatalf("dynamic form not parsed correctly: dynamic=%v form=%+v", f.Steps[0].DynamicForm, f.Steps[0].Form)
	}
	workflow := WorkflowFromFormula(f)
	node := workflow.Graph.Nodes[ir.NodeID("triage")]
	step, ok := node.Step.(steps.AgentStep)
	if node == nil || !ok || !step.DynamicForm {
		t.Fatalf("compiled dynamic form mismatch: %+v", node)
	}
}
