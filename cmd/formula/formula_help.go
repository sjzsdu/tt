package formulacmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/formula"
	spec "github.com/sjzsdu/tt/internal/formula/spec"
)

func (a *App) newFormulaHelpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "help <name>",
		Short: "Show usage help for a formula",
		Long: `Show practical usage help for a formula.

The output includes what the formula does, required and optional variables,
copyable run/schedule examples, and a compact step overview.`,
		Args: cobra.ExactArgs(1),
		RunE: runFormulaHelp,
	}
}

func runFormulaHelp(cmd *cobra.Command, args []string) error {
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

	renderFormulaHelp(cmd.OutOrStdout(), resolved)
	return nil
}

func renderFormulaHelp(out io.Writer, f *spec.Formula) {
	if f == nil {
		return
	}
	name := strings.TrimSpace(f.Formula)
	if name == "" {
		name = "<formula>"
	}
	title := strings.TrimSpace(f.Title)
	if title == "" {
		title = name
	}

	fmt.Fprintf(out, "# %s\n\n", title)
	fmt.Fprintf(out, "Formula: %s\n", name)
	if f.Description != "" {
		fmt.Fprintf(out, "Description: %s\n", strings.TrimSpace(f.Description))
	}
	if f.Category != "" {
		fmt.Fprintf(out, "Category: %s\n", f.Category)
	}
	if len(f.Tags) > 0 {
		fmt.Fprintf(out, "Tags: %s\n", strings.Join(f.Tags, ", "))
	}
	if f.Source != "" {
		fmt.Fprintf(out, "Source: %s\n", f.Source)
	}
	fmt.Fprintln(out)

	fmt.Fprintln(out, "## Usage")
	required := f.RequiredVarNames()
	if len(required) == 1 {
		fmt.Fprintf(out, "  tt formula run %s <%s>\n", name, required[0])
	} else {
		fmt.Fprintf(out, "  tt formula run %s\n", name)
	}
	fmt.Fprintf(out, "  tt formula run %s --var key=value\n", name)
	fmt.Fprintf(out, "  tt formula schedule %s --every 2m --run-now\n", name)
	fmt.Fprintln(out)
	if len(required) == 1 {
		fmt.Fprintf(out, "Tip: because this formula has exactly one required variable (%s), you may pass it positionally.\n\n", required[0])
	} else if len(required) > 1 {
		fmt.Fprintf(out, "Tip: this formula has %d required variables, so pass them with repeated --var key=value flags.\n\n", len(required))
	}

	if len(f.Vars) > 0 {
		fmt.Fprintln(out, "## Variables")
		for _, vname := range sortedVarNames(f.Vars) {
			def := f.Vars[vname]
			if def == nil {
				continue
			}
			fmt.Fprintf(out, "- %s%s\n", vname, formulaVarHelpSuffix(def))
			if desc := strings.TrimSpace(def.Description); desc != "" {
				fmt.Fprintf(out, "  %s\n", desc)
			}
		}
		fmt.Fprintln(out)
	}

	if len(f.Steps) > 0 {
		fmt.Fprintf(out, "## Steps (%d)\n", len(f.Steps))
		for i, step := range f.Steps {
			if step == nil {
				continue
			}
			title := strings.TrimSpace(step.Title)
			if title == "" {
				title = step.ID
			}
			kind := formulaStepKind(step)
			deps := ""
			if len(step.DependsOn) > 0 {
				deps = fmt.Sprintf("; depends on %s", strings.Join(step.DependsOn, ", "))
			} else if len(step.Needs) > 0 {
				deps = fmt.Sprintf("; needs %s", strings.Join(step.Needs, ", "))
			}
			condition := ""
			if strings.TrimSpace(step.Condition) != "" {
				condition = fmt.Sprintf("; if %s", step.Condition)
			}
			fmt.Fprintf(out, "%d. %s [%s]%s%s\n", i+1, title, kind, deps, condition)
		}
		fmt.Fprintln(out)
	}

	fmt.Fprintln(out, "## More")
	fmt.Fprintf(out, "  tt formula show %s\n", name)
	fmt.Fprintf(out, "  tt formula show %s --markdown\n", name)
	fmt.Fprintf(out, "  tt formula copy %s ./公式.toml\n", name)
}

func formulaVarHelpSuffix(def *spec.VarDef) string {
	if def == nil {
		return ""
	}
	parts := []string{}
	if def.Required {
		parts = append(parts, "required")
	}
	if def.Type != "" {
		parts = append(parts, "type: "+def.Type)
	}
	if def.Default != nil && strings.TrimSpace(*def.Default) != "" {
		parts = append(parts, "default: "+*def.Default)
	}
	if len(def.Enum) > 0 {
		parts = append(parts, "one of: "+strings.Join(def.Enum, "|"))
	}
	if def.Pattern != "" {
		parts = append(parts, "pattern: "+def.Pattern)
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func formulaStepKind(step *spec.Step) string {
	if step == nil {
		return "step"
	}
	if step.Execution != "" {
		return step.Execution
	}
	if step.Type != "" {
		return step.Type
	}
	if step.Loop != nil {
		return "loop"
	}
	if step.Script != nil {
		return "script"
	}
	if step.ExternalAgent != nil {
		return "external_agent"
	}
	if step.Tool != nil {
		return "tool"
	}
	if step.Form != nil || step.DynamicForm {
		return "human_input"
	}
	return "agent"
}
