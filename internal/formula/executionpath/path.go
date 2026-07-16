package executionpath

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type SegmentKind string

const (
	SegmentStep      SegmentKind = "step"
	SegmentFormula   SegmentKind = "formula"
	SegmentIteration SegmentKind = "iteration"
)

// Segment is one typed component of a concrete runtime execution address.
// Formula segments are reserved for nested FormulaCall runs; legacy loop
// addresses contain step and iteration segments only.
type Segment struct {
	Kind  SegmentKind `json:"kind"`
	ID    string      `json:"id,omitempty"`
	Index int         `json:"index,omitempty"`
}

// Path is the structured identity of one concrete runtime step instance.
type Path struct {
	Segments []Segment `json:"segments,omitempty"`
}

var structuredBoundaryPattern = regexp.MustCompile(`\.(?:iter(\d+)|formula\(([^)]*)\))\.`)

func Parse(address string) Path {
	address = strings.TrimSpace(address)
	if address == "" {
		return Path{}
	}
	matches := structuredBoundaryPattern.FindAllStringSubmatchIndex(address, -1)
	if len(matches) == 0 {
		return Path{Segments: []Segment{{Kind: SegmentStep, ID: address}}}
	}
	segments := make([]Segment, 0, len(matches)*2+1)
	start := 0
	for _, match := range matches {
		if id := strings.TrimSpace(address[start:match[0]]); id != "" {
			segments = append(segments, Segment{Kind: SegmentStep, ID: id})
		}
		if match[2] >= 0 {
			index, _ := strconv.Atoi(address[match[2]:match[3]])
			segments = append(segments, Segment{Kind: SegmentIteration, Index: index})
		} else if match[4] >= 0 {
			segments = append(segments, Segment{Kind: SegmentFormula, ID: address[match[4]:match[5]]})
		}
		start = match[1]
	}
	if id := strings.TrimSpace(address[start:]); id != "" {
		segments = append(segments, Segment{Kind: SegmentStep, ID: id})
	}
	return Path{Segments: segments}
}

func RootStep(id string) Path {
	return Path{Segments: []Segment{{Kind: SegmentStep, ID: strings.TrimSpace(id)}}}
}

func (p Path) ChildStep(id string) Path {
	return p.append(Segment{Kind: SegmentStep, ID: strings.TrimSpace(id)})
}

func (p Path) Formula(name string) Path {
	return p.append(Segment{Kind: SegmentFormula, ID: strings.TrimSpace(name)})
}

func (p Path) Iteration(index int) Path {
	return p.append(Segment{Kind: SegmentIteration, Index: index})
}

// Append returns a path with all segments from child appended. It is used to
// map a child workflow's local addresses into its FormulaCall prefix.
func (p Path) Append(child Path) Path {
	segments := append([]Segment(nil), p.Segments...)
	segments = append(segments, child.Segments...)
	return Path{Segments: segments}
}

func (p Path) append(segment Segment) Path {
	segments := append([]Segment(nil), p.Segments...)
	segments = append(segments, segment)
	return Path{Segments: segments}
}

func (p Path) DefinitionStepID() string {
	for i := len(p.Segments) - 1; i >= 0; i-- {
		if p.Segments[i].Kind == SegmentStep {
			return p.Segments[i].ID
		}
	}
	return ""
}

func (p Path) ParentLoopID() string {
	for i, segment := range p.Segments {
		if segment.Kind != SegmentIteration {
			continue
		}
		for j := i - 1; j >= 0; j-- {
			if p.Segments[j].Kind == SegmentStep {
				return p.Segments[j].ID
			}
		}
	}
	return ""
}

func (p Path) IterationPath() []int {
	var out []int
	for _, segment := range p.Segments {
		if segment.Kind == SegmentIteration {
			out = append(out, segment.Index)
		}
	}
	return out
}

func (p Path) FormulaPath() []string {
	var out []string
	for _, segment := range p.Segments {
		if segment.Kind == SegmentFormula && segment.ID != "" {
			out = append(out, segment.ID)
		}
	}
	return out
}

// String preserves the existing loop address spelling. Formula segments use
// an explicit marker so a future FormulaCall path remains unambiguous.
func (p Path) String() string {
	segments := canonicalStringSegments(p.Segments)
	var b strings.Builder
	for i, segment := range segments {
		if i > 0 {
			b.WriteByte('.')
		}
		switch segment.Kind {
		case SegmentIteration:
			fmt.Fprintf(&b, "iter%d", segment.Index)
		case SegmentFormula:
			fmt.Fprintf(&b, "formula(%s)", segment.ID)
		case SegmentStep:
			b.WriteString(segment.ID)
		}
	}
	return b.String()
}

// canonicalStringSegments removes malformed empty or dangling boundaries and
// merges adjacent step segments into the legacy dotted step spelling. Valid
// runtime paths already have the shape step.(formula|iteration).step.
func canonicalStringSegments(segments []Segment) []Segment {
	out := make([]Segment, 0, len(segments))
	for i, segment := range segments {
		segment.ID = strings.TrimSpace(segment.ID)
		switch segment.Kind {
		case SegmentStep:
			if segment.ID == "" {
				continue
			}
			if len(out) > 0 && out[len(out)-1].Kind == SegmentStep {
				out[len(out)-1].ID += "." + segment.ID
				continue
			}
			out = append(out, segment)
		case SegmentFormula, SegmentIteration:
			if segment.Kind == SegmentFormula && segment.ID == "" {
				continue
			}
			if len(out) == 0 || out[len(out)-1].Kind != SegmentStep || !nextNonEmptyStep(segments[i+1:]) {
				continue
			}
			out = append(out, segment)
		}
	}
	return out
}

func nextNonEmptyStep(segments []Segment) bool {
	for _, segment := range segments {
		switch segment.Kind {
		case SegmentStep:
			if strings.TrimSpace(segment.ID) != "" {
				return true
			}
		case SegmentFormula:
			if strings.TrimSpace(segment.ID) != "" {
				return false
			}
		case SegmentIteration:
			return false
		}
	}
	return false
}
