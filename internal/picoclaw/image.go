package picoclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	pcauth "github.com/sipeed/picoclaw/pkg/auth"
	pcconfig "github.com/sipeed/picoclaw/pkg/config"
	pclogger "github.com/sipeed/picoclaw/pkg/logger"
	pcproviders "github.com/sipeed/picoclaw/pkg/providers"
)

type ImageOptions struct {
	Prompt  string
	Model   string
	Size    string
	Quality string
	Style   string
	Debug   bool
	Quiet   bool
}

func (rt *Runtime) GenerateImage(ctx context.Context, opt ImageOptions) (string, error) {
	if rt == nil || rt.Config == nil {
		return "", fmt.Errorf("picoclaw runtime not loaded")
	}
	prompt := strings.TrimSpace(opt.Prompt)
	if prompt == "" {
		return "", fmt.Errorf("image prompt is required")
	}

	pclogger.ConfigureFromEnv()
	if opt.Quiet && !opt.Debug {
		pclogger.DisableConsole()
	}
	if opt.Debug {
		pclogger.SetLevel(pclogger.DEBUG)
	}

	modelCfg, modelID, protocol, err := rt.resolveImageModel(opt.Model)
	if err != nil {
		return "", err
	}
	apiBase := pcproviders.ResolveAPIBase(modelCfg)
	if apiBase == "" {
		return "", fmt.Errorf("image model %q has no api_base and no known provider default", modelCfg.ModelName)
	}

	body := map[string]any{
		"model":           modelID,
		"prompt":          prompt,
		"n":               1,
		"response_format": "b64_json",
	}
	if size := strings.TrimSpace(opt.Size); size != "" {
		body["size"] = size
	}
	if quality := strings.TrimSpace(opt.Quality); quality != "" {
		body["quality"] = quality
	}
	if style := strings.TrimSpace(opt.Style); style != "" {
		body["style"] = style
	}
	for k, v := range modelCfg.ExtraBody {
		if _, exists := body[k]; !exists {
			body[k] = v
		}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(apiBase, "/")+"/images/generations", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if ua := strings.TrimSpace(modelCfg.UserAgent); ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	authToken, err := imageAuthToken(modelCfg, protocol)
	if err != nil {
		return "", err
	}
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}
	for k, v := range modelCfg.CustomHeaders {
		req.Header.Set(k, v)
	}

	client := http.DefaultClient
	if modelCfg.RequestTimeout > 0 {
		client = &http.Client{Timeout: time.Duration(modelCfg.RequestTimeout) * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("generate image request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(respBody))
		if strings.Contains(strings.ToLower(msg), "api.model.images.request") {
			return "", fmt.Errorf("generate image returned HTTP %d: the configured credential does not have OpenAI image generation scope (api.model.images.request). Configure an API key with image permissions for model %q or re-authorize picoclaw with image scope. Response: %s", resp.StatusCode, modelCfg.ModelName, msg)
		}
		return "", fmt.Errorf("generate image returned HTTP %d: %s", resp.StatusCode, msg)
	}
	if strings.TrimSpace(string(respBody)) == "" {
		return "", fmt.Errorf("generate image returned an empty response")
	}
	return strings.TrimSpace(string(respBody)), nil
}

func (rt *Runtime) resolveImageModel(modelOverride string) (*pcconfig.ModelConfig, string, string, error) {
	cfg := cloneConfig(rt.Config)
	modelName := strings.TrimSpace(modelOverride)
	if modelName == "" {
		modelName = strings.TrimSpace(cfg.Agents.Defaults.ImageModel)
	}
	if modelName == "" {
		modelName = cfg.Agents.Defaults.GetModelName()
	}
	if modelName == "" {
		return nil, "", "", fmt.Errorf("no model specified and no default model configured")
	}
	modelCfg, err := cfg.GetModelConfig(modelName)
	if err != nil {
		return nil, "", "", fmt.Errorf("image model %q not found: %w", modelName, err)
	}
	protocol, modelID := pcproviders.ExtractProtocol(modelCfg)
	if modelID == "" {
		modelID = modelCfg.Model
	}
	if protocol == "" {
		return nil, "", "", fmt.Errorf("image model %q has no provider/protocol", modelName)
	}
	modelID = imageEndpointModelID(protocol, modelID)
	return modelCfg, modelID, protocol, nil
}

func imageEndpointModelID(protocol, modelID string) string {
	protocol = strings.TrimSpace(protocol)
	modelID = strings.TrimSpace(modelID)
	prefix := protocol + "/"
	if protocol != "" && strings.HasPrefix(modelID, prefix) {
		return strings.TrimPrefix(modelID, prefix)
	}
	return modelID
}

func imageAuthToken(modelCfg *pcconfig.ModelConfig, protocol string) (string, error) {
	if modelCfg == nil {
		return "", nil
	}
	if apiKey := strings.TrimSpace(modelCfg.APIKey()); apiKey != "" {
		return apiKey, nil
	}
	authMethod := strings.ToLower(strings.TrimSpace(modelCfg.AuthMethod))
	if authMethod != "oauth" && authMethod != "token" {
		return "", nil
	}
	cred, err := pcauth.GetCredential(protocol)
	if err != nil {
		return "", fmt.Errorf("load %s auth credential failed: %w", protocol, err)
	}
	if cred == nil || strings.TrimSpace(cred.AccessToken) == "" {
		return "", fmt.Errorf("image model %q uses %s auth but no %s credential is available", modelCfg.ModelName, authMethod, protocol)
	}
	return strings.TrimSpace(cred.AccessToken), nil
}
