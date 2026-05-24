package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	ttconfig "github.com/sjzsdu/tt/internal/ttconfig"
)

var (
	imagePrompt  string
	imageModel   string
	imageOutput  string
	imageSize    string
	imageQuality string
	imageStyle   string
	imageDebug   bool
	imageHome    string
	imageConfig  string
)

var imageCmd = &cobra.Command{
	Use:     "image [prompt]",
	Aliases: []string{"img"},
	Short:   "Generate an image from a text prompt using picoclaw",
	Long: `Generate an image from a text prompt using a picoclaw text-to-image model.

The command calls the configured OpenAI-compatible /images/generations endpoint
and saves the generated image to --output, or to ./tt-image-<timestamp>.png by default.`,
	Args: cobra.ArbitraryArgs,
	Example: `tt image --model dall-e-3 "a tiny robot reading Go code"
tt img -m "a watercolor lobster" -o lobster.png
tt image --picoclaw-home ~/.picoclaw-dev --model dall-e-3 -m "futuristic city"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runImage(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(imageCmd)
	imageCmd.Flags().StringVarP(&imagePrompt, "message", "m", "", "text prompt for image generation")
	imageCmd.Flags().StringVar(&imageModel, "model", "", "text-to-image model to use; defaults to picoclaw config default")
	imageCmd.Flags().StringVarP(&imageOutput, "output", "o", "", "write generated image to this path; defaults to ./tt-image-<timestamp>.png")
	imageCmd.Flags().StringVar(&imageSize, "size", "1024x1024", "image size to request, for example 1024x1024")
	imageCmd.Flags().StringVar(&imageQuality, "quality", "", "image quality to request, for example standard or hd")
	imageCmd.Flags().StringVar(&imageStyle, "style", "", "image style to request, for example vivid or natural")
	imageCmd.Flags().BoolVarP(&imageDebug, "debug", "d", false, "enable debug logging")
	imageCmd.Flags().StringVar(&imageHome, "picoclaw-home", "", "override PICOCLAW_HOME for this run")
	imageCmd.Flags().StringVar(&imageConfig, "picoclaw-config", "", "override PICOCLAW_CONFIG for this run")
}

func runImage(cmd *cobra.Command, args []string) error {
	prompt := strings.TrimSpace(imagePrompt)
	if prompt == "" && len(args) > 0 {
		prompt = strings.TrimSpace(strings.Join(args, " "))
	}
	if prompt == "" {
		return fmt.Errorf("image prompt is required")
	}

	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	merged := loaded.Merged
	cli := ttconfig.Config{}
	if cmd.Flags().Changed("model") {
		cli.Agent.Model = imageModel
	}
	if cmd.Flags().Changed("debug") {
		cli.Agent.Debug = ttconfig.BoolPtr(imageDebug)
	}
	if cmd.Flags().Changed("picoclaw-home") {
		cli.Picoclaw.Home = imageHome
	}
	if cmd.Flags().Changed("picoclaw-config") {
		cli.Picoclaw.Config = imageConfig
	}
	merged = ttconfig.Merge(merged, cli)
	if err := ensurePicoclawConfigAvailable(merged.Picoclaw.Home, merged.Picoclaw.Config); err != nil {
		return err
	}

	rt, err := pcwrap.Load(pcwrap.Options{Home: merged.Picoclaw.Home, Config: merged.Picoclaw.Config, TTConfig: merged, TTSources: loaded.Sources})
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}

	debug := imageDebug
	if merged.Agent.Debug != nil {
		debug = *merged.Agent.Debug
	}
	loading := startLLMLoading("正在生成图片", debug)
	defer loading.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	content, err := rt.GenerateImage(ctx, pcwrap.ImageOptions{Prompt: prompt, Model: merged.Agent.Model, Size: imageSize, Quality: imageQuality, Style: imageStyle, Debug: debug, Quiet: !debug})
	loading.Stop()
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	outputPath := strings.TrimSpace(imageOutput)
	if outputPath == "" {
		outputPath = defaultImageOutputPath()
	}
	if err := saveImageResponse(outputPath, content); err != nil {
		fmt.Fprintln(cmd.OutOrStdout(), content)
		return fmt.Errorf("save generated image failed: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Saved image to %s\n", outputPath)
	return nil
}

func defaultImageOutputPath() string {
	return fmt.Sprintf("tt-image-%s.png", time.Now().Format("20060102-150405"))
}

func saveImageResponse(path, content string) error {
	if path == "" {
		return fmt.Errorf("output path is required")
	}
	data, err := imageResponseBytes(content)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func imageResponseBytes(content string) ([]byte, error) {
	content = strings.TrimSpace(content)
	if value := imagePayloadFromJSON(content); value != "" {
		content = value
	}
	if strings.HasPrefix(content, "data:") {
		idx := strings.Index(content, ",")
		if idx < 0 {
			return nil, fmt.Errorf("invalid data URL image response")
		}
		return base64.StdEncoding.DecodeString(strings.TrimSpace(content[idx+1:]))
	}
	if strings.HasPrefix(content, "http://") || strings.HasPrefix(content, "https://") {
		resp, err := http.Get(content)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("download image returned HTTP %d", resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	}
	return base64.StdEncoding.DecodeString(content)
}

func imagePayloadFromJSON(content string) string {
	var value any
	if err := json.Unmarshal([]byte(content), &value); err != nil {
		return ""
	}
	return findImagePayload(value)
}

func findImagePayload(value any) string {
	switch v := value.(type) {
	case map[string]any:
		for _, key := range []string{"url", "b64_json", "image", "data_url"} {
			if s, ok := v[key].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
		for _, child := range v {
			if s := findImagePayload(child); s != "" {
				return s
			}
		}
	case []any:
		for _, child := range v {
			if s := findImagePayload(child); s != "" {
				return s
			}
		}
	}
	return ""
}
