package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTeamIndex(t *testing.T) {
	body := string(TeamIndex())
	if !strings.Contains(body, `<div id="root"></div>`) {
		t.Fatal("team dashboard root missing")
	}
	if !strings.Contains(body, "/assets/") {
		t.Fatal("team dashboard asset reference missing")
	}
}

func TestTeamFaviconHandler(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/favicon.svg", nil)
	response := httptest.NewRecorder()
	TeamFaviconHandler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "image/svg+xml") {
		t.Fatalf("content-type = %q", contentType)
	}
}
