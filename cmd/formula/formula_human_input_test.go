package formulacmd

import (
	"github.com/sjzsdu/tt/internal/formulaui"
	"reflect"
	"testing"

	"github.com/sjzsdu/tt/internal/formula"
	"github.com/sjzsdu/tt/internal/formularunview"
)

func TestParseHumanInputFieldsDuplicatesBecomeArrays(t *testing.T) {
	got, err := parseHumanInputFields([]string{"goal=learn", "style=文章", "style=项目"})
	if err != nil {
		t.Fatalf("parseHumanInputFields() error = %v", err)
	}
	want := map[string]any{"goal": "learn", "style": []string{"文章", "项目"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("response = %#v, want %#v", got, want)
	}
}

func TestValidateHumanInputResponse(t *testing.T) {
	request := &formulaui.HumanInputRequest{Form: &formula.FormSpec{Fields: []*formula.FormField{
		{Name: "level", Label: "水平", Type: "radio", Required: true, Options: []string{"新手", "熟练"}},
		{Name: "goal", Label: "目标", Type: "textarea"},
	}}}
	if err := validateHumanInputResponse(request, map[string]any{"level": "新手"}); err != nil {
		t.Fatalf("validateHumanInputResponse() error = %v", err)
	}
	if err := validateHumanInputResponse(request, map[string]any{"goal": "学习"}); err == nil {
		t.Fatal("expected missing required field error")
	}
	if err := validateHumanInputResponse(request, map[string]any{"level": "新手", "extra": "x"}); err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestResolveFormulaRunStepIDRequiresWaitingStep(t *testing.T) {
	snapshot := formulaui.Snapshot{Steps: []formulaui.Step{
		{ID: "demo.profile", Status: formulaui.StatusWaitingInput},
		{ID: "demo.plan", Status: formulaui.StatusPending},
	}}
	got, err := formularunview.ResolveWaitingInputStepID(snapshot, "profile")
	if err != nil {
		t.Fatalf("formularunview.ResolveWaitingInputStepID() error = %v", err)
	}
	if got != "demo.profile" {
		t.Fatalf("step id = %q", got)
	}
	if _, err := formularunview.ResolveWaitingInputStepID(snapshot, "plan"); err == nil {
		t.Fatal("expected non-waiting step error")
	}
}
