package formula

import (
	"embed"
	"fmt"
	spec "github.com/sjzsdu/tt/internal/formula/spec"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed builtin/formulas builtin/atomics builtin/scripts
var builtinFormulaFS embed.FS

type BuiltinEntry struct {
	Name        string
	Title       string
	Description string
	Aliases     []string
	Category    string
	Tags        []string
	Source      string
}

func BuiltinFormulas() ([]BuiltinEntry, error) {
	return builtinFormulasInDir("builtin/formulas")
}

func BuiltinAtomicFormulas() ([]BuiltinEntry, error) {
	return builtinFormulasInDir("builtin/atomics")
}

func builtinFormulasInDir(dir string) ([]BuiltinEntry, error) {
	paths, err := builtinFormulaPathsInDir(dir)
	if err != nil {
		return nil, err
	}
	out := []BuiltinEntry{}
	p := NewParser()
	for _, path := range paths {
		data, err := builtinFormulaFS.ReadFile(path)
		if err != nil {
			return nil, err
		}
		f, err := p.ParseTOML(data)
		if err != nil {
			return nil, fmt.Errorf("parse builtin %s: %w", path, err)
		}
		name := f.Formula
		if name == "" {
			base := filepath.Base(path)
			name = strings.TrimSuffix(base, filepath.Ext(base))
		}
		out = append(out, BuiltinEntry{Name: name, Title: f.Title, Description: f.Description, Aliases: f.Aliases, Category: f.Category, Tags: f.Tags, Source: "builtin:" + name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func builtinFormulaPathsInDir(dir string) ([]string, error) {
	paths := []string{}
	err := fs.WalkDir(builtinFormulaFS, dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !IsTOMLFilename(entry.Name()) {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func BuiltinFormulaContent(name string) ([]byte, bool, error) {
	data, _, ok, err := BuiltinFormulaContentWithPath(name)
	return data, ok, err
}

func BuiltinFormulaContentWithPath(name string) ([]byte, string, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, "", false, nil
	}
	candidates := []string{name}
	if !strings.HasSuffix(name, ".toml") {
		candidates = append(candidates, name+CanonicalTOMLExt)
	}
	for _, c := range candidates {
		base := filepath.Base(c)
		data, path, ok, err := builtinFormulaContentByBaseWithPath(base)
		if err != nil {
			return nil, "", false, err
		}
		if ok {
			return data, path, true, nil
		}
	}
	entries, err := allBuiltinEntries()
	if err != nil {
		return nil, "", false, err
	}
	for _, e := range entries {
		if e.Name == name {
			for _, ext := range []string{CanonicalTOMLExt} {
				data, path, ok, err := builtinFormulaContentByBaseWithPath(e.Name + ext)
				if err != nil {
					return nil, "", false, err
				}
				if ok {
					return data, path, true, nil
				}
			}
		}
	}
	return nil, "", false, nil
}

func builtinFormulaContentByBase(base string) ([]byte, bool, error) {
	data, _, ok, err := builtinFormulaContentByBaseWithPath(base)
	return data, ok, err
}

func builtinFormulaContentByBaseWithPath(base string) ([]byte, string, bool, error) {
	for _, dir := range []string{"builtin/formulas", "builtin/atomics"} {
		paths, err := builtinFormulaPathsInDir(dir)
		if err != nil {
			return nil, "", false, err
		}
		for _, path := range paths {
			if filepath.Base(path) != base {
				continue
			}
			data, err := builtinFormulaFS.ReadFile(path)
			if err != nil {
				return nil, "", false, err
			}
			return data, path, true, nil
		}
	}
	return nil, "", false, nil
}

func allBuiltinEntries() ([]BuiltinEntry, error) {
	regular, err := BuiltinFormulas()
	if err != nil {
		return nil, err
	}
	atomics, err := BuiltinAtomicFormulas()
	if err != nil {
		return nil, err
	}
	return append(regular, atomics...), nil
}

func (p *Parser) ParseBuiltin(name string) (*spec.Formula, error) {
	data, sourcePath, ok, err := BuiltinFormulaContentWithPath(name)
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
	f.Source = "builtin:" + sourcePath
	spec.SetSourceInfo(f)
	if err := resolveFormulaScriptCommands(f); err != nil {
		return nil, err
	}
	f.Source = "builtin:" + f.Formula
	p.cache[f.Formula] = f
	p.cache[f.Source] = f
	return f, nil
}
