// Package docsvisual embeds the vendored diagram-design skill so that the tt
// binary can provision it into the picoclaw skills home at runtime without
// depending on a source checkout. The skill data is staged by
// scripts/fetch-diagram-design.sh (version + sha256 pinned) and is gitignored;
// run "make skill-fetch" (or the script) before building.
package docsvisual

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed all:skilldata
var skillFS embed.FS

// SkillName is the picoclaw skill name expected by the docs-visual-writer agent.
const SkillName = "diagram-design"

// StagedError reports that the pinned skill data has not been fetched yet.
type StagedError struct{ Dir string }

func (e *StagedError) Error() string {
	return fmt.Sprintf("diagram-design skill data not staged at %s; run: bash scripts/fetch-diagram-design.sh", e.Dir)
}

// FS returns the embedded skill subtree rooted at the skill directory.
func FS() (fs.FS, error) {
	sub, err := fs.Sub(skillFS, filepath.Join("skilldata", SkillName))
	if err != nil {
		return nil, fmt.Errorf("resolve embedded diagram-design skill: %w", err)
	}
	if _, statErr := fs.Stat(sub, "SKILL.md"); statErr != nil {
		return nil, &StagedError{Dir: "internal/docsvisual/skilldata/diagram-design"}
	}
	return sub, nil
}

// EnsureProvisioned extracts the embedded skill into <home>/skills/diagram-design
// so the picoclaw skills loader can discover it. It is idempotent: an existing
// directory with SKILL.md is left untouched.
func EnsureProvisioned(home string) error {
	dest := filepath.Join(home, "skills", SkillName)
	if info, err := os.Stat(filepath.Join(dest, "SKILL.md")); err == nil && !info.IsDir() {
		return nil
	}
	src, err := FS()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("create skill destination: %w", err)
	}
	return copyFS(src, dest)
}

func copyFS(src fs.FS, dest string) error {
	return fs.WalkDir(src, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.FromSlash(path))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(src, path)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", path, err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		return nil
	})
}
