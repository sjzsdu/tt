package cmd

import (
	"context"
	"fmt"
	"strings"
	"sync"

	teamruntime "github.com/sjzsdu/tt/internal/team"
)

type teamDashboardControls struct {
	Busy              bool `json:"busy"`
	CanFollowUp       bool `json:"can_follow_up"`
	CanResume         bool `json:"can_resume"`
	CanStop           bool `json:"can_stop"`
	CanRetryMemory    bool `json:"can_retry_memory"`
	CanRollbackMemory bool `json:"can_rollback_memory"`
}

type teamDashboardActions interface {
	FollowUp(string) error
	Resume() error
	Stop() error
	RetryMemory() error
	RollbackMemory(int) error
	Controls() teamDashboardControls
}

type teamRunController struct {
	mu       sync.Mutex
	engine   *teamruntime.Engine
	parent   context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	running  bool
	onChange func()
}

func newTeamRunController(engine *teamruntime.Engine, parent context.Context) *teamRunController {
	if parent == nil {
		parent = context.Background()
	}
	return &teamRunController{engine: engine, parent: parent}
}

func (c *teamRunController) SetOnChange(onChange func()) {
	c.mu.Lock()
	c.onChange = onChange
	c.mu.Unlock()
}

func (c *teamRunController) Run(ctx context.Context, question string, resume bool) (teamruntime.RunResult, error) {
	runCtx, err := c.begin(ctx, resume)
	if err != nil {
		return teamruntime.RunResult{}, err
	}
	return c.runStarted(runCtx, strings.TrimSpace(question), resume)
}

func (c *teamRunController) FollowUp(question string) error {
	question = strings.TrimSpace(question)
	if question == "" {
		return fmt.Errorf("follow-up question is required")
	}
	runCtx, err := c.begin(c.parent, false)
	if err != nil {
		return err
	}
	go func() {
		_, _ = c.runStarted(runCtx, question, false)
	}()
	return nil
}

func (c *teamRunController) Resume() error {
	runCtx, err := c.begin(c.parent, true)
	if err != nil {
		return err
	}
	go func() {
		_, _ = c.runStarted(runCtx, "", true)
	}()
	return nil
}

func (c *teamRunController) Stop() error {
	c.mu.Lock()
	if !c.running || c.cancel == nil {
		c.mu.Unlock()
		return fmt.Errorf("team has no active run to stop")
	}
	cancel := c.cancel
	onChange := c.onChange
	c.mu.Unlock()
	cancel()
	if onChange != nil {
		onChange()
	}
	return nil
}

func (c *teamRunController) RetryMemory() error {
	runCtx, err := c.beginMaintenance(c.parent)
	if err != nil {
		return err
	}
	go func() {
		_, _ = c.engine.RetryMemory(runCtx)
		c.finish()
	}()
	return nil
}

func (c *teamRunController) RollbackMemory(version int) error {
	if version < 1 {
		return fmt.Errorf("memory version must be greater than zero")
	}
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("team already has an active run")
	}
	c.mu.Unlock()
	if _, err := c.engine.RollbackMemory(version); err != nil {
		return err
	}
	c.mu.Lock()
	onChange := c.onChange
	c.mu.Unlock()
	if onChange != nil {
		onChange()
	}
	return nil
}

func (c *teamRunController) Wait() {
	c.mu.Lock()
	done := c.done
	c.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (c *teamRunController) Controls() teamDashboardControls {
	c.mu.Lock()
	running := c.running
	c.mu.Unlock()
	controls := teamDashboardControls{
		Busy:    running,
		CanStop: running,
	}
	if c.engine == nil || c.engine.Store == nil {
		return controls
	}
	_, state, _, err := c.engine.Store.Snapshot()
	if err != nil {
		return controls
	}
	if running {
		return controls
	}
	review, memoryErr := teamruntime.LoadMemoryReview(c.engine.Store.Thread.MemoryPath, c.engine.Store.Thread.Team)
	if memoryErr == nil {
		controls.CanRollbackMemory = len(review.Versions) > 0
	}
	if state.Current == nil || state.Current.Status == teamruntime.RoundStatusCompleted {
		controls.CanFollowUp = true
		if state.Current != nil {
			events, eventErr := c.engine.Store.Events()
			if eventErr == nil {
				for i := len(events) - 1; i >= 0; i-- {
					if events[i].Round != state.Current.Number {
						continue
					}
					if events[i].Type == "memory_update_failed" {
						controls.CanRetryMemory = true
					}
					if events[i].Type == "memory_updated" {
						break
					}
				}
			}
		}
		return controls
	}
	controls.CanResume = true
	return controls
}

func (c *teamRunController) beginMaintenance(parent context.Context) (context.Context, error) {
	if c == nil || c.engine == nil || c.engine.Store == nil {
		return nil, fmt.Errorf("team runtime is unavailable")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return nil, fmt.Errorf("team already has an active run")
	}
	if parent == nil {
		parent = c.parent
	}
	runCtx, cancel := context.WithCancel(parent)
	c.running = true
	c.cancel = cancel
	c.done = make(chan struct{})
	if c.onChange != nil {
		go c.onChange()
	}
	return runCtx, nil
}

func (c *teamRunController) begin(parent context.Context, resume bool) (context.Context, error) {
	if c == nil || c.engine == nil || c.engine.Store == nil {
		return nil, fmt.Errorf("team runtime is unavailable")
	}
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return nil, fmt.Errorf("team already has an active run")
	}
	_, state, _, err := c.engine.Store.Snapshot()
	if err != nil {
		c.mu.Unlock()
		return nil, err
	}
	if resume {
		if state.Current == nil || state.Current.Status == teamruntime.RoundStatusCompleted {
			c.mu.Unlock()
			return nil, fmt.Errorf("team has no interrupted or failed round to resume")
		}
	} else if state.Current != nil && state.Current.Status != teamruntime.RoundStatusCompleted {
		c.mu.Unlock()
		return nil, fmt.Errorf("team round %d is %s; resume or stop it before asking a follow-up", state.Current.Number, state.Current.Status)
	}
	if parent == nil {
		parent = c.parent
	}
	runCtx, cancel := context.WithCancel(parent)
	c.running = true
	c.cancel = cancel
	c.done = make(chan struct{})
	onChange := c.onChange
	c.mu.Unlock()
	if onChange != nil {
		onChange()
	}
	return runCtx, nil
}

func (c *teamRunController) runStarted(ctx context.Context, question string, resume bool) (teamruntime.RunResult, error) {
	var (
		result teamruntime.RunResult
		err    error
	)
	if resume {
		result, err = c.engine.Resume(ctx)
	} else {
		result, err = c.engine.RunRound(ctx, question)
	}
	c.finish()
	return result, err
}

func (c *teamRunController) finish() {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	done := c.done
	c.cancel = nil
	c.done = nil
	c.running = false
	onChange := c.onChange
	c.mu.Unlock()
	if done != nil {
		close(done)
	}
	if onChange != nil {
		onChange()
	}
}
