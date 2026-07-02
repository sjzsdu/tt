package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed slide/dist
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
