package steps

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sjzsdu/tt/internal/formula/ast"
)

func TestScriptStepWithExternalScript(t *testing.T) {
	// Create a temporary script file
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.py")
	scriptContent := `#!/usr/bin/env python3
import json, os
print(json.dumps({"ok": True, "msg": "hello from external script"}))
`
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("write test script: %v", err)
	}

	// Create StepDecl with external script
	decl := ast.StepDecl{
		ID:        "test",
		Kind:      "script",
		SourceDir: tmpDir,
		Raw:       []byte(`{"script": "test.py"}`),
	}

	// Decode the step
	decoder := ScriptDecoder{}
	step, err := decoder.Decode(decl)
	if err != nil {
		t.Fatalf("decode step: %v", err)
	}

	scriptStep, ok := step.(ScriptStep)
	if !ok {
		t.Fatal("expected ScriptStep")
	}

	// Verify command was loaded from file
	if len(scriptStep.Command) == 0 {
		t.Fatal("expected command to be loaded from external script")
	}

	// Check interpreter detection
	if scriptStep.Command[0] != "python3" {
		t.Errorf("expected python3 interpreter, got %q", scriptStep.Command[0])
	}
	if scriptStep.Command[1] != scriptPath {
		t.Errorf("expected script path %q, got %q", scriptPath, scriptStep.Command[1])
	}
}

func TestScriptDecoderLoadsExternalScript(t *testing.T) {
	// Create a temporary script file
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.sh")
	scriptContent := `#!/bin/bash
echo '{"ok": true}'
`
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("write test script: %v", err)
	}

	// Create StepDecl with external script
	decl := ast.StepDecl{
		ID:        "test",
		Kind:      "script",
		SourceDir: tmpDir,
		Raw:       []byte(`{"script": "test.sh"}`),
	}

	// Decode the step
	decoder := ScriptDecoder{}
	step, err := decoder.Decode(decl)
	if err != nil {
		t.Fatalf("decode step: %v", err)
	}

	scriptStep, ok := step.(ScriptStep)
	if !ok {
		t.Fatal("expected ScriptStep")
	}

	// Verify command was loaded
	if len(scriptStep.Command) == 0 {
		t.Fatal("expected command to be loaded from external script")
	}

	// Check interpreter detection
	if scriptStep.Command[0] != "bash" {
		t.Errorf("expected bash interpreter, got %q", scriptStep.Command[0])
	}
}

func TestScriptDecoderRelativePathResolution(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	scriptsDir := filepath.Join(tmpDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatalf("create scripts dir: %v", err)
	}

	// Create script in subdirectory
	scriptPath := filepath.Join(scriptsDir, "helper.py")
	scriptContent := `#!/usr/bin/env python3
print('{"ok": true}')
`
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("write test script: %v", err)
	}

	// Create StepDecl with relative path
	decl := ast.StepDecl{
		ID:        "test",
		Kind:      "script",
		SourceDir: tmpDir,
		Raw:       []byte(`{"script": "scripts/helper.py"}`),
	}

	// Decode the step
	decoder := ScriptDecoder{}
	step, err := decoder.Decode(decl)
	if err != nil {
		t.Fatalf("decode step: %v", err)
	}

	scriptStep, ok := step.(ScriptStep)
	if !ok {
		t.Fatal("expected ScriptStep")
	}

	// Verify command uses resolved path
	if len(scriptStep.Command) < 2 {
		t.Fatal("expected command with interpreter and path")
	}

	if scriptStep.Command[1] != scriptPath {
		t.Errorf("expected resolved path %q, got %q", scriptPath, scriptStep.Command[1])
	}
}

func TestDetectInterpreter(t *testing.T) {
	tests := []struct {
		path     string
		content  string
		expected string
	}{
		{"test.py", "", "python3"},
		{"test.sh", "", "bash"},
		{"test.js", "", "node"},
		{"test.rb", "", "ruby"},
		{"test.go", "", ""},
		{"noext", "#!/usr/bin/env python3\nprint('hello')", "python3"},
		{"noext", "#!/bin/bash\necho hello", "bash"},
		{"noext", "", ""},
	}

	for _, tt := range tests {
		result := detectInterpreter(tt.path, tt.content)
		if result != tt.expected {
			t.Errorf("detectInterpreter(%q, %q) = %q, want %q", tt.path, tt.content, result, tt.expected)
		}
	}
}
