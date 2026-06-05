package steps

import (
	"testing"

	"github.com/sjzsdu/tt/internal/formula/ast"
)

func TestMetadataIdempotentRoundTripsFromStepDecl(t *testing.T) {
	decl := ast.StepDecl{ID: "ask", Idempotent: true}
	metadata := metadataFromDecl(decl, KindAgent)
	if !metadata.Idempotent {
		t.Fatal("idempotent = false, want true")
	}
}

func TestAgentKindDefaultsToIdempotent(t *testing.T) {
	decl := ast.StepDecl{ID: "ask"}
	metadata := metadataFromDecl(decl, KindAgent)
	if !metadata.Idempotent {
		t.Fatal("idempotent = false, want true")
	}
}

func TestExternalAgentKindDefaultsToIdempotent(t *testing.T) {
	decl := ast.StepDecl{ID: "ask"}
	metadata := metadataFromDecl(decl, KindExternalAgent)
	if !metadata.Idempotent {
		t.Fatal("idempotent = false, want true")
	}
}

func TestScriptKindDefaultsToNonIdempotent(t *testing.T) {
	decl := ast.StepDecl{ID: "run"}
	metadata := metadataFromDecl(decl, KindScript)
	if metadata.Idempotent {
		t.Fatal("idempotent = true, want false")
	}
}

func TestScriptKindRespectsExplicitTrue(t *testing.T) {
	decl := ast.StepDecl{ID: "run", Idempotent: true}
	metadata := metadataFromDecl(decl, KindScript)
	if !metadata.Idempotent {
		t.Fatal("idempotent = false, want true")
	}
}

func TestLoopKindDefaultsToNonIdempotent(t *testing.T) {
	decl := ast.StepDecl{ID: "loop"}
	metadata := metadataFromDecl(decl, KindLoop)
	if metadata.Idempotent {
		t.Fatal("idempotent = true, want false")
	}
}
