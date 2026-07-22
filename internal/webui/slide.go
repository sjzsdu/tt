package webui

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:slide/dist
var slideDist embed.FS

//go:embed slide/templates slide/widgets
var slideResources embed.FS

func SlideResources() fs.FS {
	return slideResources
}

func SlideIndex() []byte {
	b, err := slideDist.ReadFile("slide/dist/index.html")
	if err != nil {
		return []byte("<!doctype html><title>tt slide</title><p>slide web UI is not built. Run <code>make web-build</code>.</p>")
	}
	return b
}

func SlideAssetsHandler() http.Handler {
	sub, err := fs.Sub(slideDist, "slide/dist/assets")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.StripPrefix("/assets/", http.FileServer(http.FS(sub)))
}

func SlideFaviconHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" || path == "favicon.ico" {
			path = "favicon.svg"
		}
		f, err := slideDist.Open("slide/dist/" + path)
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