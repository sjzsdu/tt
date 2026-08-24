package coder

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type ProjectRecord struct {
	ID        string
	Dir       string
	Project   Project
	UpdatedAt string
}

type Store struct {
	Root    string
	Dir     string
	Project Project
}

func DefaultRoot(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	return filepath.Join(workspace, ".tt", "coder", "projects")
}

func NewProject(id, name, vision string) Project {
	now := nowString()
	id = slug(firstNonEmpty(id, name, "project"))
	return Project{
		SchemaVersion: CurrentSchemaVersion,
		ID:            id,
		Name:          strings.TrimSpace(name),
		Vision:        strings.TrimSpace(vision),
		Status:        ProjectStatusExploring,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func CreateStore(root string, project Project) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		root = DefaultRoot("")
	}
	project = normalizeProject(project)
	if strings.TrimSpace(project.ID) == "" {
		return nil, fmt.Errorf("coder project id is required")
	}
	dir := filepath.Join(root, slug(project.ID))
	if err := os.MkdirAll(filepath.Join(dir, "context"), 0o755); err != nil {
		return nil, fmt.Errorf("create coder project store: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "gates"), 0o755); err != nil {
		return nil, fmt.Errorf("create coder gate store: %w", err)
	}
	store := &Store{Root: root, Dir: dir, Project: project}
	if err := store.SaveProject(project); err != nil {
		return nil, err
	}
	return store, nil
}

func OpenStore(root, projectID string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		root = DefaultRoot("")
	}
	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("coder project id is required")
	}
	dir := filepath.Join(root, slug(projectID))
	var project Project
	if err := readJSON(filepath.Join(dir, "project.json"), &project); err != nil {
		return nil, err
	}
	return &Store{Root: root, Dir: dir, Project: project}, nil
}

func ListProjects(root string) ([]ProjectRecord, error) {
	if strings.TrimSpace(root) == "" {
		root = DefaultRoot("")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	records := make([]ProjectRecord, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		var project Project
		if err := readJSON(filepath.Join(dir, "project.json"), &project); err != nil {
			continue
		}
		records = append(records, ProjectRecord{ID: project.ID, Dir: dir, Project: project, UpdatedAt: project.UpdatedAt})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].UpdatedAt == records[j].UpdatedAt {
			return records[i].ID < records[j].ID
		}
		return records[i].UpdatedAt > records[j].UpdatedAt
	})
	return records, nil
}

func (s *Store) SaveProject(project Project) error {
	if s == nil {
		return fmt.Errorf("coder store is required")
	}
	project = normalizeProject(project)
	if strings.TrimSpace(project.ID) == "" {
		return fmt.Errorf("coder project id is required")
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(s.Dir, "project.json"), project); err != nil {
		return err
	}
	s.Project = project
	return nil
}

func (s *Store) LoadProject() (Project, error) {
	if s == nil {
		return Project{}, fmt.Errorf("coder store is required")
	}
	var project Project
	if err := readJSON(filepath.Join(s.Dir, "project.json"), &project); err != nil {
		return Project{}, err
	}
	return project, nil
}

func (s *Store) SaveContextPacket(packet ContextPacket) (ContextPacket, error) {
	if s == nil {
		return ContextPacket{}, fmt.Errorf("coder store is required")
	}
	packet = normalizeContextPacket(packet, s.Project.ID)
	if packet.Version <= 0 {
		version, err := s.NextContextVersion()
		if err != nil {
			return ContextPacket{}, err
		}
		packet.Version = version
	}
	if packet.ID == "" {
		packet.ID = fmt.Sprintf("%s-context-v%04d", s.Project.ID, packet.Version)
	}
	path := filepath.Join(s.Dir, "context", contextFilename(packet.Version))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ContextPacket{}, err
	}
	if err := writeJSONAtomic(path, packet); err != nil {
		return ContextPacket{}, err
	}
	project := s.Project
	project.CurrentContext = packet.Version
	project.UpdatedAt = nowString()
	if err := s.SaveProject(project); err != nil {
		return ContextPacket{}, err
	}
	return packet, nil
}

func (s *Store) LoadContextPacket(version int) (ContextPacket, error) {
	if s == nil {
		return ContextPacket{}, fmt.Errorf("coder store is required")
	}
	if version <= 0 {
		latest, err := s.LatestContextVersion()
		if err != nil {
			return ContextPacket{}, err
		}
		version = latest
	}
	var packet ContextPacket
	if err := readJSON(filepath.Join(s.Dir, "context", contextFilename(version)), &packet); err != nil {
		return ContextPacket{}, err
	}
	return packet, nil
}

func (s *Store) NextContextVersion() (int, error) {
	latest, err := s.LatestContextVersion()
	if err != nil {
		if os.IsNotExist(err) {
			return 1, nil
		}
		return 0, err
	}
	return latest + 1, nil
}

func (s *Store) LatestContextVersion() (int, error) {
	if s == nil {
		return 0, fmt.Errorf("coder store is required")
	}
	entries, err := os.ReadDir(filepath.Join(s.Dir, "context"))
	if err != nil {
		return 0, err
	}
	latest := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "v") || !strings.HasSuffix(name, ".json") {
			continue
		}
		version, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "v"), ".json"))
		if err == nil && version > latest {
			latest = version
		}
	}
	if latest == 0 {
		return 0, os.ErrNotExist
	}
	return latest, nil
}

func (s *Store) SaveReviewGate(gate ReviewGate) error {
	if s == nil {
		return fmt.Errorf("coder store is required")
	}
	gate = normalizeReviewGate(gate, s.Project.ID)
	if strings.TrimSpace(gate.ID) == "" {
		return fmt.Errorf("review gate id is required")
	}
	dir := filepath.Join(s.Dir, "gates", slug(gate.ID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeJSONAtomic(filepath.Join(dir, "gate.json"), gate)
}

func (s *Store) LoadReviewGate(id string) (ReviewGate, error) {
	var gate ReviewGate
	if s == nil {
		return gate, fmt.Errorf("coder store is required")
	}
	if err := readJSON(filepath.Join(s.Dir, "gates", slug(id), "gate.json"), &gate); err != nil {
		return gate, err
	}
	return gate, nil
}

func (s *Store) SaveDynamicFormSpec(form DynamicFormSpec) error {
	if s == nil {
		return fmt.Errorf("coder store is required")
	}
	form = normalizeDynamicFormSpec(form)
	if strings.TrimSpace(form.GateID) == "" {
		return fmt.Errorf("dynamic form gate id is required")
	}
	if strings.TrimSpace(form.ID) == "" {
		return fmt.Errorf("dynamic form id is required")
	}
	dir := filepath.Join(s.Dir, "gates", slug(form.GateID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeJSONAtomic(filepath.Join(dir, "form.json"), form)
}

func (s *Store) LoadDynamicFormSpec(gateID string) (DynamicFormSpec, error) {
	var form DynamicFormSpec
	if s == nil {
		return form, fmt.Errorf("coder store is required")
	}
	if err := readJSON(filepath.Join(s.Dir, "gates", slug(gateID), "form.json"), &form); err != nil {
		return form, err
	}
	return form, nil
}

func (s *Store) SaveHumanReviewResponse(response HumanReviewResponse) error {
	if s == nil {
		return fmt.Errorf("coder store is required")
	}
	response = normalizeHumanReviewResponse(response, s.Project.ID)
	if strings.TrimSpace(response.GateID) == "" {
		return fmt.Errorf("human review response gate id is required")
	}
	if strings.TrimSpace(response.ID) == "" {
		return fmt.Errorf("human review response id is required")
	}
	dir := filepath.Join(s.Dir, "gates", slug(response.GateID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeJSONAtomic(filepath.Join(dir, "response.json"), response)
}

func (s *Store) LoadHumanReviewResponse(gateID string) (HumanReviewResponse, error) {
	var response HumanReviewResponse
	if s == nil {
		return response, fmt.Errorf("coder store is required")
	}
	if err := readJSON(filepath.Join(s.Dir, "gates", slug(gateID), "response.json"), &response); err != nil {
		return response, err
	}
	return response, nil
}

func (s *Store) AppendDecision(decision Decision) error {
	decision = normalizeDecision(decision, s.Project.ID)
	return s.appendJSONL("decisions.jsonl", decision)
}

func (s *Store) Decisions() ([]Decision, error) {
	return readJSONL[Decision](filepath.Join(s.Dir, "decisions.jsonl"))
}

func (s *Store) AppendTask(task Task) error {
	task = normalizeTask(task, s.Project.ID)
	return s.appendJSONL("tasks.jsonl", task)
}

func (s *Store) Tasks() ([]Task, error) {
	return readJSONL[Task](filepath.Join(s.Dir, "tasks.jsonl"))
}

func (s *Store) AppendArtifact(artifact Artifact) error {
	artifact = normalizeArtifact(artifact, s.Project.ID)
	return s.appendJSONL("artifacts.jsonl", artifact)
}

func (s *Store) Artifacts() ([]Artifact, error) {
	return readJSONL[Artifact](filepath.Join(s.Dir, "artifacts.jsonl"))
}

func (s *Store) appendJSONL(name string, value any) error {
	if s == nil {
		return fmt.Errorf("coder store is required")
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(s.Dir, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	line, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

func contextFilename(version int) string {
	return fmt.Sprintf("v%04d.json", version)
}

func normalizeProject(project Project) Project {
	now := nowString()
	project.ID = slug(project.ID)
	project.Name = strings.TrimSpace(project.Name)
	project.Vision = strings.TrimSpace(project.Vision)
	if project.SchemaVersion == 0 {
		project.SchemaVersion = CurrentSchemaVersion
	}
	if strings.TrimSpace(project.Status) == "" {
		project.Status = ProjectStatusExploring
	}
	if strings.TrimSpace(project.CreatedAt) == "" {
		project.CreatedAt = now
	}
	project.UpdatedAt = firstNonEmpty(project.UpdatedAt, now)
	return project
}

func normalizeContextPacket(packet ContextPacket, projectID string) ContextPacket {
	if packet.SchemaVersion == 0 {
		packet.SchemaVersion = CurrentSchemaVersion
	}
	packet.ProjectID = firstNonEmpty(packet.ProjectID, projectID)
	if strings.TrimSpace(packet.CreatedAt) == "" {
		packet.CreatedAt = nowString()
	}
	return packet
}

func normalizeReviewGate(gate ReviewGate, projectID string) ReviewGate {
	if gate.SchemaVersion == 0 {
		gate.SchemaVersion = CurrentSchemaVersion
	}
	gate.ID = slug(gate.ID)
	gate.ProjectID = firstNonEmpty(gate.ProjectID, projectID)
	if strings.TrimSpace(gate.Status) == "" {
		gate.Status = GateStatusPending
	}
	if strings.TrimSpace(gate.CreatedAt) == "" {
		gate.CreatedAt = nowString()
	}
	return gate
}

func normalizeDynamicFormSpec(form DynamicFormSpec) DynamicFormSpec {
	if form.SchemaVersion == 0 {
		form.SchemaVersion = CurrentSchemaVersion
	}
	form.ID = slug(form.ID)
	form.GateID = slug(form.GateID)
	if strings.TrimSpace(form.CreatedAt) == "" {
		form.CreatedAt = nowString()
	}
	return form
}

func normalizeHumanReviewResponse(response HumanReviewResponse, projectID string) HumanReviewResponse {
	if response.SchemaVersion == 0 {
		response.SchemaVersion = CurrentSchemaVersion
	}
	response.ID = slug(response.ID)
	response.GateID = slug(response.GateID)
	response.ProjectID = firstNonEmpty(response.ProjectID, projectID)
	if strings.TrimSpace(response.Reviewer) == "" {
		response.Reviewer = "human"
	}
	if strings.TrimSpace(response.CreatedAt) == "" {
		response.CreatedAt = nowString()
	}
	return response
}

func normalizeDecision(decision Decision, projectID string) Decision {
	if decision.SchemaVersion == 0 {
		decision.SchemaVersion = CurrentSchemaVersion
	}
	decision.ID = firstNonEmpty(slug(decision.ID), newID("decision"))
	decision.ProjectID = firstNonEmpty(decision.ProjectID, projectID)
	if strings.TrimSpace(decision.CreatedAt) == "" {
		decision.CreatedAt = nowString()
	}
	return decision
}

func normalizeTask(task Task, projectID string) Task {
	if task.SchemaVersion == 0 {
		task.SchemaVersion = CurrentSchemaVersion
	}
	task.ID = firstNonEmpty(slug(task.ID), newID("task"))
	task.ProjectID = firstNonEmpty(task.ProjectID, projectID)
	if strings.TrimSpace(task.CreatedAt) == "" {
		task.CreatedAt = nowString()
	}
	if strings.TrimSpace(task.UpdatedAt) == "" {
		task.UpdatedAt = task.CreatedAt
	}
	return task
}

func normalizeArtifact(artifact Artifact, projectID string) Artifact {
	if artifact.SchemaVersion == 0 {
		artifact.SchemaVersion = CurrentSchemaVersion
	}
	artifact.ID = firstNonEmpty(slug(artifact.ID), newID("artifact"))
	artifact.ProjectID = firstNonEmpty(artifact.ProjectID, projectID)
	if strings.TrimSpace(artifact.CreatedAt) == "" {
		artifact.CreatedAt = nowString()
	}
	return artifact
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSONL[T any](path string) ([]T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var values []T
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var value T
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "project-" + randomHex(4)
	}
	return result
}

func newID(prefix string) string {
	return slug(prefix) + "-" + time.Now().UTC().Format("20060102-150405") + "-" + randomHex(3)
}

func randomHex(bytes int) string {
	if bytes <= 0 {
		bytes = 3
	}
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}

func nowString() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
