package agents

import "testing"

func TestEmbeddedAgentsLoadFromMarkdown(t *testing.T) {
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
