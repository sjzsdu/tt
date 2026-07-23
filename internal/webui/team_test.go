package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTeamIndexHandler(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	TeamIndexHandler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "text/html") {
		t.Fatalf("content-type = %q", contentType)
	}
	if !strings.Contains(response.Body.String(), "Team members") {
		t.Fatal("team dashboard content missing")
	}
}
