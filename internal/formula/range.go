package formula

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type RangeSpec struct {
	Start int
	End   int
}

var rangePattern = regexp.MustCompile(`^(.+)\.\.(.+)$`)
var rangeVarPattern = regexp.MustCompile(`\{(\w+)\}`)

func ParseRange(expr string, vars map[string]string) (*RangeSpec, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("empty range expression")
	}

	m := rangePattern.FindStringSubmatch(expr)
	if m == nil {
		return nil, fmt.Errorf("invalid range format %q: expected start..end", expr)
	}

	startExpr := strings.TrimSpace(m[1])
	endExpr := strings.TrimSpace(m[2])

	start, err := EvaluateExpr(startExpr, vars)
	if err != nil {
		return nil, fmt.Errorf("evaluating range start %q: %w", startExpr, err)
	}

	end, err := EvaluateExpr(endExpr, vars)
	if err != nil {
		return nil, fmt.Errorf("evaluating range end %q: %w", endExpr, err)
	}

	return &RangeSpec{Start: start, End: end}, nil
}

func EvaluateExpr(expr string, vars map[string]string) (int, error) {
	expr = substituteVars(expr, vars)
	tokens, err := tokenize(expr)
	if err != nil {
		return 0, err
	}
	result, err := parseExpr(tokens)
	if err != nil {
		return 0, err
	}
	return int(result), nil
}

func substituteVars(expr string, vars map[string]string) string {
	if vars == nil {
		return expr
	}
	return rangeVarPattern.ReplaceAllStringFunc(expr, func(match string) string {
		name := match[1 : len(match)-1]
		if val, ok := vars[name]; ok {
			return val
		}
		return match
	})
}

type tokenType int

const (
	tokNumber tokenType = iota
	tokPlus
	tokMinus
	tokMul
	tokDiv
	tokPow
	tokLParen
	tokRParen
	tokEOF
)

type token struct {
	typ tokenType
	val float64
}

func tokenize(expr string) ([]token, error) {
	var tokens []token
	i := 0

	for i < len(expr) {
		ch := expr[i]

		if unicode.IsSpace(rune(ch)) {
			i++
			continue
		}

		if unicode.IsDigit(rune(ch)) {
			j := i
			for j < len(expr) && (unicode.IsDigit(rune(expr[j])) || expr[j] == '.') {
				j++
			}
			val, err := strconv.ParseFloat(expr[i:j], 64)
			if err != nil {
				return nil, fmt.Errorf("invalid number %q", expr[i:j])
			}
			tokens = append(tokens, token{tokNumber, val})
			i = j
			continue
		}

		switch ch {
		case '+':
			tokens = append(tokens, token{tokPlus, 0})
		case '-':
			if len(tokens) == 0 || (tokens[len(tokens)-1].typ != tokNumber && tokens[len(tokens)-1].typ != tokRParen) {
				j := i + 1
				for j < len(expr) && (unicode.IsDigit(rune(expr[j])) || expr[j] == '.') {
					j++
				}
				if j > i+1 {
					val, err := strconv.ParseFloat(expr[i:j], 64)
					if err != nil {
						return nil, fmt.Errorf("invalid number %q", expr[i:j])
					}
					tokens = append(tokens, token{tokNumber, val})
					i = j
					continue
				}
			}
			tokens = append(tokens, token{tokMinus, 0})
		case '*':
			tokens = append(tokens, token{tokMul, 0})
		case '/':
			tokens = append(tokens, token{tokDiv, 0})
		case '^':
			tokens = append(tokens, token{tokPow, 0})
		case '(':
			tokens = append(tokens, token{tokLParen, 0})
		case ')':
			tokens = append(tokens, token{tokRParen, 0})
		default:
			return nil, fmt.Errorf("unexpected character %q in expression", ch)
		}
		i++
	}

	tokens = append(tokens, token{tokEOF, 0})
	return tokens, nil
}

type exprParser struct {
	tokens []token
	pos    int
}

func (p *exprParser) current() token {
	if p.pos >= len(p.tokens) {
		return token{tokEOF, 0}
	}
	return p.tokens[p.pos]
}

func (p *exprParser) advance() {
	p.pos++
}

func parseExpr(tokens []token) (float64, error) {
	p := &exprParser{tokens: tokens}
	result, err := p.parseAddSub()
	if err != nil {
		return 0, err
	}
	if p.current().typ != tokEOF {
		return 0, fmt.Errorf("unexpected token after expression")
	}
	return result, nil
}

func (p *exprParser) parseAddSub() (float64, error) {
	left, err := p.parseMulDiv()
	if err != nil {
		return 0, err
	}

	for {
		switch p.current().typ {
		case tokPlus:
			p.advance()
			right, err := p.parseMulDiv()
			if err != nil {
				return 0, err
			}
			left += right
		case tokMinus:
			p.advance()
			right, err := p.parseMulDiv()
			if err != nil {
				return 0, err
			}
			left -= right
		default:
			return left, nil
		}
	}
}

func (p *exprParser) parseMulDiv() (float64, error) {
	left, err := p.parsePow()
	if err != nil {
		return 0, err
	}

	for {
		switch p.current().typ {
		case tokMul:
			p.advance()
			right, err := p.parsePow()
			if err != nil {
				return 0, err
			}
			left *= right
		case tokDiv:
			p.advance()
			right, err := p.parsePow()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			left /= right
		default:
			return left, nil
		}
	}
}

func (p *exprParser) parsePow() (float64, error) {
	base, err := p.parseUnary()
	if err != nil {
		return 0, err
	}

	if p.current().typ == tokPow {
		p.advance()
		exp, err := p.parsePow()
		if err != nil {
			return 0, err
		}
		return math.Pow(base, exp), nil
	}

	return base, nil
}

func (p *exprParser) parseUnary() (float64, error) {
	if p.current().typ == tokMinus {
		p.advance()
		val, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		return -val, nil
	}
	return p.parsePrimary()
}

func (p *exprParser) parsePrimary() (float64, error) {
	switch p.current().typ {
	case tokNumber:
		val := p.current().val
		p.advance()
		return val, nil
	case tokLParen:
		p.advance()
		val, err := p.parseAddSub()
		if err != nil {
			return 0, err
		}
		if p.current().typ != tokRParen {
			return 0, fmt.Errorf("expected closing parenthesis")
		}
		p.advance()
		return val, nil
	default:
		return 0, fmt.Errorf("unexpected token in expression")
	}
}

func ValidateRange(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return fmt.Errorf("empty range expression")
	}

	m := rangePattern.FindStringSubmatch(expr)
	if m == nil {
		return fmt.Errorf("invalid range format: expected start..end")
	}

	placeholderVars := make(map[string]string)
	rangeVarPattern.ReplaceAllStringFunc(expr, func(match string) string {
		name := match[1 : len(match)-1]
		placeholderVars[name] = "1"
		return "1"
	})

	startExpr := strings.TrimSpace(m[1])
	startExpr = substituteVars(startExpr, placeholderVars)
	if _, err := tokenize(startExpr); err != nil {
		return fmt.Errorf("invalid start expression: %w", err)
	}

	endExpr := strings.TrimSpace(m[2])
	endExpr = substituteVars(endExpr, placeholderVars)
	if _, err := tokenize(endExpr); err != nil {
		return fmt.Errorf("invalid end expression: %w", err)
	}

	return nil
}
