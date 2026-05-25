package formula

import (
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed builtin/formulas/*.toml
var builtinFormulaFS embed.FS

type BuiltinEntry struct {
	Name        string
	Title       string
	Description string
	Category    string
	Tags        []string
	Source      string
}

func BuiltinFormulas() ([]BuiltinEntry, error) {
	entries, err := builtinFormulaFS.ReadDir("builtin/formulas")
	if err != nil {
		return nil, err
	}
	out := []BuiltinEntry{}
	p := NewParser()
	for _, e := range entries {
		if e.IsDir() || !IsTOMLFilename(e.Name()) {
			continue
		}
		path := "builtin/formulas/" + e.Name()
		data, err := builtinFormulaFS.ReadFile(path)
		if err != nil {
			return nil, err
		}
		f, err := p.ParseTOML(data)
		if err != nil {
			return nil, fmt.Errorf("parse builtin %s: %w", e.Name(), err)
		}
		name := f.Formula
		if name == "" {
			name = strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		}
		out = append(out, BuiltinEntry{Name: name, Title: f.Title, Description: f.Description, Category: f.Category, Tags: f.Tags, Source: "builtin:" + name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func BuiltinFormulaContent(name string) ([]byte, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, false, nil
	}
	candidates := []string{name}
	if !strings.HasSuffix(name, ".toml") {
		candidates = append(candidates, name+CanonicalTOMLExt)
	}
	for _, c := range candidates {
		base := filepath.Base(c)
		data, err := builtinFormulaFS.ReadFile("builtin/formulas/" + base)
		if err == nil {
			return data, true, nil
		}
	}
	entries, err := BuiltinFormulas()
	if err != nil {
		return nil, false, err
	}
	for _, e := range entries {
		if e.Name == name {
			for _, ext := range []string{CanonicalTOMLExt} {
				data, err := builtinFormulaFS.ReadFile("builtin/formulas/" + e.Name + ext)
				if err == nil {
					return data, true, nil
				}
			}
		}
	}
	return nil, false, nil
}

func (p *Parser) ParseBuiltin(name string) (*Formula, error) {
	data, ok, err := BuiltinFormulaContent(name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("builtin formula %q not found", name)
	}
	f, err := p.ParseTOML(data)
	if err != nil {
		return nil, err
	}
	f.Source = "builtin:" + f.Formula
	SetSourceInfo(f)
	p.cache[f.Formula] = f
	p.cache[f.Source] = f
	return f, nil
}
