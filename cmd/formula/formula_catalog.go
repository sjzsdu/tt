package formulacmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/formula"
	"github.com/sjzsdu/tt/internal/formula/ir"
)

func runFormulaList(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	showBuiltin := !formulaListUser
	showUser := !formulaListBuiltin
	category := strings.TrimSpace(formulaListCategory)

	if showBuiltin {
		entries, err := formula.BuiltinFormulas()
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			fmt.Fprintln(out, "BUILTIN")
			for _, entry := range entries {
				if category != "" && entry.Category != category {
					continue
				}
				desc := entry.Description
				if desc == "" {
					desc = "(no description)"
				}
				fmt.Fprintf(out, "  %-22s %-14s %s\n", entry.Name, entry.Category, desc)
			}
			if showUser {
				fmt.Fprintln(out)
			}
		}
	}

	if !showUser {
		return nil
	}

	paths := getSearchPaths()
	if len(paths) == 0 {
		fmt.Fprintln(out, "No formula search paths configured.")
		fmt.Fprintln(out, "Create formulas in .tt/formulas/ or ~/.tt/formulas/")
		return nil
	}

	found := false
	seen := make(map[string]bool)
	fmt.Fprintln(out, "USER")
	for _, dir := range paths {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !isFormulaFile(name) {
				continue
			}

			formulaName := extractFormulaName(name)
			if seen[formulaName] {
				continue
			}
			path := filepath.Join(dir, name)

			p := formula.NewParser(dir)
			f, err := p.ParseFile(path)
			if err != nil {
				fmt.Printf("  %s (parse error: %v)\n", formulaName, err)
				continue
			}

			desc := f.Description
			if category != "" && f.Category != category {
				continue
			}
			if desc == "" {
				desc = "(no description)"
			}
			fmt.Fprintf(out, "  %-22s %-14s %s\n", formulaName, f.Category, desc)
			seen[formulaName] = true
			found = true
		}
	}

	if !found {
		fmt.Fprintln(out, "No user formulas found.")
		fmt.Fprintln(out, "Create formulas in .tt/formulas/ or ~/.tt/formulas/")
	}
	return nil
}

func isFormulaFile(name string) bool {
	ext := filepath.Ext(name)
	if ext == ".toml" || ext == ".json" {
		return true
	}
	return false
}

func extractFormulaName(filename string) string {
	name := filename
	for _, ext := range []string{".formula.json", ".toml", ".json"} {
		if strings.HasSuffix(name, ext) {
			name = strings.TrimSuffix(name, ext)
			break
		}
	}
	return name
}

func runFormulaShow(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		if formulaMarkdown {
			return runFormulaShowAllMarkdown()
		}
		return runFormulaList(cmd, args)
	}

	name := args[0]
	p := formula.NewParser(getSearchPaths()...)

	f, err := p.LoadByName(name)
	if err != nil {
		return err
	}

	resolved, err := p.Resolve(f)
	if err != nil {
		return fmt.Errorf("resolving: %w", err)
	}

	if formulaMarkdown {
		return runFormulaShowMarkdown(resolved)
	}

	fmt.Printf("Formula: %s\n", resolved.Formula)
	if resolved.Title != "" {
		fmt.Printf("Title: %s\n", resolved.Title)
	}
	if resolved.Description != "" {
		fmt.Printf("Description: %s\n", resolved.Description)
	}
	if resolved.Category != "" {
		fmt.Printf("Category: %s\n", resolved.Category)
	}
	if len(resolved.Tags) > 0 {
		fmt.Printf("Tags: %s\n", strings.Join(resolved.Tags, ", "))
	}
	if resolved.Source != "" {
		fmt.Printf("Source: %s\n", resolved.Source)
	}
	fmt.Printf("Version: %d\n", resolved.Version)
	fmt.Printf("Type: %s\n", resolved.Type)
	if resolved.Phase != "" {
		fmt.Printf("Phase: %s\n", resolved.Phase)
	}
	fmt.Println()

	if len(resolved.Vars) > 0 {
		fmt.Println("Variables:")
		for _, vname := range sortedVarNames(resolved.Vars) {
			def := resolved.Vars[vname]
			if def == nil {
				continue
			}
			req := ""
			if def.Required {
				req = " (required)"
			}
			defVal := ""
			if def.Default != nil {
				defVal = fmt.Sprintf(" (default: %s)", *def.Default)
			}
			desc := def.Description
			if desc == "" {
				desc = vname
			}
			fmt.Printf("  %-20s %s%s%s\n", vname, desc, req, defVal)
		}
		fmt.Println()
	}

	fmt.Printf("Steps (%d):\n", len(resolved.Steps))
	for i, step := range resolved.Steps {
		priority := ""
		if step.Priority != nil {
			priority = fmt.Sprintf("[P%d] ", *step.Priority)
		}
		deps := ""
		if len(step.DependsOn) > 0 {
			deps = fmt.Sprintf(" (depends: %s)", strings.Join(step.DependsOn, ", "))
		}
		if len(step.Needs) > 0 {
			deps = fmt.Sprintf(" (needs: %s)", strings.Join(step.Needs, ", "))
		}
		fmt.Printf("  %d. %s%-15s → %s%s\n", i+1, priority, step.ID, step.Title, deps)
	}

	return nil
}

func sortedVarNames(vars map[string]*formula.VarDef) []string {
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[i] > names[j] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	return names
}

func runFormulaCompile(cmd *cobra.Command, args []string) error {
	name := args[0]
	vars := parseVars()

	workflow, err := formula.CompileWorkflowByName(context.Background(), name, getSearchPaths(), vars)
	if err != nil {
		return err
	}

	fmt.Printf("Workflow: %s\n", workflow.Name)
	fmt.Printf("Nodes (%d):\n", len(workflow.Graph.Nodes))
	ids := make([]string, 0, len(workflow.Graph.Nodes))
	for id := range workflow.Graph.Nodes {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, id := range ids {
		node := workflow.Graph.Nodes[ir.NodeID(id)]
		meta := node.Step.Meta()
		fmt.Printf("  - %-30s kind=%s title=%q\n", id, meta.Kind, meta.Title)
	}
	fmt.Printf("Edges (%d):\n", len(workflow.Graph.Edges))
	for _, edge := range workflow.Graph.Edges {
		fmt.Printf("  - %s -> %s (%s)\n", edge.From, edge.To, edge.Type)
	}
	return nil
}

func runFormulaValidate(cmd *cobra.Command, args []string) error {
	path := args[0]

	p := formula.NewParser()
	f, err := p.ParseFile(path)
	if err != nil {
		return err
	}

	if err := f.Validate(); err != nil {
		return err
	}

	fmt.Printf("Formula %q is valid.\n", f.Formula)
	return nil
}

func runFormulaCopy(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	data, ok, err := formula.BuiltinFormulaContent(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("builtin formula %q not found", name)
	}
	outPath := ""
	if len(args) > 1 {
		outPath = args[1]
	} else {
		outPath = filepath.Join(formulaDefaultDir(formulaMustLoadTTConfig()), name+formula.CanonicalTOMLExt)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(outPath); err == nil {
		return fmt.Errorf("output file already exists: %s", outPath)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Copied builtin formula %q to %s\n", name, outPath)
	return nil
}
