package agents

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedAgentsLoadFromMarkdown(t *testing.T) {
	core := Core()
	if len(core) != 6 {
		t.Fatalf("Core len = %d, want 6", len(core))
	}
	wantCoreIDs := []string{CoderID, FullStackID, PlannerID, ProductManagerID, TesterID, UIID}
	for i, want := range wantCoreIDs {
		if core[i].ID != want {
			t.Fatalf("Core[%d] ID = %q, want %q", i, core[i].ID, want)
		}
		if core[i].Prompt == "" || core[i].Soul == "" {
			t.Fatalf("Core[%d] prompt and soul should be loaded", i)
		}
	}

	all := All()
	if len(all) < len(core) {
		t.Fatalf("All len = %d, want at least %d", len(all), len(core))
	}

	translate := TranslateMaster()
	if translate.ID != TranslateMasterID {
		t.Fatalf("TranslateMaster ID = %q, want %q", translate.ID, TranslateMasterID)
	}
	if translate.Name != "翻译大师" {
		t.Fatalf("TranslateMaster name = %q", translate.Name)
	}
	if !translate.NoHistory {
		t.Fatalf("TranslateMaster should disable history")
	}
	if translate.Prompt == "" || translate.Soul == "" {
		t.Fatalf("TranslateMaster prompt and soul should be loaded")
	}

	stock := StockDiscussion()
	if len(stock) != 7 {
		t.Fatalf("StockDiscussion len = %d, want 7", len(stock))
	}
	wantIDs := []string{
		StockBeginnerID,
		StockOldHandID,
		StockDiscussionHostID,
		StockMacroStrategistID,
		StockQuantTechnicianID,
		StockNewsEventAnalystID,
		StockSectorSpecialistID,
	}
	for i, want := range wantIDs {
		if stock[i].ID != want {
			t.Fatalf("StockDiscussion[%d] ID = %q, want %q", i, stock[i].ID, want)
		}
		if stock[i].NoHistory {
			t.Fatalf("StockDiscussion[%d] should keep history", i)
		}
		if !stock[i].EnableResearchTools {
			t.Fatalf("StockDiscussion[%d] should enable research tools", i)
		}
		if len(stock[i].Skills) != 2 {
			t.Fatalf("StockDiscussion[%d] skills = %v, want 2 skills", i, stock[i].Skills)
		}
	}
}

func TestListLoadsFilesystemAgentsAutomatically(t *testing.T) {
	tmp := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir(tmp) error = %v", err)
	}

	dir := filepath.Join(".tt", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	const md = `---
id: local-demo
name: Local Demo
soul: Test soul
---
You are a local demo agent.
`
	if err := os.WriteFile(filepath.Join(dir, "local-demo.md"), []byte(md), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	all, err := List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	found := false
	for _, a := range all {
		if a.ID == "local-demo" {
			found = true
			if a.Name != "Local Demo" || a.Soul != "Test soul" || a.Prompt == "" {
				t.Fatalf("local-demo fields mismatch: %+v", a)
			}
		}
	}
	if !found {
		t.Fatalf("List() missing filesystem agent local-demo")
	}

	got, err := Get("local-demo")
	if err != nil {
		t.Fatalf("Get(local-demo) error = %v", err)
	}
	if got.ID != "local-demo" {
		t.Fatalf("Get(local-demo).ID = %q", got.ID)
	}
}

func TestFilesystemAgentOverridesEmbeddedAgent(t *testing.T) {
	tmp := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir(tmp) error = %v", err)
	}

	dir := filepath.Join(".tt", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	const md = `---
id: writer
name: Custom Writer
soul: Override soul
---
You are an overridden writer agent.
`
	if err := os.WriteFile(filepath.Join(dir, "writer.md"), []byte(md), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	all, err := List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	count := 0
	for _, a := range all {
		if a.ID == "writer" {
			count++
			if a.Name != "Custom Writer" || a.Soul != "Override soul" || a.Prompt != "You are an overridden writer agent." {
				t.Fatalf("writer override mismatch: %+v", a)
			}
		}
	}
	if count != 1 {
		t.Fatalf("writer count = %d, want 1", count)
	}
}
