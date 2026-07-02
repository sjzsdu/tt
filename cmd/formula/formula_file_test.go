package formulacmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFormulaFileParsesFormulaAndVars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.toml")
	if err := os.WriteFile(path, []byte(`formula = "web-feature-test"

[vars]
url = "https://example.com"
prompt = "测试登录"
only_failed = "true"
`), 0o600); err != nil {
		t.Fatalf("write formula file: %v", err)
	}
	name, vars, err := loadFormulaFile(path)
	if err != nil {
		t.Fatalf("loadFormulaFile() error = %v", err)
	}
	if name != "web-feature-test" {
		t.Fatalf("name = %q", name)
	}
	want := map[string]string{"url": "https://example.com", "prompt": "测试登录", "only_failed": "true"}
	for key, value := range want {
		if vars[key] != value {
			t.Fatalf("vars[%q] = %q, want %q", key, vars[key], value)
		}
	}
}

func TestLoadFormulaFileAllowsVarsOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.toml")
	if err := os.WriteFile(path, []byte("[vars]\ntopic = \"x\"\n"), 0o600); err != nil {
		t.Fatalf("write formula file: %v", err)
	}
	name, vars, err := loadFormulaFile(path)
	if err != nil {
		t.Fatalf("loadFormulaFile() error = %v", err)
	}
	if name != "" {
		t.Fatalf("name = %q, want empty", name)
	}
	if vars["topic"] != "x" {
		t.Fatalf("vars[topic] = %q", vars["topic"])
	}
}

func TestLoadFormulaFileTreatsTopLevelScalarsAsVars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.toml")
	if err := os.WriteFile(path, []byte(`prompt = """
1. 测试 Probe 面板
2. 验证保存
"""
only_failed = true
max_cases = 2
`), 0o600); err != nil {
		t.Fatalf("write formula file: %v", err)
	}
	name, vars, err := loadFormulaFile(path)
	if err != nil {
		t.Fatalf("loadFormulaFile() error = %v", err)
	}
	if name != "" {
		t.Fatalf("name = %q, want empty", name)
	}
	if vars["prompt"] != "1. 测试 Probe 面板\n2. 验证保存\n" {
		t.Fatalf("prompt = %q", vars["prompt"])
	}
	if vars["only_failed"] != "true" {
		t.Fatalf("only_failed = %q", vars["only_failed"])
	}
	if vars["max_cases"] != "2" {
		t.Fatalf("max_cases = %q", vars["max_cases"])
	}
}

func TestLoadFormulaFileVarsTableOverridesTopLevelVars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.toml")
	if err := os.WriteFile(path, []byte(`prompt = "top-level"

[vars]
prompt = "vars-table"
`), 0o600); err != nil {
		t.Fatalf("write formula file: %v", err)
	}
	_, vars, err := loadFormulaFile(path)
	if err != nil {
		t.Fatalf("loadFormulaFile() error = %v", err)
	}
	if vars["prompt"] != "vars-table" {
		t.Fatalf("prompt = %q, want vars-table", vars["prompt"])
	}
}

func TestMergeFormulaVarsLetsOverridesWin(t *testing.T) {
	merged := mergeFormulaVars(map[string]string{"url": "file", "prompt": "file"}, map[string]string{"prompt": "cli"})
	if merged["url"] != "file" || merged["prompt"] != "cli" {
		t.Fatalf("merged vars = %#v", merged)
	}
}
