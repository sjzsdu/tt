package agents

import "testing"

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
	if len(stock) != 3 {
		t.Fatalf("StockDiscussion len = %d, want 3", len(stock))
	}
	wantIDs := []string{StockBeginnerID, StockOldHandID, StockDiscussionHostID}
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
