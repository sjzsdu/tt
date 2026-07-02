package formulacmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

type formulaFileConfig struct {
	Formula string            `toml:"formula"`
	Vars    map[string]string `toml:"vars"`
}

func loadFormulaFile(path string) (string, map[string]string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("read formula file %q failed: %w", path, err)
	}
	var rf formulaFileConfig
	if _, err := toml.Decode(string(data), &rf); err != nil {
		return "", nil, fmt.Errorf("parse formula file %q failed: %w", path, err)
	}
	formulaName := strings.TrimSpace(rf.Formula)
	vars := make(map[string]string, len(rf.Vars))
	for key, value := range rf.Vars {
		key = strings.TrimSpace(key)
		if key == "" {
			return "", nil, fmt.Errorf("formula file %q contains an empty vars key", path)
		}
		vars[key] = value
	}
	return formulaName, vars, nil
}

func mergeFormulaVars(base map[string]string, overrides map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(overrides))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overrides {
		merged[key] = value
	}
	return merged
}
