package repo2skill

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type resolvedRepo struct {
	Path    string
	Source  string
	TempDir string
	Name    string
}

func resolveRepo(input string, opts Options) (*resolvedRepo, func(), error) {
	if strings.TrimSpace(input) == "" {
		return nil, nil, fmt.Errorf("repository path or URL required")
	}
	if st, err := os.Stat(input); err == nil && st.IsDir() {
		abs, _ := filepath.Abs(input)
		return &resolvedRepo{Path: abs, Source: input, Name: filepath.Base(abs)}, func() {}, nil
	}
	if isGitHubShorthand(input) {
		input = "https://" + strings.TrimPrefix(input, "https://")
	}
	if isURL(input) {
		tmp, err := os.MkdirTemp("", "repo2skill-*")
		if err != nil {
			return nil, nil, err
		}
		cleanup := func() {
			if !opts.KeepTemp {
				os.RemoveAll(tmp)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeoutOr(opts.Timeout, 2*time.Minute))
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", input, tmp)
		if out, err := cmd.CombinedOutput(); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("git clone failed: %w - %s", err, strings.TrimSpace(string(out)))
		}
		return &resolvedRepo{Path: tmp, Source: input, TempDir: tmp, Name: repoNameFromURL(input)}, cleanup, nil
	}
	return nil, nil, fmt.Errorf("%q is not a directory or cloneable URL", input)
}

func timeoutOr(v, def time.Duration) time.Duration {
	if v == 0 {
		return def
	}
	return v
}
func isURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}
func isGitHubShorthand(s string) bool {
	return regexp.MustCompile(`^(github\.com/|[^/\s]+/[^/\s]+$)`).MatchString(s) && !strings.HasPrefix(s, ".")
}
func repoNameFromURL(s string) string {
	u, _ := url.Parse(s)
	b := filepath.Base(strings.TrimSuffix(u.Path, ".git"))
	if b == "." || b == "/" || b == "" {
		return "repo"
	}
	return b
}
