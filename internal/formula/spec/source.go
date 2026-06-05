package spec

import (
	"fmt"
	"os"
	"path/filepath"
)

// SetSourceInfo populates the SourceFormula and SourceLocation fields on each step.
func SetSourceInfo(formula *Formula) {
	setSourceInfoRecursive(formula.Steps, formula.Formula, "steps")
	setSourceInfoRecursive(formula.Template, formula.Formula, "template")
}

func setSourceInfoRecursive(steps []*Step, formulaName, pathPrefix string) {
	for i, step := range steps {
		if step == nil {
			continue
		}
		step.SourceFormula = formulaName
		step.SourceLocation = fmt.Sprintf("%s[%d]", pathPrefix, i)

		if len(step.Children) > 0 {
			childPath := fmt.Sprintf("%s[%d].children", pathPrefix, i)
			setSourceInfoRecursive(step.Children, formulaName, childPath)
		}

		if step.Loop != nil && len(step.Loop.Body) > 0 {
			bodyPath := fmt.Sprintf("%s[%d].loop.body", pathPrefix, i)
			setSourceInfoRecursive(step.Loop.Body, formulaName, bodyPath)
		}
	}
}

// ResolveDescriptionFiles walks all steps and replaces DescriptionFile
// with the file's contents.
func ResolveDescriptionFiles(steps []*Step, baseDir string) {
	for _, step := range steps {
		if step == nil || step.DescriptionFile == "" {
			continue
		}
		path := step.DescriptionFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		data, err := os.ReadFile(path)
		if err == nil {
			step.Description = string(data)
		}
		step.DescriptionFile = ""
		if len(step.Children) > 0 {
			ResolveDescriptionFiles(step.Children, baseDir)
		}
	}
}
