package coder

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreProjectRoundTrip(t *testing.T) {
	root := t.TempDir()
	project := NewProject("AI Hiring", "AI Hiring", "Help small teams screen resumes")
	project.OwnerIntent = "MVP first"
	project.TargetUsers = []string{"founders", "hiring managers"}

	store, err := CreateStore(root, project)
	if err != nil {
		t.Fatal(err)
	}
	if store.Project.ID != "ai-hiring" {
		t.Fatalf("project id = %q", store.Project.ID)
	}

	loaded, err := OpenStore(root, "ai-hiring")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Project.Name != "AI Hiring" || loaded.Project.Vision != project.Vision {
		t.Fatalf("loaded project = %+v", loaded.Project)
	}
	if loaded.Project.SchemaVersion != CurrentSchemaVersion || loaded.Project.Status != ProjectStatusExploring {
		t.Fatalf("normalized project = %+v", loaded.Project)
	}
}

func TestContextPacketVersioningAndLatestLoad(t *testing.T) {
	store := newTestStore(t)

	first, err := store.SaveContextPacket(ContextPacket{
		Product: ProductContext{Vision: "Screen resumes", HumanDirection: "Keep it simple"},
		Phase:   PhaseContext{Name: "product", Objective: "Confirm MVP"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 {
		t.Fatalf("first version = %d", first.Version)
	}

	second, err := store.SaveContextPacket(ContextPacket{
		Product: ProductContext{Vision: "Screen resumes", HumanDirection: "PDF only"},
		Phase:   PhaseContext{Name: "architecture", Objective: "Choose stack"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Version != 2 {
		t.Fatalf("second version = %d", second.Version)
	}

	latest, err := store.LoadContextPacket(0)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Version != 2 || latest.Phase.Name != "architecture" {
		t.Fatalf("latest context = %+v", latest)
	}
	loadedProject, err := store.LoadProject()
	if err != nil {
		t.Fatal(err)
	}
	if loadedProject.CurrentContext != 2 {
		t.Fatalf("project current context = %d", loadedProject.CurrentContext)
	}
}

func TestReviewGateFormAndResponseRoundTrip(t *testing.T) {
	store := newTestStore(t)
	gate := ReviewGate{
		ID:             "product-scope",
		Type:           GateTypeFeatureScope,
		Status:         GateStatusWaitingHuman,
		Title:          "确认 MVP 功能范围",
		CreatedByAgent: "product-agent",
	}
	if err := store.SaveReviewGate(gate); err != nil {
		t.Fatal(err)
	}
	form := DynamicFormSpec{
		ID:          "product-scope-form",
		GateID:      gate.ID,
		Title:       "功能范围确认",
		Description: "选择本轮要做的功能",
		Fields: []FormField{{
			ID:       "features",
			Label:    "MVP 功能",
			Type:     "feature_matrix",
			Required: true,
		}},
		SubmitActions: []string{ReviewDecisionApprove, ReviewDecisionApproveWithChanges, ReviewDecisionRequestRevision},
	}
	if err := store.SaveDynamicFormSpec(form); err != nil {
		t.Fatal(err)
	}
	response := HumanReviewResponse{
		ID:       "product-scope-response",
		GateID:   gate.ID,
		Decision: ReviewDecisionApproveWithChanges,
		Answers: map[string]any{
			"priority": "MVP",
		},
		FreeformComment: "先不做权限",
	}
	if err := store.SaveHumanReviewResponse(response); err != nil {
		t.Fatal(err)
	}

	loadedGate, err := store.LoadReviewGate("product-scope")
	if err != nil {
		t.Fatal(err)
	}
	if loadedGate.Type != GateTypeFeatureScope || loadedGate.ProjectID != store.Project.ID {
		t.Fatalf("gate = %+v", loadedGate)
	}
	loadedForm, err := store.LoadDynamicFormSpec("product-scope")
	if err != nil {
		t.Fatal(err)
	}
	if loadedForm.Fields[0].Type != "feature_matrix" {
		t.Fatalf("form = %+v", loadedForm)
	}
	loadedResponse, err := store.LoadHumanReviewResponse("product-scope")
	if err != nil {
		t.Fatal(err)
	}
	if loadedResponse.Reviewer != "human" || loadedResponse.Answers["priority"] != "MVP" {
		t.Fatalf("response = %+v", loadedResponse)
	}
}

func TestAppendOnlyLogsRoundTrip(t *testing.T) {
	store := newTestStore(t)
	if err := store.AppendDecision(Decision{ID: "scope", Source: "human_review", Content: "PDF only", Reason: "Ship MVP"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendDecision(Decision{ID: "deploy", Content: "Docker Compose"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendTask(Task{ID: "upload", Title: "Implement upload", Status: "pending", OwnerAgent: "implementer"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendArtifact(Artifact{ID: "design-v1", Type: "design", PathOrURL: "artifacts/design-v1.md"}); err != nil {
		t.Fatal(err)
	}

	decisions, err := store.Decisions()
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 2 || decisions[0].ProjectID != store.Project.ID || decisions[1].ID != "deploy" {
		t.Fatalf("decisions = %+v", decisions)
	}
	tasks, err := store.Tasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Title != "Implement upload" {
		t.Fatalf("tasks = %+v", tasks)
	}
	artifacts, err := store.Artifacts()
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].Type != "design" {
		t.Fatalf("artifacts = %+v", artifacts)
	}
}

func TestMissingFilesReturnNotExist(t *testing.T) {
	root := t.TempDir()
	if _, err := OpenStore(root, "missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("OpenStore error = %v, want not exist", err)
	}

	store := newTestStore(t)
	if _, err := store.LoadContextPacket(1); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadContextPacket error = %v, want not exist", err)
	}
	if _, err := store.LoadReviewGate("missing-gate"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadReviewGate error = %v, want not exist", err)
	}
	if _, err := store.Decisions(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Decisions error = %v, want not exist", err)
	}
}

func TestListProjectsReturnsNewestFirst(t *testing.T) {
	root := t.TempDir()
	first, err := CreateStore(root, NewProject("first", "First", "First project"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := CreateStore(root, NewProject("second", "Second", "Second project"))
	if err != nil {
		t.Fatal(err)
	}
	project := second.Project
	project.UpdatedAt = "2999-01-01T00:00:00Z"
	if err := second.SaveProject(project); err != nil {
		t.Fatal(err)
	}
	project = first.Project
	project.UpdatedAt = "2000-01-01T00:00:00Z"
	if err := first.SaveProject(project); err != nil {
		t.Fatal(err)
	}

	records, err := ListProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].ID != "second" || records[1].ID != "first" {
		t.Fatalf("records = %+v", records)
	}
}

func TestDefaultRootUsesWorkspace(t *testing.T) {
	workspace := t.TempDir()
	want := filepath.Join(workspace, ".tt", "coder", "projects")
	if got := DefaultRoot(workspace); got != want {
		t.Fatalf("DefaultRoot = %q, want %q", got, want)
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := CreateStore(t.TempDir(), NewProject("resume-helper", "Resume Helper", "Screen resumes"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}
