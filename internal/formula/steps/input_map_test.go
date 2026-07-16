package steps

import (
	"encoding/json"
	"testing"
)

func TestInputMapResolvesNestedNamedInput(t *testing.T) {
	inputs := InputMap{"upstream": {Type: "json", Raw: json.RawMessage(`{"items":[{"name":"first"}]}`)}}
	value, ok := inputs.Get("upstream.items.0.name")
	if !ok || string(value.Raw) != `"first"` {
		t.Fatalf("value=%s ok=%v", value.Raw, ok)
	}
}

func TestInputMapDoesNotPrefixMatchSimpleNames(t *testing.T) {
	inputs := InputMap{
		"result":  {Type: "json", Raw: json.RawMessage(`{"value":"short"}`)},
		"results": {Type: "json", Raw: json.RawMessage(`{"value":"exact"}`)},
	}
	value, ok := inputs.Get("results")
	if !ok || string(value.Raw) != `{"value":"exact"}` {
		t.Fatalf("exact value=%s ok=%v", value.Raw, ok)
	}
	nested, ok := inputs.Get("results.value")
	if !ok || string(nested.Raw) != `"exact"` {
		t.Fatalf("nested value=%s ok=%v", nested.Raw, ok)
	}
	if _, ok := inputs.Get("resulting"); ok {
		t.Fatal("simple name must not use prefix matching")
	}
}

func TestRunResultNormalizesLegacyOutputToResultPort(t *testing.T) {
	result := &RunResult{Status: StatusCompleted, Output: Value{Type: "json", Raw: json.RawMessage(`{"ok":true}`)}}
	result.NormalizeOutputs()
	if got := string(result.Outputs[OutputResult].Raw); got != `{"ok":true}` {
		t.Fatalf("result output = %s", got)
	}
}

func TestRunResultPrefersReportPort(t *testing.T) {
	result := &RunResult{Status: StatusCompleted, Outputs: map[string]Value{
		"data":       {Type: "json", Raw: json.RawMessage(`{"count":1}`)},
		OutputReport: {Type: "markdown", Raw: json.RawMessage(`"# Report"`)},
	}}
	result.NormalizeOutputs()
	if got := string(result.Output.Raw); got != `"# Report"` {
		t.Fatalf("primary output = %s", got)
	}
}
