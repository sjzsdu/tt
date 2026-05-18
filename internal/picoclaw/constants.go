package picoclaw

import "strings"

const (
	DefaultAgentID  = "main"
	defaultAgentID  = DefaultAgentID
	defaultSession  = "tt:default"
	defaultModel    = "main"
	envPicoclawHome = "PICOCLAW_HOME"
	envPicoclawCfg  = "PICOCLAW_CONFIG"
)

func str(s string) string {
	return strings.TrimSpace(s)
}
