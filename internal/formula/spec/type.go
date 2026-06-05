package spec

// Type categorizes a formula's structural role.
type Type string

// Formula types. A formula may declare exactly one of these in its `type`
// field. An empty `type` is treated as a workflow for backwards compat.
const (
	TypeWorkflow  Type = "workflow"
	TypeExpansion Type = "expansion"
	TypeAspect    Type = "aspect"
	TypeAtomic    Type = "atomic"
)

// IsValid reports whether t is a recognized formula type.
func (t Type) IsValid() bool {
	switch t {
	case TypeWorkflow, TypeExpansion, TypeAspect, TypeAtomic:
		return true
	}
	return false
}
