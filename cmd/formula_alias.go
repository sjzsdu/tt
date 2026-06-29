package cmd

import (
	"fmt"
	"strings"

	formulacmd "github.com/sjzsdu/tt/cmd/formula"
	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/formula"
)

var formulaShortcutsRegistered bool

func registerFormulaShortcutCommands() {
	if formulaShortcutsRegistered {
		return
	}
	formulaShortcutsRegistered = true
	entries, err := formula.BuiltinFormulas()
	if err != nil {
		return
	}
	existing := rootCommandNamesAndAliases()
	for _, entry := range entries {
		formulaName := entry.Name
		for _, alias := range entry.Aliases {
			alias = strings.TrimSpace(alias)
			if alias == "" || existing[alias] {
				continue
			}
			shortcut := newFormulaShortcutCommand(alias, formulaName, entry.Description)
			rootCmd.AddCommand(shortcut)
			existing[alias] = true
		}
	}
}

func rootCommandNamesAndAliases() map[string]bool {
	existing := map[string]bool{}
	for _, command := range rootCmd.Commands() {
		existing[command.Name()] = true
		for _, alias := range command.Aliases {
			existing[alias] = true
		}
	}
	return existing
}

func newFormulaShortcutCommand(alias, formulaName, description string) *cobra.Command {
	short := fmt.Sprintf("Shortcut for `tt formula ... %s`", formulaName)
	if strings.TrimSpace(description) != "" {
		short = description
	}
	cmd := &cobra.Command{
		Use:   alias,
		Short: short,
		Long:  fmt.Sprintf("Formula shortcut for %s. Examples: `tt %s run`, `tt %s show`, `tt %s runs`.", formulaName, alias, alias, alias),
	}
	for _, sub := range []string{"run", "show", "help", "compile", "copy", "runs"} {
		subcmd := sub
		cmd.AddCommand(&cobra.Command{
			Use:                subcmd + " [args...]",
			Short:              fmt.Sprintf("Shortcut for `tt formula %s %s`", subcmd, formulaName),
			DisableFlagParsing: true,
			Args:               cobra.ArbitraryArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runFormulaShortcut(cmd, formulaName, subcmd, args)
			},
		})
	}
	return cmd
}

func runFormulaShortcut(cmd *cobra.Command, formulaName, subcmd string, args []string) error {
	forward := formulaShortcutForwardArgs(formulaName, subcmd, args)
	formulaCmd := formulacmd.New(formulaCommandDeps)
	formulaCmd.SetArgs(forward)
	formulaCmd.SetIn(cmd.InOrStdin())
	formulaCmd.SetOut(cmd.OutOrStdout())
	formulaCmd.SetErr(cmd.ErrOrStderr())
	formulaCmd.SetContext(cmd.Context())
	return formulaCmd.Execute()
}

func formulaShortcutForwardArgs(formulaName, subcmd string, args []string) []string {
	out := []string{subcmd}
	switch subcmd {
	case "runs":
		out = append(out, "--formula", formulaName)
		out = append(out, args...)
	case "run", "show", "help", "compile", "copy":
		out = append(out, formulaName)
		out = append(out, args...)
	default:
		out = append(out, args...)
	}
	return out
}
