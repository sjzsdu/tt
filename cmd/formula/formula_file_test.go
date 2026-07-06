package formulacmd

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLoadDefaultFormulaVarsFileInDirFindsFormulaDirectory(t *testing.T) {
	dir := t.TempDir()
	varsDir := filepath.Join(dir, ".tt", "formula")
	if err := os.MkdirAll(varsDir, 0o755); err != nil {
		t.Fatalf("mkdir vars dir: %v", err)
	}
	path := filepath.Join(varsDir, "web-feature-test.toml")
	if err := os.WriteFile(path, []byte(`prompt = "default prompt"
max_cases = 3

[vars]
only_failed = "true"
`), 0o600); err != nil {
		t.Fatalf("write default vars file: %v", err)
	}

	foundPath, vars, name, found, err := loadDefaultFormulaVarsFileInDir(dir, "web-feature-test")
	if err != nil {
		t.Fatalf("loadDefaultFormulaVarsFileInDir() error = %v", err)
	}
	if !found {
		t.Fatalf("found = false, want true")
	}
	if foundPath != path {
		t.Fatalf("path = %q, want %q", foundPath, path)
	}
	if name != "" {
		t.Fatalf("name = %q, want empty", name)
	}
	want := map[string]string{"prompt": "default prompt", "max_cases": "3", "only_failed": "true"}
	for key, value := range want {
		if vars[key] != value {
			t.Fatalf("vars[%q] = %q, want %q", key, vars[key], value)
		}
	}
}

func TestLoadDefaultFormulaVarsFileInDirCandidatePrecedence(t *testing.T) {
	dir := t.TempDir()
	for rel, value := range map[string]string{
		filepath.Join(".tt", "web-feature-test.toml"):                 "root",
		filepath.Join(".tt", "formula", "web-feature-test.vars.toml"): "vars-suffix",
		filepath.Join(".tt", "formula", "web-feature-test.toml"):      "formula",
	} {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte("source = \""+value+"\"\n"), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	_, vars, _, found, err := loadDefaultFormulaVarsFileInDir(dir, "web-feature-test")
	if err != nil {
		t.Fatalf("loadDefaultFormulaVarsFileInDir() error = %v", err)
	}
	if !found {
		t.Fatalf("found = false, want true")
	}
	if vars["source"] != "formula" {
		t.Fatalf("source = %q, want formula", vars["source"])
	}
}

func TestLoadDefaultFormulaVarsFileInDirRejectsMismatchedFormula(t *testing.T) {
	dir := t.TempDir()
	varsDir := filepath.Join(dir, ".tt", "formula")
	if err := os.MkdirAll(varsDir, 0o755); err != nil {
		t.Fatalf("mkdir vars dir: %v", err)
	}
	path := filepath.Join(varsDir, "web-feature-test.toml")
	if err := os.WriteFile(path, []byte("formula = \"other-formula\"\n"), 0o600); err != nil {
		t.Fatalf("write default vars file: %v", err)
	}

	_, _, _, _, err := loadDefaultFormulaVarsFileInDir(dir, "web-feature-test")
	if err == nil {
		t.Fatalf("expected mismatched formula error")
	}
	if got := err.Error(); !strings.Contains(got, "declares formula") || !strings.Contains(got, "web-feature-test") {
		t.Fatalf("error = %q, want mismatch details", got)
	}
}
