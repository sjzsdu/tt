package webui

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:team/dist
var teamDist embed.FS

func TeamIndex() []byte {
	b, err := teamDist.ReadFile("team/dist/index.html")
	if err != nil {
		return []byte("<!doctype html><title>tt team dashboard</title><p>team dashboard web UI is not built. Run <code>make web-build-team</code>.</p>")
	}
	return b
}

func TeamAssetsHandler() http.Handler {
	sub, err := fs.Sub(teamDist, "team/dist/assets")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.StripPrefix("/assets/", http.FileServer(http.FS(sub)))
}

func TeamFaviconHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" || path == "favicon.ico" {
			path = "favicon.svg"
		}
		f, err := teamDist.Open("team/dist/" + path)
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
