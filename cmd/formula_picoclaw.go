package cmd

import (
	"fmt"

	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	ttconfig "github.com/sjzsdu/tt/internal/ttconfig"
)

type formulaPicoclawRuntime struct {
	Loaded       ttconfig.Loaded
	Config       ttconfig.Config
	Runtime      *pcwrap.Runtime
	Workspace    string
	PicoclawHome string
	PicoclawConf string
	cleanup      func()
}

func newFormulaPicoclawRuntime(projectRoot string) (*formulaPicoclawRuntime, error) {
	loaded, err := formulaLoadTTConfig()
	if err != nil {
		return nil, err
	}
	merged := loaded.Merged
	workspace := formulaAgentWorkspace(projectRoot)
	_, resolvedHome, resolvedConfig, restoreStorage, err := useTTAgentStorage(merged.Picoclaw.Home, merged.Picoclaw.Config)
	if err != nil {
		return nil, err
	}
	cleanup := func() { restoreStorage() }
	merged.Picoclaw.Home = resolvedHome
	merged.Picoclaw.Config = resolvedConfig
	if err := ensurePicoclawConfigAvailable(merged.Picoclaw.Home, merged.Picoclaw.Config); err != nil {
		cleanup()
		return nil, err
	}
	rt, err := pcwrap.Load(pcwrap.Options{
		Home:      merged.Picoclaw.Home,
		Config:    merged.Picoclaw.Config,
		TTConfig:  merged,
		TTSources: loaded.Sources,
	})
	if err != nil {
		cleanup()
		return nil, picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	return &formulaPicoclawRuntime{
		Loaded:       loaded,
		Config:       merged,
		Runtime:      rt,
		Workspace:    workspace,
		PicoclawHome: merged.Picoclaw.Home,
		PicoclawConf: merged.Picoclaw.Config,
		cleanup:      cleanup,
	}, nil
}

func (r *formulaPicoclawRuntime) Close() {
	if r != nil && r.cleanup != nil {
		r.cleanup()
		r.cleanup = nil
	}
}

func (r *formulaPicoclawRuntime) UnavailableError(err error) error {
	if err == nil {
		return nil
	}
	if r == nil {
		return err
	}
	return picoclawUnavailableError(err, r.PicoclawHome, r.PicoclawConf)
}

func (r *formulaPicoclawRuntime) Validate() error {
	if r == nil || r.Runtime == nil {
		return fmt.Errorf("formula picoclaw runtime is not initialized")
	}
	return nil
}
