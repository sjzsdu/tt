package formulacmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sjzsdu/tt/internal/formula"
	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formuladoc"
)

func runFormulaShowMarkdown(resolved *formula.Formula) error {
	workflow, err := formula.CompileWorkflowByName(context.Background(), resolved.Formula, getSearchPaths(), nil)
	if err != nil {
		return err
	}

	md := generateFormulaMarkdown(resolved, workflow)

	tmpDir, err := os.MkdirTemp("", "tt-formula-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	mdPath := filepath.Join(tmpDir, resolved.Formula+".md")
	if err := os.WriteFile(mdPath, []byte(md), 0644); err != nil {
		os.RemoveAll(tmpDir)
		return fmt.Errorf("write formula file: %w", err)
	}

	defer os.RemoveAll(tmpDir)
	return runMarkdownPreview(MarkdownPreviewOptions{
		Root:        tmpDir,
		Port:        formulaPort,
		InitialPath: "/view/" + resolved.Formula + ".md",
	})
}

func runFormulaShowAllMarkdown() error {
	formulas := collectFormulaShowAllMarkdownFormulas()
	if len(formulas) == 0 {
		return fmt.Errorf("no formulas found in search paths or builtins")
	}

	tmpDir, err := os.MkdirTemp("", "tt-formulas-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	for _, f := range formulas {
		workflow, err := formula.CompileWorkflowByName(context.Background(), f.Formula, getSearchPaths(), nil)
		if err != nil {
			continue
		}

		md := generateFormulaMarkdown(f, workflow)
		mdPath := filepath.Join(tmpDir, f.Formula+".md")
		if err := os.WriteFile(mdPath, []byte(md), 0644); err != nil {
			return fmt.Errorf("write %s: %w", f.Formula, err)
		}
	}

	fmt.Printf("Generated %d formula files in %s\n", len(formulas), tmpDir)
	defer os.RemoveAll(tmpDir)
	return runMarkdownPreview(MarkdownPreviewOptions{Root: tmpDir, Port: formulaPort})
}

func collectFormulaShowAllMarkdownFormulas() []*formula.Formula {
	paths := getSearchPaths()

	var formulas []*formula.Formula
	seen := make(map[string]bool)
	p := formula.NewParser(paths...)

	if entries, err := formula.BuiltinFormulas(); err == nil {
		for _, entry := range entries {
			f, err := p.LoadByName(entry.Name)
			if err != nil {
				continue
			}
			resolved, err := p.Resolve(f)
			if err != nil {
				continue
			}
			formulas = append(formulas, resolved)
			seen[resolved.Formula] = true
		}
	}

	for _, dir := range paths {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !isFormulaFile(entry.Name()) {
				continue
			}
			name := extractFormulaName(entry.Name())
			if seen[name] {
				continue
			}

			p := formula.NewParser(dir)
			f, err := p.ParseFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				continue
			}
			resolved, err := p.Resolve(f)
			if err != nil {
				continue
			}
			formulas = append(formulas, resolved)
			seen[name] = true
		}
	}

	return formulas
}

func generateFormulaMarkdown(f *formula.Formula, workflow *ir.Workflow) string {
	return formuladoc.GenerateMarkdown(f, workflow)
}

func generateMermaidGraph(workflow *ir.Workflow) string {
	return formuladoc.GenerateMermaidGraph(workflow)
}
