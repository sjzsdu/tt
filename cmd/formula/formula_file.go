package formulacmd

import (
	"fmt"
	"os"
	"strconv"
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
	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return "", nil, fmt.Errorf("parse formula file %q failed: %w", path, err)
	}
	formulaName := strings.TrimSpace(rf.Formula)
	vars := make(map[string]string, len(rf.Vars)+len(raw))
	for key, value := range raw {
		key = strings.TrimSpace(key)
		if key == "" {
			return "", nil, fmt.Errorf("formula file %q contains an empty top-level key", path)
		}
		if formulaFileReservedTopLevelKey(key) {
			continue
		}
		converted, ok := formulaFileScalarString(value)
		if !ok {
			return "", nil, fmt.Errorf("formula file %q top-level key %q must be a scalar variable value or be placed under [vars]", path, key)
		}
		vars[key] = converted
	}
	for key, value := range rf.Vars {
		key = strings.TrimSpace(key)
		if key == "" {
			return "", nil, fmt.Errorf("formula file %q contains an empty vars key", path)
		}
		vars[key] = value
	}
	return formulaName, vars, nil
}

func formulaFileReservedTopLevelKey(key string) bool {
	switch key {
	case "formula", "vars":
		return true
	default:
		return false
	}
}

func formulaFileScalarString(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case bool:
		return strconv.FormatBool(v), true
	case int:
		return strconv.Itoa(v), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	default:
		return "", false
	}
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
