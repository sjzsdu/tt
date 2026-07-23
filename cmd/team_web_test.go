package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	teamruntime "github.com/sjzsdu/tt/internal/team"
)

func newTestTeamDashboard(t *testing.T) (*teamDashboardServer, *teamruntime.Store) {
	t.Helper()
	definition, err := teamruntime.Parse([]byte(starterTeamDefinition("dashboard-test")))
	if err != nil {
		t.Fatal(err)
	}
	store, err := teamruntime.NewStore(t.TempDir(), definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartRound("How should the dashboard work?"); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(teamruntime.Event{
		Type:    "agent_message",
		Phase:   teamruntime.PhaseInitial,
		From:    "architect",
		Session: "secret-session-key",
		Content: "Use the persisted event stream.",
	}); err != nil {
		t.Fatal(err)
	}
	return newTeamDashboardServer(store, definition), store
}

func TestTeamDashboardStateHandler(t *testing.T) {
	dashboard, store := newTestTeamDashboard(t)
	request := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	response := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var state teamDashboardState
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Thread.ID != store.Thread.ID || state.Round == nil || state.Round.Phase != teamruntime.PhaseInitial {
		t.Fatalf("state = %+v", state)
	}
	if len(state.Agents) != 3 || len(state.Events) < 3 {
		t.Fatalf("agents/events = %+v %+v", state.Agents, state.Events)
	}
	for _, event := range state.Events {
		if event.Session != "" {
			t.Fatalf("dashboard leaked session key: %+v", event)
		}
	}
	before := len(state.Events)
	if err := store.AppendEvent(teamruntime.Event{
		Type:    "agent_message",
		Phase:   teamruntime.PhaseReview,
		Wave:    1,
		From:    "engineer",
		Content: "Polling should reflect new persisted events.",
	}); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, request)
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Events) != before+1 {
		t.Fatalf("event count after update = %d, want %d", len(state.Events), before+1)
	}
	if cache := response.Header().Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("cache-control = %q", cache)
	}
}

func TestTeamDashboardIndexHandler(t *testing.T) {
	dashboard, _ := newTestTeamDashboard(t)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	body := response.Body.String()
	for _, text := range []string{`<div id="root"></div>`, "/assets/"} {
		if !strings.Contains(body, text) {
			t.Fatalf("dashboard index missing %q", text)
		}
	}
	if cache := response.Header().Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("cache-control = %q", cache)
	}

	assetRequest := httptest.NewRequest(http.MethodGet, firstTeamAssetPath(t, body), nil)
	assetResponse := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(assetResponse, assetRequest)
	if assetResponse.Code != http.StatusOK {
		t.Fatalf("asset status = %d", assetResponse.Code)
	}
}

func firstTeamAssetPath(t *testing.T, body string) string {
	t.Helper()
	start := strings.Index(body, "/assets/")
	if start < 0 {
		t.Fatal("dashboard index has no asset path")
	}
	end := strings.IndexAny(body[start:], `"'`)
	if end < 0 {
		t.Fatal("dashboard asset path is unterminated")
	}
	return body[start : start+end]
}

func TestTeamDashboardStartsOnLoopback(t *testing.T) {
	dashboard, _ := newTestTeamDashboard(t)
	dashboard.open = nil
	if err := dashboard.start(0, false); err != nil {
		t.Fatal(err)
	}
	defer dashboard.close()
	if !strings.HasPrefix(dashboard.url(), "http://127.0.0.1:") {
		t.Fatalf("url = %q", dashboard.url())
	}
	response, err := http.Get(dashboard.url() + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	if err := dashboard.start(0, false); err == nil {
		t.Fatal("expected already-running error")
	}
}
