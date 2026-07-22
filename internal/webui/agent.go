package webui

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:agent/dist
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

func AgentFaviconHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" || path == "favicon.ico" {
			path = "favicon.svg"
		}
		f, err := agentDist.Open("agent/dist/" + path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil || stat.IsDir() {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, stat.Name(), stat.ModTime(), f.(io.ReadSeeker))
	})
}
