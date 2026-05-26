package runtime

import (
	"encoding/json"
	"testing"

	"github.com/sjzsdu/tt/internal/formula/steps"
)

func TestExecutorSeedVarsAddsRuntimeTemplateValues(t *testing.T) {
	exec := NewExecutor(nil, steps.Capabilities{})
	exec.SeedVars(map[string]string{"env_name": "dev", "tag": "v1"})
	value, ok := exec.Context.Get("env_name")
	if !ok {
		t.Fatal("env_name was not seeded")
	}
	var got string
	if err := json.Unmarshal(value.Raw, &got); err != nil {
		t.Fatal(err)
	}
	if got != "dev" {
		t.Fatalf("env_name = %q, want dev", got)
	}
}
