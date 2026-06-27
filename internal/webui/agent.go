package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

// agentDist embeds the built React app for tt agent web UI.
//
//go:embed agent/dist
var agentDist embed.FS

func AgentIndex() []byte {
	b, err := agentDist.ReadFile("agent/dist/index.html")
	if err != nil {
		return []byte("<!doctype html><title>tt agent web</title><p>agent web UI is not built. Run <code>make web-build</code>.</p>")
	}
	return b
}

func AgentAssetsHandler() http.Handler {
	sub, err := fs.Sub(agentDist, "agent/dist/assets")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.StripPrefix("/assets/", http.FileServer(http.FS(sub)))
}
