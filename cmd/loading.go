package cmd

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

type loadingStatus struct {
	done chan struct{}
	once sync.Once
}

func startLLMLoading(message string, quiet bool) *loadingStatus {
	if quiet || !stderrIsTerminal() {
		return &loadingStatus{done: make(chan struct{})}
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "正在调用大模型"
	}
	ls := &loadingStatus{done: make(chan struct{})}
	go func() {
		start := time.Now()
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		render := func() {
			elapsed := time.Since(start).Round(time.Second)
			fmt.Fprintf(os.Stderr, "\r%s... 已执行 %s", message, elapsed)
		}
		render()
		for {
			select {
			case <-ticker.C:
				render()
			case <-ls.done:
				fmt.Fprint(os.Stderr, "\r\033[K")
				return
			}
		}
	}()
	return ls
}

func (ls *loadingStatus) Stop() {
	if ls == nil {
		return
	}
	ls.once.Do(func() {
		close(ls.done)
	})
}

func stderrIsTerminal() bool {
	stat, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return stat.Mode()&os.ModeCharDevice != 0
}
