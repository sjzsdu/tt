package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var (
	cmd2skillTargetDir string
	cmd2skillDryRun    bool
	cmd2skillExamples  bool
	cmd2skillDepth     int
	cmd2skillMarkdown  bool
)

var cmd2skillCmd = &cobra.Command{
	Use:   "cmd2skill [command]",
	Short: "Convert a CLI command into comprehensive skill files",
	Long: `Parse a CLI command and its subcommands to generate comprehensive skill files.
For each subcommand, detailed help including flags, options, and examples is fetched.
Output is organized into multiple files for better maintainability.`,
	Example: `tt cmd2skill git
tt cmd2skill git --depth 2
tt cmd2skill docker --examples
tt cmd2skill kubectl --target-dir ./.forge/skills
tt cmd2skill git --markdown`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCmd2Skill(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(cmd2skillCmd)
	cmd2skillCmd.Flags().StringVar(&cmd2skillTargetDir, "target-dir", "~/.agents/skills", "directory to write skill files")
	cmd2skillCmd.Flags().BoolVar(&cmd2skillDryRun, "dry-run", false, "print skill content to stdout instead of writing files")
	cmd2skillCmd.Flags().BoolVar(&cmd2skillExamples, "examples", false, "fetch examples for subcommands")
	cmd2skillCmd.Flags().IntVarP(&cmd2skillDepth, "depth", "d", 1, "recursion depth for subcommand help (0 = top-level only, 1+ = fetch subcommand help)")
	cmd2skillCmd.Flags().BoolVar(&cmd2skillMarkdown, "markdown", false, "open generated skill content directly with markdown command instead of writing files")
}

type CommandSpec struct {
	Name        string
	MainCommand string
	Description string
	Usage       string
	Subcommands []SubcommandSpec
	Flags       []FlagSpec
	RawHelp     string
}

type SubcommandSpec struct {
	Name        string
	Description string
	Usage       string
	Flags       []FlagSpec
	Examples    []ExampleSpec
	RawHelp     string
}

type FlagSpec struct {
	Name        string
	Shorthand   string
	Description string
}

type ExampleSpec struct {
	Command string
	Desc    string
}

func runCmd2Skill(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("command name required")
	}

	commandName := args[0]
	spec, err := parseCommandDeep(commandName, cmd2skillDepth)
	if err != nil {
		return fmt.Errorf("parse command %s: %w", commandName, err)
	}

	if cmd2skillDryRun {
		generateAllSkills(spec, os.Stdout)
		return nil
	}

	if cmd2skillMarkdown {
		tempDir, err := os.MkdirTemp("", "cmd2skill-*")
		if err != nil {
			return fmt.Errorf("create temp dir: %w", err)
		}
		defer os.RemoveAll(tempDir)

		if err := writeSkillFiles(spec, tempDir); err != nil {
			return err
		}

		openCmd := exec.Command(os.Args[0], "markdown", tempDir)
		openCmd.Stdout = os.Stdout
		openCmd.Stderr = os.Stderr
		if err := openCmd.Run(); err != nil {
			return fmt.Errorf("run markdown command: %w", err)
		}
		return nil
	}

	skillDir := filepath.Join(cmd2skillTargetDir, spec.Name)
	if cmd2skillTargetDir == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home dir: %w", err)
		}
		skillDir = filepath.Join(home, "skills", spec.Name)
	} else if strings.HasPrefix(cmd2skillTargetDir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home dir: %w", err)
		}
		skillDir = filepath.Join(home, strings.TrimPrefix(cmd2skillTargetDir, "~/"), spec.Name)
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}

	if err := writeSkillFiles(spec, skillDir); err != nil {
		return err
	}

	fmt.Printf("Generated skills for %s in %s/\n", spec.Name, skillDir)
	return nil
}

func parseCommandDeep(name string, depth int) (*CommandSpec, error) {
	spec := &CommandSpec{
		Name:        name,
		MainCommand: name,
	}

	helpOutput, err := runCommand(name, "--help")
	if err != nil {
		manOutput, manErr := runCommand("man", "-P", "cat", name)
		if manErr != nil {
			return nil, fmt.Errorf("failed to get help: %w (man also failed: %v)", err, manErr)
		}
		helpOutput = manOutput
	}

	spec.RawHelp = helpOutput
	spec.Description = extractDescriptionFromHelp(helpOutput, name)
	spec.Usage = extractUsage(helpOutput)
	spec.Flags = extractFlagsFromHelp(helpOutput)
	spec.Subcommands = extractSubcommandsFromHelp(helpOutput)

	if depth > 0 {
		for i := range spec.Subcommands {
			sc := &spec.Subcommands[i]
			subHelp, err := runCommand(name, sc.Name, "--help")
			if err != nil {
				continue
			}
			sc.RawHelp = subHelp
			sc.Usage = extractUsage(subHelp)
			sc.Flags = extractFlagsFromHelp(subHelp)
			sc.Examples = extractExamplesFromHelp(subHelp)
			if sc.Description == "" {
				sc.Description = extractDescriptionFromHelp(subHelp, name+" "+sc.Name)
			}
		}
	}

	return spec, nil
}

func extractDescriptionFromHelp(output, cmdName string) string {
	lines := strings.Split(output, "\n")

	afterCmdName := false
	foundDesc := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		lower := strings.ToLower(trimmed)

		if strings.HasPrefix(lower, "usage:") || strings.HasPrefix(lower, "用法：") {
			break
		}

		if strings.HasPrefix(lower, "available commands") ||
			strings.HasPrefix(lower, "common commands") ||
			strings.HasPrefix(lower, "subcommands") ||
			strings.HasPrefix(lower, "commands:") ||
			strings.HasPrefix(lower, "子命令") ||
			strings.HasPrefix(lower, "常用命令") ||
			strings.HasPrefix(lower, "命令：") ||
			strings.HasPrefix(lower, "flags:") ||
			strings.HasPrefix(lower, "options:") ||
			strings.HasPrefix(lower, "global flags:") ||
			strings.HasPrefix(lower, "see also") ||
			strings.HasPrefix(lower, "（参见：") {
			break
		}

		if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "--") {
			continue
		}

		if strings.Contains(trimmed, "--help") {
			afterCmdName = true
			continue
		}

		if afterCmdName || len(trimmed) > 10 {
			foundDesc = trimmed
			break
		}

		if len(trimmed) > 3 && !strings.Contains(trimmed, "--help") {
			if foundDesc == "" {
				foundDesc = trimmed
			}
		}
	}

	if foundDesc != "" {
		return cleanDescription(foundDesc)
	}
	return ""
}

func extractUsage(output string) string {
	lines := strings.Split(output, "\n")
	var usageLines []string
	inUsage := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "usage:") || strings.HasPrefix(lower, "用法：") {
			inUsage = true
			usageLines = append(usageLines, trimmed)
			continue
		}

		if inUsage {
			if trimmed == "" {
				break
			}
			usageLines = append(usageLines, line)
			if strings.HasPrefix(trimmed, "Available Commands") ||
				strings.HasPrefix(trimmed, "Flags:") ||
				strings.HasPrefix(trimmed, "Options:") {
				break
			}
		}
	}

	if len(usageLines) > 0 {
		usage := strings.Join(usageLines, "\n")
		return strings.TrimSpace(usage)
	}
	return ""
}

func extractSubcommandsFromHelp(output string) []SubcommandSpec {
	var subcommands []SubcommandSpec

	inCommands := false
	commandHeaders := []string{
		"available commands",
		"common commands",
		"management commands",
		"subcommands",
		"commands:",
		"子命令",
		"常用命令",
		"命令：",
		"这些是",
		"开始一个工作区",
		"在当前变更上工作",
		"检查历史和状态",
		"扩展、标记和调校",
	}

	stopHeaders := []string{
		"flags:",
		"options:",
		"global flags:",
		"global options:",
		"use \"",
		"examples:",
		"示例:",
	}

	seen := make(map[string]bool)

	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		lower := strings.ToLower(trimmed)

		isStopHeader := false
		for _, header := range stopHeaders {
			if strings.HasPrefix(lower, header) {
				isStopHeader = true
				break
			}
		}

		if isStopHeader {
			break
		}

		isHeader := false
		for _, header := range commandHeaders {
			if strings.HasPrefix(lower, strings.ToLower(header)) || strings.Contains(lower, strings.ToLower(header)) {
				if !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "--") {
					isHeader = true
					break
				}
			}
		}

		if isHeader {
			inCommands = true
			continue
		}

		if strings.Contains(lower, "（参见：") || strings.Contains(lower, "(see also") {
			continue
		}

		if inCommands {
			if strings.TrimSpace(line) == "" && len(subcommands) > 0 {
				break
			}

			if strings.HasPrefix(strings.TrimSpace(line), "-") || strings.HasPrefix(strings.TrimSpace(line), "--") {
				continue
			}

			if strings.Contains(trimmed, "：") && len(trimmed) > 10 && !strings.HasPrefix(trimmed, "git") {
				continue
			}

			if strings.HasPrefix(trimmed, "请") || strings.HasPrefix(trimmed, "有关") {
				continue
			}

			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				name := parts[0]
				desc := strings.Join(parts[1:], " ")

				if !strings.HasPrefix(name, "-") && !strings.HasPrefix(name, "[") &&
					!strings.Contains(name, "：") && !strings.Contains(name, "、") &&
					!strings.Contains(name, ",") && len(name) > 2 && len(name) < 20 &&
					!strings.HasPrefix(name, "帮助") && !strings.HasPrefix(desc, "help") &&
					!strings.HasPrefix(name, "请") && !strings.HasPrefix(name, "有关") &&
					!strings.Contains(desc, "git help") {

					name = strings.TrimSuffix(name, ":")
					name = strings.TrimSuffix(name, "：")

					if !seen[name] {
						seen[name] = true
						subcommands = append(subcommands, SubcommandSpec{
							Name:        name,
							Description: cleanDescription(desc),
						})
					}
				}
			}
		}
	}

	return subcommands
}

func extractFlagsFromHelp(output string) []FlagSpec {
	var flags []FlagSpec

	lines := strings.Split(output, "\n")
	inOptions := false
	inGlobalFlags := false

	flagHeaders := []string{"flags:", "options:", "选项：", "flags (available"}
	globalHeaders := []string{"global flags:", "global options:", "全局选项："}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		lower := strings.ToLower(trimmed)

		isFlagHeader := false
		for _, header := range flagHeaders {
			if strings.HasPrefix(lower, header) {
				isFlagHeader = true
				break
			}
		}

		isGlobalHeader := false
		for _, header := range globalHeaders {
			if strings.HasPrefix(lower, header) {
				isGlobalHeader = true
				break
			}
		}

		if isFlagHeader {
			inOptions = true
			inGlobalFlags = false
			continue
		}

		if isGlobalHeader {
			inOptions = true
			inGlobalFlags = true
			continue
		}

		if inOptions {
			if trimmed == "" {
				break
			}

			if strings.HasPrefix(trimmed, "Available Commands") ||
				strings.HasPrefix(trimmed, "Use \"") ||
				strings.HasPrefix(lower, "examples") {
				break
			}

			if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "--") {
				flag := parseFlagLine(trimmed)
				if flag.Name != "" && flag.Name != "help" {
					if inGlobalFlags {
						flag.Description += " (global)"
					}
					flags = append(flags, flag)
				}
			} else if len(flags) > 0 {
				flags[len(flags)-1].Description += " " + trimmed
			}
		}
	}

	seen := make(map[string]bool)
	var unique []FlagSpec
	for _, f := range flags {
		if !seen[f.Name] {
			seen[f.Name] = true
			f.Description = cleanDescription(f.Description)
			unique = append(unique, f)
		}
	}

	return unique
}

func parseFlagLine(line string) FlagSpec {
	flag := FlagSpec{}

	line = strings.TrimSpace(line)

	if strings.HasPrefix(line, "-") {
		parts := strings.Fields(line)
		if len(parts) >= 1 {
			firstPart := parts[0]

			if strings.Contains(firstPart, ",") {
				shortAndLong := strings.Split(firstPart, ",")
				if len(shortAndLong) >= 2 {
					flag.Shorthand = strings.TrimSpace(strings.TrimPrefix(shortAndLong[0], "-"))
					flag.Name = strings.TrimSpace(strings.TrimPrefix(shortAndLong[1], "--"))
				} else {
					flag.Name = strings.TrimSpace(strings.TrimPrefix(firstPart, "-"))
				}
			} else {
				if strings.HasPrefix(firstPart, "--") {
					flag.Name = strings.TrimPrefix(firstPart, "--")
				} else {
					flag.Name = strings.TrimPrefix(firstPart, "-")
				}
			}

			if len(parts) > 1 {
				flag.Description = strings.Join(parts[1:], " ")
			}
		}
	}

	return flag
}

func extractExamplesFromHelp(output string) []ExampleSpec {
	var examples []ExampleSpec

	lines := strings.Split(output, "\n")
	inExamples := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		lower := strings.ToLower(trimmed)

		if strings.Contains(lower, "example") ||
			strings.Contains(lower, "实例") ||
			strings.Contains(lower, "示例") {
			inExamples = true
			continue
		}

		if inExamples {
			if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
				continue
			}

			if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "--") {
				break
			}

			if strings.TrimSpace(line) == "" && len(examples) > 0 {
				break
			}

			if len(trimmed) > 2 && !strings.HasPrefix(trimmed, "git") {
				examples = append(examples, ExampleSpec{
					Command: trimmed,
					Desc:    "",
				})
			}
		}
	}

	return examples
}

func cleanDescription(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	return s
}

func generateAllSkills(spec *CommandSpec, out *os.File) {
	generateMainSkill(spec, out)
	out.WriteString("\n---\n\n")

	grouped := groupSubcommandsForSkills(spec.Subcommands)

	for groupName, cmds := range grouped {
		generateGroupSkill(spec.Name, groupName, cmds, out)
		out.WriteString("\n---\n\n")
	}
}

func generateMainSkill(spec *CommandSpec, out *os.File) {
	desc := spec.Description
	if desc == "" {
		desc = spec.Name + " command"
	}

	out.WriteString("---\n")
	out.WriteString(fmt.Sprintf("name: %s\n", spec.Name))
	out.WriteString(fmt.Sprintf("description: %s\n", desc))
	out.WriteString("---\n\n")

	out.WriteString(fmt.Sprintf("# %s\n\n", spec.Name))

	if spec.RawHelp != "" {
		out.WriteString("```\n")
		out.WriteString(spec.RawHelp)
		if !strings.HasSuffix(spec.RawHelp, "\n") {
			out.WriteString("\n")
		}
		out.WriteString("```\n\n")
	}

	if len(spec.Subcommands) > 0 {
		out.WriteString("## References\n\n")
		out.WriteString("| Command | Description |\n")
		out.WriteString("|---------|-------------|\n")

		groups := groupSubcommandsForSkills(spec.Subcommands)

		for groupName, cmds := range groups {
			if len(cmds) > 3 {
				out.WriteString(fmt.Sprintf("| [%s](references/%s.md) | %s |\n",
					groupName, groupName, groupName))
			}

			for _, sc := range cmds {
				out.WriteString(fmt.Sprintf("| [%s](references/%s.md) | %s |\n",
					sc.Name, sc.Name, sc.Description))
			}
		}
		out.WriteString("\n")
	}
}

type skillGroup struct {
	name string
	cmds []SubcommandSpec
}

func groupSubcommandsForSkills(subcommands []SubcommandSpec) map[string][]SubcommandSpec {
	groups := make(map[string][]SubcommandSpec)
	groups["worktree-commands"] = []SubcommandSpec{}
	groups["history-commands"] = []SubcommandSpec{}
	groups["branch-commands"] = []SubcommandSpec{}
	groups["collab-commands"] = []SubcommandSpec{}
	groups["other-commands"] = []SubcommandSpec{}

	workCmds := map[string]bool{
		"add": true, "mv": true, "rm": true, "restore": true, "status": true,
		"diff": true, "clean": true, "checkout": true, "reset": true,
	}
	historyCmds := map[string]bool{
		"log": true, "show": true, "blame": true, "bisect": true,
		"reflog": true, "shortlog": true, "describe": true,
	}
	branchCmds := map[string]bool{
		"branch": true, "commit": true, "merge": true, "rebase": true,
		"switch": true, "tag": true, "stash": true, "worktree": true,
	}
	collabCmds := map[string]bool{
		"clone": true, "fetch": true, "pull": true, "push": true,
		"remote": true, "submodule": true, "init": true,
	}

	for _, sc := range subcommands {
		group := "other-commands"
		if workCmds[sc.Name] {
			group = "worktree-commands"
		} else if historyCmds[sc.Name] {
			group = "history-commands"
		} else if branchCmds[sc.Name] {
			group = "branch-commands"
		} else if collabCmds[sc.Name] {
			group = "collab-commands"
		}
		groups[group] = append(groups[group], sc)
	}

	for g := range groups {
		if len(groups[g]) == 0 {
			delete(groups, g)
		}
	}

	return groups
}

func generateGroupSkill(mainCmd, groupName string, subcommands []SubcommandSpec, out *os.File) {
	skillName := mainCmd + "-" + groupName

	out.WriteString("---\n")
	out.WriteString(fmt.Sprintf("name: %s\n", skillName))
	out.WriteString(fmt.Sprintf("description: %s %s commands - %s\n", mainCmd, groupName, summarizeGroup(subcommands)))
	out.WriteString("---\n\n")

	out.WriteString(fmt.Sprintf("# %s\n\n", skillName))
	out.WriteString(fmt.Sprintf("**Commands**: %s\n\n", summarizeGroup(subcommands)))

	out.WriteString("## Commands\n\n")
	for _, sc := range subcommands {
		out.WriteString(fmt.Sprintf("### %s %s\n\n", mainCmd, sc.Name))

		if sc.RawHelp != "" {
			out.WriteString("```\n")
			out.WriteString(sc.RawHelp)
			if !strings.HasSuffix(sc.RawHelp, "\n") {
				out.WriteString("\n")
			}
			out.WriteString("```\n\n")
		} else {
			out.WriteString(fmt.Sprintf("**Description**: %s\n\n", sc.Description))
			if sc.Usage != "" {
				out.WriteString(fmt.Sprintf("**Usage**: `%s`\n\n", sc.Usage))
			} else {
				out.WriteString(fmt.Sprintf("**Usage**: `%s %s [options]`\n\n", mainCmd, sc.Name))
			}
		}

		out.WriteString("---\n\n")
	}
}

func summarizeGroup(subcommands []SubcommandSpec) string {
	names := []string{}
	for _, sc := range subcommands {
		if len(names) < 5 {
			names = append(names, sc.Name)
		}
	}
	if len(subcommands) > 5 {
		return strings.Join(names, ", ") + fmt.Sprintf(" and %d more", len(subcommands)-5)
	}
	return strings.Join(names, ", ")
}

func writeSkillFiles(spec *CommandSpec, skillDir string) error {
	mainSkillPath := filepath.Join(skillDir, "SKILL.md")
	mainFile, err := os.Create(mainSkillPath)
	if err != nil {
		return fmt.Errorf("create main skill file: %w", err)
	}
	generateMainSkill(spec, mainFile)
	mainFile.Close()
	fmt.Printf("  wrote: SKILL.md\n")

	refDir := filepath.Join(skillDir, "references")
	if err := os.MkdirAll(refDir, 0o755); err != nil {
		return fmt.Errorf("create references dir: %w", err)
	}

	groups := groupSubcommandsForSkills(spec.Subcommands)
	written := make(map[string]bool)

	for groupName, cmds := range groups {
		if len(cmds) > 3 {
			groupFile, err := os.Create(filepath.Join(refDir, groupName+".md"))
			if err != nil {
				return fmt.Errorf("create group ref file: %w", err)
			}
			generateGroupSkill(spec.Name, groupName, cmds, groupFile)
			groupFile.Close()
			fmt.Printf("  wrote: references/%s.md\n", groupName)
			written[groupName] = true
		}

		for _, sc := range cmds {
			if written[sc.Name] {
				continue
			}
			scFile, err := os.Create(filepath.Join(refDir, sc.Name+".md"))
			if err != nil {
				return fmt.Errorf("create subcommand ref file: %w", err)
			}
			generateSingleSubcommandSkill(spec.Name, &sc, scFile)
			scFile.Close()
			fmt.Printf("  wrote: references/%s.md\n", sc.Name)
			written[sc.Name] = true
		}
	}

	return nil
}

func generateSingleSubcommandSkill(mainCmd string, spec *SubcommandSpec, out *os.File) {
	skillName := mainCmd + "-" + spec.Name

	desc := spec.Description
	if desc == "" {
		desc = fmt.Sprintf("%s %s command", mainCmd, spec.Name)
	}

	out.WriteString("---\n")
	out.WriteString(fmt.Sprintf("name: %s\n", skillName))
	out.WriteString(fmt.Sprintf("description: %s\n", desc))
	out.WriteString("---\n\n")

	out.WriteString(fmt.Sprintf("# %s %s\n\n", mainCmd, spec.Name))

	if spec.RawHelp != "" {
		out.WriteString("```\n")
		out.WriteString(spec.RawHelp)
		if !strings.HasSuffix(spec.RawHelp, "\n") {
			out.WriteString("\n")
		}
		out.WriteString("```\n\n")
	}
}

func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w - %s", name, strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}
