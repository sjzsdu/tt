package cmd2skill

import (
	"regexp"
	"sort"
	"strings"
)

const (
	maxSkillDescriptionLen   = 320
	maxCommandDescriptionLen = 240
)

var descriptionWordPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_-]*`)

var descriptionStopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true, "be": true,
	"by": true, "for": true, "from": true, "in": true, "into": true, "is": true, "it": true,
	"of": true, "on": true, "or": true, "the": true, "this": true, "to": true, "with": true,
	"using": true, "used": true, "use": true, "command": true, "commands": true, "cli": true,
	"tool": true, "tools": true, "more": true, "about": true, "specific": true, "available": true,
}

func SkillDescription(model *CLIModel) string {
	if model == nil || model.Root == nil {
		return "Use this command line tool."
	}
	root := model.Root
	rootDesc := cleanSentence(root.Description)
	if rootDesc == "" {
		rootDesc = root.Name + " command line tool"
	}
	commands := commandNames(root.Children, 14)
	keywords := descriptionKeywords(root.Children, 14)

	parts := []string{"Use " + model.Name + ". " + rootDesc + "."}
	if len(commands) > 0 {
		parts = append(parts, "Common commands include "+strings.Join(commands, ", ")+".")
	}
	if len(keywords) > 0 {
		parts = append(parts, "Useful for "+strings.Join(keywords, ", ")+".")
	}
	return truncateDescription(strings.Join(parts, " "), maxSkillDescriptionLen)
}

func CommandDescription(n *CommandNode) string {
	if n == nil {
		return "Use this command."
	}
	path := strings.Join(n.Path, " ")
	desc := cleanSentence(n.Description)
	if desc == "" {
		desc = path + " command"
	}
	children := commandNames(n.Children, 12)
	keywords := descriptionKeywords(n.Children, 10)

	parts := []string{"Use " + path + ". " + desc + "."}
	if len(children) > 0 {
		parts = append(parts, "Includes "+strings.Join(children, ", ")+".")
	}
	if len(keywords) > 0 {
		parts = append(parts, "Useful for "+strings.Join(keywords, ", ")+".")
	}
	return truncateDescription(strings.Join(parts, " "), maxCommandDescriptionLen)
}

func commandNames(nodes []*CommandNode, limit int) []string {
	seen := map[string]bool{}
	var names []string
	for _, n := range sortedNodes(nodes) {
		name := n.Name
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
		if len(names) >= limit {
			break
		}
	}
	return names
}

func descriptionKeywords(nodes []*CommandNode, limit int) []string {
	counts := map[string]int{}
	order := map[string]int{}
	idx := 0
	for _, n := range sortedNodes(nodes) {
		for _, word := range wordsForDescription(n.Name + " " + n.Description) {
			if _, ok := order[word]; !ok {
				order[word] = idx
				idx++
			}
			counts[word]++
		}
		for _, child := range sortedNodes(n.Children) {
			for _, word := range wordsForDescription(child.Name + " " + child.Description) {
				if _, ok := order[word]; !ok {
					order[word] = idx
					idx++
				}
				counts[word]++
			}
		}
	}
	words := make([]string, 0, len(counts))
	for w := range counts {
		words = append(words, w)
	}
	sort.SliceStable(words, func(i, j int) bool {
		if counts[words[i]] == counts[words[j]] {
			return order[words[i]] < order[words[j]]
		}
		return counts[words[i]] > counts[words[j]]
	})
	if len(words) > limit {
		words = words[:limit]
	}
	return words
}

func wordsForDescription(text string) []string {
	matches := descriptionWordPattern.FindAllString(strings.ToLower(text), -1)
	seen := map[string]bool{}
	var words []string
	for _, w := range matches {
		w = strings.Trim(w, "_-")
		if len(w) < 3 || descriptionStopWords[w] || seen[w] {
			continue
		}
		seen[w] = true
		words = append(words, w)
	}
	return words
}

func cleanSentence(s string) string {
	s = cleanDescription(s)
	s = strings.TrimSuffix(s, ".")
	return s
}

func truncateDescription(s string, maxLen int) string {
	s = cleanDescription(s)
	if len(s) <= maxLen {
		return s
	}
	cut := strings.LastIndex(s[:maxLen-1], " ")
	if cut < maxLen/2 {
		cut = maxLen - 1
	}
	return strings.TrimSpace(s[:cut]) + "…"
}
