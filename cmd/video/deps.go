package videocmd

import (
	"fmt"

	ttconfig "github.com/sjzsdu/tt/internal/ttconfig"
)

type SlideServerStop func()

type Dependencies struct {
	LoadTTConfig     func() (ttconfig.Loaded, error)
	StartSlideServer func(plan *Plan) (stop SlideServerStop, baseURL string, err error)
	BuildSlideURL    func(baseURL string, plan *Plan, slide int) string
}

var videoDeps Dependencies

func configureDependencies(deps Dependencies) {
	videoDeps = deps
}

func loadVideoTTConfig() (ttconfig.Loaded, error) {
	if videoDeps.LoadTTConfig != nil {
		return videoDeps.LoadTTConfig()
	}
	return ttconfig.Load("")
}

func startSlideServer(plan *Plan) (SlideServerStop, string, error) {
	if videoDeps.StartSlideServer == nil {
		return nil, "", fmt.Errorf("video slide server dependency is not configured")
	}
	return videoDeps.StartSlideServer(plan)
}

func buildSlideURL(baseURL string, plan *Plan, slide int) string {
	if videoDeps.BuildSlideURL != nil {
		return videoDeps.BuildSlideURL(baseURL, plan, slide)
	}
	return ""
}
