package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	videocmd "github.com/sjzsdu/tt/cmd/video"
	"github.com/sjzsdu/tt/internal/webui"
)

func init() {
	rootCmd.AddCommand(videocmd.New(videocmd.Dependencies{
		LoadTTConfig:     loadTTConfig,
		StartSlideServer: startVideoSlideServerForCommand,
		BuildSlideURL:    buildVideoSlideURLForCommand,
	}))
}

func startVideoSlideServerForCommand(plan *videocmd.Plan) (videocmd.SlideServerStop, string, error) {
	oldRoot, oldFiles := slideRoot, slideFiles
	oldTemplate, oldTransition := slideTemplate, slideTransition
	oldControls, oldProgress, oldSlideNumber := slideControls, slideProgress, slideSlideNumber
	oldOverview, oldCenter, oldAutoSlide := slideOverview, slideCenter, slideAutoSlide
	oldWidth, oldHeight, oldMargin := slideWidth, slideHeight, slideMargin

	slideMu.Lock()
	slideRoot = filepath.Dir(plan.Meta.Slides)
	slideFiles = []string{plan.Meta.Slides}
	slideTemplate = plan.Meta.Template
	slideTransition = firstNonEmptyString(plan.Meta.Transition, "none")
	slideControls = false
	slideProgress = false
	slideSlideNumber = "false"
	slideOverview = false
	slideCenter = "auto"
	slideAutoSlide = 0
	slideWidth = plan.Meta.Width
	slideHeight = plan.Meta.Height
	slideMargin = plan.Meta.Margin
	slideMu.Unlock()

	mux := http.NewServeMux()
	mux.Handle("/assets/", webui.SlideAssetsHandler())
	mux.HandleFunc("/", handleSlideApp)
	mux.HandleFunc("/raw-content", handleSlideRawContent)
	mux.HandleFunc("/raw/", handleSlideRawFile)
	mux.HandleFunc("/api/list", handleSlideList)
	mux.HandleFunc("/api/slide/content", handleSlideSaveContent)
	mux.HandleFunc("/api/slide/rewrite", handleSlideRewrite)
	mux.HandleFunc("/api/widgets", handleSlideWidgets)
	mux.HandleFunc("/api/template/", handleSlideTemplate)
	mux.HandleFunc("/template-assets/", handleSlideTemplateAsset)
	mux.HandleFunc("/api/d2", handleD2Render)
	mux.HandleFunc("/images/", handleSlideImages)
	mux.HandleFunc("/ws", handleSlideWS)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", fmt.Errorf("start slide capture server failed: %w", err)
	}
	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	stop := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		slideMu.Lock()
		slideRoot, slideFiles = oldRoot, oldFiles
		slideTemplate, slideTransition = oldTemplate, oldTransition
		slideControls, slideProgress, slideSlideNumber = oldControls, oldProgress, oldSlideNumber
		slideOverview, slideCenter, slideAutoSlide = oldOverview, oldCenter, oldAutoSlide
		slideWidth, slideHeight, slideMargin = oldWidth, oldHeight, oldMargin
		slideMu.Unlock()
	}
	return stop, "http://" + listener.Addr().String(), nil
}

func buildVideoSlideURLForCommand(baseURL string, plan *videocmd.Plan, slide int) string {
	params := url.Values{}
	params.Set("file", slideRelPath(plan.Meta.Slides))
	if plan.Meta.Template != "" {
		params.Set("template", plan.Meta.Template)
	}
	appendSlideConfigParams(params)
	return baseURL + "/?" + params.Encode() + fmt.Sprintf("#/%d", slide-1)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
