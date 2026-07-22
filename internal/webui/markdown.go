package webui

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:markdown/dist
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

func MarkdownFaviconHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" || path == "favicon.ico" {
			path = "favicon.svg"
		}
		f, err := markdownDist.Open("markdown/dist/" + path)
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