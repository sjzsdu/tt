package team

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed builtin/teams
var builtinTeamFS embed.FS

type BuiltinEntry struct {
	Name        string
	Title       string
	Description string
	Agents      int
}

func BuiltinTeams() ([]BuiltinEntry, error) {
	return builtinTeamsInDir("builtin/teams")
}

func builtinTeamsInDir(dir string) ([]BuiltinEntry, error) {
	entries, err := fs.ReadDir(builtinTeamFS, dir)
	if err != nil {
		return nil, fmt.Errorf("read builtin teams dir: %w", err)
	}
	var out []BuiltinEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		defPath := filepath.Join(dir, entry.Name())
		data, err := fs.ReadFile(builtinTeamFS, defPath)
		if err != nil {
			continue
		}
		definition, err := Parse(data)
		if err != nil {
			continue
		}
		out = append(out, BuiltinEntry{
			Name:        definition.Team,
			Title:       definition.Title,
			Description: definition.Description,
			Agents:      len(definition.Agents),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func BuiltinTeamContent(name string) ([]byte, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, false, nil
	}
	candidates := []string{
		filepath.Join("builtin/teams", name+".toml"),
		filepath.Join("builtin/teams", name, DefinitionFilename),
	}
	for _, path := range candidates {
		data, err := fs.ReadFile(builtinTeamFS, path)
		if err != nil {
			continue
		}
		return data, true, nil
	}
	return nil, false, nil
}
