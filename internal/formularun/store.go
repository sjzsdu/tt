package formularun

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/sjzsdu/tt/internal/formula"
)

const (
	StatusRunning     = "running"
	StatusCompleted   = "completed"
	StatusFailed      = "failed"
	StatusInterrupted = "interrupted"
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
	WorkspaceDir string            `json:"workspace_dir,omitempty"`
	StatePath    string            `json:"state_path,omitempty"`
	RecipePath   string            `json:"recipe_path,omitempty"`
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
	if root == "" {
		root = DefaultRoot(workspace)
	}
	if recipe == nil {
		return nil, fmt.Errorf("recipe is required")
	}
	id := NewID(recipe.Name, time.Now())
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(filepath.Join(dir, "steps"), 0o755); err != nil {
		return nil, err
	}
	meta := Metadata{
		RunID:        id,
		Formula:      recipe.Name,
		Description:  recipe.Description,
		Status:       StatusRunning,
		StartedAt:    time.Now().Format(time.RFC3339),
		Vars:         cloneMap(vars),
		Agent:        agent,
		Model:        model,
		Session:      session,
		WorkspaceDir: workspace,
		StatePath:    "state.json",
		RecipePath:   "recipe.json",
	}
	store := &Store{Root: root, Dir: dir, Meta: meta}
	if err := store.SaveRecipe(recipe); err != nil {
		return nil, err
	}
	if err := store.SaveMetadata(); err != nil {
		return nil, err
	}
	return store, nil
}

func NewID(name string, t time.Time) string {
	return fmt.Sprintf("%s-%s-%s", t.UTC().Format("20060102-150405"), slug(name), randHex(3))
}

func (s *Store) SaveMetadata() error { return writeJSON(filepath.Join(s.Dir, "run.json"), s.Meta) }
func (s *Store) SaveRecipe(recipe *formula.Recipe) error {
	return writeJSON(filepath.Join(s.Dir, "recipe.json"), recipe)
}
func (s *Store) SaveState(state any) error {
	return writeJSON(filepath.Join(s.Dir, "state.json"), state)
}

func (s *Store) Finish(status, errMsg string) error {
	s.Meta.Status = status
	s.Meta.Error = strings.TrimSpace(errMsg)
	s.Meta.FinishedAt = time.Now().Format(time.RFC3339)
	return s.SaveMetadata()
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

func (s *Store) stepPath(stepID, suffix string) string {
	return filepath.Join(s.Dir, "steps", safeStepID(stepID)+"."+suffix)
}

func List(root string) ([]Record, error) {
	if root == "" {
		root = DefaultRoot("")
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		meta, err := LoadMetadata(dir)
		if err != nil {
			continue
		}
		records = append(records, Record{ID: e.Name(), Dir: dir, Metadata: meta})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Metadata.StartedAt > records[j].Metadata.StartedAt })
	return records, nil
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
		if r.ID == id || strings.HasPrefix(r.ID, id) {
			return r, nil
		}
	}
	return Record{}, fmt.Errorf("formula run %q not found", id)
}

func LoadMetadata(dir string) (Metadata, error) {
	var meta Metadata
	if err := readJSON(filepath.Join(dir, "run.json"), &meta); err != nil {
		return meta, err
	}
	return meta, nil
}

func LoadState(dir string, out any) error { return readJSON(filepath.Join(dir, "state.json"), out) }

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
