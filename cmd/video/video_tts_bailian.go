package videocmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type videoBailianTTSProvider struct {
	apiKeyEnv            string
	apiKey               string
	baseURL              string
	model                string
	voice                string
	languageType         string
	instructions         string
	optimizeInstructions *bool
	workDir              string
	httpClient           *http.Client
}

type videoBailianTTSRequestPayload struct {
	Model string                 `json:"model"`
	Input map[string]interface{} `json:"input"`
}

type videoBailianTTSResponse struct {
	StatusCode int    `json:"status_code"`
	RequestID  string `json:"request_id"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Output     struct {
		FinishReason string `json:"finish_reason"`
		Audio        struct {
			Data      string `json:"data"`
			URL       string `json:"url"`
			ID        string `json:"id"`
			ExpiresAt int64  `json:"expires_at"`
		} `json:"audio"`
	} `json:"output"`
}

func newVideoBailianTTSProvider(opts videoTTSOptions) (videoTTSProvider, error) {
	apiKeyEnv := firstNonEmpty(opts.BailianAPIKeyEnv, "DASHSCOPE_API_KEY")
	apiKey := strings.TrimSpace(os.Getenv(apiKeyEnv))
	if apiKey == "" {
		return nil, fmt.Errorf("environment variable %s is required when --tts bailian", apiKeyEnv)
	}
	baseURL := strings.TrimSpace(opts.BailianBaseURL)
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/api/v1"
	}
	workDir := opts.WorkDir
	if strings.TrimSpace(workDir) == "" {
		workDir = filepath.Join(os.TempDir(), "tt-video-audio")
	}
	abs, err := filepath.Abs(workDir)
	if err != nil {
		return nil, fmt.Errorf("resolve workdir failed: %w", err)
	}
	return videoBailianTTSProvider{
		apiKeyEnv:            apiKeyEnv,
		apiKey:               apiKey,
		baseURL:              baseURL,
		model:                firstNonEmpty(opts.BailianModel, "qwen3-tts-flash"),
		voice:                firstNonEmpty(opts.BailianVoice, "Cherry"),
		languageType:         firstNonEmpty(opts.BailianLanguageType, "Auto"),
		instructions:         strings.TrimSpace(opts.BailianInstructions),
		optimizeInstructions: opts.BailianOptimizeInstructions,
		workDir:              abs,
		httpClient:           &http.Client{Timeout: 120 * time.Second},
	}, nil
}

func (p videoBailianTTSProvider) Synthesize(ctx context.Context, req videoTTSRequest) (videoTTSResult, error) {
	if strings.TrimSpace(req.Text) == "" {
		return videoTTSResult{}, nil
	}
	output := req.Output
	if output == "" {
		output = filepath.Join(p.workDir, fmt.Sprintf("%03d.wav", req.Index))
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return videoTTSResult{}, fmt.Errorf("create Bailian TTS output directory failed: %w", err)
	}
	voice := firstNonEmpty(req.Voice, p.voice)
	input := map[string]interface{}{
		"text":          req.Text,
		"voice":         voice,
		"language_type": p.languageType,
	}
	if p.instructions != "" {
		input["instructions"] = p.instructions
	}
	if p.optimizeInstructions != nil {
		input["optimize_instructions"] = *p.optimizeInstructions
	}
	payload := videoBailianTTSRequestPayload{Model: p.model, Input: input}
	body, err := json.Marshal(payload)
	if err != nil {
		return videoTTSResult{}, fmt.Errorf("marshal Bailian TTS request failed: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(), bytes.NewReader(body))
	if err != nil {
		return videoTTSResult{}, fmt.Errorf("create Bailian TTS request failed: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return videoTTSResult{}, fmt.Errorf("call Bailian TTS failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return videoTTSResult{}, fmt.Errorf("read Bailian TTS response failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return videoTTSResult{}, fmt.Errorf("Bailian TTS returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var decoded videoBailianTTSResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return videoTTSResult{}, fmt.Errorf("decode Bailian TTS response failed: %w", err)
	}
	if decoded.StatusCode != 0 && decoded.StatusCode != http.StatusOK {
		return videoTTSResult{}, fmt.Errorf("Bailian TTS failed: status=%d code=%s message=%s request_id=%s", decoded.StatusCode, decoded.Code, decoded.Message, decoded.RequestID)
	}
	if data := strings.TrimSpace(decoded.Output.Audio.Data); data != "" {
		if err := writeBailianBase64Audio(output, data); err != nil {
			return videoTTSResult{}, err
		}
	} else if audioURL := strings.TrimSpace(decoded.Output.Audio.URL); audioURL != "" {
		if err := p.downloadAudio(ctx, audioURL, output); err != nil {
			return videoTTSResult{}, err
		}
	} else {
		return videoTTSResult{}, fmt.Errorf("Bailian TTS response did not include audio data or url; request_id=%s", decoded.RequestID)
	}
	duration := probeAudioDuration(ctx, output)
	return videoTTSResult{AudioPath: output, Duration: duration}, nil
}

func (p videoBailianTTSProvider) endpoint() string {
	base := strings.TrimRight(strings.TrimSpace(p.baseURL), "/")
	if strings.Contains(base, "/services/aigc/multimodal-generation/generation") {
		return base
	}
	return base + "/services/aigc/multimodal-generation/generation"
}

func (p videoBailianTTSProvider) downloadAudio(ctx context.Context, audioURL, output string) error {
	parsed, err := url.Parse(audioURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("Bailian TTS returned invalid audio url %q", audioURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, audioURL, nil)
	if err != nil {
		return fmt.Errorf("create Bailian audio download request failed: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download Bailian audio failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("download Bailian audio returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	file, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("create Bailian audio file failed: %w", err)
	}
	defer file.Close()
	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("write Bailian audio file failed: %w", err)
	}
	return nil
}

func writeBailianBase64Audio(output, data string) error {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return fmt.Errorf("decode Bailian base64 audio failed: %w", err)
	}
	if err := os.WriteFile(output, decoded, 0o644); err != nil {
		return fmt.Errorf("write Bailian base64 audio failed: %w", err)
	}
	return nil
}

func probeAudioDuration(ctx context.Context, path string) time.Duration {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return 0
	}
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path)
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}
