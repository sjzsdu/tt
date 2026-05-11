package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

// markdownDist embeds the built React app for tt markdown.
//
//go:embed markdown/dist
var markdownDist embed.FS

func MarkdownIndex() []byte {
	b, err := markdownDist.ReadFile("markdown/dist/index.html")
	if err != nil {
		return []byte("<!doctype html><title>tt markdown</title><p>markdown web UI is not built. Run <code>make web-build</code>.</p>")
	}
	return b
}

func MarkdownAssetsHandler() http.Handler {
	sub, err := fs.Sub(markdownDist, "markdown/dist/assets")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.StripPrefix("/assets/", http.FileServer(http.FS(sub)))
}
