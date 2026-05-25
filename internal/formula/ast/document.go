package ast

import "encoding/json"

type SourcePos struct {
	File   string
	Line   int
	Column int
}

type Document struct {
	Name        string
	Title       string
	Description string
	Version     int
	Contract    string
	Vars        map[string]VarDecl
	Steps       []StepDecl
	Source      SourcePos
}

type VarDecl struct {
	Description string
	Default     *string
	Required    bool
	Enum        []string
	Pattern     string
	Type        string
}

type StepDecl struct {
	ID        string
	Kind      string
	Title     string
	DependsOn []string
	Raw       json.RawMessage
	Source    SourcePos
}
