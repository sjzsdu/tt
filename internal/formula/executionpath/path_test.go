package executionpath

import (
	"reflect"
	"testing"
)

func TestParseLegacyNestedLoopAddress(t *testing.T) {
	path := Parse("outer.iter3.inner.iter2.summarize")
	if got := path.DefinitionStepID(); got != "summarize" {
		t.Fatalf("definition = %q", got)
	}
	if got := path.ParentLoopID(); got != "outer" {
		t.Fatalf("parent loop = %q", got)
	}
	if got := path.IterationPath(); !reflect.DeepEqual(got, []int{3, 2}) {
		t.Fatalf("iterations = %v", got)
	}
	if got := path.String(); got != "outer.iter3.inner.iter2.summarize" {
		t.Fatalf("round trip = %q", got)
	}
}

func TestBuildFormulaAndLoopPath(t *testing.T) {
	path := RootStep("implementation").Formula("coding").ChildStep("review").Iteration(2).ChildStep("fix")
	if got := path.FormulaPath(); !reflect.DeepEqual(got, []string{"coding"}) {
		t.Fatalf("formula path = %v", got)
	}
	if got := path.IterationPath(); !reflect.DeepEqual(got, []int{2}) {
		t.Fatalf("iteration path = %v", got)
	}
	if got := path.DefinitionStepID(); got != "fix" {
		t.Fatalf("definition = %q", got)
	}
	roundTrip := Parse(path.String())
	if !reflect.DeepEqual(roundTrip.Segments, path.Segments) {
		t.Fatalf("round trip = %+v, want %+v", roundTrip.Segments, path.Segments)
	}
}

func TestPathStringCanonicalizesEmptyAndDanglingSegments(t *testing.T) {
	path := Path{Segments: []Segment{
		{Kind: SegmentFormula, ID: "orphan"},
		{Kind: SegmentStep, ID: " root "},
		{Kind: SegmentStep},
		{Kind: SegmentFormula},
		{Kind: SegmentStep, ID: "child"},
		{Kind: SegmentIteration, Index: 2},
	}}
	if got := path.String(); got != "root.child" {
		t.Fatalf("canonical path = %q", got)
	}
	if got := Parse(path.String()).String(); got != path.String() {
		t.Fatalf("parse/string is not stable: %q", got)
	}
}
