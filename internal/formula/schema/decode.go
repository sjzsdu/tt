package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/sjzsdu/tt/internal/formula/ast"
)

type documentFile struct {
	Formula     string             `json:"formula" toml:"formula"`
	Title       string             `json:"title" toml:"title"`
	Description string             `json:"description" toml:"description"`
	Version     int                `json:"version" toml:"version"`
	Contract    string             `json:"contract" toml:"contract"`
	Vars        map[string]varFile `json:"vars" toml:"vars"`
	Steps       []map[string]any   `json:"steps" toml:"steps"`
	Workspace   *workspaceFile     `json:"workspace" toml:"workspace"`
	Worktree    bool               `json:"worktree" toml:"worktree"`
}

type varFile struct {
	Description string   `json:"description" toml:"description"`
	Default     *string  `json:"default" toml:"default"`
	Required    bool     `json:"required" toml:"required"`
	Enum        []string `json:"enum" toml:"enum"`
	Pattern     string   `json:"pattern" toml:"pattern"`
	Type        string   `json:"type" toml:"type"`
}

type workspaceFile struct {
	Kind    string `json:"kind" toml:"kind"`
	Path    string `json:"path" toml:"path"`
	Cleanup *bool  `json:"cleanup" toml:"cleanup"`
}

func LoadFile(path string) (*ast.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := Decode(path, data)
	if err != nil {
		return nil, err
	}
	doc.Source.File = path
	return doc, nil
}

func Decode(name string, data []byte) (*ast.Document, error) {
	var raw documentFile
	switch strings.ToLower(strings.TrimSpace(fileExt(name))) {
	case ".json":
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("decode formula json: %w", err)
		}
	default:
		if err := toml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("decode formula toml: %w", err)
		}
	}
	return normalize(raw)
}

func normalize(raw documentFile) (*ast.Document, error) {
	if strings.TrimSpace(raw.Formula) == "" {
		return nil, fmt.Errorf("formula is required")
	}
	doc := &ast.Document{Name: raw.Formula, Title: raw.Title, Description: raw.Description, Version: raw.Version, Contract: raw.Contract, Vars: map[string]ast.VarDecl{}}
	for name, v := range raw.Vars {
		doc.Vars[name] = ast.VarDecl{Description: v.Description, Default: v.Default, Required: v.Required, Enum: v.Enum, Pattern: v.Pattern, Type: v.Type}
	}
	if raw.Workspace != nil {
		doc.Workspace = &ast.WorkspaceSpec{Kind: strings.TrimSpace(raw.Workspace.Kind), Path: strings.TrimSpace(raw.Workspace.Path), Cleanup: raw.Workspace.Cleanup}
	}
	doc.Worktree = raw.Worktree
	if doc.Workspace == nil && doc.Worktree {
		doc.Workspace = &ast.WorkspaceSpec{Kind: "worktree"}
	}
	if doc.Workspace != nil && strings.TrimSpace(doc.Workspace.Kind) == "" {
		doc.Workspace.Kind = "worktree"
	}
	for i, rawStep := range raw.Steps {
		decl, err := normalizeStep(i, rawStep)
		if err != nil {
			return nil, err
		}
		doc.Steps = append(doc.Steps, decl)
	}
	return doc, nil
}

func normalizeStep(index int, raw map[string]any) (ast.StepDecl, error) {
	id, _ := raw["id"].(string)
	if strings.TrimSpace(id) == "" {
		return ast.StepDecl{}, fmt.Errorf("steps[%d].id is required", index)
	}
	kind, _ := raw["kind"].(string)
	if kind == "" {
		kind, _ = raw["execution"].(string)
	}
	if kind == "" {
		kind = "agent"
	}
	title, _ := raw["title"].(string)
	depends := stringSlice(raw["depends_on"])
	if len(depends) == 0 {
		depends = stringSlice(raw["needs"])
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return ast.StepDecl{}, fmt.Errorf("steps[%d]: %w", index, err)
	}
	return ast.StepDecl{ID: id, Kind: kind, Title: title, DependsOn: depends, Raw: data}, nil
}

func stringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func fileExt(name string) string {
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		return name[idx:]
	}
	return ""
}
