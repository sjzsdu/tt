package formula

import (
	"path/filepath"
)

const (
	CanonicalTOMLExt = ".toml"
	FormulaExtJSON   = ".formula.json"
	FormulaExt       = FormulaExtJSON
)

func IsTOMLFilename(name string) bool {
	return filepath.Ext(name) == CanonicalTOMLExt
}
