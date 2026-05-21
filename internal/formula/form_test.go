package formula

import "testing"

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
	recipe, err := toRecipe(f)
	if err != nil {
		t.Fatalf("toRecipe() error = %v", err)
	}
	step := recipe.StepByID("human-demo.profile")
	if step == nil {
		t.Fatal("compiled human input step not found")
	}
	if step.Execution != "human_input" {
		t.Fatalf("Execution = %q, want human_input", step.Execution)
	}
	if step.Form == nil || step.Form.Title != "请补充背景" || len(step.Form.Fields) != 2 {
		t.Fatalf("compiled form mismatch: %+v", step.Form)
	}
	if step.Form.Fields[0].Type != "radio" || len(step.Form.Fields[0].Options) != 3 {
		t.Fatalf("compiled first field mismatch: %+v", step.Form.Fields[0])
	}
}

func TestFormValidationRejectsInvalidFields(t *testing.T) {
	f := &Formula{Formula: "bad", Version: 1, Type: TypeWorkflow, Steps: []*Step{{
		ID:        "input",
		Title:     "Input",
		Execution: "human_input",
		Form: &FormSpec{Fields: []*FormField{{
			Name:  "choice",
			Label: "Choice",
			Type:  "select",
		}}},
	}}}
	if err := f.Validate(); err == nil {
		t.Fatal("expected validation error for select field without options")
	}
}
