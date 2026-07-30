package beads

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Fake bd binary helpers
// ---------------------------------------------------------------------------

// writeFakeBd creates a shell script at dir/bd that prints stdout and exits
// with the given code. It returns the path to the script.
func writeFakeBd(t *testing.T, dir, stdout string, exitCode int) string {
	t.Helper()
	path := filepath.Join(dir, "bd")
	var body string
	if runtime.GOOS == "windows" {
		t.Skip("fake bd scripts require unix shell")
	}
	body = "#!/bin/sh\n"
	if stdout != "" {
		body += "printf '%s' '" + escapeShell(stdout) + "'\n"
	}
	if exitCode != 0 {
		body += "exit " + strings.Repeat("1", exitCode) + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("writing fake bd: %v", err)
	}
	return path
}

// writeFakeBdFunc creates a shell script that dispatches on the first argument
// (the bd subcommand) and prints different JSON per subcommand.
func writeFakeBdFunc(t *testing.T, dir string, handler func(subcmd string) (string, int)) string {
	t.Helper()
	path := filepath.Join(dir, "bd")
	if runtime.GOOS == "windows" {
		t.Skip("fake bd scripts require unix shell")
	}
	// We write a Go test helper binary instead to avoid shell escaping issues.
	// But for simplicity, we use a case-based shell script.
	body := "#!/bin/sh\n"
	body += "case \"$1\" in\n"
	body += "  list)\n"
	out, code := handler("list")
	body += "    printf '%s' '" + escapeShell(out) + "'\n"
	body += "    exit " + itoa(code) + "\n"
	body += "    ;;\n"
	body += "  show)\n"
	out, code = handler("show")
	body += "    printf '%s' '" + escapeShell(out) + "'\n"
	body += "    exit " + itoa(code) + "\n"
	body += "    ;;\n"
	body += "  search)\n"
	out, code = handler("search")
	body += "    printf '%s' '" + escapeShell(out) + "'\n"
	body += "    exit " + itoa(code) + "\n"
	body += "    ;;\n"
	body += "  graph)\n"
	out, code = handler("graph")
	body += "    printf '%s' '" + escapeShell(out) + "'\n"
	body += "    exit " + itoa(code) + "\n"
	body += "    ;;\n"
	body += "  create)\n"
	out, code = handler("create")
	body += "    printf '%s' '" + escapeShell(out) + "'\n"
	body += "    exit " + itoa(code) + "\n"
	body += "    ;;\n"
	body += "  update)\n"
	out, code = handler("update")
	body += "    printf '%s' '" + escapeShell(out) + "'\n"
	body += "    exit " + itoa(code) + "\n"
	body += "    ;;\n"
	body += "  *)\n"
	body += "    echo \"unknown command: $1\" >&2\n"
	body += "    exit 1\n"
	body += "    ;;\n"
	body += "esac\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("writing fake bd: %v", err)
	}
	return path
}

func escapeShell(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	return strings.Repeat("1", n)
}

// newTestAdapter creates an Adapter with a fake bd binary and a temp workspace.
func newTestAdapter(t *testing.T, bin string) *Adapter {
	t.Helper()
	workspace := t.TempDir()
	// Create .beads/ directory so workspace validation passes.
	if err := os.MkdirAll(filepath.Join(workspace, ".beads"), 0o755); err != nil {
		t.Fatalf("creating workspace: %v", err)
	}
	runner := &Runner{Bin: bin, Workspace: workspace, Timeout: 5 * time.Second}
	return NewAdapter(runner)
}

// ---------------------------------------------------------------------------
// Model tests
// ---------------------------------------------------------------------------

func TestIsValidStatus(t *testing.T) {
	tests := []struct {
		status IssueStatus
		want   bool
	}{
		{StatusOpen, true},
		{StatusInProgress, true},
		{StatusBlocked, true},
		{StatusClosed, true},
		{StatusDeferred, true},
		{IssueStatus("unknown"), false},
		{IssueStatus(""), false},
	}
	for _, tt := range tests {
		if got := IsValidStatus(tt.status); got != tt.want {
			t.Errorf("IsValidStatus(%q) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestIsValidDependencyType(t *testing.T) {
	tests := []struct {
		depType DependencyType
		want    bool
	}{
		{DepBlocks, true},
		{DepParentChild, true},
		{DepRelated, true},
		{DependencyType("unknown"), false},
		{DependencyType(""), false},
	}
	for _, tt := range tests {
		if got := IsValidDependencyType(tt.depType); got != tt.want {
			t.Errorf("IsValidDependencyType(%q) = %v, want %v", tt.depType, got, tt.want)
		}
	}
}

func TestIsValidIssueID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"tt-abc", true},
		{"tt-mgxy.1", true},
		{"abc-123", true},
		{"", false},
		{"tt-abc; rm -rf /", false},
		{"tt-abc | cat", false},
		{"tt-abc$(whoami)", false},
	}
	for _, tt := range tests {
		if got := isValidIssueID(tt.id); got != tt.want {
			t.Errorf("isValidIssueID(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Runner tests
// ---------------------------------------------------------------------------

func TestNewRunner_BdNotFound(t *testing.T) {
	// Override PATH to ensure bd is not found.
	t.Setenv("PATH", t.TempDir())
	_, err := NewRunner("")
	if err == nil {
		t.Fatal("expected error when bd is not on PATH")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestNewRunner_WorkspaceNotFound(t *testing.T) {
	// bd exists on PATH.
	dir := t.TempDir()
	writeFakeBd(t, dir, "[]", 0)
	t.Setenv("PATH", dir)

	// Use a workspace that has no .beads/.
	emptyDir := t.TempDir()
	_, err := NewRunner(emptyDir)
	if err == nil {
		t.Fatal("expected error for non-beads workspace")
	}
	if !errors.Is(err, ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got: %v", err)
	}
}

func TestRunner_ExitError(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeBd(t, dir, "", 1)
	a := newTestAdapter(t, bin)

	_, err := a.List(context.Background())
	if err == nil {
		t.Fatal("expected error from non-zero exit")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got: %T: %v", err, err)
	}
	if exitErr.ExitCode == 0 {
		t.Error("expected non-zero exit code")
	}
}

func TestRunner_Timeout(t *testing.T) {
	dir := t.TempDir()
	// Write a script that sleeps for 10 seconds.
	path := filepath.Join(dir, "bd")
	body := "#!/bin/sh\nsleep 10\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("writing slow bd: %v", err)
	}
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".beads"), 0o755); err != nil {
		t.Fatalf("creating workspace: %v", err)
	}
	runner := &Runner{Bin: path, Workspace: workspace, Timeout: 100 * time.Millisecond}
	a := NewAdapter(runner)

	ctx := context.Background()
	_, err := a.List(ctx)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("expected ErrTimeout, got: %v", err)
	}
}

func TestRunner_Cancellation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bd")
	body := "#!/bin/sh\nsleep 10\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("writing slow bd: %v", err)
	}
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".beads"), 0o755); err != nil {
		t.Fatalf("creating workspace: %v", err)
	}
	runner := &Runner{Bin: path, Workspace: workspace, Timeout: 30 * time.Second}
	a := NewAdapter(runner)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := a.List(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// ---------------------------------------------------------------------------
// Adapter read operation tests
// ---------------------------------------------------------------------------

func TestAdapter_List_Success(t *testing.T) {
	issues := []Issue{
		{ID: "tt-abc", Title: "First issue", Status: "open", Priority: 1},
		{ID: "tt-def", Title: "Second issue", Status: "in_progress", Priority: 2},
	}
	data, _ := json.Marshal(issues)
	dir := t.TempDir()
	bin := writeFakeBd(t, dir, string(data), 0)
	a := newTestAdapter(t, bin)

	got, err := a.List(context.Background())
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List() returned %d issues, want 2", len(got))
	}
	if got[0].ID != "tt-abc" {
		t.Errorf("got[0].ID = %q, want %q", got[0].ID, "tt-abc")
	}
	if got[1].ID != "tt-def" {
		t.Errorf("got[1].ID = %q, want %q", got[1].ID, "tt-def")
	}
}

func TestAdapter_List_Empty(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeBd(t, dir, "[]", 0)
	a := newTestAdapter(t, bin)

	got, err := a.List(context.Background())
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List() returned %d issues, want 0", len(got))
	}
}

func TestAdapter_List_EmptyOutput(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeBd(t, dir, "", 0)
	a := newTestAdapter(t, bin)

	got, err := a.List(context.Background())
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if got != nil {
		t.Errorf("List() returned %v, want nil for empty output", got)
	}
}

func TestAdapter_Show_Success(t *testing.T) {
	issue := Issue{
		ID:          "tt-abc",
		Title:       "Test issue",
		Description: "A test description",
		Status:      "open",
		Priority:    1,
		Labels:      []string{"backend", "test"},
		Dependencies: []Dependency{
			{IssueID: "tt-abc", DependsOnID: "tt-xyz", Type: "blocks"},
		},
	}
	data, _ := json.Marshal([]Issue{issue})
	dir := t.TempDir()
	bin := writeFakeBd(t, dir, string(data), 0)
	a := newTestAdapter(t, bin)

	got, err := a.Show(context.Background(), "tt-abc")
	if err != nil {
		t.Fatalf("Show() error: %v", err)
	}
	if got.ID != "tt-abc" {
		t.Errorf("Show().ID = %q, want %q", got.ID, "tt-abc")
	}
	if got.Title != "Test issue" {
		t.Errorf("Show().Title = %q, want %q", got.Title, "Test issue")
	}
	if len(got.Labels) != 2 {
		t.Errorf("Show().Labels = %v, want 2 labels", got.Labels)
	}
	if len(got.Dependencies) != 1 {
		t.Errorf("Show().Dependencies = %v, want 1 dependency", got.Dependencies)
	}
}

func TestAdapter_Show_NotFound(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeBd(t, dir, "[]", 0)
	a := newTestAdapter(t, bin)

	_, err := a.Show(context.Background(), "tt-abc")
	if err == nil {
		t.Fatal("expected error for missing issue")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestAdapter_Show_InvalidID(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeBd(t, dir, "[]", 0)
	a := newTestAdapter(t, bin)

	_, err := a.Show(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty issue ID")
	}
	_, err = a.Show(context.Background(), "tt-abc; rm -rf /")
	if err == nil {
		t.Fatal("expected error for shell-injection issue ID")
	}
}

func TestAdapter_Search_Success(t *testing.T) {
	issues := []Issue{
		{ID: "tt-abc", Title: "Beads dashboard", Status: "open"},
	}
	data, _ := json.Marshal(issues)
	dir := t.TempDir()
	bin := writeFakeBd(t, dir, string(data), 0)
	a := newTestAdapter(t, bin)

	got, err := a.Search(context.Background(), "dashboard")
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Search() returned %d issues, want 1", len(got))
	}
}

func TestAdapter_Search_EmptyQuery(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeBd(t, dir, "[]", 0)
	a := newTestAdapter(t, bin)

	got, err := a.Search(context.Background(), "")
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if got != nil {
		t.Errorf("Search() with empty query should return nil, got: %v", got)
	}
}

func TestAdapter_Graph_Success(t *testing.T) {
	result := GraphResult{
		Issues: []Issue{
			{ID: "tt-abc", Title: "Root", Status: "open"},
			{ID: "tt-def", Title: "Child", Status: "open"},
		},
		Dependencies: []Dependency{
			{IssueID: "tt-def", DependsOnID: "tt-abc", Type: "parent-child"},
		},
	}
	data, _ := json.Marshal(result)
	dir := t.TempDir()
	bin := writeFakeBd(t, dir, string(data), 0)
	a := newTestAdapter(t, bin)

	got, err := a.Graph(context.Background(), "tt-abc")
	if err != nil {
		t.Fatalf("Graph() error: %v", err)
	}
	if len(got.Issues) != 2 {
		t.Errorf("Graph() returned %d issues, want 2", len(got.Issues))
	}
	if len(got.Dependencies) != 1 {
		t.Errorf("Graph() returned %d dependencies, want 1", len(got.Dependencies))
	}
}

// ---------------------------------------------------------------------------
// Adapter mutation tests
// ---------------------------------------------------------------------------

func TestAdapter_CreateIssue_Success(t *testing.T) {
	created := Issue{ID: "tt-new", Title: "New issue", Status: "open"}
	data, _ := json.Marshal([]Issue{created})
	dir := t.TempDir()
	bin := writeFakeBd(t, dir, string(data), 0)
	a := newTestAdapter(t, bin)

	got, err := a.CreateIssue(context.Background(), CreateIssueInput{
		Title:       "New issue",
		Description: "A new issue",
		Priority:    1,
		Labels:      []string{"test"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error: %v", err)
	}
	if got.ID != "tt-new" {
		t.Errorf("CreateIssue().ID = %q, want %q", got.ID, "tt-new")
	}
}

func TestAdapter_CreateIssue_EmptyTitle(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeBd(t, dir, "[]", 0)
	a := newTestAdapter(t, bin)

	_, err := a.CreateIssue(context.Background(), CreateIssueInput{})
	if err == nil {
		t.Fatal("expected error for empty title")
	}
	if !strings.Contains(err.Error(), "title is required") {
		t.Errorf("expected 'title is required' error, got: %v", err)
	}
}

func TestAdapter_UpdateIssue_Success(t *testing.T) {
	updated := Issue{ID: "tt-abc", Title: "Updated title", Status: "in_progress"}
	data, _ := json.Marshal([]Issue{updated})
	dir := t.TempDir()
	bin := writeFakeBd(t, dir, string(data), 0)
	a := newTestAdapter(t, bin)

	newTitle := "Updated title"
	newStatus := StatusInProgress
	got, err := a.UpdateIssue(context.Background(), "tt-abc", UpdateIssueInput{
		Title:  &newTitle,
		Status: &newStatus,
	})
	if err != nil {
		t.Fatalf("UpdateIssue() error: %v", err)
	}
	if got.Title != "Updated title" {
		t.Errorf("UpdateIssue().Title = %q, want %q", got.Title, "Updated title")
	}
	if got.Status != "in_progress" {
		t.Errorf("UpdateIssue().Status = %q, want %q", got.Status, "in_progress")
	}
}

func TestAdapter_UpdateIssue_InvalidStatus(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeBd(t, dir, "[]", 0)
	a := newTestAdapter(t, bin)

	badStatus := IssueStatus("invalid")
	_, err := a.UpdateIssue(context.Background(), "tt-abc", UpdateIssueInput{
		Status: &badStatus,
	})
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
	if !strings.Contains(err.Error(), "invalid status") {
		t.Errorf("expected 'invalid status' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Schema tolerance tests
// ---------------------------------------------------------------------------

func TestAdapter_SchemaTolerance_ExtraFields(t *testing.T) {
	// JSON with extra fields that are not in our DTO should be ignored.
	raw := `[{"id":"tt-abc","title":"Test","status":"open","unknown_field":"value","nested_unknown":{"a":1}}]`
	dir := t.TempDir()
	bin := writeFakeBd(t, dir, raw, 0)
	a := newTestAdapter(t, bin)

	got, err := a.List(context.Background())
	if err != nil {
		t.Fatalf("List() with extra fields error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("List() returned %d issues, want 1", len(got))
	}
	if got[0].ID != "tt-abc" {
		t.Errorf("got[0].ID = %q, want %q", got[0].ID, "tt-abc")
	}
}

func TestAdapter_DecodeError(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeBd(t, dir, "not json at all", 0)
	a := newTestAdapter(t, bin)

	_, err := a.List(context.Background())
	if err == nil {
		t.Fatal("expected decode error for invalid JSON")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Errorf("expected DecodeError, got: %T: %v", err, err)
	}
}

// ---------------------------------------------------------------------------
// Workspace discovery tests
// ---------------------------------------------------------------------------

func TestFindWorkspace(t *testing.T) {
	// Create a nested directory structure with .beads/ at the root.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := findWorkspace(child)
	if err != nil {
		t.Fatalf("findWorkspace() error: %v", err)
	}
	if got != root {
		t.Errorf("findWorkspace() = %q, want %q", got, root)
	}
}

func TestFindWorkspace_NotFound(t *testing.T) {
	root := t.TempDir()
	_, err := findWorkspace(root)
	if err == nil {
		t.Fatal("expected error when no .beads/ found")
	}
	if !errors.Is(err, ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got: %v", err)
	}
}
