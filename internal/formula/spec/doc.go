// Package spec defines the data contract for formula documents.
//
// spec is a leaf package: it contains the pure data structures, parsing
// helpers, and structural validators that describe a formula file. It does
// not import anything from the parent `formula` package or from any
// runtime/builder sub-package, so it can be consumed by the parent
// `formula` package (parser, expander, compiler), by sibling UI/store
// sub-packages, and by external callers (cmd, tests) without creating
// import cycles.
//
// The split between spec and the rest of the `internal/formula` tree is:
//
//   - spec          — data contract (this package)
//   - formula       — parser, expander, compiler, IR builders
//   - formula/spec  — this package
//   - formula/doc   — markdown/mermaid rendering
//   - formula/ui    — dashboard snapshot, graph, human-input state
//   - formula/run   — run state store
//   - formula/runview — run snapshot mutations
//
// spec deliberately contains no behaviour beyond structural validation and
// `{{variable}}` substitution. Higher-level behaviour lives in the parent
// `formula` package or in the runtime sub-packages.
package spec
