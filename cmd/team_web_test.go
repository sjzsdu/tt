package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	teamruntime "github.com/sjzsdu/tt/internal/team"
)

type fakeTeamDashboardActions struct {
	mu        sync.Mutex
	followUps []string
	resumes   int
	stops     int
	err       error
	controls  teamDashboardControls
}

func (f *fakeTeamDashboardActions) FollowUp(message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.followUps = append(f.followUps, message)
	return nil
}

func (f *fakeTeamDashboardActions) Resume() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.resumes++
	return nil
}

func (f *fakeTeamDashboardActions) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.stops++
	return nil
}

func (f *fakeTeamDashboardActions) Controls() teamDashboardControls {
	return f.controls
}

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
	operation := teamruntime.BlackboardOperation{
		Action:  teamruntime.BlackboardActionUpsert,
		Kind:    teamruntime.BlackboardFact,
		Key:     "dashboard-source",
		Content: "The event stream is authoritative.",
	}
	if err := store.AppendEvent(teamruntime.Event{
		Type:       "blackboard_upsert",
		Phase:      teamruntime.PhaseInitial,
		From:       "architect",
		Blackboard: &operation,
	}); err != nil {
		t.Fatal(err)
	}
	return newTeamDashboardServer(store, definition), store
}

func TestTeamDashboardStateHandler(t *testing.T) {
	dashboard, store := newTestTeamDashboard(t)
	actions := &fakeTeamDashboardActions{
		controls: teamDashboardControls{Busy: true, CanStop: true},
	}
	dashboard.setActions(actions)
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
	if len(state.Agents) != 3 || len(state.Events) < 4 {
		t.Fatalf("agents/events = %+v %+v", state.Agents, state.Events)
	}
	if len(state.Board.Entries) != 1 || state.Board.Entries[0].Key != "dashboard-source" {
		t.Fatalf("blackboard = %+v", state.Board)
	}
	if !state.Controls.Busy || !state.Controls.CanStop {
		t.Fatalf("controls = %+v", state.Controls)
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

func TestTeamDashboardMutationHandlers(t *testing.T) {
	dashboard, _ := newTestTeamDashboard(t)
	actions := &fakeTeamDashboardActions{}
	dashboard.setActions(actions)

	followUp := localMutationRequest(http.MethodPost, "/api/follow-up", `{"message":"Review migration cost."}`)
	response := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, followUp)
	if response.Code != http.StatusAccepted || len(actions.followUps) != 1 ||
		actions.followUps[0] != "Review migration cost." {
		t.Fatalf("follow-up response/actions = %d, %+v", response.Code, actions)
	}

	for _, path := range []string{"/api/resume", "/api/stop"} {
		response = httptest.NewRecorder()
		dashboard.routes().ServeHTTP(response, localMutationRequest(http.MethodPost, path, ""))
		if response.Code != http.StatusAccepted {
			t.Fatalf("%s status = %d, body = %s", path, response.Code, response.Body.String())
		}
	}
	if actions.resumes != 1 || actions.stops != 1 {
		t.Fatalf("resume/stop actions = %+v", actions)
	}
}

func TestTeamDashboardMutationRejectsInvalidTransitionsAndOrigins(t *testing.T) {
	dashboard, _ := newTestTeamDashboard(t)
	actions := &fakeTeamDashboardActions{err: errors.New("invalid team transition")}
	dashboard.setActions(actions)

	response := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, localMutationRequest(http.MethodPost, "/api/resume", ""))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "invalid team transition") {
		t.Fatalf("transition response = %d, %s", response.Code, response.Body.String())
	}

	remote := localMutationRequest(http.MethodPost, "/api/stop", "")
	remote.RemoteAddr = "203.0.113.8:9000"
	response = httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, remote)
	if response.Code != http.StatusForbidden {
		t.Fatalf("remote mutation status = %d", response.Code)
	}

	crossOrigin := localMutationRequest(http.MethodPost, "/api/stop", "")
	crossOrigin.Header.Set("Origin", "https://example.com")
	response = httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, crossOrigin)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin mutation status = %d", response.Code)
	}

	response = httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, localMutationRequest(http.MethodGet, "/api/stop", ""))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET mutation status = %d", response.Code)
	}
}

func TestTeamDashboardSSEStreamsStateAndCleansUpDisconnect(t *testing.T) {
	dashboard, store := newTestTeamDashboard(t)
	server := httptest.NewServer(dashboard.routes())
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(response.Body)
	first := readSSEState(t, reader)
	if first.Thread.ID != store.Thread.ID {
		t.Fatalf("initial SSE state = %+v", first.Thread)
	}

	if err := store.AppendEvent(teamruntime.Event{
		Type:    "agent_message",
		Phase:   teamruntime.PhaseReview,
		From:    "engineer",
		Content: "Stream this update.",
	}); err != nil {
		t.Fatal(err)
	}
	dashboard.notifyState()
	second := readSSEState(t, reader)
	if len(second.Events) != len(first.Events)+1 {
		t.Fatalf("streamed event count = %d, want %d", len(second.Events), len(first.Events)+1)
	}

	cancel()
	_ = response.Body.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		dashboard.mu.Lock()
		subscribers := len(dashboard.subs)
		dashboard.mu.Unlock()
		if subscribers == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("SSE subscriber was not removed after disconnect")
}

func localMutationRequest(method, path, body string) *http.Request {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.RemoteAddr = "127.0.0.1:43120"
	request.Host = "127.0.0.1:9715"
	request.Header.Set("Origin", "http://127.0.0.1:9715")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}

func readSSEState(t *testing.T, reader *bufio.Reader) teamDashboardState {
	t.Helper()
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				t.Fatal("SSE stream ended before state event")
			}
			t.Fatal(err)
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var state teamDashboardState
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data: "))), &state); err != nil {
			t.Fatal(err)
		}
		return state
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
