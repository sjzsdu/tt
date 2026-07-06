package videocmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

type videoProgress struct {
	quiet    bool
	terminal bool
	out      io.Writer
	start    time.Time
	message  string
	mu       sync.Mutex
}

func startVideoProgress(message string, quiet bool) *videoProgress {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "正在生成视频"
	}
	p := &videoProgress{
		quiet:    quiet,
		terminal: videoStderrIsTerminal(),
		out:      os.Stderr,
		start:    time.Now(),
		message:  message,
	}
	p.Step("%s", message)
	return p
}

func (p *videoProgress) Step(format string, args ...interface{}) {
	if p == nil || p.quiet {
		return
	}
	message := strings.TrimSpace(fmt.Sprintf(format, args...))
	if message == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.message = message
	elapsed := time.Since(p.start).Round(time.Second)
	if p.terminal {
		fmt.Fprintf(p.out, "\r%s... 已执行 %s", message, elapsed)
		return
	}
	fmt.Fprintf(p.out, "video: %s... 已执行 %s\n", message, elapsed)
}

func (p *videoProgress) Done(format string, args ...interface{}) {
	if p == nil || p.quiet {
		return
	}
	message := strings.TrimSpace(fmt.Sprintf(format, args...))
	p.mu.Lock()
	defer p.mu.Unlock()
	elapsed := time.Since(p.start).Round(time.Second)
	if p.terminal {
		fmt.Fprint(p.out, "\r\033[K")
		if message != "" {
			fmt.Fprintf(p.out, "%s，用时 %s\n", message, elapsed)
		}
		return
	}
	if message != "" {
		fmt.Fprintf(p.out, "video: %s，用时 %s\n", message, elapsed)
	}
}

func (p *videoProgress) Clear() {
	if p == nil || p.quiet || !p.terminal {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprint(p.out, "\r\033[K")
}

func videoStderrIsTerminal() bool {
	stat, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return stat.Mode()&os.ModeCharDevice != 0
}
