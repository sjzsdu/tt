package picoclaw

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pcconfig "github.com/sipeed/picoclaw/pkg/config"
)

func TestGenerateImageCallsImagesEndpoint(t *testing.T) {
	wantBytes := []byte("png-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("path = %q, want /v1/images/generations", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("Authorization = %q, want Bearer sk-test", got)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req["model"] != "dall-e-3" {
			t.Fatalf("model = %v, want dall-e-3", req["model"])
		}
		if req["prompt"] != "a lobster at sunset" {
			t.Fatalf("prompt = %v", req["prompt"])
		}
		if req["size"] != "1024x1024" {
			t.Fatalf("size = %v", req["size"])
		}
		if req["response_format"] != "b64_json" {
			t.Fatalf("response_format = %v", req["response_format"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(wantBytes)}},
		})
	}))
	defer server.Close()

	rt := &Runtime{Config: &pcconfig.Config{
		Agents: pcconfig.AgentsConfig{Defaults: pcconfig.AgentDefaults{ImageModel: "image-alias"}},
		ModelList: []*pcconfig.ModelConfig{{
			ModelName: "image-alias",
			Provider:  "openai",
			Model:     "openai/dall-e-3",
			APIBase:   server.URL + "/v1",
			APIKeys:   pcconfig.SimpleSecureStrings("sk-test"),
		}},
	}}

	content, err := rt.GenerateImage(context.Background(), ImageOptions{Prompt: "a lobster at sunset", Size: "1024x1024"})
	if err != nil {
		t.Fatalf("GenerateImage() error = %v", err)
	}
	if got := strings.TrimSpace(content); got == "" {
		t.Fatal("GenerateImage() returned empty content")
	}
}
