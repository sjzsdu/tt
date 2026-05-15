package formula

import (
	"path/filepath"
	"strings"
)

const (
	CanonicalTOMLExt = ".toml"
	LegacyTOMLExt    = ".formula.toml"
	FormulaExtJSON   = ".formula.json"
	FormulaExt       = FormulaExtJSON
)

func IsTOMLFilename(name string) bool {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return ext == CanonicalTOMLExt || ext == LegacyTOMLExt || strings.HasSuffix(base, ".formula") && ext == ".toml"
}
