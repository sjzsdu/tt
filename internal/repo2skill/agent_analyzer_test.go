package repo2skill

import "testing"

type fakeProcessor struct{ resp string }

func (f fakeProcessor) ProcessDirect(string) (string, error) { return f.resp, nil }

func TestAgentAnalyzerParsesJSONFence(t *testing.T) {
	p := &RepoProfile{Name: "demo", InstallHints: []string{"go get example.com/demo"}, PublicAPIs: []APISymbol{{Name: "Do", Source: "demo.go"}}}
	a := AgentAnalyzer{Processor: fakeProcessor{resp: "```json\n{\"purpose\":\"Demo library\",\"install\":[\"go get example.com/demo\"],\"public_api\":[{\"name\":\"Do\",\"kind\":\"function\",\"source\":\"demo.go\",\"evidence\":\"demo.go\"}],\"recipes\":[{\"title\":\"Call Do\",\"description\":\"Use Do.\",\"example\":\"Do()\",\"evidence\":[\"demo.go\"]}],\"best_practices\":[\"Use public API.\"]}\n```"}}
	m, err := a.Analyze(p)
	if err != nil {
		t.Fatal(err)
	}
	if m.Purpose != "Demo library" || len(m.Recipes) != 1 || m.Recipes[0].Title != "Call Do" {
		t.Fatalf("unexpected model: %#v", m)
	}
}
