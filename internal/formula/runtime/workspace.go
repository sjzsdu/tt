package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sjzsdu/tt/internal/formula/ir"
)

type workspaceSession struct {
	repoRoot     string
	path         string
	cleanup      bool
	created      bool
	sparsePaths  []string
	invocationWD string
}

var workspacePathSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func (e *Executor) SeedRunID(runID string) {
	if e == nil {
		return
	}
	e.runID = strings.TrimSpace(runID)
}

func (e *Executor) SeedFormulaRunDir(dir string) {
	if e == nil {
		return
	}
	e.formulaRunDir = strings.TrimSpace(dir)
}

func (e *Executor) prepareWorkspace(ctx context.Context) (*workspaceSession, error) {
	policy := workspacePolicy(e.Workflow)
	if policy == nil {
		return nil, nil
	}
	if !strings.EqualFold(strings.TrimSpace(policy.Kind), "worktree") {
		return nil, fmt.Errorf("unsupported workspace kind %q", policy.Kind)
	}
	if e.Context == nil {
		e.Context = NewContextStore()
	}
	env, err := e.environmentContext()
	if err != nil {
		return nil, err
	}
	if !env.Git.IsRepo {
		return nil, fmt.Errorf("workspace policy requires a git repository")
	}
	repoRoot := canonicalFilesystemPath(env.Git.Root)
	invocationWD := canonicalFilesystemPath(env.CWD)
	path, err := e.resolveWorkspacePath(invocationWD, policy)
	if err != nil {
		return nil, err
	}
	session := &workspaceSession{
		repoRoot:     repoRoot,
		path:         path,
		cleanup:      policy.Cleanup,
		invocationWD: invocationWD,
		sparsePaths:  autoSparsePaths(repoRoot, invocationWD),
	}
	if err := session.ensure(ctx); err != nil {
		return nil, err
	}
	if err := e.prepareWorkspaceBranch(ctx, session, policy); err != nil {
		return nil, err
	}
	e.SeedWorkspaceEnvironment(path, invocationWD, e.formulaRunDir)
	return session, nil
}

func (e *Executor) environmentContext() (EnvironmentContext, error) {
	if e == nil || e.Context == nil {
		return EnvironmentContext{}, fmt.Errorf("environment context is required")
	}
	value, ok := e.Context.Get(EnvironmentContextKey)
	if !ok {
		return EnvironmentContext{}, fmt.Errorf("environment context is required")
	}
	var env EnvironmentContext
	if err := json.Unmarshal(value.Raw, &env); err != nil {
		return EnvironmentContext{}, fmt.Errorf("decode environment context: %w", err)
	}
	return env, nil
}

func canonicalFilesystemPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

func workspacePolicy(wf *ir.Workflow) *ir.WorkspacePolicy {
	if wf == nil {
		return nil
	}
	if wf.Workspace == nil {
		return nil
	}
	policy := *wf.Workspace
	if policy.Kind == "" {
		policy.Kind = "worktree"
	}
	return &policy
}

func autoSparsePaths(repoRoot, invocationWD string) []string {
	rel, err := filepath.Rel(repoRoot, invocationWD)
	if err != nil {
		return nil
	}
	rel = filepath.Clean(rel)
	if rel == "." {
		return nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	return []string{filepath.ToSlash(rel)}
}

func (e *Executor) resolveWorkspacePath(invocationWD string, policy *ir.WorkspacePolicy) (string, error) {
	if policy == nil {
		return "", nil
	}
	if path := strings.TrimSpace(policy.Path); path != "" {
		if !filepath.IsAbs(path) {
			path = filepath.Join(invocationWD, path)
		}
		return filepath.Clean(path), nil
	}
	baseDir := filepath.Join(invocationWD, ".tt", "worktrees")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", err
	}
	seed := strings.TrimSpace(e.runID)
	if seed == "" {
		return os.MkdirTemp(baseDir, sanitizeWorkspaceComponent(workspacePathPrefix(e.Workflow))+"-")
	}
	seed = sanitizeWorkspaceComponent(seed)
	return filepath.Join(baseDir, seed), nil
}

func workspacePathPrefix(wf *ir.Workflow) string {
	if wf == nil || strings.TrimSpace(wf.Name) == "" {
		return "worktree"
	}
	return wf.Name
}

func sanitizeWorkspaceComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = workspacePathSanitizer.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-._")
	return value
}

func (s *workspaceSession) ensure(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	if !s.isRegisteredWorktree(ctx) {
		if _, err := os.Stat(s.path); err == nil {
			empty, emptyErr := directoryIsEmpty(s.path)
			if emptyErr != nil {
				return emptyErr
			}
			if !empty {
				return fmt.Errorf("workspace path already exists and is not an empty directory or git worktree: %s", s.path)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := s.create(ctx); err != nil {
			return err
		}
		s.created = true
	}
	return s.applySparseCheckout(ctx)
}

func directoryIsEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func (s *workspaceSession) create(ctx context.Context) error {
	args := []string{"git", "-C", s.repoRoot, "worktree", "add", "--detach"}
	if len(s.sparsePaths) > 0 {
		args = append(args, "--no-checkout")
	}
	args = append(args, s.path, "HEAD")
	_, err := runGit(ctx, args...)
	return err
}

func (s *workspaceSession) applySparseCheckout(ctx context.Context) error {
	if s == nil || len(s.sparsePaths) == 0 {
		return nil
	}
	if _, err := runGit(ctx, "git", "-C", s.path, "sparse-checkout", "init", "--cone"); err != nil {
		return err
	}
	args := append([]string{"git", "-C", s.path, "sparse-checkout", "set", "--cone"}, s.sparsePaths...)
	if _, err := runGit(ctx, args...); err != nil {
		return err
	}
	if _, err := runGit(ctx, "git", "-C", s.path, "checkout", "HEAD"); err != nil {
		return err
	}
	return nil
}

func (e *Executor) prepareWorkspaceBranch(ctx context.Context, session *workspaceSession, policy *ir.WorkspacePolicy) error {
	if session == nil || policy == nil || strings.TrimSpace(session.path) == "" {
		return nil
	}
	branch := e.resolveWorkspaceBranch(policy)
	if branch == "" {
		return nil
	}
	branch = workspaceBranchAvoidingLocalRefPathConflict(ctx, session.path, branch)
	base := e.renderWorkspacePolicyText(policy.Base)
	if isUnresolvedTemplate(base) || strings.TrimSpace(base) == "" {
		base = "origin/main"
	}
	if _, err := runGit(ctx, "git", "-C", session.path, "rev-parse", "--verify", base+"^{commit}"); err != nil {
		base = "HEAD"
	}
	current, _ := runGit(ctx, "git", "-C", session.path, "branch", "--show-current")
	if strings.TrimSpace(current) == branch {
		return nil
	}
	if _, err := runGit(ctx, "git", "-C", session.path, "rev-parse", "--verify", branch); err == nil {
		_, err = runGit(ctx, "git", "-C", session.path, "checkout", branch)
		return err
	}
	_, err := runGit(ctx, "git", "-C", session.path, "checkout", "-b", branch, base)
	return err
}

func workspaceBranchAvoidingLocalRefPathConflict(ctx context.Context, repoPath, branch string) string {
	branch = strings.Trim(strings.TrimSpace(branch), "/")
	if branch == "" || !strings.Contains(branch, "/") {
		return branch
	}
	parts := strings.Split(branch, "/")
	for i := 1; i < len(parts); i++ {
		prefix := strings.Join(parts[:i], "/")
		if _, err := runGit(ctx, "git", "-C", repoPath, "show-ref", "--verify", "--quiet", "refs/heads/"+prefix); err == nil {
			return sanitizeGitBranch(strings.ReplaceAll(branch, "/", "-"))
		}
	}
	return branch
}

func (e *Executor) resolveWorkspaceBranch(policy *ir.WorkspacePolicy) string {
	if policy == nil {
		return ""
	}
	branch := e.renderWorkspacePolicyText(policy.Branch)
	if !isUnresolvedTemplate(branch) && strings.TrimSpace(branch) != "" {
		return sanitizeGitBranch(branch)
	}
	seed := strings.TrimSpace(policy.BranchSlugFrom)
	if seed == "" {
		return ""
	}
	value := e.renderWorkspacePolicyText("{{" + seed + "}}")
	if isUnresolvedTemplate(value) || strings.TrimSpace(value) == "" {
		return ""
	}
	prefix := strings.Trim(strings.TrimSpace(policy.BranchPrefix), "/")
	if prefix == "" {
		prefix = "feature"
	}
	return sanitizeGitBranch(prefix + "/" + slugifyWorkspaceBranch(value))
}

func (e *Executor) renderWorkspacePolicyText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || e == nil || e.Context == nil {
		return text
	}
	return workspaceTemplatePattern.ReplaceAllStringFunc(text, func(match string) string {
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(match, "{{"), "}}"))
		if name == "" {
			return ""
		}
		value, ok := e.Context.Get(name)
		if !ok {
			return match
		}
		return strings.TrimSpace(valueForWorkspaceTemplate(value.Raw))
	})
}

var workspaceTemplatePattern = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

func valueForWorkspaceTemplate(raw []byte) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	return strings.TrimSpace(string(raw))
}

func isUnresolvedTemplate(value string) bool {
	value = strings.TrimSpace(value)
	return strings.Contains(value, "{{") || strings.Contains(value, "}}")
}

func slugifyWorkspaceBranch(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = workspacePathSanitizer.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-._/")
	if len(value) > 48 {
		value = strings.Trim(value[:48], "-._/")
	}
	if value == "" {
		return "feature"
	}
	return value
}

func sanitizeGitBranch(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "/")
	value = strings.ReplaceAll(value, " ", "-")
	return value
}

func (s *workspaceSession) isRegisteredWorktree(ctx context.Context) bool {
	if s == nil || s.path == "" {
		return false
	}
	out, err := runGit(ctx, "git", "-C", s.repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "worktree ") {
			path := filepath.Clean(strings.TrimSpace(strings.TrimPrefix(line, "worktree ")))
			if path == s.path {
				return true
			}
		}
	}
	return false
}

func (s *workspaceSession) cleanupWorktree() error {
	if s == nil || !s.cleanup || s.path == "" {
		return nil
	}
	if !s.isRegisteredWorktree(context.Background()) {
		if _, err := os.Stat(s.path); os.IsNotExist(err) {
			return nil
		}
		return nil
	}
	if _, err := runGit(context.Background(), "git", "-C", s.repoRoot, "worktree", "remove", "--force", s.path); err != nil {
		return err
	}
	_, _ = runGit(context.Background(), "git", "-C", s.repoRoot, "worktree", "prune")
	return nil
}

func runGit(ctx context.Context, args ...string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("git command is required")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (e *Executor) finalizeWorkspace(session *workspaceSession) error {
	if session == nil {
		return nil
	}
	return session.cleanupWorktree()
}
