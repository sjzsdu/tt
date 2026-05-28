package formula

import "testing"

func TestParserNormalizesWorktreeAlias(t *testing.T) {
	p := NewParser()
	f, err := p.ParseTOML([]byte(`formula = "demo"
worktree = true
`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Workspace == nil {
		t.Fatal("workspace policy missing")
	}
	if f.Workspace.Kind != "worktree" {
		t.Fatalf("workspace kind = %q", f.Workspace.Kind)
	}
}
