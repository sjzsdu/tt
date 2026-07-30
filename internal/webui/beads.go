package webui

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:beads/dist
var beadsDist embed.FS

func BeadsIndex() []byte {
	b, err := beadsDist.ReadFile("beads/dist/index.html")
	if err != nil {
		return []byte("<!doctype html><title>tt beads dashboard</title><p>beads dashboard web UI is not built. Run <code>make web-build-beads</code>.</p>")
	}
	return b
}

func BeadsAssetsHandler() http.Handler {
	sub, err := fs.Sub(beadsDist, "beads/dist/assets")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.StripPrefix("/assets/", http.FileServer(http.FS(sub)))
}

func BeadsFaviconHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" || path == "favicon.ico" {
			path = "favicon.svg"
		}
		f, err := beadsDist.Open("beads/dist/" + path)
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
