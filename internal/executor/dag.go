package executor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sjzsdu/tt/internal/formula"
)

func TopologicalBatches(recipe *formula.Recipe) ([][]*formula.RecipeStep, error) {
	inDegree := make(map[string]int)
	adj := make(map[string][]string)
	stepMap := make(map[string]*formula.RecipeStep)

	for _, step := range recipe.Steps {
		stepMap[step.ID] = &step
		inDegree[step.ID] = 0
	}

	for _, dep := range recipe.Deps {
		if dep.Type == "parent-child" {
			continue
		}
		if _, ok := stepMap[dep.DependsOnID]; !ok {
			continue
		}
		if _, ok := stepMap[dep.StepID]; !ok {
			continue
		}
		adj[dep.DependsOnID] = append(adj[dep.DependsOnID], dep.StepID)
		inDegree[dep.StepID]++
	}

	var batches [][]*formula.RecipeStep
	remaining := len(stepMap)

	for remaining > 0 {
		var batch []*formula.RecipeStep
		for id, deg := range inDegree {
			if deg == 0 && stepMap[id] != nil {
				batch = append(batch, stepMap[id])
				delete(inDegree, id)
			}
		}

		if len(batch) == 0 {
			return nil, fmt.Errorf("cycle detected in dependency graph")
		}

		batches = append(batches, batch)

		for _, step := range batch {
			for _, next := range adj[step.ID] {
				inDegree[next]--
			}
			delete(stepMap, step.ID)
			remaining--
		}
	}

	return batches, nil
}

func EvaluateCondition(expr string, ctx map[string]string) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true
	}

	if strings.Contains(expr, "||") {
		parts := strings.SplitN(expr, "||", 2)
		return EvaluateCondition(strings.TrimSpace(parts[0]), ctx) ||
			EvaluateCondition(strings.TrimSpace(parts[1]), ctx)
	}

	if strings.Contains(expr, "&&") {
		parts := strings.SplitN(expr, "&&", 2)
		return EvaluateCondition(strings.TrimSpace(parts[0]), ctx) &&
			EvaluateCondition(strings.TrimSpace(parts[1]), ctx)
	}

	return evalSingleCondition(expr, ctx)
}

func evalSingleCondition(expr string, ctx map[string]string) bool {
	expr = strings.TrimSpace(expr)

	if strings.Contains(expr, " =~ ") {
		parts := strings.SplitN(expr, " =~ ", 2)
		left := resolveValue(strings.TrimSpace(parts[0]), ctx)
		pattern := unquote(strings.TrimSpace(parts[1]))
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		return re.MatchString(left)
	}

	if strings.Contains(expr, " == ") {
		parts := strings.SplitN(expr, " == ", 2)
		left := resolveValue(strings.TrimSpace(parts[0]), ctx)
		right := unquote(strings.TrimSpace(parts[1]))
		return left == right
	}

	if strings.Contains(expr, " != ") {
		parts := strings.SplitN(expr, " != ", 2)
		left := resolveValue(strings.TrimSpace(parts[0]), ctx)
		right := unquote(strings.TrimSpace(parts[1]))
		return left != right
	}

	val := resolveValue(expr, ctx)
	return val != "" && val != "false" && val != "0"
}

func resolveValue(s string, ctx map[string]string) string {
	if v, ok := ctx[s]; ok {
		return v
	}
	return unquote(s)
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') ||
			(s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
