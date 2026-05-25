package formularun

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/sjzsdu/tt/internal/formula"
)

const (
	StatusRunning      = "running"
	StatusCompleted    = "completed"
	StatusFailed       = "failed"
	StatusWaitingInput = "waiting_input"
	StatusInterrupted  = "interrupted"
	StatusStale        = "stale"
)

type Metadata struct {
	RunID        string            `json:"run_id"`
	Formula      string            `json:"formula"`
	Description  string            `json:"description,omitempty"`
	Status       string            `json:"status"`
	Error        string            `json:"error,omitempty"`
	StartedAt    string            `json:"started_at"`
	FinishedAt   string            `json:"finished_at,omitempty"`
	Vars         map[string]string `json:"vars,omitempty"`
	Agent        string            `json:"agent,omitempty"`
	Model        string            `json:"model,omitempty"`
	Session      string            `json:"session,omitempty"`
	PID          int               `json:"pid,omitempty"`
	TTVersion    string            `json:"tt_version,omitempty"`
	GitBranch    string            `json:"git_branch,omitempty"`
	GitCommit    string            `json:"git_commit,omitempty"`
	GitDirty     bool              `json:"git_dirty,omitempty"`
	WorkspaceDir string            `json:"workspace_dir,omitempty"`
	StatePath    string            `json:"state_path,omitempty"`
	RecipePath   string            `json:"recipe_path,omitempty"`
	LogsPath     string            `json:"logs_path,omitempty"`
}

type Event struct {
	Type       string         `json:"type"`
	At         string         `json:"at"`
	RunID      string         `json:"run_id,omitempty"`
	StepID     string         `json:"step_id,omitempty"`
	Agent      string         `json:"agent,omitempty"`
	Model      string         `json:"model,omitempty"`
	Session    string         `json:"session,omitempty"`
	Status     string         `json:"status,omitempty"`
	Error      string         `json:"error,omitempty"`
	DurationMS int64          `json:"duration_ms,omitempty"`
	OutputPath string         `json:"output_path,omitempty"`
	Extra      map[string]any `json:"extra,omitempty"`
}

type Record struct {
	ID       string
	Dir      string
	Metadata Metadata
}

type Store struct {
	Root string
	Dir  string
	Meta Metadata
}

func DefaultRoot(workspace string) string {
	if strings.TrimSpace(workspace) == "" {
		workspace, _ = os.Getwd()
	}
	return filepath.Join(workspace, ".tt", "runs", "formula")
}

func New(root string, recipe *formula.Recipe, vars map[string]string, agent, model, session, workspace string) (*Store, error) {
	return NewWithMetadata(root, recipe, vars, agent, model, session, workspace, "")
}

func NewWithMetadata(root string, recipe *formula.Recipe, vars map[string]string, agent, model, session, workspace, ttVersion string) (*Store, error) {
	if root == "" {
		if err := EnsureWorkspaceState(workspace); err != nil {
			return nil, err
		}
		root = DefaultRoot(workspace)
	}
	if recipe == nil {
		return nil, fmt.Errorf("recipe is required")
	}
	id := NewID(recipe.Name, time.Now())
	formulaSlug := slug(recipe.Name)
	dir := filepath.Join(root, formulaSlug, id)
	if err := os.MkdirAll(filepath.Join(dir, "steps"), 0o755); err != nil {
		return nil, err
	}
	git := collectGitMetadata(workspace)
	meta := Metadata{
		RunID:        filepath.ToSlash(filepath.Join(formulaSlug, id)),
		Formula:      recipe.Name,
		Description:  recipe.Description,
		Status:       StatusRunning,
		StartedAt:    time.Now().Format(time.RFC3339),
		Vars:         cloneMap(vars),
		Agent:        agent,
		Model:        model,
		Session:      session,
		PID:          os.Getpid(),
		TTVersion:    ttVersion,
		GitBranch:    git.Branch,
		GitCommit:    git.Commit,
		GitDirty:     git.Dirty,
		WorkspaceDir: workspace,
		StatePath:    "state.json",
		RecipePath:   "recipe.json",
		LogsPath:     "logs.jsonl",
	}
	store := &Store{Root: root, Dir: dir, Meta: meta}
	if err := store.SaveRecipe(recipe); err != nil {
		return nil, err
	}
	if err := store.SaveMetadata(); err != nil {
		return nil, err
	}
	if err := store.AppendEvent(Event{Type: "run_started", Status: StatusRunning}); err != nil {
		return nil, err
	}
	return store, nil
}

func EnsureWorkspaceState(workspace string) error {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		var err error
		workspace, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".tt"), 0o755); err != nil {
		return err
	}
	return ensureGitIgnoreEntry(filepath.Join(workspace, ".gitignore"), ".tt/")
}

func ensureGitIgnoreEntry(path, entry string) error {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == entry || strings.TrimSpace(line) == strings.TrimSuffix(entry, "/") {
			return nil
		}
	}
	var b strings.Builder
	b.WriteString(content)
	if content != "" && !strings.HasSuffix(content, "\n") {
		b.WriteString("\n")
	}
	if !strings.Contains(content, "# Local AI/config state") {
		b.WriteString("\n# Local AI/config state\n")
	}
	b.WriteString(entry)
	b.WriteString("\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func NewID(_ string, t time.Time) string {
	return fmt.Sprintf("%s-%s", t.UTC().Format("20060102-150405"), randHex(3))
}

func (s *Store) SaveMetadata() error { return writeJSON(filepath.Join(s.Dir, "run.json"), s.Meta) }
func (s *Store) SaveRecipe(recipe *formula.Recipe) error {
	return writeJSON(filepath.Join(s.Dir, "recipe.json"), recipe)
}
func (s *Store) SaveWorkflow(workflow any) error {
	return writeJSON(filepath.Join(s.Dir, "workflow.json"), workflow)
}
func (s *Store) SaveState(state any) error {
	return writeJSON(filepath.Join(s.Dir, "state.json"), state)
}

func (s *Store) Finish(status, errMsg string) error {
	s.Meta.Status = status
	s.Meta.Error = strings.TrimSpace(errMsg)
	s.Meta.FinishedAt = time.Now().Format(time.RFC3339)
	if err := s.SaveMetadata(); err != nil {
		return err
	}
	return s.AppendEvent(Event{Type: "run_finished", Status: status, Error: s.Meta.Error})
}

func (s *Store) MarkWaitingInput(stepID string) error {
	s.Meta.Status = StatusWaitingInput
	s.Meta.Error = ""
	s.Meta.FinishedAt = ""
	if err := s.SaveMetadata(); err != nil {
		return err
	}
	return s.AppendEvent(Event{Type: "human_input_required", StepID: stepID, Status: StatusWaitingInput})
}

func (s *Store) AppendEvent(event Event) error {
	if s == nil {
		return nil
	}
	if strings.TrimSpace(event.Type) == "" {
		return fmt.Errorf("event type is required")
	}
	if event.At == "" {
		event.At = time.Now().Format(time.RFC3339)
	}
	if event.RunID == "" {
		event.RunID = s.Meta.RunID
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(s.Dir, "logs.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

func (s *Store) SaveStepPrompt(stepID, content string) error {
	return writeText(s.stepPath(stepID, "prompt.md"), content)
}
func (s *Store) SaveStepOutput(stepID, content string) error {
	return writeText(s.stepPath(stepID, "output.md"), content)
}
func (s *Store) SaveStepError(stepID, content string) error {
	return writeText(s.stepPath(stepID, "error.txt"), content)
}

func (s *Store) SaveStepHumanInputRequest(stepID string, request any) error {
	return writeJSON(s.stepPath(stepID, "human_input_request.json"), request)
}

func (s *Store) LoadStepHumanInputRequest(stepID string, out any) error {
	return readJSON(s.stepPath(stepID, "human_input_request.json"), out)
}

func (s *Store) SaveStepHumanInputResponse(stepID string, response any) error {
	return writeJSON(s.stepPath(stepID, "human_input_response.json"), response)
}

func StepArtifactPath(runDir, stepID, suffix string) string {
	return filepath.Join(runDir, "steps", safeStepID(stepID)+"."+suffix)
}

func (s *Store) stepPath(stepID, suffix string) string {
	return StepArtifactPath(s.Dir, stepID, suffix)
}

func List(root string) ([]Record, error) {
	if root == "" {
		root = DefaultRoot("")
	}
	dirs, err := listRunDirs(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(dirs))
	for _, dir := range dirs {
		meta, err := LoadMetadata(dir)
		if err != nil {
			continue
		}
		if markStaleIfNeeded(dir, &meta) {
			_ = writeJSON(filepath.Join(dir, "run.json"), meta)
		}
		id := runIDFromDir(root, dir)
		if strings.TrimSpace(meta.RunID) != "" {
			id = meta.RunID
		}
		records = append(records, Record{ID: id, Dir: dir, Metadata: meta})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Metadata.StartedAt > records[j].Metadata.StartedAt })
	return records, nil
}

func listRunDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "run.json")); err == nil {
			dirs = append(dirs, dir)
			continue
		}
		children, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, child := range children {
			if !child.IsDir() {
				continue
			}
			childDir := filepath.Join(dir, child.Name())
			if _, err := os.Stat(filepath.Join(childDir, "run.json")); err == nil {
				dirs = append(dirs, childDir)
			}
		}
	}
	return dirs, nil
}

func runIDFromDir(root, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." {
		return filepath.Base(dir)
	}
	return filepath.ToSlash(rel)
}

func Resolve(root, id string) (Record, error) {
	records, err := List(root)
	if err != nil {
		return Record{}, err
	}
	if id == "latest" || id == "" {
		if len(records) == 0 {
			return Record{}, fmt.Errorf("no formula runs found")
		}
		return records[0], nil
	}
	for _, r := range records {
		leafID := filepath.Base(filepath.FromSlash(r.ID))
		if r.ID == id || leafID == id || strings.HasPrefix(r.ID, id) || strings.HasPrefix(leafID, id) {
			return r, nil
		}
	}
	return Record{}, fmt.Errorf("formula run %q not found", id)
}

func Delete(root, id string) (Record, error) {
	record, err := Resolve(root, id)
	if err != nil {
		return Record{}, err
	}
	if err := os.RemoveAll(record.Dir); err != nil {
		return Record{}, err
	}
	return record, nil
}

func LoadMetadata(dir string) (Metadata, error) {
	var meta Metadata
	if err := readJSON(filepath.Join(dir, "run.json"), &meta); err != nil {
		return meta, err
	}
	if markStaleIfNeeded(dir, &meta) {
		_ = writeJSON(filepath.Join(dir, "run.json"), meta)
	}
	return meta, nil
}

func LoadState(dir string, out any) error { return readJSON(filepath.Join(dir, "state.json"), out) }

func LoadRecipe(dir string) (*formula.Recipe, error) {
	var recipe formula.Recipe
	if err := readJSON(filepath.Join(dir, "recipe.json"), &recipe); err != nil {
		return nil, err
	}
	return &recipe, nil
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
func writeText(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

type gitMetadata struct {
	Branch string
	Commit string
	Dirty  bool
}

func collectGitMetadata(workspace string) gitMetadata {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	branch := gitOutput(workspace, "rev-parse", "--abbrev-ref", "HEAD")
	commit := gitOutput(workspace, "rev-parse", "--short", "HEAD")
	dirty := strings.TrimSpace(gitOutput(workspace, "status", "--porcelain")) != ""
	return gitMetadata{Branch: branch, Commit: commit, Dirty: dirty}
}

func gitOutput(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func markStaleIfNeeded(dir string, meta *Metadata) bool {
	if meta == nil || meta.Status != StatusRunning || meta.FinishedAt != "" {
		return false
	}
	if meta.PID > 0 && processExists(meta.PID) {
		return false
	}
	meta.Status = StatusStale
	meta.Error = "run process is no longer active"
	meta.FinishedAt = time.Now().Format(time.RFC3339)
	_ = appendEventToDir(dir, Event{Type: "run_stale", RunID: meta.RunID, Status: StatusStale, Error: meta.Error})
	return true
}

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil || process == nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func appendEventToDir(dir string, event Event) error {
	if event.Type == "" {
		return nil
	}
	if event.At == "" {
		event.At = time.Now().Format(time.RFC3339)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(dir, "logs.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}
func cloneMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

var nonSlug = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func slug(s string) string {
	s = strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "-"), "-._")
	if s == "" {
		return "formula"
	}
	if len(s) > 48 {
		return s[:48]
	}
	return s
}
func safeStepID(s string) string { return slug(strings.ReplaceAll(s, string(filepath.Separator), "-")) }
func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	return hex.EncodeToString(b)
}
