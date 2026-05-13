package cmd2skill

import (
	"regexp"
	"sort"
	"strings"
)

func parseHelp(output string, path []string) *CommandNode {
	n := &CommandNode{
		Name:        path[len(path)-1],
		Path:        append([]string{}, path...),
		RawHelp:     output,
		Description: extractDescriptionFromHelp(output),
		Usage:       extractUsage(output),
		Flags:       extractFlagsFromHelp(output),
		Examples:    extractExamplesFromHelp(output),
		Children:    extractSubcommandsFromHelp(output),
	}
	for _, child := range n.Children {
		child.Path = append(append([]string{}, path...), child.Name)
	}
	sort.SliceStable(n.Flags, func(i, j int) bool { return n.Flags[i].Name < n.Flags[j].Name })
	sort.SliceStable(n.Children, func(i, j int) bool { return n.Children[i].Name < n.Children[j].Name })
	return n
}

func extractDescriptionFromHelp(output string) string {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "usage:") || strings.HasPrefix(lower, "用法：") || isSectionHeader(lower) {
			continue
		}
		if strings.HasPrefix(trimmed, "-") || strings.Contains(trimmed, "--help") {
			continue
		}
		if len(trimmed) > 3 {
			return cleanDescription(trimmed)
		}
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
			if trimmed == "" || isSectionHeader(lower) {
				break
			}
			usageLines = append(usageLines, line)
		}
	}
	return strings.TrimSpace(strings.Join(usageLines, "\n"))
}

func extractSubcommandsFromHelp(output string) []*CommandNode {
	var subcommands []*CommandNode
	seen := map[string]bool{}
	inCommands := false
	section := "commands"
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if isCommandHeader(lower) {
			inCommands = true
			section = strings.TrimSuffix(trimmed, ":")
			continue
		}
		if inCommands && isStopHeader(lower) {
			break
		}
		if !inCommands || strings.HasPrefix(trimmed, "-") {
			continue
		}
		parts := strings.Fields(trimmed)
		if len(parts) < 2 {
			continue
		}
		name := strings.TrimSuffix(strings.TrimSuffix(parts[0], ":"), "：")
		desc := strings.Join(parts[1:], " ")
		if !validCommandName(name) || seen[name] || strings.HasPrefix(strings.ToLower(desc), "help") {
			continue
		}
		seen[name] = true
		subcommands = append(subcommands, &CommandNode{Name: name, Description: cleanDescription(desc), Section: section})
	}
	return subcommands
}

func validCommandName(name string) bool {
	if len(name) < 2 || len(name) > 40 {
		return false
	}
	if strings.HasPrefix(name, "-") || strings.HasPrefix(name, "[") {
		return false
	}
	if strings.ContainsAny(name, "：、,") {
		return false
	}
	return regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]*$`).MatchString(name)
}

func extractFlagsFromHelp(output string) []Flag {
	var flags []Flag
	lines := strings.Split(output, "\n")
	inOptions := false
	global := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if isFlagHeader(lower) {
			inOptions = true
			global = false
			continue
		}
		if isGlobalHeader(lower) {
			inOptions = true
			global = true
			continue
		}
		if inOptions && isStopHeader(lower) {
			break
		}
		if !inOptions {
			continue
		}
		if strings.HasPrefix(trimmed, "-") {
			flag := parseFlagLine(line)
			if flag.Name != "" && flag.Name != "help" {
				flag.Global = global
				flags = append(flags, flag)
			}
		} else if len(flags) > 0 && (strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")) {
			flags[len(flags)-1].Description += " " + trimmed
		}
	}
	seen := map[string]bool{}
	var unique []Flag
	for _, f := range flags {
		key := f.Name + "/" + f.Shorthand
		if seen[key] {
			continue
		}
		seen[key] = true
		f.Description = cleanDescription(f.Description)
		unique = append(unique, f)
	}
	return unique
}

var flagLinePattern = regexp.MustCompile(`^\s*((?:-[A-Za-z0-9](?:,\s*)?)?(?:--[A-Za-z0-9][A-Za-z0-9_.-]*)?)\s*([^\s]+)?\s*(.*)$`)

func parseFlagLine(line string) Flag {
	flag := Flag{}
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "-") {
		return flag
	}
	match := flagLinePattern.FindStringSubmatch(trimmed)
	if len(match) == 0 {
		return flag
	}
	flagPart := strings.ReplaceAll(match[1], " ", "")
	for _, part := range strings.Split(flagPart, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "--") {
			flag.Name = strings.TrimPrefix(part, "--")
		} else if strings.HasPrefix(part, "-") {
			flag.Shorthand = strings.TrimPrefix(part, "-")
		}
	}
	if flag.Name == "" && flag.Shorthand != "" {
		flag.Name = flag.Shorthand
	}
	if isTypeKeyword(match[2]) {
		flag.Type = match[2]
		flag.Description = strings.TrimSpace(match[3])
	} else {
		flag.Description = strings.TrimSpace(match[2] + " " + match[3])
	}
	return flag
}

func isTypeKeyword(s string) bool {
	return map[string]bool{"string": true, "int": true, "bool": true, "float": true, "float64": true, "int64": true, "uint": true, "uint64": true, "duration": true, "time": true, "[]string": true, "strings": true}[s]
}

func extractExamplesFromHelp(output string) []Example {
	var examples []Example
	lines := strings.Split(output, "\n")
	inExamples := false
	current := Example{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if current.Command != "" {
				examples = append(examples, current)
				current = Example{}
			}
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "example") || strings.Contains(lower, "实例") || strings.Contains(lower, "示例") {
			inExamples = true
			continue
		}
		if inExamples && isStopHeader(lower) {
			break
		}
		if !inExamples {
			continue
		}
		if strings.HasPrefix(trimmed, "$ ") || strings.HasPrefix(trimmed, "> ") {
			if current.Command != "" {
				examples = append(examples, current)
			}
			current = Example{Command: strings.TrimSpace(trimmed[2:])}
			continue
		}
		if current.Command == "" {
			current.Command = trimmed
		} else {
			current.Desc = cleanDescription(current.Desc + " " + trimmed)
		}
	}
	if current.Command != "" {
		examples = append(examples, current)
	}
	return examples
}

func isSectionHeader(lower string) bool {
	return isCommandHeader(lower) || isFlagHeader(lower) || isGlobalHeader(lower) || strings.HasPrefix(lower, "examples") || strings.HasPrefix(lower, "see also")
}
func isCommandHeader(lower string) bool {
	for _, h := range []string{"available commands", "common commands", "management commands", "subcommands", "commands:", "子命令", "常用命令", "命令："} {
		if strings.HasPrefix(lower, h) {
			return true
		}
	}
	return false
}
func isFlagHeader(lower string) bool {
	for _, h := range []string{"flags:", "options:", "选项：", "flags (available"} {
		if strings.HasPrefix(lower, h) {
			return true
		}
	}
	return false
}
func isGlobalHeader(lower string) bool {
	for _, h := range []string{"global flags:", "global options:", "全局选项："} {
		if strings.HasPrefix(lower, h) {
			return true
		}
	}
	return false
}
func isStopHeader(lower string) bool {
	return isFlagHeader(lower) || isGlobalHeader(lower) || strings.HasPrefix(lower, "use ") || strings.HasPrefix(lower, "examples") || strings.HasPrefix(lower, "示例") || strings.HasPrefix(lower, "see also")
}

func cleanDescription(s string) string {
	return regexp.MustCompile(`\s+`).ReplaceAllString(strings.TrimSpace(s), " ")
}
