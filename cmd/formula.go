package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/formula"
	"github.com/sjzsdu/tt/internal/molecule"
)

var (
	formulaDir    string
	formulaVars   []string
	formulaOutput string
	formulaTitle  string
)

var formulaCmd = &cobra.Command{
	Use:   "formula",
	Short: "Manage and instantiate formula templates",
	Long: `Formula templates define structured task workflows with variables,
dependencies, and control flow. Compile and instantiate formulas to generate
task trees for complex work.`,
}

var formulaListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available formulas",
	Args:  cobra.NoArgs,
	RunE:  runFormulaList,
}

var formulaShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show formula details",
	Args:  cobra.ExactArgs(1),
	RunE:  runFormulaShow,
}

var formulaCompileCmd = &cobra.Command{
	Use:   "compile <name>",
	Short: "Compile a formula and show the recipe",
	Args:  cobra.ExactArgs(1),
	RunE:  runFormulaCompile,
}

var formulaInstantiateCmd = &cobra.Command{
	Use:   "instantiate <name>",
	Short: "Instantiate a formula into a task tree",
	Args:  cobra.ExactArgs(1),
	RunE:  runFormulaInstantiate,
}

var formulaValidateCmd = &cobra.Command{
	Use:   "validate <file>",
	Short: "Validate a formula file",
	Args:  cobra.ExactArgs(1),
	RunE:  runFormulaValidate,
}

func init() {
	formulaCmd.PersistentFlags().StringVarP(&formulaDir, "dir", "d", "", "formula search directory (default: .tt/formulas, ~/.tt/formulas)")
	formulaCmd.PersistentFlags().StringArrayVar(&formulaVars, "var", nil, "variable override (key=value, repeatable)")

	formulaInstantiateCmd.Flags().StringVarP(&formulaOutput, "output", "o", "json", "output format: json, yaml, text, prompt")
	formulaInstantiateCmd.Flags().StringVarP(&formulaTitle, "title", "t", "", "override root task title")

	formulaCmd.AddCommand(formulaListCmd)
	formulaCmd.AddCommand(formulaShowCmd)
	formulaCmd.AddCommand(formulaCompileCmd)
	formulaCmd.AddCommand(formulaInstantiateCmd)
	formulaCmd.AddCommand(formulaValidateCmd)

	rootCmd.AddCommand(formulaCmd)
}

func getSearchPaths() []string {
	if formulaDir != "" {
		return []string{formulaDir}
	}

	paths := []string{}
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(cwd, ".tt", "formulas"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".tt", "formulas"))
	}
	return paths
}

func parseVars() map[string]string {
	vars := make(map[string]string)
	for _, v := range formulaVars {
		key, value, ok := strings.Cut(v, "=")
		if ok && key != "" {
			vars[key] = value
		}
	}
	return vars
}

func runFormulaList(cmd *cobra.Command, args []string) error {
	paths := getSearchPaths()
	if len(paths) == 0 {
		fmt.Println("No formula search paths configured.")
		fmt.Println("Create formulas in .tt/formulas/ or ~/.tt/formulas/")
		return nil
	}

	found := false
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
			path := filepath.Join(dir, name)

			p := formula.NewParser(dir)
			f, err := p.ParseFile(path)
			if err != nil {
				fmt.Printf("  %s (parse error: %v)\n", formulaName, err)
				continue
			}

			desc := f.Description
			if desc == "" {
				desc = "(no description)"
			}
			fmt.Printf("  %-30s %s\n", formulaName, desc)
			found = true
		}
	}

	if !found {
		fmt.Println("No formulas found.")
		fmt.Println("Create formulas in .tt/formulas/ or ~/.tt/formulas/")
	}
	return nil
}

func isFormulaFile(name string) bool {
	ext := filepath.Ext(name)
	if ext == ".toml" || ext == ".json" {
		return true
	}
	return strings.HasSuffix(name, ".formula.toml") || strings.HasSuffix(name, ".formula.json")
}

func extractFormulaName(filename string) string {
	name := filename
	for _, ext := range []string{".formula.toml", ".formula.json", ".toml", ".json"} {
		if strings.HasSuffix(name, ext) {
			name = strings.TrimSuffix(name, ext)
			break
		}
	}
	return name
}

func runFormulaShow(cmd *cobra.Command, args []string) error {
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

	fmt.Printf("Formula: %s\n", resolved.Formula)
	if resolved.Description != "" {
		fmt.Printf("Description: %s\n", resolved.Description)
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

	recipe, err := formula.Compile(context.Background(), name, getSearchPaths(), vars)
	if err != nil {
		return err
	}

	fmt.Printf("Recipe: %s\n", recipe.Name)
	if recipe.Description != "" {
		fmt.Printf("Description: %s\n", recipe.Description)
	}
	fmt.Printf("RootOnly: %v\n", recipe.RootOnly)
	fmt.Printf("Steps (%d):\n", len(recipe.Steps))

	for i, step := range recipe.Steps {
		priority := ""
		if step.Priority != nil {
			priority = fmt.Sprintf("[P%d] ", *step.Priority)
		}
		deps := ""
		for _, dep := range recipe.Deps {
			if dep.StepID == step.ID && dep.Type != "parent-child" {
				if deps != "" {
					deps += ", "
				}
				deps += fmt.Sprintf("%s(%s)", dep.DependsOnID, dep.Type)
			}
		}
		if deps != "" {
			deps = " (blocks: " + deps + ")"
		}
		fmt.Printf("  %d. %s%-30s %s%s\n", i+1, priority, step.ID, step.Title, deps)
	}

	return nil
}

func runFormulaInstantiate(cmd *cobra.Command, args []string) error {
	name := args[0]
	vars := parseVars()

	recipe, err := formula.Compile(context.Background(), name, getSearchPaths(), vars)
	if err != nil {
		return err
	}

	opts := molecule.Options{
		Title: formulaTitle,
		Vars:  vars,
	}

	result, err := molecule.Instantiate(recipe, opts)
	if err != nil {
		return err
	}

	switch formulaOutput {
	case "json":
		return outputJSON(result)
	case "text":
		return outputText(result)
	case "prompt":
		return outputPrompt(result)
	default:
		return fmt.Errorf("unknown output format: %s", formulaOutput)
	}
}

func outputJSON(result *molecule.Result) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func outputText(result *molecule.Result) error {
	fmt.Printf("Root: %s\n", result.RootID)
	fmt.Printf("Tasks: %d\n\n", result.Created)

	for i, task := range result.Tasks {
		priority := ""
		if task.Priority != nil {
			priority = fmt.Sprintf("[P%d] ", *task.Priority)
		}
		deps := ""
		if len(task.Dependencies) > 0 {
			deps = fmt.Sprintf(" (after: %s)", strings.Join(task.Dependencies, ", "))
		}
		fmt.Printf("%d. %s%s\n", i+1, priority, task.Title)
		if task.Description != "" {
			fmt.Printf("   %s\n", task.Description)
		}
		if deps != "" {
			fmt.Printf("   %s\n", deps)
		}
		fmt.Println()
	}
	return nil
}

func outputPrompt(result *molecule.Result) error {
	fmt.Println("请按以下顺序完成任务，每个任务完成后确认再继续下一个：")
	fmt.Println()

	for i, task := range result.Tasks {
		if task.IsRoot {
			continue
		}
		priority := "普通"
		if task.Priority != nil {
			switch *task.Priority {
			case 0:
				priority = "最高"
			case 1:
				priority = "高"
			case 2:
				priority = "中"
			case 3:
				priority = "低"
			case 4:
				priority = "最低"
			}
		}
		fmt.Printf("## 任务 %d (优先级: %s)\n", i, priority)
		fmt.Printf("**%s**\n", task.Title)
		if task.Description != "" {
			fmt.Println(task.Description)
		}
		if task.Notes != "" {
			fmt.Println(task.Notes)
		}
		if len(task.Dependencies) > 0 {
			fmt.Printf("依赖: 任务 %s 完成后开始\n", strings.Join(task.Dependencies, ", "))
		}
		fmt.Println()
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
