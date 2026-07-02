package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedAgentsLoadFromMarkdown(t *testing.T) {
	paths, err := embeddedAgentPaths()
	if err != nil {
		t.Fatalf("embeddedAgentPaths() error = %v", err)
	}
	foundNested := false
	for _, path := range paths {
		if path == "embedded/core/coder.md" {
			foundNested = true
			break
		}
	}
	if !foundNested {
		t.Fatalf("embeddedAgentPaths() should include nested embedded/core/coder.md; got %v", paths)
	}

	core, err := Core()
	if err != nil {
		t.Fatalf("Core() error = %v", err)
	}
	if len(core) != 4 {
		t.Fatalf("Core len = %d, want 4", len(core))
	}
	wantCoreIDs := []string{CoderID, CodeResearchID, PlannerID, TesterID}
	for i, want := range wantCoreIDs {
		if core[i].ID != want {
			t.Fatalf("Core[%d] ID = %q, want %q", i, core[i].ID, want)
		}
		if core[i].Prompt == "" || core[i].Soul == "" {
			t.Fatalf("Core[%d] prompt and soul should be loaded", i)
		}
		if core[i].ID == CoderID && !containsString(core[i].Skills, "code-context") {
			t.Fatalf("Coder skills = %v, want code-context", core[i].Skills)
		}
	}

	all, err := All()
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	if len(all) < len(core) {
		t.Fatalf("All len = %d, want at least %d", len(all), len(core))
	}

	translate, err := TranslateMaster()
	if err != nil {
		t.Fatalf("TranslateMaster() error = %v", err)
	}
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

	beadManager, err := Get(BeadManagerID)
	if err != nil {
		t.Fatalf("Get(BeadManagerID) error = %v", err)
	}
	if beadManager.Name != "Bead 事务管理员" {
		t.Fatalf("BeadManager name = %q", beadManager.Name)
	}
	if !containsString(beadManager.Tools, "skills") || !containsString(beadManager.Tools, "exec") {
		t.Fatalf("BeadManager tools = %v, want skills and exec", beadManager.Tools)
	}
	if !containsString(beadManager.Skills, "bead") {
		t.Fatalf("BeadManager skills = %v, want bead", beadManager.Skills)
	}
	if !containsString(beadManager.Aliases, "bd") {
		t.Fatalf("BeadManager aliases = %v, want bd", beadManager.Aliases)
	}
	if beadManager.Prompt == "" || beadManager.Soul == "" {
		t.Fatalf("BeadManager prompt and soul should be loaded")
	}

	yiJing, err := Get(YiJingID)
	if err != nil {
		t.Fatalf("Get(YiJingID) error = %v", err)
	}
	if yiJing.Name != "易经决策参谋" {
		t.Fatalf("YiJing name = %q", yiJing.Name)
	}
	if !containsString(yiJing.Aliases, "yj") {
		t.Fatalf("YiJing aliases = %v, want yj", yiJing.Aliases)
	}
	if yiJing.Prompt == "" || yiJing.Soul == "" {
		t.Fatalf("YiJing prompt and soul should be loaded")
	}
	if !strings.Contains(yiJing.Prompt, "不是“算命”") || !strings.Contains(yiJing.Soul, "善易者") {
		t.Fatalf("YiJing prompt/soul should define Yijing decision boundaries")
	}

	codeContextManager, err := Get(CodeContextManagerID)
	if err != nil {
		t.Fatalf("Get(CodeContextManagerID) error = %v", err)
	}
	if codeContextManager.Name != "Code Context 管理员" {
		t.Fatalf("CodeContextManager name = %q", codeContextManager.Name)
	}
	if !containsString(codeContextManager.Tools, "skills") {
		t.Fatalf("CodeContextManager tools = %v, want skills", codeContextManager.Tools)
	}
	if !containsString(codeContextManager.Skills, "code-context") {
		t.Fatalf("CodeContextManager skills = %v, want code-context", codeContextManager.Skills)
	}
	if !containsString(codeContextManager.Aliases, "cc") {
		t.Fatalf("CodeContextManager aliases = %v, want cc", codeContextManager.Aliases)
	}
	if codeContextManager.Prompt == "" || codeContextManager.Soul == "" {
		t.Fatalf("CodeContextManager prompt and soul should be loaded")
	}

	agentBrowser, err := Get(AgentBrowserID)
	if err != nil {
		t.Fatalf("Get(AgentBrowserID) error = %v", err)
	}
	if agentBrowser.Name != "Agent Browser 管理员" {
		t.Fatalf("AgentBrowser name = %q", agentBrowser.Name)
	}
	if !containsString(agentBrowser.Tools, "skills") {
		t.Fatalf("AgentBrowser tools = %v, want skills", agentBrowser.Tools)
	}
	if !containsString(agentBrowser.Tools, "exec") {
		t.Fatalf("AgentBrowser tools = %v, want exec", agentBrowser.Tools)
	}
	if !containsString(agentBrowser.Skills, "agent-browser") {
		t.Fatalf("AgentBrowser skills = %v, want agent-browser", agentBrowser.Skills)
	}
	if !containsString(agentBrowser.Aliases, "ab") {
		t.Fatalf("AgentBrowser aliases = %v, want ab", agentBrowser.Aliases)
	}
	if agentBrowser.Prompt == "" || agentBrowser.Soul == "" {
		t.Fatalf("AgentBrowser prompt and soul should be loaded")
	}

	stock, err := StockDiscussion()
	if err != nil {
		t.Fatalf("StockDiscussion() error = %v", err)
	}
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
		if !containsString(stock[i].Tools, "web_search") || !containsString(stock[i].Tools, "web_fetch") {
			t.Fatalf("StockDiscussion[%d] should include research tools, got %v", i, stock[i].Tools)
		}
		if len(stock[i].Skills) != 2 {
			t.Fatalf("StockDiscussion[%d] skills = %v, want 2 skills", i, stock[i].Skills)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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

func TestListLoadsFilesystemAgentsFromSubdirectories(t *testing.T) {
	tmp := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir(tmp) error = %v", err)
	}

	dir := filepath.Join(".tt", "agents", "custom", "review")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	const md = `---
id: nested-local-demo
name: Nested Local Demo
soul: Nested soul
---
You are a nested local demo agent.
`
	path := filepath.Join(dir, "nested-local-demo.md")
	if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := Get("nested-local-demo")
	if err != nil {
		t.Fatalf("Get(nested-local-demo) error = %v", err)
	}
	if got.Name != "Nested Local Demo" || got.Soul != "Nested soul" || got.Prompt == "" {
		t.Fatalf("nested-local-demo fields mismatch: %+v", got)
	}

	gotPath, err := FilePathForID("nested-local-demo")
	if err != nil {
		t.Fatalf("FilePathForID(nested-local-demo) error = %v", err)
	}
	if gotPath != path {
		t.Fatalf("FilePathForID(nested-local-demo) = %q, want %q", gotPath, path)
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

func TestCoreAgentMetadataContract(t *testing.T) {
	wantIDs := map[string]struct{}{
		CoderID:        {},
		PlannerID:      {},
		TesterID:       {},
		CodeResearchID: {},
	}

	all, err := All()
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	seen := map[string]struct{}{}
	for _, agent := range all {
		if _, ok := wantIDs[agent.ID]; !ok {
			continue
		}
		seen[agent.ID] = struct{}{}
		if strings.TrimSpace(agent.Description) == "" {
			t.Fatalf("%s missing description", agent.ID)
		}
		if agent.ID == CoderID && !containsString(agent.Tools, "exec") {
			t.Fatalf("%s should include exec tool, got %v", agent.ID, agent.Tools)
		}
	}
	for id := range wantIDs {
		if _, ok := seen[id]; !ok {
			t.Fatalf("metadata contract did not see core agent %s", id)
		}
	}
}

func TestAgentIDsAreUniqueAndPromptsReasonable(t *testing.T) {
	all, err := All()
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	seen := map[string]struct{}{}
	coreMetadataAgents := map[string]struct{}{
		CoderID:        {},
		PlannerID:      {},
		TesterID:       {},
		CodeResearchID: {},
	}
	for _, agent := range all {
		if strings.TrimSpace(agent.ID) == "" {
			t.Fatalf("agent missing id: %+v", agent)
		}
		if _, ok := seen[agent.ID]; ok {
			t.Fatalf("duplicate agent id %q", agent.ID)
		}
		seen[agent.ID] = struct{}{}
		if strings.TrimSpace(agent.Prompt) == "" {
			t.Fatalf("%s missing prompt body", agent.ID)
		}
		_, isCoreMetadataAgent := coreMetadataAgents[agent.ID]
		if isCoreMetadataAgent && len(agent.Prompt) > 12000 {
			t.Fatalf("%s prompt too long: %d chars", agent.ID, len(agent.Prompt))
		}
	}
}
