package steps

import (
	"context"
	"fmt"

	"github.com/sjzsdu/tt/internal/formula/ast"
)

type Decoder interface {
	Kind() Kind
	Decode(ast.StepDecl) (Step, error)
}

type Runner interface {
	Kind() Kind
	Run(context.Context, RunRequest) (*RunResult, error)
}

type Registry struct {
	decoders map[Kind]Decoder
}

func NewRegistry() *Registry {
	return &Registry{decoders: make(map[Kind]Decoder)}
}

func (r *Registry) RegisterDecoder(decoder Decoder) error {
	if decoder == nil {
		return fmt.Errorf("step decoder is nil")
	}
	kind := decoder.Kind()
	if kind == "" {
		return fmt.Errorf("step decoder kind is required")
	}
	if _, exists := r.decoders[kind]; exists {
		return fmt.Errorf("step decoder for %q already registered", kind)
	}
	r.decoders[kind] = decoder
	return nil
}

func (r *Registry) Decode(decl ast.StepDecl) (Step, error) {
	kind := Kind(decl.Kind)
	if kind == "" {
		kind = KindAgent
	}
	decoder, ok := r.decoders[kind]
	if !ok {
		return nil, fmt.Errorf("unsupported step kind %q", kind)
	}
	return decoder.Decode(decl)
}

func NewDefaultRegistry() *Registry {
	r := NewRegistry()
	for _, decoder := range []Decoder{
		NoopDecoder{},
		AgentDecoder{},
		ScriptDecoder{},
		HumanInputDecoder{},
		LoopDecoder{},
		RetryDecoder{},
	} {
		_ = r.RegisterDecoder(decoder)
	}
	return r
}
