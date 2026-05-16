package formula

import (
	"fmt"
	"regexp"
	"strings"
)

type ConditionType string

const (
	ConditionTypeField     ConditionType = "field"
	ConditionTypeAggregate ConditionType = "aggregate"
	ConditionTypeExternal  ConditionType = "external"
)

type Operator string

const (
	OpEqual        Operator = "=="
	OpNotEqual     Operator = "!="
	OpGreater      Operator = ">"
	OpGreaterEqual Operator = ">="
	OpLess         Operator = "<"
	OpLessEqual    Operator = "<="
	OpRegexMatch   Operator = "=~"
)

type Condition struct {
	Raw           string
	Type          ConditionType
	StepRef       string
	Field         string
	Operator      Operator
	Value         string
	AggregateFunc string
	AggregateOver string
	ExternalType  string
	ExternalArg   string
}

var (
	fieldPattern       = regexp.MustCompile(`^(\w+(?:\.\w+)*)\s*([=!<>~]+)\s*(.+)$`)
	aggregatePattern   = regexp.MustCompile(`^(children|descendants|steps)\((\w+)\)\.(all|any|count)\((.+)\)(.*)$`)
	fileExistsPattern  = regexp.MustCompile(`^file\.exists\(['"](.+)['"]\)$`)
	envPattern         = regexp.MustCompile(`^env\.(\w+)\s*([=!<>]+)\s*(.+)$`)
	stepsStatPattern   = regexp.MustCompile(`^steps\.(\w+)\s*([=!<>]+)\s*(\d+)$`)
	countComparePattern = regexp.MustCompile(`\s*([=!<>]+)\s*(\d+)$`)
)

func ParseCondition(expr string) (*Condition, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("empty condition")
	}

	if m := fileExistsPattern.FindStringSubmatch(expr); m != nil {
		return &Condition{
			Raw:          expr,
			Type:         ConditionTypeExternal,
			ExternalType: "file.exists",
			ExternalArg:  m[1],
		}, nil
	}

	if m := envPattern.FindStringSubmatch(expr); m != nil {
		return &Condition{
			Raw:          expr,
			Type:         ConditionTypeExternal,
			ExternalType: "env",
			ExternalArg:  m[1],
			Operator:     Operator(m[2]),
			Value:        unquoteCond(m[3]),
		}, nil
	}

	if m := aggregatePattern.FindStringSubmatch(expr); m != nil {
		innerCond, err := ParseCondition(m[4])
		if err != nil {
			return nil, fmt.Errorf("parsing aggregate inner condition: %w", err)
		}
		cond := &Condition{
			Raw:           expr,
			Type:          ConditionTypeAggregate,
			AggregateOver: m[1],
			StepRef:       m[2],
			AggregateFunc: m[3],
			Field:         innerCond.Field,
			Operator:      innerCond.Operator,
			Value:         innerCond.Value,
		}
		if m[5] != "" {
			if countMatch := countComparePattern.FindStringSubmatch(m[5]); countMatch != nil {
				cond.AggregateFunc = "count"
				cond.Operator = Operator(countMatch[1])
				cond.Value = countMatch[2]
			}
		}
		return cond, nil
	}

	if m := stepsStatPattern.FindStringSubmatch(expr); m != nil {
		return &Condition{
			Raw:           expr,
			Type:          ConditionTypeAggregate,
			AggregateOver: "steps",
			AggregateFunc: "count",
			Field:         m[1],
			Operator:      Operator(m[2]),
			Value:         m[3],
		}, nil
	}

	if m := fieldPattern.FindStringSubmatch(expr); m != nil {
		fieldPath := m[1]
		parts := strings.SplitN(fieldPath, ".", 2)

		stepRef := "step"
		field := fieldPath

		if len(parts) >= 2 {
			switch parts[0] {
			case "step":
				field = parts[1]
			case "output":
				field = fieldPath
			default:
				stepRef = parts[0]
				field = parts[1]
			}
		}

		return &Condition{
			Raw:      expr,
			Type:     ConditionTypeField,
			StepRef:  stepRef,
			Field:    field,
			Operator: Operator(m[2]),
			Value:    unquoteCond(m[3]),
		}, nil
	}

	return nil, fmt.Errorf("unrecognized condition format: %s", expr)
}

func unquoteCond(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
