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

func TestGenerateImageCallsOpenRouterChatCompletions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/chat/completions" {
			t.Fatalf("path = %q, want /api/v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-or" {
			t.Fatalf("Authorization = %q, want Bearer sk-or", got)
		}
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Model != "x-ai/grok-imagine-image-quality" {
			t.Fatalf("model = %q", req.Model)
		}
		if len(req.Messages) != 1 || req.Messages[0].Role != "user" || req.Messages[0].Content != "sunset cat" {
			t.Fatalf("messages = %#v", req.Messages)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"images": []map[string]any{{
						"type": "image_url",
						"image_url": map[string]string{
							"url": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString([]byte("jpg-bytes")),
						},
					}},
				},
			}},
		})
	}))
	defer server.Close()

	rt := &Runtime{Config: &pcconfig.Config{
		Agents: pcconfig.AgentsConfig{Defaults: pcconfig.AgentDefaults{ImageModel: "openrouter-grok-imagine"}},
		ModelList: []*pcconfig.ModelConfig{{
			ModelName: "openrouter-grok-imagine",
			Model:     "openrouter/x-ai/grok-imagine-image-quality",
			APIBase:   server.URL + "/api/v1",
			APIKeys:   pcconfig.SimpleSecureStrings("sk-or"),
		}},
	}}

	content, err := rt.GenerateImage(context.Background(), ImageOptions{Prompt: "sunset cat"})
	if err != nil {
		t.Fatalf("GenerateImage() error = %v", err)
	}
	if !strings.Contains(content, "data:image/jpeg;base64") {
		t.Fatalf("content = %q", content)
	}
}
